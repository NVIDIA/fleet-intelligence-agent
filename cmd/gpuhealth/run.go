package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	componentsnvidiagpucounts "github.com/leptonai/gpud/components/accelerator/nvidia/gpu-counts"
	nvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	infinibandtypes "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/urfave/cli"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/NVIDIA/gpuhealth/internal/config"
	"github.com/NVIDIA/gpuhealth/internal/server"
	"github.com/NVIDIA/gpuhealth/internal/version"
)

// parseDuration parses duration in HH:MM:SS format
func parseDuration(durationStr string) (time.Duration, error) {
	// Regular expression to match HH:MM:SS format
	re := regexp.MustCompile(`^(\d{1,2}):(\d{2}):(\d{2})$`)
	matches := re.FindStringSubmatch(durationStr)
	if matches == nil {
		return 0, fmt.Errorf("invalid duration format, expected HH:MM:SS")
	}

	hours, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("invalid hours: %v", err)
	}

	minutes, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, fmt.Errorf("invalid minutes: %v", err)
	}

	seconds, err := strconv.Atoi(matches[3])
	if err != nil {
		return 0, fmt.Errorf("invalid seconds: %v", err)
	}

	if minutes >= 60 || seconds >= 60 {
		return 0, fmt.Errorf("minutes and seconds must be less than 60")
	}

	totalDuration := time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
	return totalDuration, nil
}

// Helper functions for environment variable configuration

func setBoolFromEnv(envKey string, target *bool, logMsg, logKey string) error {
	if val := os.Getenv(envKey); val != "" {
		b, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("invalid %s value: %v", envKey, val)
		}
		*target = b
		log.Logger.Infow(logMsg, logKey, b)
	}
	return nil
}

func setDurationFromEnv(envKey string, target *metav1.Duration, logMsg, logKey string, min, max time.Duration) error {
	if val := os.Getenv(envKey); val != "" {
		d, err := time.ParseDuration(val)
		if err != nil {
			return fmt.Errorf("invalid %s value: %v", envKey, val)
		}
		if min > 0 && d < min {
			return fmt.Errorf("%s must be at least %v, got %v", envKey, min, d)
		}
		if max > 0 && d > max {
			return fmt.Errorf("%s must be at most %v, got %v", envKey, max, d)
		}
		target.Duration = d
		log.Logger.Infow(logMsg, logKey, d)
	}
	return nil
}

func setIntFromEnv(envKey string, target *int, logMsg, logKey string, min int) error {
	if val := os.Getenv(envKey); val != "" {
		i, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("invalid %s value: %v", envKey, val)
		}
		if i < min {
			return fmt.Errorf("%s must be at least %d, got %d", envKey, min, i)
		}
		*target = i
		log.Logger.Infow(logMsg, logKey, i)
	}
	return nil
}

// configureHealthExporterFromEnv reads environment variables and applies them to the health exporter configuration
func configureHealthExporterFromEnv(cfg *config.Config) error {
	// Ensure HealthExporter config exists
	if cfg.HealthExporter == nil {
		return fmt.Errorf("health exporter config is nil")
	}
	he := cfg.HealthExporter

	if err := setBoolFromEnv("GPUHEALTH_INCLUDE_METRICS", &he.IncludeMetrics, "set health exporter include metrics from env", "include_metrics"); err != nil {
		return err
	}

	if err := setBoolFromEnv("GPUHEALTH_INCLUDE_EVENTS", &he.IncludeEvents, "set health exporter include events from env", "include_events"); err != nil {
		return err
	}

	if err := setBoolFromEnv("GPUHEALTH_INCLUDE_MACHINEINFO", &he.IncludeMachineInfo, "set health exporter include machine info from env", "include_machine_info"); err != nil {
		return err
	}

	if err := setBoolFromEnv("GPUHEALTH_INCLUDE_HEALTHCHECKS", &he.IncludeComponentData, "set health exporter include component data from env", "include_component_data"); err != nil {
		return err
	}

	if err := setDurationFromEnv("GPUHEALTH_COLLECT_INTERVAL", &he.Interval, "set health exporter interval from env", "interval", time.Second, 24*time.Hour); err != nil {
		return err
	}

	// GPUHEALTH_ATTESTATION_JITTER_ENABLED - Enable/disable attestation jitter
	if err := setBoolFromEnv("GPUHEALTH_ATTESTATION_JITTER_ENABLED", &he.Attestation.JitterEnabled, "set attestation jitter enabled from env", "attestation_jitter_enabled"); err != nil {
		return err
	}

	if err := setDurationFromEnv("GPUHEALTH_ATTESTATION_INTERVAL", &he.Attestation.Interval, "set attestation interval from env", "attestation_interval", 0, 0); err != nil {
		return err
	}

	// Lookbacks
	if err := setDurationFromEnv("GPUHEALTH_METRICS_LOOKBACK", &he.MetricsLookback, "set health exporter metrics lookback from env", "metrics_lookback", 0, 0); err != nil {
		return err
	}

	if err := setDurationFromEnv("GPUHEALTH_EVENTS_LOOKBACK", &he.EventsLookback, "set health exporter events lookback from env", "events_lookback", 0, 0); err != nil {
		return err
	}

	if err := setDurationFromEnv("GPUHEALTH_CHECK_INTERVAL", &he.HealthCheckInterval, "set health exporter health check interval from env", "health_check_interval", time.Second, 24*time.Hour); err != nil {
		return err
	}

	if err := setIntFromEnv("GPUHEALTH_RETRY_MAX_ATTEMPTS", &he.RetryMaxAttempts, "set health exporter retry max attempts from env", "retry_max_attempts", 0); err != nil {
		return err
	}

	return nil
}

