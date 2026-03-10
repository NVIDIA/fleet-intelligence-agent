// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package nvml

import (
	"errors"
	"testing"
)

type fakeDynamicLibrary struct {
	openErr  error
	lookups  map[string]error
	opened   bool
	closed   bool
	lookuped []string
}

func (f *fakeDynamicLibrary) Open() error {
	f.opened = true
	return f.openErr
}

func (f *fakeDynamicLibrary) Close() error {
	f.closed = true
	return nil
}

func (f *fakeDynamicLibrary) Lookup(symbol string) error {
	f.lookuped = append(f.lookuped, symbol)
	if err, ok := f.lookups[symbol]; ok {
		return err
	}
	return errors.New("symbol not found")
}

func TestPlatformInfoSupported(t *testing.T) {
	t.Run("open fails", func(t *testing.T) {
		origFactory := newDynamicLibrary
		defer func() { newDynamicLibrary = origFactory }()

		newDynamicLibrary = func(name string, flags int) dynamicLibrary {
			return &fakeDynamicLibrary{openErr: errors.New("open failed")}
		}

		if PlatformInfoSupported() {
			t.Fatalf("expected false when open fails")
		}
	})

	t.Run("primary symbol present", func(t *testing.T) {
		origFactory := newDynamicLibrary
		defer func() { newDynamicLibrary = origFactory }()

		fake := &fakeDynamicLibrary{
			lookups: map[string]error{
				platformInfoSymbol: nil,
			},
		}
		newDynamicLibrary = func(name string, flags int) dynamicLibrary { return fake }

		if !PlatformInfoSupported() {
			t.Fatalf("expected true when primary symbol is found")
		}
		if !fake.closed {
			t.Fatalf("expected library to be closed")
		}
	})

	t.Run("no symbols", func(t *testing.T) {
		origFactory := newDynamicLibrary
		defer func() { newDynamicLibrary = origFactory }()

		fake := &fakeDynamicLibrary{
			lookups: map[string]error{
				platformInfoSymbol: errors.New("missing"),
			},
		}
		newDynamicLibrary = func(name string, flags int) dynamicLibrary { return fake }

		if PlatformInfoSupported() {
			t.Fatalf("expected false when no symbols are found")
		}
	})
}
