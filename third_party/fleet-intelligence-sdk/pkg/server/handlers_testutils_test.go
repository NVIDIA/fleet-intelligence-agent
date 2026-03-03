package server

import (
	"github.com/gin-gonic/gin"

	"github.com/NVIDIA/fleet-intelligence-sdk/components"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/config"
)

// setupTestHandler creates a test handler with the given registry and metrics store
func setupTestHandler(comps []components.Component) (*globalHandler, *mockRegistry, *mockMetricsStore) {
	registry := newMockRegistry()
	for _, comp := range comps {
		registry.AddMockComponent(comp)
	}

	cfg := &config.Config{}
	store := &mockMetricsStore{}

	handler := newGlobalHandler(cfg, registry, store, nil, nil)
	return handler, registry, store
}

// setupRouterWithPath sets up a Gin router with the given path groups
func setupRouterWithPath(path string) (engine *gin.Engine, group *gin.RouterGroup) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group(path)
	return r, g
}
