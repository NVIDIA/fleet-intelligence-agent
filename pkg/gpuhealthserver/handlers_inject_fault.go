package gpuhealthserver

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/leptonai/gpud/pkg/errdefs"
	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
	"github.com/leptonai/gpud/pkg/log"
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
	if s.faultInjector == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": errdefs.ErrNotFound, "message": "fault injector not set up"})
		return
	}

	// read the request body
	request := new(pkgfaultinjector.Request)
	if err := json.NewDecoder(c.Request.Body).Decode(request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "failed to decode request body: " + err.Error()})
		return
	}
	if err := request.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "invalid request: " + err.Error()})
		return
	}
	log.Logger.Infow("ERRORINJECTOR: request", "request", request)

	switch {
	case request.KernelMessage != nil:
		if err := s.faultInjector.KmsgWriter().Write(request.KernelMessage); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": errdefs.ErrUnknown, "message": "failed to inject kernel message: " + err.Error()})
			return
		}

	case request.ComponentError != nil:
		if injector, ok := s.faultInjector.(pkgfaultinjector.ComponentErrorInjector); ok {
			if err := injector.InjectComponentError(c.Request.Context(), s.componentsRegistry, request.ComponentError); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": errdefs.ErrUnknown, "message": "failed to inject component error: " + err.Error()})
				return
			}
		} else {
			c.JSON(http.StatusNotImplemented, gin.H{"code": errdefs.ErrNotImplemented, "message": "component error injection not supported"})
			return
		}

	case request.Event != nil:
		if injector, ok := s.faultInjector.(pkgfaultinjector.EventInjector); ok {
			if err := injector.InjectEvent(c.Request.Context(), s.componentsRegistry, request.Event); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": errdefs.ErrUnknown, "message": "failed to inject event: " + err.Error()})
				return
			}
		} else {
			c.JSON(http.StatusNotImplemented, gin.H{"code": errdefs.ErrNotImplemented, "message": "event injection not supported"})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": errdefs.ErrInvalidArgument, "message": "one of kernel_message, component_error, or event is required"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "fault injected"})
}
