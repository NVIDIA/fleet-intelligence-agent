// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package session

import (
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
)

// processInjectFault handles fault injection requests
func (s *Session) processInjectFault(payload Request, response *Response) {
	if payload.InjectFaultRequest == nil {
		log.Logger.Warnw("fault inject request is nil")
		return
	}

	if s.faultInjector == nil {
		response.Error = "fault injector is not initialized"
		return
	}

	if err := payload.InjectFaultRequest.Validate(); err != nil {
		response.Error = err.Error()
		log.Logger.Errorw("invalid fault inject request", "error", err)
		return
	}

	switch {
	case payload.InjectFaultRequest.KernelMessage != nil:
		if err := s.faultInjector.KmsgWriter().Write(payload.InjectFaultRequest.KernelMessage); err != nil {
			response.Error = err.Error()
			log.Logger.Errorw("failed to inject kernel message", "message", payload.InjectFaultRequest.KernelMessage.Message, "error", err)
		} else {
			log.Logger.Infow("successfully injected kernel message", "message", payload.InjectFaultRequest.KernelMessage.Message)
		}

	default:
		log.Logger.Warnw("fault inject request is nil or kernel message is nil")
	}
}
