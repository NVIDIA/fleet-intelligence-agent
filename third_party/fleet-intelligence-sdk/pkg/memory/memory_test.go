// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package memory

import (
	"testing"
)

func TestGetCurrentProcessRSSInBytes(t *testing.T) {
	bytes, err := GetCurrentProcessRSSInBytes()
	if err != nil {
		t.Fatalf("failed to get bytes: %v", err)
	}
	t.Logf("bytes: %v", bytes)
}
