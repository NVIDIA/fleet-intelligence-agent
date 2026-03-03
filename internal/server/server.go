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

// Package healthserver provides a simplified HTTP server for Fleet Intelligence metrics export.
// This server focuses only on health monitoring and metrics export, removing all
// management functionality like package management, control plane connectivity,
// fault injection, and plugin systems.
package server

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
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/eventstore"
	pkgfaultinjector "github.com/NVIDIA/fleet-intelligence-sdk/pkg/fault-injector"
	pkghost "github.com/NVIDIA/fleet-intelligence-sdk/pkg/host"
	pkgkmsgwriter "github.com/NVIDIA/fleet-intelligence-sdk/pkg/kmsg/writer"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	pkgmetadata "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metadata"
	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
	pkgmetricsrecorder "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics/recorder"
	pkgmetricsscraper "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics/scraper"
	pkgmetricsstore "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics/store"
	pkgmetricssyncer "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics/syncer"
	nvidiadcgm "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/dcgm"
	nvidianvml "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/sqlite"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/exporter"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/registry"
)

// Server is a simplified health metrics exporter server
type Server struct {
	auditLogger log.AuditLogger
	dbRW        *sql.DB
	dbRO        *sql.DB

	componentsRegistry components.Registry
	gpudInstance       *components.GPUdInstance

	config *config.Config

	// healthExporter is the health exporter instance
	healthExporter exporter.Exporter

	// faultInjector is the fault injector for testing
	faultInjector pkgfaultinjector.Injector

	machineID string
}

// initializeDatabases opens and initializes database connections
func initializeDatabases(ctx context.Context, config *config.Config) (*sql.DB, *sql.DB, error) {
	stateFile := ":memory:"
	if config.State != "" {
		stateFile = config.State
	}

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open state file (for read-write): %w", err)
	}

	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open state file (for read-only): %w", err)
	}

	if err := pkgmetadata.CreateTableMetadata(ctx, dbRW); err != nil {
		return nil, nil, fmt.Errorf("failed to create metadata table: %w", err)
	}

	return dbRW, dbRO, nil
}

// initializeMachineID retrieves or creates a machine ID
// This establishes the agent's stable identity for metrics reporting.
// Priority: DB (persisted) → dmidecode (hardware UUID) → random UUID
func initializeMachineID(ctx context.Context, dbRW, dbRO *sql.DB) (string, error) {
	// Try to read existing machine ID from database
	machineID, err := pkgmetadata.ReadMachineID(ctx, dbRO)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("failed to read machine uid: %w", err)
	}

	// If no machine ID found in database, initialize a new one
	if machineID == "" {
		// First, try to get hardware UUID from dmidecode
		machineID, err = pkghost.GetDmidecodeUUID(ctx)
		if err != nil || machineID == "" {
			// If dmidecode fails (permissions, not available, etc.), generate a random UUID
			machineID = uuid.New().String()
			log.Logger.Warnw("Failed to get hardware UUID, generated random agent ID",
				"error", err,
				"generated_id", machineID)
		} else {
			log.Logger.Infow("Initialized agent ID from hardware UUID", "machine_id", machineID)
		}

		// Store the machine ID in database for persistence
		if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyMachineID, machineID); err != nil {
			return "", fmt.Errorf("failed to store machine ID in database: %w", err)
		}
		log.Logger.Infow("Persisted agent ID to database", "machine_id", machineID)
	} else {
		log.Logger.Infow("Using persisted agent ID from database", "machine_id", machineID)
	}

	return machineID, nil
}

// getHealthCheckInterval determines the health check interval from config
func getHealthCheckInterval(config *config.Config) time.Duration {
	healthCheckInterval := time.Minute // default
	if config.HealthExporter != nil && config.HealthExporter.HealthCheckInterval.Duration > 0 {
		healthCheckInterval = config.HealthExporter.HealthCheckInterval.Duration
	}
	return healthCheckInterval
}

// shouldEnableComponent determines if a component should be enabled based on configuration
func shouldEnableComponent(name string, enabledByDefault bool, config *config.Config) bool {
	shouldEnable := enabledByDefault

	// If specific components are configured, check if this one is selected
	if len(config.Components) > 0 && config.Components[0] != "*" && config.Components[0] != "all" {
		shouldEnable = config.ShouldEnable(name)
	}

	// Explicit disable takes precedence
	if config.ShouldDisable(name) {
		shouldEnable = false
	}

	return shouldEnable
}

