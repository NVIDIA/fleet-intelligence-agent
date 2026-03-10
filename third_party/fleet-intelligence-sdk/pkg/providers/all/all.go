// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

// Package all provides a list of known providers.
package all

import (
	"context"
	"fmt"
	"time"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	pkgproviders "github.com/NVIDIA/fleet-intelligence-sdk/pkg/providers"
	pkgprovidersaws "github.com/NVIDIA/fleet-intelligence-sdk/pkg/providers/aws"
	pkgprovidersazure "github.com/NVIDIA/fleet-intelligence-sdk/pkg/providers/azure"
	pkgprovidersgcp "github.com/NVIDIA/fleet-intelligence-sdk/pkg/providers/gcp"
)

var All = []pkgproviders.Detector{
	pkgprovidersaws.New(),
	pkgprovidersazure.New(),
	pkgprovidersgcp.New(),
}

// Detect detects the provider and returns the provider info.
func Detect(ctx context.Context) (*pkgproviders.Info, error) {
	var detector pkgproviders.Detector
	for _, d := range All {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		provider, err := d.Provider(cctx)
		cancel()
		if err != nil {
			if d != nil {
				log.Logger.Debugw("failed to get provider", "name", d.Name(), "error", err)
			} else {
				log.Logger.Debugw("failed to get provider", "error", err)
			}
			continue
		}

		if provider != "" {
			detector = d
			log.Logger.Infow("detected provider", "provider", provider)
			break
		}
	}

	if detector == nil {
		return &pkgproviders.Info{
			Provider: "unknown",
		}, nil
	}

	info := &pkgproviders.Info{
		Provider: detector.Name(),
	}

	publicIP, err := detector.PublicIPv4(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get public IP: %w (provider: %s)", err, detector.Name())
	}
	info.PublicIP = publicIP

	privateIP, err := detector.PrivateIPv4(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get private IP: %w (provider: %s)", err, detector.Name())
	}
	info.PrivateIP = privateIP
	log.Logger.Infow("successfully detected private IP", "provider", detector.Name(), "privateIP", privateIP)

	vmEnvironment, err := detector.VMEnvironment(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM environment: %w (provider: %s)", err, detector.Name())
	}
	info.VMEnvironment = vmEnvironment

	instanceID, err := detector.InstanceID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance ID: %w (provider: %s)", err, detector.Name())
	}
	info.InstanceID = instanceID
	log.Logger.Infow("successfully detected instance ID", "provider", detector.Name(), "instanceID", instanceID)

	return info, nil
}
