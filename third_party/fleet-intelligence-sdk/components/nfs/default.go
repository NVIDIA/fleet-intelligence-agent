package nfs

import (
	"sync"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
	pkgnfschecker "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nfs-checker"
)

var (
	defaultConfigsMu sync.RWMutex
	defaultConfigs   = make(pkgnfschecker.Configs, 0)
)

func GetDefaultConfigs() pkgnfschecker.Configs {
	defaultConfigsMu.RLock()
	defer defaultConfigsMu.RUnlock()

	return defaultConfigs
}

func SetDefaultConfigs(cfgs pkgnfschecker.Configs) {
	log.Logger.Infow("setting default nfs checker configs", "count", len(cfgs))

	defaultConfigsMu.Lock()
	defer defaultConfigsMu.Unlock()
	defaultConfigs = cfgs
}
