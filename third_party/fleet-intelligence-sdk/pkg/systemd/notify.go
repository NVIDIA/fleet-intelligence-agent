// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package systemd

import (
	"context"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	sd "github.com/coreos/go-systemd/v22/daemon"
)

// NotifyReady notifies systemd that the daemon is ready to serve requests
func NotifyReady(_ context.Context) error {
	return sdNotify(sd.SdNotifyReady)
}

// NotifyStopping notifies systemd that the daemon is about to be stopped
func NotifyStopping(_ context.Context) error {
	return sdNotify(sd.SdNotifyStopping)
}

func sdNotify(state string) error {
	notified, err := sd.SdNotify(false, state)
	log.Logger.Debugw("sd notification", "state", state, "notified", notified, "error", err)
	return err
}
