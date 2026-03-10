// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package nvml

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/eventstore"
)

type mockEventBucket struct {
	eventstore.Bucket
}

func TestOpApplyOpts(t *testing.T) {
	t.Run("default options", func(t *testing.T) {
		op := &Op{}
		op.applyOpts(nil)
		assert.Nil(t, op.hwSlowdownEventBucket)
	})

	t.Run("with events bucket", func(t *testing.T) {
		bucket := &mockEventBucket{}
		op := &Op{}
		op.applyOpts([]OpOption{
			WithHWSlowdownEventBucket(bucket),
		})
		assert.Equal(t, bucket, op.hwSlowdownEventBucket)
	})
}

func TestWithHWSlowdownEventBucket(t *testing.T) {
	bucket := &mockEventBucket{}
	op := &Op{}
	opt := WithHWSlowdownEventBucket(bucket)
	opt(op)
	assert.Equal(t, bucket, op.hwSlowdownEventBucket)
}

func TestOpOptionsErrorHandling(t *testing.T) {
	t.Run("nil events bucket", func(t *testing.T) {
		op := &Op{}
		op.applyOpts([]OpOption{
			WithHWSlowdownEventBucket(nil),
		})
		assert.Nil(t, op.hwSlowdownEventBucket)
	})
}
