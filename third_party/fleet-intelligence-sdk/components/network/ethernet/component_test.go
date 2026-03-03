package ethernet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func touch(t *testing.T, path string) {
	t.Helper()
	writeFile(t, path, "")
}

func TestDiscoverPhysicalEthernetInterfaces(t *testing.T) {
	orig := sysClassNetPath
	t.Cleanup(func() { sysClassNetPath = orig })

	root := t.TempDir()
	sysClassNetPath = filepath.Join(root, "sys/class/net")

	// Physical ethernet: should be included.
	enp := filepath.Join(sysClassNetPath, "enp0s31f6")
	if err := os.MkdirAll(enp, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	touch(t, filepath.Join(enp, "device"))
	writeFile(t, filepath.Join(enp, "type"), "1\n")
	// no wireless dir

	// Wifi interface: excluded via /wireless.
	wlp := filepath.Join(sysClassNetPath, "wlp0s20f3")
	if err := os.MkdirAll(wlp, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	touch(t, filepath.Join(wlp, "device"))
	writeFile(t, filepath.Join(wlp, "type"), "1\n")
	if err := os.MkdirAll(filepath.Join(wlp, "wireless"), 0o755); err != nil {
		t.Fatalf("mkdir wireless: %v", err)
	}

	// Loopback: excluded.
	lo := filepath.Join(sysClassNetPath, "lo")
	if err := os.MkdirAll(lo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	touch(t, filepath.Join(lo, "device"))
	writeFile(t, filepath.Join(lo, "type"), "772\n")

	// Non-ethernet (type != 1): excluded even if has device.
	ib := filepath.Join(sysClassNetPath, "ib0")
	if err := os.MkdirAll(ib, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	touch(t, filepath.Join(ib, "device"))
	writeFile(t, filepath.Join(ib, "type"), "32\n")

	// Virtual net device: excluded if symlink resolves to a path containing "/virtual/".
	// Simulate /sys/class/net/vnet0 -> <root>/sys/devices/virtual/net/vnet0
	virtualTarget := filepath.Join(root, "sys/devices/virtual/net/vnet0")
	if err := os.MkdirAll(virtualTarget, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	touch(t, filepath.Join(virtualTarget, "device"))
	writeFile(t, filepath.Join(virtualTarget, "type"), "1\n")
	if err := os.Symlink(virtualTarget, filepath.Join(sysClassNetPath, "vnet0")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	ifaces, err := discoverPhysicalEthernetInterfaces()
	if err != nil {
		t.Fatalf("discoverPhysicalEthernetInterfaces: %v", err)
	}
	if len(ifaces) != 1 || ifaces[0] != "enp0s31f6" {
		t.Fatalf("unexpected interfaces: %v", ifaces)
	}
}

func TestReadInterfaceStats_LinkUpAndCounters(t *testing.T) {
	orig := sysClassNetPath
	t.Cleanup(func() { sysClassNetPath = orig })

	root := t.TempDir()
	sysClassNetPath = filepath.Join(root, "sys/class/net")

	iface := "enp0s31f6"
	base := filepath.Join(sysClassNetPath, iface)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(base, "operstate"), "up\n")
	writeFile(t, filepath.Join(base, "carrier"), "1\n")

	stats := filepath.Join(base, "statistics")
	writeFile(t, filepath.Join(stats, "rx_bytes"), "10\n")
	writeFile(t, filepath.Join(stats, "rx_packets"), "11\n")
	writeFile(t, filepath.Join(stats, "rx_errors"), "12\n")
	writeFile(t, filepath.Join(stats, "rx_dropped"), "13\n")
	writeFile(t, filepath.Join(stats, "tx_bytes"), "20\n")
	writeFile(t, filepath.Join(stats, "tx_packets"), "21\n")
	writeFile(t, filepath.Join(stats, "tx_errors"), "22\n")
	writeFile(t, filepath.Join(stats, "tx_dropped"), "23\n")

	st := InterfaceStats{Interface: iface, ReadErrors: make(map[string]string)}
	readLinkState(&st)
	if !st.LinkUp || st.OperState != "up" || st.Carrier == nil || *st.Carrier != true {
		t.Fatalf("unexpected link state: %+v", st)
	}

	resetEthernetMetrics()
	if err := setInterfaceCounters(prometheus.Labels{"interface": iface}, &st); err != nil {
		t.Fatalf("setInterfaceCounters: %v", err)
	}
	if st.RxBytes != 10 || st.RxPackets != 11 || st.RxErrors != 12 || st.RxDropped != 13 ||
		st.TxBytes != 20 || st.TxPackets != 21 || st.TxErrors != 22 || st.TxDropped != 23 {
		t.Fatalf("unexpected counters: %+v", st)
	}
}

func TestReadInterfaceStats_CarrierFallback(t *testing.T) {
	orig := sysClassNetPath
	t.Cleanup(func() { sysClassNetPath = orig })

	root := t.TempDir()
	sysClassNetPath = filepath.Join(root, "sys/class/net")

	iface := "enp0s31f6"
	base := filepath.Join(sysClassNetPath, iface)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Operstate unknown, so LinkUp should fall back to carrier.
	writeFile(t, filepath.Join(base, "operstate"), "unknown\n")
	writeFile(t, filepath.Join(base, "carrier"), "0\n")

	stats := filepath.Join(base, "statistics")
	writeFile(t, filepath.Join(stats, "rx_bytes"), "0\n")
	writeFile(t, filepath.Join(stats, "rx_packets"), "0\n")
	writeFile(t, filepath.Join(stats, "rx_errors"), "0\n")
	writeFile(t, filepath.Join(stats, "rx_dropped"), "0\n")
	writeFile(t, filepath.Join(stats, "tx_bytes"), "0\n")
	writeFile(t, filepath.Join(stats, "tx_packets"), "0\n")
	writeFile(t, filepath.Join(stats, "tx_errors"), "0\n")
	writeFile(t, filepath.Join(stats, "tx_dropped"), "0\n")

	st := InterfaceStats{Interface: iface, ReadErrors: make(map[string]string)}
	readLinkState(&st)
	if st.LinkUp {
		t.Fatalf("expected link down, got link up: %+v", st)
	}
	if st.Carrier == nil || *st.Carrier != false {
		t.Fatalf("expected carrier false, got: %+v", st)
	}
}

func TestReadInterfaceStats_PartialReturnsErrorAfterRead(t *testing.T) {
	orig := sysClassNetPath
	t.Cleanup(func() { sysClassNetPath = orig })

	root := t.TempDir()
	sysClassNetPath = filepath.Join(root, "sys/class/net")

	iface := "enp0s31f6"
	base := filepath.Join(sysClassNetPath, iface)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(base, "operstate"), "up\n")
	writeFile(t, filepath.Join(base, "carrier"), "1\n")

	stats := filepath.Join(base, "statistics")
	// Provide only a subset of files to force partial stats.
	writeFile(t, filepath.Join(stats, "rx_bytes"), "1\n")
	writeFile(t, filepath.Join(stats, "tx_bytes"), "2\n")

	st := InterfaceStats{Interface: iface, ReadErrors: make(map[string]string)}
	readLinkState(&st)
	if !st.LinkUp {
		t.Fatalf("expected link up, got: %+v", st)
	}
	resetEthernetMetrics()
	err := setInterfaceCounters(prometheus.Labels{"interface": iface}, &st)
	if err == nil {
		t.Fatalf("expected error for partial stats, got nil")
	}
	if len(st.MissingStats) == 0 {
		t.Fatalf("expected missing stats list, got none")
	}
	if st.ReadErrors == nil {
		t.Fatalf("expected read_errors to be set")
	}
}

func TestComputeHealth(t *testing.T) {
	t.Run("no interfaces => degraded", func(t *testing.T) {
		h, _ := computeHealth(nil, nil, nil, 0, nil)
		if h != apiv1.HealthStateTypeDegraded {
			t.Fatalf("expected degraded, got %q", h)
		}
	})

	t.Run("interfaces but none link up => unhealthy", func(t *testing.T) {
		ifaces := []string{"eth0", "eth1"}
		linkUp := []string{}
		partialUp := []string{}
		linkDown := []string{"eth0", "eth1"}
		h, _ := computeHealth(ifaces, linkUp, partialUp, 0, linkDown)
		if h != apiv1.HealthStateTypeUnhealthy {
			t.Fatalf("expected unhealthy, got %q", h)
		}
	})

	t.Run("link up but no complete stats => degraded", func(t *testing.T) {
		ifaces := []string{"eth0"}
		linkUp := []string{"eth0"}
		partialUp := []string{"eth0"}
		linkDown := []string{}
		h, _ := computeHealth(ifaces, linkUp, partialUp, 1, linkDown)
		if h != apiv1.HealthStateTypeDegraded {
			t.Fatalf("expected degraded, got %q", h)
		}
	})

	t.Run("link up and at least one complete stats => healthy", func(t *testing.T) {
		ifaces := []string{"eth0", "eth1"}
		linkUp := []string{"eth0"}
		partialUp := []string{}
		linkDown := []string{"eth1"}
		h, _ := computeHealth(ifaces, linkUp, partialUp, 0, linkDown)
		if h != apiv1.HealthStateTypeHealthy {
			t.Fatalf("expected healthy, got %q", h)
		}
	})

	t.Run("multiple up links but one has partial stats => degraded", func(t *testing.T) {
		ifaces := []string{"eth0", "eth1"}
		linkUp := []string{"eth0", "eth1"}
		partialUp := []string{"eth1"}
		linkDown := []string{}
		h, reason := computeHealth(ifaces, linkUp, partialUp, 1, linkDown)
		if h != apiv1.HealthStateTypeDegraded {
			t.Fatalf("expected degraded, got %q", h)
		}
		if reason == "" || !strings.Contains(reason, "eth1") {
			t.Fatalf("expected reason to include partial interface name eth1, got %q", reason)
		}
	})
}

func TestDeleteInterfaceMetrics(t *testing.T) {
	labels := prometheus.Labels{"interface": "test0"}
	metricLinkUp.With(labels).Set(1)
	metricRxBytes.With(labels).Set(123)

	resetEthernetMetrics()

	// After reset, deleting the series should return false (already gone).
	if metricLinkUp.Delete(labels) {
		t.Fatalf("expected metricLinkUp series to be reset")
	}
	if metricRxBytes.Delete(labels) {
		t.Fatalf("expected metricRxBytes series to be reset")
	}
}

func TestResetEthernetMetricsRemovesMissingInterfaces(t *testing.T) {
	metricLinkUp.With(prometheus.Labels{"interface": "old0"}).Set(1)
	metricRxBytes.With(prometheus.Labels{"interface": "old0"}).Set(1)

	resetEthernetMetrics()

	if metricLinkUp.Delete(prometheus.Labels{"interface": "old0"}) {
		t.Fatalf("expected old0 link_up series to be reset")
	}
	if metricRxBytes.Delete(prometheus.Labels{"interface": "old0"}) {
		t.Fatalf("expected old0 rx_bytes series to be reset")
	}
}
