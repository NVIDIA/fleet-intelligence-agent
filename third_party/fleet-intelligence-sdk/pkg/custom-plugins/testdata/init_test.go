// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package testdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	specs := ExampleSpecs()
	assert.NotNil(t, specs)
}
