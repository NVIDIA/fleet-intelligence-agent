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
	componentsinfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	componentsnfs "github.com/leptonai/gpud/components/nfs"
	"github.com/leptonai/gpud/pkg/log"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
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

// configureHealthExporterFromEnv reads environment variables and applies them to the health exporter configuration
func configureHealthExporterFromEnv(cfg *config.Config) error {
	// Ensure HealthExporter config exists
	if cfg.HealthExporter == nil {
		return fmt.Errorf("health exporter config is nil")
	}

	// GPUHEALTH_INCLUDE_METRICS - Include metrics in export
	if includeMetrics := os.Getenv("GPUHEALTH_INCLUDE_METRICS"); includeMetrics != "" {
		if val, err := strconv.ParseBool(includeMetrics); err == nil {
			cfg.HealthExporter.IncludeMetrics = val
			log.Logger.Infow("set health exporter include metrics from env", "include_metrics", val)
		} else {
			return fmt.Errorf("invalid GPUHEALTH_INCLUDE_METRICS value: %v", includeMetrics)
		}
	}

	// GPUHEALTH_INCLUDE_EVENTS - Include events in export
	if includeEvents := os.Getenv("GPUHEALTH_INCLUDE_EVENTS"); includeEvents != "" {
		if val, err := strconv.ParseBool(includeEvents); err == nil {
			cfg.HealthExporter.IncludeEvents = val
			log.Logger.Infow("set health exporter include events from env", "include_events", val)
		} else {
			return fmt.Errorf("invalid GPUHEALTH_INCLUDE_EVENTS value: %v", includeEvents)
		}
	}

	// GPUHEALTH_INCLUDE_MACHINEINFO - Include machine info in export
	if includeMachineInfo := os.Getenv("GPUHEALTH_INCLUDE_MACHINEINFO"); includeMachineInfo != "" {
		if val, err := strconv.ParseBool(includeMachineInfo); err == nil {
			cfg.HealthExporter.IncludeMachineInfo = val
			log.Logger.Infow("set health exporter include machine info from env", "include_machine_info", val)
		} else {
			return fmt.Errorf("invalid GPUHEALTH_INCLUDE_MACHINEINFO value: %v", includeMachineInfo)
		}
	}

	// GPUHEALTH_INCLUDE_HEALTHCHECKS - Include component data in export
	if includeComponentData := os.Getenv("GPUHEALTH_INCLUDE_HEALTHCHECKS"); includeComponentData != "" {
		if val, err := strconv.ParseBool(includeComponentData); err == nil {
			cfg.HealthExporter.IncludeComponentData = val
			log.Logger.Infow("set health exporter include component data from env", "include_component_data", val)
		} else {
			return fmt.Errorf("invalid GPUHEALTH_INCLUDE_HEALTHCHECKS value: %v", includeComponentData)
		}
	}

	// GPUHEALTH_COLLECT_INTERVAL - Export interval
	if interval := os.Getenv("GPUHEALTH_COLLECT_INTERVAL"); interval != "" {
		if duration, err := time.ParseDuration(interval); err == nil {
			cfg.HealthExporter.Interval = metav1.Duration{Duration: duration}
			log.Logger.Infow("set health exporter interval from env", "interval", duration)
		} else {
			return fmt.Errorf("invalid GPUHEALTH_COLLECT_INTERVAL value: %v", interval)
		}
	}

	// GPUHEALTH_ATTESTATION_INTERVAL - Attestation interval
	if interval := os.Getenv("GPUHEALTH_ATTESTATION_INTERVAL"); interval != "" {
		if duration, err := time.ParseDuration(interval); err == nil {
			cfg.HealthExporter.AttestationInterval = metav1.Duration{Duration: duration}
			log.Logger.Infow("set attestation interval from env", "attestation_interval", duration)
		} else {
			return fmt.Errorf("invalid GPUHEALTH_ATTESTATION_INTERVAL value: %v", interval)
		}
	}

	// GPUHEALTH_METRICS_LOOKBACK - Metrics lookback duration
	if metricsLookback := os.Getenv("GPUHEALTH_METRICS_LOOKBACK"); metricsLookback != "" {
		if duration, err := time.ParseDuration(metricsLookback); err == nil {
			cfg.HealthExporter.MetricsLookback = metav1.Duration{Duration: duration}
			log.Logger.Infow("set health exporter metrics lookback from env", "metrics_lookback", duration)
		} else {
			return fmt.Errorf("invalid GPUHEALTH_METRICS_LOOKBACK value: %v", metricsLookback)
		}
	}

	// GPUHEALTH_EVENTS_LOOKBACK - Events lookback duration
	if eventsLookback := os.Getenv("GPUHEALTH_EVENTS_LOOKBACK"); eventsLookback != "" {
		if duration, err := time.ParseDuration(eventsLookback); err == nil {
			cfg.HealthExporter.EventsLookback = metav1.Duration{Duration: duration}
			log.Logger.Infow("set health exporter events lookback from env", "events_lookback", duration)
		} else {
			return fmt.Errorf("invalid GPUHEALTH_EVENTS_LOOKBACK value: %v", eventsLookback)
		}
	}

	// GPUHEALTH_CHECK_INTERVAL - Component health check interval
	if healthCheckInterval := os.Getenv("GPUHEALTH_CHECK_INTERVAL"); healthCheckInterval != "" {
		if duration, err := time.ParseDuration(healthCheckInterval); err == nil {
			// Validate the interval range (1 second to 24 hours)
			if duration < time.Second {
				return fmt.Errorf("GPUHEALTH_CHECK_INTERVAL must be at least 1 second, got %v", duration)
			}
			if duration > 24*time.Hour {
				return fmt.Errorf("GPUHEALTH_CHECK_INTERVAL must be at most 24 hours, got %v", duration)
			}
			cfg.HealthExporter.HealthCheckInterval = metav1.Duration{Duration: duration}
			log.Logger.Infow("set health exporter health check interval from env", "health_check_interval", duration)
		} else {
			return fmt.Errorf("invalid GPUHEALTH_CHECK_INTERVAL value: %v", healthCheckInterval)
		}
	}

	// GPUHEALTH_RETRY_MAX_ATTEMPTS - Maximum retry attempts
	if retryMaxAttempts := os.Getenv("GPUHEALTH_RETRY_MAX_ATTEMPTS"); retryMaxAttempts != "" {
		if val, err := strconv.Atoi(retryMaxAttempts); err == nil {
			if val < 0 {
				return fmt.Errorf("GPUHEALTH_RETRY_MAX_ATTEMPTS must be non-negative, got %v", val)
			}
			cfg.HealthExporter.RetryMaxAttempts = val
			log.Logger.Infow("set health exporter retry max attempts from env", "retry_max_attempts", val)
		} else {
			return fmt.Errorf("invalid GPUHEALTH_RETRY_MAX_ATTEMPTS value: %v", retryMaxAttempts)
		}
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
	pprof := cliContext.Bool("pprof")
	retentionPeriod := cliContext.Duration("retention-period")

	ibClassRootDir := cliContext.String("infiniband-class-root-dir")
	components := cliContext.String("components")

	gpuCount := cliContext.Int("gpu-count")
	infinibandExpectedPortStates := cliContext.String("infiniband-expected-port-states")
	nfsCheckerConfigs := cliContext.String("nfs-checker-configs")

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
		var expectedPortStates infiniband.ExpectedPortStates
		if err := json.Unmarshal([]byte(infinibandExpectedPortStates), &expectedPortStates); err != nil {
			return err
		}
		componentsinfiniband.SetDefaultExpectedPortStates(expectedPortStates)

		log.Logger.Infow("set infiniband expected port states", "infinibandExpectedPortStates", infinibandExpectedPortStates)
	}

	if len(nfsCheckerConfigs) > 0 {
		groupConfigs := make(pkgnfschecker.Configs, 0)
		if err := json.Unmarshal([]byte(nfsCheckerConfigs), &groupConfigs); err != nil {
			return err
		}
		componentsnfs.SetDefaultConfigs(groupConfigs)

		log.Logger.Infow("set nfs checker group configs", "groupConfigs", groupConfigs)
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
	if pprof {
		cfg.Pprof = true
	}

	if retentionPeriod > 0 {
		cfg.RetentionPeriod = metav1.Duration{Duration: retentionPeriod}
	}

	cfg.CompactPeriod = config.DefaultCompactPeriod

	if components != "" {
		cfg.Components = strings.Split(components, ",")
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
