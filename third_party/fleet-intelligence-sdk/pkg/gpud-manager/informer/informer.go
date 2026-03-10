// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package informer

type Informer interface {
	Start(<-chan struct{})
}
