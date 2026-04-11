// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/file"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"

	"github.com/dustin/go-humanize"
)

// Runs "findmnt --target [TARGET] --json --df" and parses the output.
// Falls back to running without --df flag if it fails (e.g., in containers with overlay filesystems).
func FindMnt(ctx context.Context, target string) (*FindMntOutput, error) {
	findmntPath, err := file.LocateExecutable("findmnt")
	if err != nil {
		return nil, err
	}

	// Try with --df flag first to get disk usage statistics
	output, err := exec.CommandContext(ctx, findmntPath, "--target", target, "--json", "--df").CombinedOutput()

	// If --df flag fails (common in containers with overlay fs), try without it
	if err != nil {
		log.Logger.Debugw("findmnt with --df failed, retrying without --df flag", "error", err, "target", target)

		output, err = exec.CommandContext(ctx, findmntPath, "--target", target, "--json").CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed to read findmnt output: %w (output: %s)", err, string(output))
		}
	}

	out, err := ParseFindMntOutput(string(output))
	if err != nil {
		return nil, err
	}
	out.Target = target
	return out, nil
}

// Represents the output of the command
// "findmnt --target /var/lib/kubelet --json --df".
// ref. https://man7.org/linux/man-pages/man8/findmnt.8.html
type FindMntOutput struct {
	// The input mount target.
	Target string `json:"target"`

	Filesystems []FoundMnt `json:"filesystems"`
}

type FoundMnt struct {
	// Regardless of the input mount target, this is where the target is mounted.
	MountedPoint string `json:"mounted_point"`

	// The filesystem may use more block devices.
	// This is why findmnt provides  SOURCE and SOURCES (pl.) columns.
	// ref. https://man7.org/linux/man-pages/man8/findmnt.8.html
	Sources []string `json:"sources"`

	Fstype string `json:"fstype"`

	SizeHumanized string `json:"size_humanized"`
	SizeBytes     uint64 `json:"size_bytes"`

	UsedHumanized string `json:"used_humanized"`
	UsedBytes     uint64 `json:"used_bytes"`

	AvailableHumanized string `json:"available_humanized"`
	AvailableBytes     uint64 `json:"available_bytes"`

	UsedPercentHumanized string  `json:"used_percent_humanized"`
	UsedPercent          float64 `json:"used_percent"`
}

type rawFindMntOutput struct {
	Filesystems []rawFoundMnt `json:"filesystems"`
}

type rawFoundMnt struct {
	Target string `json:"target"`

	// The filesystem may use more block devices.
	// This is why findmnt provides  SOURCE and SOURCES (pl.) columns.
	// ref. https://man7.org/linux/man-pages/man8/findmnt.8.html
	Source string `json:"source"`

	Fstype string `json:"fstype"`
	Size   string `json:"size"`
	Used   string `json:"used"`
	Avail  string `json:"avail"`
	UseP   string `json:"use%"`
}

func ParseFindMntOutput(output string) (*FindMntOutput, error) {
	var raw rawFindMntOutput
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, err
	}
	o := &FindMntOutput{}
	for _, rawMntOutput := range raw.Filesystems {
		foundMnt := FoundMnt{
			MountedPoint: rawMntOutput.Target,
			Sources:      extractMntSources(rawMntOutput.Source),
			Fstype:       rawMntOutput.Fstype,
		}

		// Parse disk usage fields if available (from --df flag)
		// These fields may be empty when running without --df (e.g., in containers)
		if rawMntOutput.Size != "" {
			parsedSize, err := humanize.ParseBytes(rawMntOutput.Size)
			if err != nil {
				return nil, fmt.Errorf("failed to parse size %q: %w", rawMntOutput.Size, err)
			}
			foundMnt.SizeHumanized = rawMntOutput.Size
			foundMnt.SizeBytes = parsedSize
		}

		if rawMntOutput.Used != "" {
			parsedUsed, err := humanize.ParseBytes(rawMntOutput.Used)
			if err != nil {
				return nil, fmt.Errorf("failed to parse used %q: %w", rawMntOutput.Used, err)
			}
			foundMnt.UsedHumanized = rawMntOutput.Used
			foundMnt.UsedBytes = parsedUsed
		}

		if rawMntOutput.Avail != "" {
			parsedAvail, err := humanize.ParseBytes(rawMntOutput.Avail)
			if err != nil {
				return nil, fmt.Errorf("failed to parse avail %q: %w", rawMntOutput.Avail, err)
			}
			foundMnt.AvailableHumanized = rawMntOutput.Avail
			foundMnt.AvailableBytes = parsedAvail
		}

		if rawMntOutput.UseP != "" {
			usePFloat, err := strconv.ParseFloat(strings.TrimSuffix(rawMntOutput.UseP, "%"), 64)
			if err != nil {
				return nil, fmt.Errorf("failed to parse use%% %q: %w", rawMntOutput.UseP, err)
			}
			foundMnt.UsedPercentHumanized = rawMntOutput.UseP
			foundMnt.UsedPercent = usePFloat
		}

		o.Filesystems = append(o.Filesystems, foundMnt)
	}
	return o, nil
}

// extractMntSources extracts mount sources from the findmnt source output.
//
// e.g.,
// "/dev/mapper/vgroot-lvroot[/var/lib/lxc/ny2g2r14hh2-lxc/rootfs]"
// becomes
// ["/dev/mapper/vgroot-lvroot", "/var/lib/lxc/ny2g2r14hh2-lxc/rootfs"]
//
// e.g.,
// "/dev/mapper/lepton_vg-lepton_lv[/kubelet]"
// becomes
// ["/dev/mapper/lepton_vg-lepton_lv", "/kubelet"]
func extractMntSources(input string) []string {
	src := strings.TrimSuffix(input, "]")
	sources := make([]string, 0)
	for _, s := range strings.Split(src, "[") {
		if s == "" {
			continue
		}
		for _, ss := range strings.Split(s, ",") {
			sources = append(sources, strings.TrimSpace(ss))
		}
	}
	return sources
}