// New creates a new simplified health server for metrics export only
func New(ctx context.Context, auditLogger log.AuditLogger, config *config.Config) (retServer *Server, retErr error) {
	// Initialize database connections
	dbRW, dbRO, err := initializeDatabases(ctx, config)
	if err != nil {
		return nil, err
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

	// Initialize machine ID
	machineID, err := initializeMachineID(ctx, dbRW, dbRO)
	if err != nil {
		return nil, err
	}
	s.machineID = machineID

	// Initialize fault injector for testing (only if enabled)
	if config.EnableFaultInjection {
		log.Logger.Infow("fault injection enabled for testing")
		kmsgWriter := pkgkmsgwriter.NewWriter(pkgkmsgwriter.DefaultDevKmsg)
		s.faultInjector = pkgfaultinjector.NewInjector(kmsgWriter)
	} else {
		log.Logger.Infow("fault injection disabled")
		s.faultInjector = nil
	}

	nvmlInstance, err := nvidianvml.NewWithExitOnSuccessfulLoad(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create NVML instance: %w", err)
	}

	// Initialize DCGM instance
	dcgmInstance, err := nvidiadcgm.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create DCGM instance: %w", err)
	}

	// Create event store needed for health exporter
	log.Logger.Infow("initializing event store", "retention", config.RetentionPeriod.Duration)
	eventStore, err := eventstore.New(dbRW, dbRO, config.RetentionPeriod.Duration)
	if err != nil {
		return nil, fmt.Errorf("failed to open events database: %w", err)
	}

	// Create reboot event store and record reboot
	rebootEventStore := pkghost.NewRebootEventStore(eventStore)
	cctx, ccancel := context.WithTimeout(ctx, time.Minute)
	err = rebootEventStore.RecordReboot(cctx)
	ccancel()
	if err != nil {
		log.Logger.Errorw("failed to record reboot", "error", err)
	}

	// Determine health check interval
	healthCheckInterval := getHealthCheckInterval(config)

	// Create shared DCGM caches
	dcgmHealthCache := nvidiadcgm.NewHealthCache(ctx, dcgmInstance, healthCheckInterval)
	log.Logger.Infow("DCGM health check cache configured", "healthCheckInterval", healthCheckInterval)

	dcgmFieldValueCache := nvidiadcgm.NewFieldValueCache(ctx, dcgmInstance, healthCheckInterval)
	log.Logger.Infow("DCGM field value cache created", "healthCheckInterval", healthCheckInterval)

	dcgmPolicyViolationCache := nvidiadcgm.NewPolicyViolationCache(ctx, dcgmInstance)
	log.Logger.Infow("DCGM policy violation cache created")

	s.gpudInstance = &components.GPUdInstance{
		RootCtx:                  ctx,
		MachineID:                machineID,
		NVMLInstance:             nvmlInstance,
		DCGMInstance:             dcgmInstance,
		DCGMHealthCache:          dcgmHealthCache,
		DCGMFieldValueCache:      dcgmFieldValueCache,
		DCGMPolicyViolationCache: dcgmPolicyViolationCache,
		NVIDIAToolOverwrites:     config.NvidiaToolOverwrites,
		DBRW:                     dbRW,
		DBRO:                     dbRO,
		EventStore:               eventStore,
		RebootEventStore:         rebootEventStore,
		MountPoints:              []string{"/"},
		MountTargets:             []string{},
		HealthCheckInterval:      healthCheckInterval,
		EnableDCGMPolicy:         config.EnableDCGMPolicy,
	}

	// Register only enabled components for health monitoring
	s.componentsRegistry = components.NewRegistry(s.gpudInstance)
	for _, c := range registry.All() {
		if shouldEnableComponent(c.Name, c.EnabledByDefault, config) {
			s.componentsRegistry.MustRegister(c.InitFunc)
		}
	}

	// Start DCGM health cache before starting components
	if dcgmHealthCache != nil {
		if err := dcgmHealthCache.Start(); err != nil {
			log.Logger.Errorw("failed to start DCGM health cache, DCGM health monitoring disabled", "error", err)
		}
	}

	// Set up DCGM field watching after all components have registered their fields
	if dcgmFieldValueCache != nil {
		if err := dcgmFieldValueCache.SetupFieldWatching(); err != nil {
			log.Logger.Errorw("failed to set up DCGM field watching, DCGM metrics collection unavailable", "error", err)
		}
	}

	// Set up centralized DCGM policy violation watching
	if dcgmPolicyViolationCache != nil {
		if err := dcgmPolicyViolationCache.SetupPolicyWatching(); err != nil {
			log.Logger.Errorw("failed to set up DCGM policy watching, DCGM policy violations undetected", "error", err)
		}
	}

	// Start DCGM field value cache polling
	if dcgmFieldValueCache != nil {
		if err := dcgmFieldValueCache.Start(); err != nil {
			log.Logger.Errorw("failed to start DCGM field value cache, DCGM metrics polling disabled", "error", err)
		}
	}

	// Start DCGM policy violation cache distribution
	if dcgmPolicyViolationCache != nil {
		if err := dcgmPolicyViolationCache.Start(); err != nil {
			log.Logger.Errorw("failed to start DCGM policy violation cache, DCGM policy violation monitoring disabled", "error", err)
		}
	}

	// Start components for health monitoring (must be started after DCGM initialization)
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
	// Purge metrics every 5 minutes (reasonable interval to balance overhead and timely cleanup)
	metricsPurgeInterval := 5 * time.Minute
	log.Logger.Infow("initializing metrics syncer", "scrapeInterval", healthCheckInterval, "purgeInterval", metricsPurgeInterval, "retention", config.RetentionPeriod.Duration)
	syncer := pkgmetricssyncer.NewSyncer(ctx, promScraper, metricsSQLiteStore, healthCheckInterval, metricsPurgeInterval, config.RetentionPeriod.Duration)
	syncer.Start()

	promRecorder := pkgmetricsrecorder.NewPrometheusRecorder(ctx, 15*time.Minute, dbRO)
	promRecorder.Start()

	// Create and start health exporter with all dependencies if enabled
	if config.HealthExporter != nil {
		var err error
		s.healthExporter, err = exporter.New(
			ctx,
			exporter.WithConfig(config.HealthExporter),
			exporter.WithFullConfig(config),
			exporter.WithMetricsStore(metricsSQLiteStore),
			exporter.WithEventStore(eventStore),
			exporter.WithComponentsRegistry(s.componentsRegistry),
			exporter.WithNVMLInstance(nvmlInstance),
			exporter.WithDatabaseConnections(dbRW, dbRO),
			exporter.WithMachineID(machineID),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create health exporter: %w", err)
		}

		// Start the health exporter
		if err := s.healthExporter.Start(); err != nil {
			log.Logger.Errorw("failed to start health exporter", "error", err)
		}
	}

	// Start the HTTP server
	go s.startServer(ctx, nvmlInstance)

	return s, nil
}

