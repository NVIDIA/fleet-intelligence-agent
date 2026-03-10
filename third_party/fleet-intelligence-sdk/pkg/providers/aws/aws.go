// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package aws

import (
	"context"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/providers"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/providers/aws/imds"
)

const Name = "aws"

func New() providers.Detector {
	return providers.New(Name, detectProvider, imds.FetchPublicIPv4, imds.FetchLocalIPv4, nil, imds.FetchInstanceID)
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
