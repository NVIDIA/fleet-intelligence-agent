// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package kernelmodule

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ref. https://manpages.ubuntu.com/manpages/xenial/man5/modules.5.html
const DefaultEtcModulesPath = "/etc/modules"

// ref. https://www.freedesktop.org/software/systemd/man/modules-load.d.html
const DefaultModulesLoadDPath = "/etc/modules-load.d"

// parseEtcModules parses the "/etc/modules" file to list the kernel modules to load at boot time.
// ref. https://manpages.ubuntu.com/manpages/xenial/man5/modules.5.html
func parseEtcModules(b []byte) ([]string, error) {
	modules := []string{}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		modules = append(modules, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Strings(modules)
	return modules, nil
}

// parseModulesLoadD parses files in "/etc/modules-load.d/" directory to list kernel modules to load at boot time.
// ref. https://www.freedesktop.org/software/systemd/man/modules-load.d.html
func parseModulesLoadD(dirPath string) ([]string, error) {
	modules := make(map[string]struct{}) // Use map to deduplicate

	// Check if directory exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return []string{}, nil // Directory doesn't exist, return empty list
	}

	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Only process .conf files
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".conf") {
			return nil
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %q: %w", path, err)
		}

		fileModules, err := parseEtcModules(b) // Reuse the same parsing logic
		if err != nil {
			return fmt.Errorf("failed to parse %q: %w", path, err)
		}

		// Add modules to the map (deduplicates automatically)
		for _, module := range fileModules {
			modules[module] = struct{}{}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Convert map back to sorted slice
	result := make([]string, 0, len(modules))
	for module := range modules {
		result = append(result, module)
	}
	sort.Strings(result)
	return result, nil
}

func getAllModules() ([]string, error) {
	allModules := make(map[string]struct{}) // Use map to deduplicate across both sources

	// First, try to read /etc/modules (traditional approach)
	if _, err := os.Stat(DefaultEtcModulesPath); err == nil {
		b, err := os.ReadFile(DefaultEtcModulesPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read %q: %w", DefaultEtcModulesPath, err)
		}
		modules, err := parseEtcModules(b)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %q: %w", DefaultEtcModulesPath, err)
		}
		for _, module := range modules {
			allModules[module] = struct{}{}
		}
	}

	// Then, try to read /etc/modules-load.d/ (systemd approach)
	modulesLoadDModules, err := parseModulesLoadD(DefaultModulesLoadDPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse modules-load.d: %w", err)
	}
	for _, module := range modulesLoadDModules {
		allModules[module] = struct{}{}
	}

	// Convert map back to sorted slice
	result := make([]string, 0, len(allModules))
	for module := range allModules {
		result = append(result, module)
	}
	sort.Strings(result)
	return result, nil
}
