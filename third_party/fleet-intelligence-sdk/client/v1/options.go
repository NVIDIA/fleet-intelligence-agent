// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package v1

import "github.com/NVIDIA/fleet-intelligence-sdk/pkg/httputil"

type Op struct {
	requestContentType    string
	requestAcceptEncoding string
	components            map[string]any
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}
	return nil
}

// WithRequestContentTypeYAML sets the request content type to YAML.
func WithRequestContentTypeYAML() OpOption {
	return func(op *Op) {
		op.requestContentType = httputil.RequestHeaderYAML
	}
}

// WithRequestContentTypeJSON sets the request content type to JSON.
func WithRequestContentTypeJSON() OpOption {
	return func(op *Op) {
		op.requestContentType = httputil.RequestHeaderJSON
	}
}

// WithAcceptEncodingGzip requests gzip encoding for the response.
func WithAcceptEncodingGzip() OpOption {
	return func(op *Op) {
		op.requestAcceptEncoding = httputil.RequestHeaderEncodingGzip
	}
}

func WithComponent(component string) OpOption {
	return func(op *Op) {
		if op.components == nil {
			op.components = make(map[string]any)
		}
		op.components[component] = nil
	}
}
