// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestSettableCounterVecEmitsCounterMetric(t *testing.T) {
	registry := prometheus.NewRegistry()
	counter := NewSettableCounterVec(
		prometheus.CounterOpts{
			Name: "external_absolute_total",
			Help: "external absolute counter",
		},
		[]string{MetricComponentLabelKey, "device"},
	).MustCurryWith(prometheus.Labels{MetricComponentLabelKey: "component1"})

	if err := registry.Register(counter); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	counter.With(prometheus.Labels{"device": "gpu0"}).Set(42)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() failed: %v", err)
	}

	family := findMetricFamily(families, "external_absolute_total")
	if family == nil {
		t.Fatalf("metric family was not gathered")
	}
	if family.GetType() != dto.MetricType_COUNTER {
		t.Fatalf("metric type = %v, want %v", family.GetType(), dto.MetricType_COUNTER)
	}
	if got := family.GetMetric()[0].GetCounter().GetValue(); got != 42 {
		t.Fatalf("counter value = %v, want 42", got)
	}
}

func TestSettableCounterVecResetAndDelete(t *testing.T) {
	counter := NewSettableCounterVec(
		prometheus.CounterOpts{
			Name: "resettable_external_absolute_total",
			Help: "external absolute counter",
		},
		[]string{"device"},
	)

	labels := prometheus.Labels{"device": "gpu0"}
	counter.With(labels).Set(42)

	if !counter.Delete(labels) {
		t.Fatalf("Delete() = false, want true")
	}
	if counter.Delete(labels) {
		t.Fatalf("Delete() after delete = true, want false")
	}

	counter.With(labels).Set(43)
	counter.Reset()
	if counter.Delete(labels) {
		t.Fatalf("Delete() after reset = true, want false")
	}
}

func findMetricFamily(families []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	return nil
}
