// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package disk

import (
	"context"
	"time"

	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
)

var _ components.HealthSettable = &component{}

func (c *component) SetHealthy() error {
	log.Logger.Infow("set healthy event received for disk")

	if c.eventBucket != nil {
		now := c.getTimeNowFunc()
		cctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		purged, err := c.eventBucket.Purge(cctx, now.Unix())
		cancel()
		if err != nil {
			return err
		}
		log.Logger.Infow("successfully purged disk events", "count", purged)
	}

	return nil
}
