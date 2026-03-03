package disk

import (
	"os"
	"reflect"
	"testing"
)

func TestParseFindMntOutput(t *testing.T) {
	for _, file := range []string{"findmnt.0.json", "findmnt.1.json"} {
		b, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatalf("error reading test data: %v", err)
		}
		output, err := ParseFindMntOutput(string(b))
		if err != nil {
			t.Fatalf("error finding mount target output: %v", err)
		}
		t.Logf("output: %+v", output)
	}
}

func TestParseFindMntOutput_WithoutDfFlag(t *testing.T) {
	// Test parsing findmnt output without --df flag (typical in containers with overlay fs)
	// This output doesn't include size, used, avail, or use% fields
	containerOutput := `{
   "filesystems": [
      {
         "target": "/",
         "source": "overlay",
         "fstype": "overlay",
         "options": "rw,relatime,lowerdir=/var/lib/rancher/k3s/agent/containerd/io.containerd.snapshotter.v1.overlayfs/snapshots/242/fs"
      }
   ]
}`

	output, err := ParseFindMntOutput(containerOutput)
	if err != nil {
		t.Fatalf("error parsing findmnt output without --df: %v", err)
	}

	if len(output.Filesystems) != 1 {
		t.Fatalf("expected 1 filesystem, got %d", len(output.Filesystems))
	}

	fs := output.Filesystems[0]
	if fs.MountedPoint != "/" {
		t.Errorf("expected target '/', got %q", fs.MountedPoint)
	}
	if fs.Fstype != "overlay" {
		t.Errorf("expected fstype 'overlay', got %q", fs.Fstype)
	}
	if len(fs.Sources) != 1 || fs.Sources[0] != "overlay" {
		t.Errorf("expected sources ['overlay'], got %v", fs.Sources)
	}

	// Verify disk usage fields are zero/empty when not provided
	if fs.SizeBytes != 0 {
		t.Errorf("expected SizeBytes to be 0, got %d", fs.SizeBytes)
	}
	if fs.UsedBytes != 0 {
		t.Errorf("expected UsedBytes to be 0, got %d", fs.UsedBytes)
	}
	if fs.AvailableBytes != 0 {
		t.Errorf("expected AvailableBytes to be 0, got %d", fs.AvailableBytes)
	}
	if fs.UsedPercent != 0 {
		t.Errorf("expected UsedPercent to be 0, got %f", fs.UsedPercent)
	}

	t.Logf("successfully parsed container overlay fs output: %+v", fs)
}

func TestExtractMntSources(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single source without brackets",
			input:    "/dev/sda1",
			expected: []string{"/dev/sda1"},
		},
		{
			name:     "source with path in brackets",
			input:    "/dev/mapper/vgroot-lvroot[/var/lib/lxc/ny2g2r14hh2-lxc/rootfs]",
			expected: []string{"/dev/mapper/vgroot-lvroot", "/var/lib/lxc/ny2g2r14hh2-lxc/rootfs"},
		},
		{
			name:     "source with simple path in brackets",
			input:    "/dev/mapper/lepton_vg-lepton_lv[/kubelet]",
			expected: []string{"/dev/mapper/lepton_vg-lepton_lv", "/kubelet"},
		},
		{
			name:     "multiple comma-separated sources",
			input:    "source1,source2[/path1,/path2]",
			expected: []string{"source1", "source2", "/path1", "/path2"},
		},
		{
			name:     "edge case with empty sections",
			input:    "[/path]",
			expected: []string{"/path"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractMntSources(tc.input)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("extractMntSources(%q) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}
