package gpudmanager

import (
	"context"

	pkgconfig "github.com/NVIDIA/fleet-intelligence-sdk/pkg/config"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/gpud-manager/controllers"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/gpud-manager/informer"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/gpud-manager/packages"
)

type Manager struct {
	dataDir           string
	packageController *controllers.PackageController
}

var GlobalController *controllers.PackageController

func New(dataDir string) (*Manager, error) {
	return &Manager{dataDir: dataDir}, nil
}

func (a *Manager) Start(ctx context.Context) error {
	if a.dataDir == "" {
		a.dataDir = pkgconfig.DefaultDataDir
	}

	resolvedDataDir, err := pkgconfig.ResolveDataDir(a.dataDir)
	if err != nil {
		return err
	}
	a.dataDir = resolvedDataDir

	watcher := informer.NewFileInformer(a.dataDir)
	packageController := controllers.NewPackageController(watcher)
	if err := packageController.Run(ctx); err != nil {
		return err
	}
	a.packageController = packageController
	GlobalController = packageController
	return nil
}

func (a *Manager) Status(ctx context.Context) ([]packages.PackageStatus, error) {
	return a.packageController.Status(ctx)
}
