// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

//go:build darwin
// +build darwin

package uptime

func GetCurrentProcessStartTimeInUnixTime() (uint64, error) {
	return 0, nil
}
