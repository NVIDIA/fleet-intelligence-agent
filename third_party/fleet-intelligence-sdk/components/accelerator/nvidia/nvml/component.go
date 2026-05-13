// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

// Package nvml reports NVML collection errors as a component health state.
package nvml

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	nvidianvml "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const Name = "accelerator-nvidia-nvml"

var _ components.Component = &component{}
var _ components.CheckResult = &checkResult{}

var (
	registeredMu sync.RWMutex
	registered   = map[*component]struct{}{}
)

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.Instance
	nowFunc      func() time.Time

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	rootCtx := context.Background()
	var nvmlInstance nvidianvml.Instance
	if gpudInstance != nil {
		nvmlInstance = gpudInstance.NVMLInstance
		if gpudInstance.RootCtx != nil {
			rootCtx = gpudInstance.RootCtx
		}
	}

	cctx, ccancel := context.WithCancel(rootCtx)
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: nvmlInstance,
		nowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	register(c)
	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		"accelerator",
		"gpu",
		"nvidia",
		"nvml",
		Name,
	}
}

func (c *component) IsSupported() bool {
	return c != nil && c.nvmlInstance != nil && c.nvmlInstance.NVMLExists()
}

// Start is intentionally a no-op. This component is triggered by
// machine-info collection, not HealthCheckInterval.
func (c *component) Start() error { return nil }

func (c *component) Check() components.CheckResult {
	c.lastMu.RLock()
	last := c.lastCheckResult
	c.lastMu.RUnlock()
	if last != nil {
		return last
	}
	return c.checkWithErrors(nil)
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	last := c.lastCheckResult
	c.lastMu.RUnlock()
	return last.HealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) Close() error {
	unregister(c)
	c.cancel()
	return nil
}

// RunCheckWithErrors pushes the latest NVML collection errors to the NVML
// component and updates its health state immediately.
func RunCheckWithErrors(collectionErrors []string) {
	errs := append([]string(nil), collectionErrors...)
	componentsToCheck := listRegistered()
	for _, c := range componentsToCheck {
		_ = c.checkWithErrors(errs)
	}
}

func register(c *component) {
	registeredMu.Lock()
	defer registeredMu.Unlock()
	registered[c] = struct{}{}
}

func unregister(c *component) {
	registeredMu.Lock()
	defer registeredMu.Unlock()
	delete(registered, c)
}

func listRegistered() []*component {
	registeredMu.RLock()
	defer registeredMu.RUnlock()

	out := make([]*component, 0, len(registered))
	for c := range registered {
		out = append(out, c)
	}
	return out
}

func (c *component) checkWithErrors(collectionErrors []string) components.CheckResult {
	cr := &checkResult{
		ts:               c.nowFunc(),
		collectionErrors: append([]string(nil), collectionErrors...),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML instance is nil"
		return cr
	}
	if !c.nvmlInstance.NVMLExists() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML library is not loaded"
		return cr
	}
	if len(collectionErrors) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no NVML collection errors reported"
		return cr
	}

	cr.health = apiv1.HealthStateTypeUnhealthy
	cr.reason = fmt.Sprintf("detected %d NVML collection error(s)", len(collectionErrors))
	cr.incidents = make([]apiv1.HealthStateIncident, 0, len(collectionErrors))
	for _, msg := range collectionErrors {
		cr.incidents = append(cr.incidents, apiv1.HealthStateIncident{
			EntityID: extractEntityID(msg),
			Message:  msg,
			Health:   apiv1.HealthStateTypeUnhealthy,
		})
	}
	log.Logger.Warnw("NVML collection errors detected", "count", len(collectionErrors))
	return cr
}

type checkResult struct {
	ts time.Time

	health apiv1.HealthStateType
	reason string

	collectionErrors []string
	incidents        []apiv1.HealthStateIncident
}

func (cr *checkResult) ComponentName() string { return Name }

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	if len(cr.collectionErrors) == 0 {
		return cr.reason
	}
	return strings.Join(cr.collectionErrors, "\n")
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

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Time:      metav1.NewTime(time.Now().UTC()),
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Health:    cr.health,
		Incidents: cr.incidents,
	}
	if len(cr.collectionErrors) > 0 {
		b, _ := json.Marshal(cr.collectionErrors)
		state.ExtraInfo = map[string]string{
			"errors": string(b),
		}
	}
	return apiv1.HealthStates{state}
}

func extractEntityID(message string) string {
	const prefix = "gpu "
	if !strings.HasPrefix(message, prefix) {
		return ""
	}
	s := strings.TrimPrefix(message, prefix)
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return ""
	}
	return s[:idx]
}
