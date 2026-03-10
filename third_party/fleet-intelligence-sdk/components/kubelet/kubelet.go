// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package kubelet

import (
	pkgfile "github.com/NVIDIA/fleet-intelligence-sdk/pkg/file"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
)

func checkKubeletInstalled() bool {
	p, err := pkgfile.LocateExecutable("kubelet")
	if err == nil {
		log.Logger.Debugw("kubelet found in PATH", "path", p)
		return true
	}
	log.Logger.Debugw("kubelet not found in PATH", "error", err)
	return false
}
