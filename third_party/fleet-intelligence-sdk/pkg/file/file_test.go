// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package file

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocateExecutable(t *testing.T) {
	execPath, err := LocateExecutable("ls")
	require.NoError(t, err, "LocateExecutable() should not fail")
	assert.NotEmpty(t, execPath, "executable path should not be empty")
	t.Logf("found executable %q", execPath)
}

func TestLocateExecutableRejectsRelativePathResult(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "fakebin")
	require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	origWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})

	t.Setenv("PATH", "."+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err = LocateExecutable("fakebin")
	require.Error(t, err, "LocateExecutable must reject binaries found via '.' in PATH")
}
