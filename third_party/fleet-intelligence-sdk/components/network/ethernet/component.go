// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

// Package ethernet tracks per-interface ethernet statistics and current link state.
//
// Scope (initial): physical NIC ports only (exclude virtual/container/tunnel interfaces).
package ethernet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
)

// Name is the ID of the ethernet component.
const Name = "network-ethernet"

const (
	defaultHealthCheckInterval = time.Minute
)

// sysClassNetPath is the sysfs path used to discover and read interface stats.
var sysClassNetPath = "/sys/class/net"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	healthCheckInterval time.Duration

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)

	healthCheckInterval := defaultHealthCheckInterval
	if gpudInstance.HealthCheckInterval > 0 {
		healthCheckInterval = gpudInstance.HealthCheckInterval
	}

	return &component{
		ctx:                 cctx,
		cancel:              ccancel,
		healthCheckInterval: healthCheckInterval,
	}, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		"network",
		Name,
	}
}

func (c *component) IsSupported() bool { return true }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(c.healthCheckInterval)
		defer ticker.Stop()

		for {
			_ = c.Check()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	last := c.lastCheckResult
	c.lastMu.RUnlock()
	return last.HealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	// skeleton: link up/down history (kmsg/journald) will be added later
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")
	c.cancel()
	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking ethernet interfaces")

	cr := &checkResult{ts: time.Now().UTC()}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	// Reset all ethernet metrics at the beginning of each check.
	// This avoids stale series without tracking/deleting per-interface metric labels,
	// even if discovery fails.
	resetEthernetMetrics()

	ifaces, err := discoverPhysicalEthernetInterfaces()
	if err != nil {
		cr.err = err
		// Treat discovery failures as degraded: we cannot determine ethernet state.
		cr.health = apiv1.HealthStateTypeDegraded
		cr.reason = "failed to discover ethernet interfaces"
		return cr
	}

	stats := make([]InterfaceStats, 0, len(ifaces))
	// Track stat-read quality for link-UP interfaces only.
	// Down links are common.
	var partialStats int
	var linkDownIfaces []string
	var linkUpIfaces []string
	var partialUpIfaces []string
	for _, iface := range ifaces {
		st := InterfaceStats{Interface: iface}
		st.ReadErrors = make(map[string]string)

		readLinkState(&st)
		if !st.LinkUp {
			linkDownIfaces = append(linkDownIfaces, iface)
		} else {
			linkUpIfaces = append(linkUpIfaces, iface)
		}

		labels := prometheus.Labels{"interface": iface}
		// Emit link state for every interface.
		if st.LinkUp {
			metricLinkUp.With(labels).Set(1)
		} else {
			metricLinkUp.With(labels).Set(0)
		}

		// Read and emit counters only if link is up.
		// For link-down interfaces, we only emit link_up=0.
		if !st.LinkUp {
			stats = append(stats, st)
			continue
		}

		err := setInterfaceCounters(labels, &st)
		if err != nil {
			partialStats++
			partialUpIfaces = append(partialUpIfaces, iface)
			log.Logger.Warnw("partial ethernet interface stats read; emitting available counters", "interface", iface, "missing", st.MissingStats, "error", err)
		}

		stats = append(stats, st)
	}

	sort.Slice(stats, func(i, j int) bool { return stats[i].Interface < stats[j].Interface })
	cr.InterfaceStats = stats

	// Health evaluation policy (physical NIC ports only):
	// - Degraded: no physical ethernet interfaces found
	// - Unhealthy: physical interfaces exist but all links are down
	// - Degraded: at least one link is up but couldn't read stats for some of the up links
	// - Healthy: at least one physical link is up and we could read stats for all up interfaces
	//
	// Note: down ports are common; we only mark unhealthy when ALL physical links are down.
	cr.health, cr.reason = computeHealth(ifaces, linkUpIfaces, partialUpIfaces, partialStats, linkDownIfaces)
	return cr
}

