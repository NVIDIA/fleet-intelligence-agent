// Package healthserver provides a simplified HTTP server for GPU health metrics export.
// This server focuses only on health monitoring and metrics export, removing all
// management functionality like package management, control plane connectivity,
// fault injection, and plugin systems.
package gpuhealthserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	stdos "os"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/all"
	"github.com/leptonai/gpud/pkg/gpuhealthconfig"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	pkgmetricsstore "github.com/leptonai/gpud/pkg/metrics/store"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// Server is a simplified health metrics exporter server
type Server struct {
	auditLogger log.AuditLogger
	dbRW        *sql.DB
	dbRO        *sql.DB

	componentsRegistry components.Registry
	gpudInstance       *components.GPUdInstance

	config *gpuhealthconfig.Config
}

// New creates a new simplified health server for metrics export only
func New(ctx context.Context, auditLogger log.AuditLogger, config *gpuhealthconfig.Config) (retServer *Server, retErr error) {
	stateFile := ":memory:"
	if config.State != "" {
		stateFile = config.State
	}

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open state file (for read-write): %w", err)
	}
	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return nil, fmt.Errorf("failed to open state file (for read-only): %w", err)
	}

	if err := pkgmetadata.CreateTableMetadata(ctx, dbRW); err != nil {
		return nil, fmt.Errorf("failed to create metadata table: %w", err)
	}

	s := &Server{
		auditLogger: auditLogger,
		dbRW:        dbRW,
		dbRO:        dbRO,
		config:      config,
	}
	defer func() {
		if retErr != nil {
			s.Stop()
		}
	}()

	// Read machine ID for identification
	machineID, err := pkgmetadata.ReadMachineIDWithFallback(ctx, dbRW, dbRO)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to read machine uid: %w", err)
	}

	nvmlInstance, err := nvidianvml.NewWithExitOnSuccessfulLoad(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create NVML instance: %w", err)
	}

	s.gpudInstance = &components.GPUdInstance{
		RootCtx:      ctx,
		MachineID:    machineID,
		NVMLInstance: nvmlInstance,
	}

	// Register only enabled components for health monitoring
	s.componentsRegistry = components.NewRegistry(s.gpudInstance)
	for _, c := range all.All() {
		name := c.Name

		shouldEnable := config.ShouldEnable(name)
		if config.ShouldDisable(name) {
			shouldEnable = false
		}

		if shouldEnable {
			s.componentsRegistry.MustRegister(c.InitFunc)
		}
	}

	// Start components for health monitoring
	for _, c := range s.componentsRegistry.All() {
		if err = c.Start(); err != nil {
			return nil, fmt.Errorf("failed to start component %s: %w", c.Name(), err)
		}
	}

	// Start database compaction
	go s.doCompact(ctx, dbRW, config.CompactPeriod.Duration)

	// Start the HTTP server
	go s.startServer(ctx, nvmlInstance)

	return s, nil
}

// Stop gracefully stops the server
func (s *Server) Stop() {
	if s.componentsRegistry != nil {
		for _, c := range s.componentsRegistry.All() {
			c.Close()
		}
	}

	if s.dbRW != nil {
		s.dbRW.Close()
	}
	if s.dbRO != nil {
		s.dbRO.Close()
	}
}

// startServer creates and starts the HTTP server
func (s *Server) startServer(ctx context.Context, nvmlInstance nvidianvml.Instance) {
	defer func() {
		if nvmlInstance != nil {
			if err := nvmlInstance.Shutdown(); err != nil {
				log.Logger.Warnw("failed to shutdown NVML instance", "error", err)
			}
		}
		s.Stop()
	}()

	// Create metrics store for health data
	metricsSQLiteStore, err := pkgmetricsstore.NewSQLiteStore(ctx, s.dbRW, s.dbRO, pkgmetricsstore.DefaultTableName)
	if err != nil {
		log.Logger.Errorw("failed to create metrics store", "error", err)
		return
	}

	router := gin.Default()
	s.installMiddlewares(router)

	globalHandler := newGlobalHandler(s.config, s.componentsRegistry, metricsSQLiteStore, s.gpudInstance)

	v1Group := router.Group("/v1")
	v1Group.Use(gzip.Gzip(gzip.DefaultCompression))
	v1Group.GET("/states", globalHandler.getHealthStates)
	v1Group.GET("/events", globalHandler.getEvents)
	v1Group.GET("/info", globalHandler.getInfo)
	v1Group.GET("/metrics", globalHandler.getMetrics)

	// Core endpoints for health monitoring
	promHandler := promhttp.HandlerFor(pkgmetrics.DefaultGatherer(), promhttp.HandlerOpts{})
	router.GET("/metrics", func(ctx *gin.Context) {
		promHandler.ServeHTTP(ctx.Writer, ctx.Request)
	})
	router.GET("/healthz", s.healthz())
	router.GET("/machine-info", globalHandler.machineInfo)

	log.Logger.Infow("gpuhealth started serving with HTTP", "address", s.config.Address)

	srv := &http.Server{
		Addr:    s.config.Address,
		Handler: router,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Logger.Warnw("gpuhealth serve failed", "address", s.config.Address, "error", err)
		stdos.Exit(1)
	}
}

// doCompact periodically compacts the database
func (s *Server) doCompact(ctx context.Context, db *sql.DB, interval time.Duration) {
	if interval <= 0 {
		log.Logger.Debugw("compact period is disabled")
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Logger.Debugw("compacting database")
			if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
				log.Logger.Warnw("failed to compact database", "error", err)
			}
		}
	}
}

// installMiddlewares installs basic middleware for the router
func (s *Server) installMiddlewares(router *gin.Engine) {
	router.Use(gin.Recovery())
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
}

// healthz returns a simple health check handler
func (s *Server) healthz() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"version": "v1",
		})
	}
}
