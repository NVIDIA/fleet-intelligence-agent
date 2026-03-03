package dcgm

import (
	"testing"
)

// Note: Tests for CheckSentinel() and CheckSentinelV2() require actual DCGM library
// and GPU hardware to create proper FieldValue_v1/v2 instances. These are tested
// through integration tests and real usage in components.

func TestSentinelType_ShouldRetry(t *testing.T) {
	tests := []struct {
		name     string
		sentinel SentinelType
		want     bool
	}{
		{"BLANK should retry", SentinelBlank, true},
		{"NOT_FOUND should not retry", SentinelNotFound, false},
		{"NOT_SUPPORTED should not retry", SentinelNotSupported, false},
		{"NOT_PERMISSIONED should not retry", SentinelNotPermissioned, false},
		{"None should not retry", SentinelNone, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sentinel.ShouldRetry(); got != tt.want {
				t.Errorf("SentinelType.ShouldRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSentinelType_String(t *testing.T) {
	tests := []struct {
		name     string
		sentinel SentinelType
		want     string
	}{
		{"BLANK", SentinelBlank, "BLANK (no data yet)"},
		{"NOT_FOUND", SentinelNotFound, "NOT_FOUND (entity missing)"},
		{"NOT_SUPPORTED", SentinelNotSupported, "NOT_SUPPORTED (hardware doesn't support)"},
		{"NOT_PERMISSIONED", SentinelNotPermissioned, "NOT_PERMISSIONED (hostengine lacks CAP_SYS_ADMIN)"},
		{"None", SentinelNone, "OK"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sentinel.String(); got != tt.want {
				t.Errorf("SentinelType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
