// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

//go:build windows
// +build windows

package memory

func GetCurrentProcessRSSInBytes() (uint64, error) {
	return 0, nil
}
