// SPDX-FileCopyrightText: Copyright (c) 2025, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/gpuhealthconfig"
	pkghealthexporter "github.com/leptonai/gpud/pkg/healthexporter"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
	pkgmetricsscraper "github.com/leptonai/gpud/pkg/metrics/scraper"
	pkgmetricsstore "github.com/leptonai/gpud/pkg/metrics/store"
	pkgmetricssyncer "github.com/leptonai/gpud/pkg/metrics/syncer"
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

	// healthExporter is the health exporter instance
	healthExporter pkghealthexporter.Exporter

	machineID string
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

	// Try to read existing machine ID from database
	machineID, err := pkgmetadata.ReadMachineIDWithFallback(ctx, dbRW, dbRO)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to read machine uid: %w", err)
	}

	// If no machine ID found in database, use system machine ID and store it persistently
	if machineID == "" {
		machineID = pkghost.MachineID()
		if machineID == "" {
			// Fallback to dynamic lookup if not cached
			var err error
			machineID, err = pkghost.GetMachineID(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get system machine ID: %w", err)
			}
		}
		// Store the system machine ID in database for persistence across reboots
		if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyMachineID, machineID); err != nil {
			return nil, fmt.Errorf("failed to store system machine ID: %w", err)
		}
	}

	s.machineID = machineID

	nvmlInstance, err := nvidianvml.NewWithExitOnSuccessfulLoad(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create NVML instance: %w", err)
	}

	// Create event store needed for health exporter
	eventStore, err := eventstore.New(dbRW, dbRO, 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to open events database: %w", err)
	}

	// Create reboot event store
	rebootEventStore := pkghost.NewRebootEventStore(eventStore)

	// Record reboot event once when creating the server instance
	cctx, ccancel := context.WithTimeout(ctx, time.Minute)
	err = rebootEventStore.RecordReboot(cctx)
	ccancel()
	if err != nil {
		log.Logger.Errorw("failed to record reboot", "error", err)
	}

	// Determine health check interval from config, with fallback to default
	healthCheckInterval := time.Minute // default
	if config.HealthExporter != nil && config.HealthExporter.HealthCheckInterval.Duration > 0 {
		healthCheckInterval = config.HealthExporter.HealthCheckInterval.Duration
	}

	s.gpudInstance = &components.GPUdInstance{
		RootCtx:             ctx,
		MachineID:           machineID,
		NVMLInstance:        nvmlInstance,
		DBRW:                dbRW,
		DBRO:                dbRO,
		EventStore:          eventStore,
		RebootEventStore:    rebootEventStore,
		MountPoints:         []string{"/"},
		MountTargets:        []string{"/var/lib/kubelet"},
		HealthCheckInterval: healthCheckInterval,
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

	// Create metrics infrastructure needed for health exporter
	promScraper, err := pkgmetricsscraper.NewPrometheusScraper(pkgmetrics.DefaultGatherer())
	if err != nil {
		return nil, fmt.Errorf("failed to create scraper: %w", err)
	}
	metricsSQLiteStore, err := pkgmetricsstore.NewSQLiteStore(ctx, dbRW, dbRO, pkgmetricsstore.DefaultTableName)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics store: %w", err)
	}
	syncer := pkgmetricssyncer.NewSyncer(ctx, promScraper, metricsSQLiteStore, time.Minute, time.Minute, 24*time.Hour)
	syncer.Start()

	promRecorder := pkgmetricsrecorder.NewPrometheusRecorder(ctx, 15*time.Minute, dbRO)
	promRecorder.Start()

	// Start mock endpoint only if not in offline mode
	var mock *pkghealthexporter.MockEndpoint
	isOfflineMode := config.HealthExporter != nil && config.HealthExporter.OfflineMode
	if !isOfflineMode {
		// Start mock endpoint
		// TODO: Remove this once we have a real endpoint
		mock = pkghealthexporter.NewMockEndpoint(8080)
		if err := mock.Start(); err != nil {
			panic(fmt.Sprintf("Failed to start mock endpoint: %v", err))
		}
	}

	// TODO: We require user register agent with  -- datacenter and --node-group, and agent need to send metrics with above metadata information
	// gpud login --endpoint <health-endpoint> --token <sak-token> --data-center <data-center> --node-group <node-group>

	// Create and start health exporter with all dependencies if enabled
	if config.HealthExporter != nil {
		// Set endpoint based on mode and availability of mock
		if !config.HealthExporter.OfflineMode {
			if mock != nil {
				config.HealthExporter.Endpoint = mock.HealthBulkURL()
			}
			// If mock is nil, use the endpoint from config (already set)
		}

		var err error
		s.healthExporter, err = pkghealthexporter.New(
			ctx,
			pkghealthexporter.WithConfig(config.HealthExporter),
			pkghealthexporter.WithMetricsStore(metricsSQLiteStore),
			pkghealthexporter.WithEventStore(eventStore),
			pkghealthexporter.WithComponentsRegistry(s.componentsRegistry),
			pkghealthexporter.WithNVMLInstance(nvmlInstance),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create health exporter: %w", err)
		}

		// Start the health exporter
		if err := s.healthExporter.Start(); err != nil {
			log.Logger.Errorw("failed to start health exporter", "error", err)
		}
	}

	// Start database compaction
	go s.doCompact(ctx, dbRW, config.CompactPeriod.Duration)

	// Start the HTTP server
	go s.startServer(ctx, nvmlInstance)

	return s, nil
}

// GetHealthExporter returns the health exporter instance (for offline mode access)
func (s *Server) GetHealthExporter() pkghealthexporter.Exporter {
	return s.healthExporter
}

// Stop gracefully stops the server
func (s *Server) Stop() {
	// Stop health exporter if running
	if s.healthExporter != nil {
		if err := s.healthExporter.Stop(); err != nil {
			log.Logger.Errorw("failed to stop health exporter", "error", err)
		}
	}

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
