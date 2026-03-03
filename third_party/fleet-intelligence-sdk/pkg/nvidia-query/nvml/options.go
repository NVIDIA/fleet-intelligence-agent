package nvml

import (
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/eventstore"
)

type Op struct {
	hwSlowdownEventBucket eventstore.Bucket
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) {
	for _, opt := range opts {
		opt(op)
	}
}

func WithHWSlowdownEventBucket(bucket eventstore.Bucket) OpOption {
	return func(op *Op) {
		op.hwSlowdownEventBucket = bucket
	}
}