// GetHealthExporter returns the health exporter instance (for offline mode access)
func (s *Server) GetHealthExporter() exporter.Exporter {
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

	// Stop DCGM health cache polling to prevent goroutine leak
	if s.gpudInstance != nil && s.gpudInstance.DCGMHealthCache != nil {
		s.gpudInstance.DCGMHealthCache.Stop()
		log.Logger.Debugw("stopped DCGM health cache")
	}

	// Stop DCGM field value cache polling to prevent goroutine leak
	if s.gpudInstance != nil && s.gpudInstance.DCGMFieldValueCache != nil {
		s.gpudInstance.DCGMFieldValueCache.Stop()
		log.Logger.Debugw("stopped DCGM field value cache")
	}

	// Close DCGM policy violation cache to stop distributor and clear policies
	if s.gpudInstance != nil && s.gpudInstance.DCGMPolicyViolationCache != nil {
		if err := s.gpudInstance.DCGMPolicyViolationCache.Close(); err != nil {
			log.Logger.Warnw("failed to close DCGM policy violation cache", "error", err)
		} else {
			log.Logger.Debugw("closed DCGM policy violation cache")
		}
	}

	if s.componentsRegistry != nil {
		for _, component := range s.componentsRegistry.All() {
			if err := component.Close(); err != nil {
				log.Logger.Errorf("failed to close plugin %v: %v", component.Name(), err)
			}
		}
	}

	if s.dbRW != nil {
		if cerr := s.dbRW.Close(); cerr != nil {
			log.Logger.Debugw("failed to close read-write db", "error", cerr)
		} else {
			log.Logger.Debugw("successfully closed read-write db")
		}
	}
	if s.dbRO != nil {
		if cerr := s.dbRO.Close(); cerr != nil {
			log.Logger.Debugw("failed to close read-only db", "error", cerr)
		} else {
			log.Logger.Debugw("successfully closed read-only db")
		}
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

	// Only register fault injection endpoint if explicitly enabled
	if s.config.EnableFaultInjection {
		log.Logger.Infow("registering fault injection endpoint", "path", URLPathInjectFault)
		router.POST(URLPathInjectFault, s.injectFault)
	} else {
		log.Logger.Debugw("fault injection endpoint disabled")
	}

	log.Logger.Infow("fleetint started serving with HTTP", "address", s.config.Address)

	srv := &http.Server{
		Addr:    s.config.Address,
		Handler: router,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Logger.Warnw("fleetint serve failed", "address", s.config.Address, "error", err)
		stdos.Exit(1)
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
