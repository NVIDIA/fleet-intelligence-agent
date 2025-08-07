package gpuhealthconfig

import (
	"context"
	"fmt"
	stdos "os"
	"path/filepath"
	"time"

	"github.com/mitchellh/go-homedir"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nvidiacommon "github.com/leptonai/gpud/pkg/config/common"
)

const (
	// DefaultAPIVersion for health server
	DefaultAPIVersion = "v1"

	// DefaultHealthPort for health metrics export
	DefaultHealthPort = 15133
)

var (
	// DefaultRetentionPeriod - keep health data for 3 hours by default
	DefaultRetentionPeriod = metav1.Duration{Duration: 3 * time.Hour}

	// DefaultCompactPeriod - database compaction disabled by default to avoid performance impact
	DefaultCompactPeriod = metav1.Duration{Duration: 0}
)

// Default creates a default health configuration
func Default(ctx context.Context, opts ...OpOption) (*Config, error) {
	options := &Op{}
	if err := options.ApplyOpts(opts); err != nil {
		return nil, err
	}

	cfg := &Config{
		APIVersion:      DefaultAPIVersion,
		Address:         fmt.Sprintf(":%d", DefaultHealthPort),
		RetentionPeriod: DefaultRetentionPeriod,
		CompactPeriod:   DefaultCompactPeriod,
		Pprof:           false, // Profiling disabled by default for health exporter
		NvidiaToolOverwrites: nvidiacommon.ToolOverwrites{
			InfinibandClassRootDir: options.InfinibandClassRootDir,
		},
	}

	if cfg.State == "" {
		var err error
		cfg.State, err = DefaultStateFile()
		if err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

const defaultVarLibDir = "/var/lib/gpuhealth"

// setupDefaultDir creates the default directory for health data
func setupDefaultDir() (string, error) {
	asRoot := stdos.Geteuid() == 0 // running as root

	d := defaultVarLibDir
	_, err := stdos.Stat("/var/lib")
	if !asRoot || stdos.IsNotExist(err) {
		homeDir, err := homedir.Dir()
		if err != nil {
			return "", err
		}
		d = filepath.Join(homeDir, ".gpuhealth")
	}

	if _, err := stdos.Stat(d); stdos.IsNotExist(err) {
		if err = stdos.MkdirAll(d, 0755); err != nil {
			return "", err
		}
	}
	return d, nil
}

// DefaultStateFile returns the default path for the health state database
func DefaultStateFile() (string, error) {
	dir, err := setupDefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gpuhealth.state"), nil
}
