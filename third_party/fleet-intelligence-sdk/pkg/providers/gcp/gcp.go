// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package gcp

import (
	"context"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/providers"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/providers/gcp/imds"
)

const Name = "gcp"

func New() providers.Detector {
	return providers.New(Name, detectProvider, imds.FetchPublicIPv4, nil, nil, imds.FetchInstanceID)
}

func detectProvider(ctx context.Context) (string, error) {
	zone, err := imds.FetchAvailabilityZone(ctx)
	if err != nil {
		return "", err
	}
	if zone != "" {
		return Name, nil
	}
	return "", nil
}
