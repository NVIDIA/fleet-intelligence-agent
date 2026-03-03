package session

import (
	"context"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/config"
	pkghost "github.com/NVIDIA/fleet-intelligence-sdk/pkg/host"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	pkgmetadata "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metadata"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/sqlite"
)

// processLogout handles the logout request
func (s *Session) processLogout(ctx context.Context, response *Response) {
	stateFile := config.StateFilePath(s.dataDir)

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		log.Logger.Errorw("failed to open state file", "error", err)
		response.Error = err.Error()
		return
	}
	defer func() {
		_ = dbRW.Close()
	}()
	if err = pkgmetadata.DeleteAllMetadata(ctx, dbRW); err != nil {
		log.Logger.Errorw("failed to purge metadata", "error", err)
		response.Error = err.Error()
		return
	}
	err = pkghost.Stop(s.ctx, pkghost.WithDelaySeconds(10))
	if err != nil {
		log.Logger.Errorf("failed to trigger stop gpud: %v", err)
		response.Error = err.Error()
	}
}
