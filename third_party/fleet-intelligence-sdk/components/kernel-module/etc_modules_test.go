// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package kernelmodule

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEtcModules(t *testing.T) {
	// ref. https://manpages.ubuntu.com/manpages/xenial/man5/modules.5.html
	input := `
           # /etc/modules: kernel modules to load at boot time.
           #
           # This file contains the names of kernel modules that
           # should be loaded at boot time, one per line. Lines
           # beginning with "#" are ignored.

           w83781d

           3c509 irq=15
           nf_nat_ftp

`
	modules, err := parseEtcModules([]byte(input))
	assert.NoError(t, err, "failed to parse /etc/modules")
	t.Logf("modules: %v", modules)

	assert.Len(t, modules, 3, "expected 3 modules")
	assert.Equal(t, "3c509 irq=15", modules[0], "unexpected first module")
	assert.Equal(t, "nf_nat_ftp", modules[1], "unexpected second module")
	assert.Equal(t, "w83781d", modules[2], "unexpected third module")
}

func TestParseModulesLoadD(t *testing.T) {
	// Create a temporary directory structure
	tempDir := t.TempDir()
	modulesLoadDir := filepath.Join(tempDir, "modules-load.d")
	err := os.MkdirAll(modulesLoadDir, 0755)
	require.NoError(t, err)

	// Create test .conf files
	testFiles := map[string]string{
		"test1.conf": `# Test file 1
nvidia
nvidia-uvm

# Another comment
`,
		"test2.conf": `# Test file 2
i2c-dev
spi-bcm2835
`,
		"test3.conf": `# Empty lines and comments only

# Just a comment
`,
		"ignored.txt": `# This file should be ignored
should-not-be-parsed
`,
	}

	for filename, content := range testFiles {
		filePath := filepath.Join(modulesLoadDir, filename)
		err := os.WriteFile(filePath, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Test parsing the directory
	modules, err := parseModulesLoadD(modulesLoadDir)
	require.NoError(t, err)
	t.Logf("parsed modules: %v", modules)

	// Should have 4 modules (nvidia, nvidia-uvm, i2c-dev, spi-bcm2835)
	// The .txt file should be ignored, and empty files should contribute nothing
	assert.Len(t, modules, 4, "expected 4 modules")
	assert.Contains(t, modules, "nvidia")
	assert.Contains(t, modules, "nvidia-uvm")
	assert.Contains(t, modules, "i2c-dev")
	assert.Contains(t, modules, "spi-bcm2835")
	assert.NotContains(t, modules, "should-not-be-parsed")
}

func TestParseModulesLoadD_NonExistentDir(t *testing.T) {
	// Test with non-existent directory
	tempDir := t.TempDir()
	nonExistentDir := filepath.Join(tempDir, "non-existent")

	modules, err := parseModulesLoadD(nonExistentDir)
	require.NoError(t, err)
	assert.Empty(t, modules, "should return empty slice for non-existent directory")
}

func TestParseModulesLoadD_EmptyDir(t *testing.T) {
	// Test with empty directory
	tempDir := t.TempDir()
	emptyDir := filepath.Join(tempDir, "empty")
	err := os.MkdirAll(emptyDir, 0755)
	require.NoError(t, err)

	modules, err := parseModulesLoadD(emptyDir)
	require.NoError(t, err)
	assert.Empty(t, modules, "should return empty slice for empty directory")
}

func TestGetAllModules_BothSources(t *testing.T) {
	// This test checks the real system behavior
	// We can't easily mock the constants, so we test with the actual system
	modules, err := getAllModules()

	// The function should not return an error even if files don't exist
	require.NoError(t, err)
	t.Logf("getAllModules returned %d modules", len(modules))

	// We can't assert specific content since it depends on the system,
	// but we can verify the function works without errors
}
