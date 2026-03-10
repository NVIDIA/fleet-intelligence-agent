// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

// Package recorder records internal GPUd metrics to Prometheus.
package recorder

import (
	"context"
	"database/sql"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/file"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	pkgmetrics "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metrics"
)

var _ pkgmetrics.Recorder = &promRecorder{}

// NewPrometheusRecorder records internal GPUd metrics to Prometheus.
func NewPrometheusRecorder(ctx context.Context, recorderInterval time.Duration, dbRO *sql.DB) pkgmetrics.Recorder {
	return &promRecorder{
		ctx: ctx,

		recorderInterval: recorderInterval,
		dbRO:             dbRO,

		getCurrentProcessUsageFunc: file.GetCurrentProcessUsage,

		gaugeFileDescriptorUsage: metricFileDescriptorUsage,
		gaugeSQLiteDBSizeInBytes: metricSQLiteDBSizeInBytes,
	}
}

type promRecorder struct {
	ctx context.Context

	recorderInterval time.Duration
	dbRO             *sql.DB

	getCurrentProcessUsageFunc func() (uint64, error)

	gaugeFileDescriptorUsage prometheus.Gauge
	gaugeSQLiteDBSizeInBytes prometheus.Gauge
}

func (s *promRecorder) Start() {
	log.Logger.Infow("starting recorder", "interval", s.recorderInterval)

	go func() {
		for {
			if err := s.record(s.ctx); err != nil {
				log.Logger.Errorw("failed to record metrics", "error", err)
			}

			select {
			case <-s.ctx.Done():
				return
			case <-time.After(s.recorderInterval):
			}
		}
	}()
}

func (s *promRecorder) record(ctx context.Context) error {
	if s == nil || s.dbRO == nil {
		return nil
	}

	if err := recordFileDescriptorUsage(s.getCurrentProcessUsageFunc, s.gaugeFileDescriptorUsage); err != nil {
		return err
	}
	if err := recordSQLiteDBSize(ctx, s.dbRO, s.gaugeSQLiteDBSizeInBytes); err != nil {
		return err
	}

	return nil
}
