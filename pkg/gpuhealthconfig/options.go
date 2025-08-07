package gpuhealthconfig

import (
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
	infinibandclass "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/class"
)

// Op contains options for health configuration
type Op struct {
	pkgconfigcommon.ToolOverwrites
}

// OpOption is a function that modifies health configuration options
type OpOption func(*Op)

// ApplyOpts applies all the provided options to the Op struct
func (op *Op) ApplyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.InfinibandClassRootDir == "" {
		op.InfinibandClassRootDir = infinibandclass.DefaultRootDir
	}

	return nil
}

// WithInfinibandClassRootDir specifies the root directory of the InfiniBand class
func WithInfinibandClassRootDir(p string) OpOption {
	return func(op *Op) {
		op.InfinibandClassRootDir = p
	}
}
