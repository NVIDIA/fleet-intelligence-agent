// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

// Package file implements file utils.
package file

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// LocateExecutable finds bin in PATH and returns its absolute path.
// It rejects relative results to prevent PATH-poisoning attacks when the
// agent runs as root.
func LocateExecutable(bin string) (string, error) {
	execPath, err := exec.LookPath(bin)
	if err != nil {
		return "", fmt.Errorf("executable %q not found in PATH: %w", bin, err)
	}
	// exec.LookPath may return a relative path on older Go versions or when
	// "." or an empty element is present in PATH. Reject those results rather
	// than resolving them against the current working directory.
	if !filepath.IsAbs(execPath) {
		return "", fmt.Errorf("executable %q resolved to non-absolute path %q", bin, execPath)
	}
	return execPath, CheckExecutable(execPath)
}

func CheckExecutable(file string) error {
	s, err := os.Stat(file)
	if err != nil {
		return err
	}

	if s.IsDir() {
		return fmt.Errorf("%q is a directory", file)
	}

	if s.Mode()&0111 == 0 {
		return fmt.Errorf("%q is not executable", file)
	}

	return nil
}
