// SPDX-FileCopyrightText: Copyright (c) 2025, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/errdefs"
	pkgfaultinjector "github.com/NVIDIA/fleet-intelligence-sdk/pkg/fault-injector"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
)

const URLPathInjectFault = "/inject-fault"

// injectFault godoc
// @Summary Inject fault into the system
// @Description Injects a fault (such as kernel messages) into the system for testing purposes
// @ID injectFault
// @Tags fault-injection
// @Accept json
// @Produce json
// @Param request body pkgfaultinjector.Request true "Fault injection request"
// @Success 200 {object} map[string]string "Fault injected successfully"
// @Failure 400 {object} map[string]interface{} "Bad request - invalid request body or validation error"
// @Failure 404 {object} map[string]interface{} "Fault injector not set up"
// @Failure 500 {object} map[string]interface{} "Internal server error"
// @Router /inject-fault [post]
func (s *Server) injectFault(c *gin.Context) {
	// Security check: verify request is from localhost
	remoteHost, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"code": errdefs.ErrFailedPrecondition, "message": "access denied"})
		return
	}

	ip := net.ParseIP(remoteHost)
	if ip == nil || !ip.IsLoopback() {
		c.JSON(http.StatusForbidden, gin.H{"code": errdefs.ErrFailedPrecondition, "message": "access denied"})
		return
	}

	// Defense in depth: verify fault injector is enabled
	if s.faultInjector == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": errdefs.ErrNotFound, "message": "fault injector not enabled"})
		return
	}

	// read the request body (capped at 1 MiB)
	request := new(pkgfaultinjector.Request)
	if err := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)).Decode(request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to decode request body"})
		return
	}
	if err := request.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid request"})
		return
	}

	switch {
	case request.KernelMessage != nil:
		if err := s.faultInjector.KmsgWriter().Write(request.KernelMessage); err != nil {
			log.Logger.Warnw("failed to inject kernel message", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": errdefs.ErrUnknown, "message": "failed to inject kernel message"})
			return
		}

	case request.ComponentError != nil:
		if injector, ok := s.faultInjector.(pkgfaultinjector.ComponentErrorInjector); ok {
			if err := injector.InjectComponentError(c.Request.Context(), s.componentsRegistry, request.ComponentError); err != nil {
				log.Logger.Warnw("failed to inject component error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"code": errdefs.ErrUnknown, "message": "failed to inject component error"})
				return
			}
		} else {
			c.JSON(http.StatusNotImplemented, gin.H{"code": errdefs.ErrNotImplemented, "message": "component error injection not supported"})
			return
		}

	case request.Event != nil:
		if injector, ok := s.faultInjector.(pkgfaultinjector.EventInjector); ok {
			if err := injector.InjectEvent(c.Request.Context(), s.componentsRegistry, request.Event); err != nil {
				log.Logger.Warnw("failed to inject event", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"code": errdefs.ErrUnknown, "message": "failed to inject event"})
				return
			}
		} else {
			c.JSON(http.StatusNotImplemented, gin.H{"code": errdefs.ErrNotImplemented, "message": "event injection not supported"})
			return
		}

	case request.ComponentClear != nil:
		if injector, ok := s.faultInjector.(pkgfaultinjector.ComponentClearInjector); ok {
			if err := injector.ClearComponentFault(c.Request.Context(), s.componentsRegistry, request.ComponentClear); err != nil {
				log.Logger.Warnw("failed to clear component fault", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"code": errdefs.ErrUnknown, "message": "failed to clear component fault"})
				return
			}
		} else {
			c.JSON(http.StatusNotImplemented, gin.H{"code": errdefs.ErrNotImplemented, "message": "component fault clearing not supported"})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "one of kernel_message, component_error, component_clear, or event is required"})
		return
	}

	successMessage := "fault injected"
	if request.ComponentClear != nil {
		successMessage = "component fault cleared"
	}
	c.JSON(http.StatusOK, gin.H{"message": successMessage})
}