func computeHealth(ifaces, linkUpIfaces, partialUpIfaces []string, partialStats int, linkDownIfaces []string) (apiv1.HealthStateType, string) {
	switch {
	case len(ifaces) == 0:
		return apiv1.HealthStateTypeDegraded, "no physical ethernet interfaces found"
	case len(linkUpIfaces) == 0 && len(ifaces) > 0:
		return apiv1.HealthStateTypeUnhealthy, fmt.Sprintf("all ethernet links are down (%s)", strings.Join(linkDownIfaces, ", "))
	case partialStats > 0 && len(linkUpIfaces) > 0:
		return apiv1.HealthStateTypeDegraded, fmt.Sprintf("partial stats for up interfaces (%s)", strings.Join(partialUpIfaces, ", "))
	default:
		linkUp := len(ifaces) - len(linkDownIfaces)
		linkDown := len(linkDownIfaces)

		reason := fmt.Sprintf("ok (interfaces=%d, link_up=%d, link_down=%d)", len(ifaces), linkUp, linkDown)
		// Keep some signal about down links without degrading the health status.
		if linkDown > 0 {
			reason = fmt.Sprintf("%s (down: %s)", reason, strings.Join(linkDownIfaces, ", "))
		}
		return apiv1.HealthStateTypeHealthy, reason
	}
}

type InterfaceStats struct {
	Interface string `json:"interface"`

	// Current link state from sysfs.
	OperState string `json:"oper_state,omitempty"`
	Carrier   *bool  `json:"carrier,omitempty"`
	LinkUp    bool   `json:"link_up"`

	RxBytes   uint64 `json:"rx_bytes"`
	RxPackets uint64 `json:"rx_packets"`
	RxErrors  uint64 `json:"rx_errors"`
	RxDropped uint64 `json:"rx_dropped"`

	TxBytes   uint64 `json:"tx_bytes"`
	TxPackets uint64 `json:"tx_packets"`
	TxErrors  uint64 `json:"tx_errors"`
	TxDropped uint64 `json:"tx_dropped"`

	// MissingStats lists any stats files (e.g., "rx_bytes") that couldn't be read/parsed.
	MissingStats []string          `json:"missing_stats,omitempty"`
	ReadErrors   map[string]string `json:"read_errors,omitempty"`
}

func discoverPhysicalEthernetInterfaces() ([]string, error) {
	ents, err := os.ReadDir(sysClassNetPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sysClassNetPath, err)
	}

	out := make([]string, 0, len(ents))
	for _, ent := range ents {
		if !ent.IsDir() && ent.Type()&os.ModeSymlink == 0 {
			// /sys/class/net entries are typically symlinks; tolerate but ignore unexpected types.
			continue
		}
		iface := ent.Name()
		if iface == "lo" {
			continue
		}

		base := filepath.Join(sysClassNetPath, iface)

		// physical device required
		if _, err := os.Stat(filepath.Join(base, "device")); err != nil {
			continue
		}

		// exclude wifi (still "type 1" typically)
		if _, err := os.Stat(filepath.Join(base, "wireless")); err == nil {
			continue
		}

		// require ethernet ARPHRD_ETHER (=1)
		t, err := readUint64(filepath.Join(base, "type"))
		if err != nil || t != 1 {
			continue
		}

		// exclude virtual net devices (extra guard)
		if resolved, err := filepath.EvalSymlinks(base); err == nil {
			if strings.Contains(resolved, "/virtual/") {
				continue
			}
		}

		out = append(out, iface)
	}

	sort.Strings(out)
	return out, nil
}

func readLinkState(st *InterfaceStats) {
	base := filepath.Join(sysClassNetPath, st.Interface)

	oper, err := os.ReadFile(filepath.Join(base, "operstate"))
	if err == nil {
		st.OperState = strings.TrimSpace(string(oper))
	} else {
		// best-effort: do not fail the whole read on operstate
		st.ReadErrors["operstate"] = err.Error()
	}

	// carrier is best-effort (not all devices expose it)
	if b, err := os.ReadFile(filepath.Join(base, "carrier")); err == nil {
		v := strings.TrimSpace(string(b))
		switch v {
		case "1":
			t := true
			st.Carrier = &t
		case "0":
			f := false
			st.Carrier = &f
		}
	} else {
		st.ReadErrors["carrier"] = err.Error()
	}

	// derive boolean link state:
	// - prefer operstate == "up"
	// - fall back to carrier if operstate is unexpected
	st.LinkUp = (st.OperState == "up")
	if st.OperState == "" || st.OperState == "unknown" {
		if st.Carrier != nil {
			st.LinkUp = *st.Carrier
		}
	}
}