func runCommand(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	logFile := cliContext.String("log-file")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, logFile)

	log.Logger.Debugw("starting run command")

	if runtime.GOOS != "linux" {
		fmt.Printf("gpuhealth run on %q not supported\n", runtime.GOOS)
		os.Exit(1)
	}

	if zapLvl.Level() > zap.DebugLevel { // e.g., info, warn, error
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	listenAddress := cliContext.String("listen-address")
	retentionPeriod := cliContext.Duration("retention-period")

	ibClassRootDir := cliContext.String("infiniband-class-root-dir")
	components := cliContext.String("components")

	gpuCount := cliContext.Int("gpu-count")
	infinibandExpectedPortStates := cliContext.String("infiniband-expected-port-states")
	enableDCGMPolicy := cliContext.Bool("enable-dcgm-policy")
	enableFaultInjection := cliContext.Bool("enable-fault-injection")

	// GPU Health Exporter configuration
	offlineMode := cliContext.Bool("offline-mode")
	offlineModePath := cliContext.String("path")
	offlineModeDurationStr := cliContext.String("duration")
	offlineModeOutputFormat := cliContext.String("format")

	var offlineModeDuration time.Duration

	if offlineMode {
		if offlineModePath == "" {
			return fmt.Errorf("--path is required when using --offline-mode")
		}
		if offlineModeDurationStr == "" {
			return fmt.Errorf("--duration is required when using --offline-mode")
		} else {
			parsedDuration, err := parseDuration(offlineModeDurationStr)
			if err != nil {
				return fmt.Errorf("invalid duration: %v", err)
			}
			offlineModeDuration = parsedDuration
		}
	}

	if gpuCount > 0 {
		componentsnvidiagpucounts.SetDefaultExpectedGPUCounts(componentsnvidiagpucounts.ExpectedGPUCounts{
			Count: gpuCount,
		})

		log.Logger.Infow("set gpu count", "gpuCount", gpuCount)
	}

	if len(infinibandExpectedPortStates) > 0 {
		var expectedPortStates infinibandtypes.ExpectedPortStates
		if err := json.Unmarshal([]byte(infinibandExpectedPortStates), &expectedPortStates); err != nil {
			return err
		}
		nvidiainfiniband.SetDefaultExpectedPortStates(expectedPortStates)

		log.Logger.Infow("set infiniband expected port states", "infinibandExpectedPortStates", infinibandExpectedPortStates)
	}

	configOpts := []config.OpOption{
		config.WithInfinibandClassRootDir(ibClassRootDir),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	cfg, err := config.Default(ctx, configOpts...)
	cancel()
	if err != nil {
		return err
	}

	// Apply environment variable overrides to health exporter configuration
	if err := configureHealthExporterFromEnv(cfg); err != nil {
		return fmt.Errorf("failed to configure health exporter from environment variables: %w", err)
	}
	log.Logger.Infow("health exporter configuration", "cfg", cfg.HealthExporter)

	if listenAddress != "" {
		cfg.Address = listenAddress
	}

	if retentionPeriod > 0 {
		cfg.RetentionPeriod = metav1.Duration{Duration: retentionPeriod}
	}

	if components != "" {
		cfg.Components = strings.Split(components, ",")
	}

	cfg.EnableDCGMPolicy = enableDCGMPolicy
	if enableDCGMPolicy {
		log.Logger.Infow("DCGM policy violation monitoring enabled for all policies (XID, PCIe, DBE, NVLink, Power, Thermal, Page Retirement)")
	} else {
		log.Logger.Infow("DCGM policy violation monitoring disabled by default (including XID policy)")
	}

	// Only apply CLI flag if true, to avoid overwriting env var setting
	// (since we can't distinguish between explicit --enable-fault-injection=false and default false)
	if enableFaultInjection {
		cfg.EnableFaultInjection = true
	}

	if cfg.EnableFaultInjection {
		log.Logger.Infow("fault injection endpoint enabled for testing")
	} else {
		log.Logger.Infow("fault injection endpoint disabled")
	}

	// Configure offline mode if enabled
	if offlineMode {
		log.Logger.Infow("configuring offline mode", "path", offlineModePath, "duration", offlineModeDuration)

		// Create output directory if it doesn't exist
		if err := os.MkdirAll(offlineModePath, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %v", err)
		}

		// Create health exporter configuration for offline mode
		cfg.HealthExporter = &config.HealthExporterConfig{
			OfflineMode:          true,
			OutputPath:           offlineModePath,
			Duration:             offlineModeDuration,
			OutputFormat:         offlineModeOutputFormat,
			IncludeMetrics:       cfg.HealthExporter.IncludeMetrics,
			IncludeEvents:        cfg.HealthExporter.IncludeEvents,
			IncludeMachineInfo:   cfg.HealthExporter.IncludeMachineInfo,
			IncludeComponentData: cfg.HealthExporter.IncludeComponentData,
			Interval:             cfg.HealthExporter.Interval,
			Timeout:              cfg.HealthExporter.Timeout,
			MetricsLookback:      cfg.HealthExporter.MetricsLookback,
			EventsLookback:       cfg.HealthExporter.EventsLookback,
			HealthCheckInterval:  cfg.HealthExporter.HealthCheckInterval,
		}
	}

	auditLogger := log.NewNopAuditLogger()
	if logFile != "" {
		logAuditFile := log.CreateAuditLogFilepath(logFile)
		auditLogger = log.NewAuditLogger(logAuditFile)
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	// Create context with appropriate timeout for offline mode
	var rootCtx context.Context
	var rootCancel context.CancelFunc

	if offlineMode && offlineModeDuration > 0 {
		// For duration-based offline mode, set context timeout
		rootCtx, rootCancel = context.WithTimeout(context.Background(), offlineModeDuration)
	} else {
		// For regular mode, use cancellable context
		rootCtx, rootCancel = context.WithCancel(context.Background())
	}
	defer rootCancel()

	start := time.Now()

	signals := make(chan os.Signal, 1)
	done := make(chan struct{})

	log.Logger.Infof("starting gpuhealth %v", version.Version)

	// Setup signal handling for graceful shutdown
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)

	server, err := server.New(rootCtx, auditLogger, cfg)
	if err != nil {
		return err
	}

	// Handle shutdown signals and offline mode duration
	go func() {
		if offlineMode && offlineModeDuration > 0 {
			// For duration-based offline mode, create a timer
			timer := time.NewTimer(offlineModeDuration)
			defer timer.Stop()

			select {
			case sig := <-signals:
				log.Logger.Warnw("received signal -- stopping server", "signal", sig)
			case <-timer.C:
				log.Logger.Infow("offline mode duration expired -- stopping server", "duration", offlineModeDuration)
			}
		} else {
			// For regular mode or static-only mode, just wait for signals
			sig := <-signals
			log.Logger.Warnw("received signal -- stopping server", "signal", sig)
		}

		rootCancel() // Cancel the context
		server.Stop()
		close(done)
	}()

	log.Logger.Infow("successfully booted", "tookSeconds", time.Since(start).Seconds())

	<-done

	return nil
}
