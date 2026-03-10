// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package infiniband

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/NVIDIA/fleet-intelligence-sdk/components/accelerator/nvidia/infiniband/types"
)

func TestDefaultExpectedPortStates(t *testing.T) {
	// Test default values
	defaults := GetDefaultExpectedPortStates()
	assert.Equal(t, 0, defaults.AtLeastPorts)
	assert.Equal(t, 0, defaults.AtLeastRate)

	// Test setting new values
	newStates := types.ExpectedPortStates{
		AtLeastPorts: 2,
		AtLeastRate:  200,
	}
	SetDefaultExpectedPortStates(newStates)

	updated := GetDefaultExpectedPortStates()
	assert.Equal(t, newStates.AtLeastPorts, updated.AtLeastPorts)
	assert.Equal(t, newStates.AtLeastRate, updated.AtLeastRate)
}