func setInterfaceCounters(labels prometheus.Labels, st *InterfaceStats) error {
	base := filepath.Join(sysClassNetPath, st.Interface)
	statsDir := filepath.Join(base, "statistics")

	readStat := func(name string, set func(uint64)) {
		v, err := readUint64(filepath.Join(statsDir, name))
		if err != nil {
			st.MissingStats = append(st.MissingStats, name)
			st.ReadErrors[name] = err.Error()
			return
		}
		set(v)
	}

	// Read all required fields; best-effort set each counter metric when its stat is readable.
	// Return error only after completing all reads.
	readStat("rx_bytes", func(v uint64) {
		st.RxBytes = v
		metricRxBytes.With(labels).Set(float64(v))
	})
	readStat("rx_packets", func(v uint64) {
		st.RxPackets = v
		metricRxPackets.With(labels).Set(float64(v))
	})
	readStat("rx_errors", func(v uint64) {
		st.RxErrors = v
		metricRxErrors.With(labels).Set(float64(v))
	})
	readStat("rx_dropped", func(v uint64) {
		st.RxDropped = v
		metricRxDropped.With(labels).Set(float64(v))
	})
	readStat("tx_bytes", func(v uint64) {
		st.TxBytes = v
		metricTxBytes.With(labels).Set(float64(v))
	})
	readStat("tx_packets", func(v uint64) {
		st.TxPackets = v
		metricTxPackets.With(labels).Set(float64(v))
	})
	readStat("tx_errors", func(v uint64) {
		st.TxErrors = v
		metricTxErrors.With(labels).Set(float64(v))
	})
	readStat("tx_dropped", func(v uint64) {
		st.TxDropped = v
		metricTxDropped.With(labels).Set(float64(v))
	})

	if len(st.MissingStats) > 0 {
		return fmt.Errorf("missing stats: %s", strings.Join(st.MissingStats, ", "))
	}
	return nil
}

func readUint64(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	return strconv.ParseUint(s, 10, 64)
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	InterfaceStats []InterfaceStats `json:"interface_stats,omitempty"`

	ts     time.Time
	err    error
	health apiv1.HealthStateType
	reason string
}

func (cr *checkResult) ComponentName() string { return Name }

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"Interface", "Link", "OperState", "RX bytes", "RX pkts", "RX err", "RX drop", "TX bytes", "TX pkts", "TX err", "TX drop"})
	for _, st := range cr.InterfaceStats {
		link := "down"
		if st.LinkUp {
			link = "up"
		}
		table.Append([]string{
			st.Interface,
			link,
			st.OperState,
			fmt.Sprintf("%d", st.RxBytes),
			fmt.Sprintf("%d", st.RxPackets),
			fmt.Sprintf("%d", st.RxErrors),
			fmt.Sprintf("%d", st.RxDropped),
			fmt.Sprintf("%d", st.TxBytes),
			fmt.Sprintf("%d", st.TxPackets),
			fmt.Sprintf("%d", st.TxErrors),
			fmt.Sprintf("%d", st.TxDropped),
		})
	}
	table.Render()
	return buf.String()
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *checkResult) getError() string {
	if cr == nil || cr.err == nil {
		return ""
	}
	return cr.err.Error()
}

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{{
			Time:      metav1.NewTime(time.Now().UTC()),
			Component: Name,
			Name:      Name,
			Health:    apiv1.HealthStateTypeHealthy,
			Reason:    "no data yet",
		}}
	}

	state := apiv1.HealthState{
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}
	if len(cr.InterfaceStats) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
