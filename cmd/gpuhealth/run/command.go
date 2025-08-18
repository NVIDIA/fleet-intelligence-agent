// Package run implements the "run" command.
package run

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
	"github.com/urfave/cli"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	componentsnvidiagpucounts "github.com/leptonai/gpud/components/accelerator/nvidia/gpu-counts"
	componentsinfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	componentsnfs "github.com/leptonai/gpud/components/nfs"
	"github.com/leptonai/gpud/pkg/gpuhealthconfig"
	"github.com/leptonai/gpud/pkg/gpuhealthserver"
	"github.com/leptonai/gpud/pkg/log"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	"github.com/leptonai/gpud/version"
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

func Command(cliContext *cli.Context) error {
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

	configOpts := []gpuhealthconfig.OpOption{
		gpuhealthconfig.WithInfinibandClassRootDir(ibClassRootDir),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	cfg, err := gpuhealthconfig.Default(ctx, configOpts...)
	cancel()
	if err != nil {
		return err
	}

	if listenAddress != "" {
		cfg.Address = listenAddress
	}
	if pprof {
		cfg.Pprof = true
	}

	if retentionPeriod > 0 {
		cfg.RetentionPeriod = metav1.Duration{Duration: retentionPeriod}
	}

	cfg.CompactPeriod = gpuhealthconfig.DefaultCompactPeriod

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
		cfg.HealthExporter = &gpuhealthconfig.HealthExporterConfig{
			Enabled:              true,
			OfflineMode:          true,
			OutputPath:           offlineModePath,
			Duration:             offlineModeDuration,
			IncludeMetrics:       true,
			IncludeEvents:        true,
			IncludeMachineInfo:   true,
			IncludeComponentData: true,
			// Set reasonable intervals for data collection
			Interval:        metav1.Duration{Duration: 30 * time.Second},
			MetricsLookback: metav1.Duration{Duration: 30 * time.Second},
			EventsLookback:  metav1.Duration{Duration: 30 * time.Second},
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

	server, err := gpuhealthserver.New(rootCtx, auditLogger, cfg)
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
