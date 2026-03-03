package dcgm

import (
	"context"
	"fmt"
	"sync"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
)

// PolicyViolationCache centralizes policy violation monitoring for all DCGM policy types.
// It creates a single DCGM listener for all 7 policy types and distributes violations
// to components based on their subscriptions.
type PolicyViolationCache struct {
	ctx      context.Context
	cancel   context.CancelFunc
	instance Instance

	// Per-policy-type subscriber channels (buffered for each component)
	subscribersMu sync.RWMutex
	subscribers   map[string][]chan dcgm.PolicyViolation

	// Single master channel receiving all violations from DCGM
	masterChannel <-chan dcgm.PolicyViolation

	// policiesToSetMu protects policiesToSet
	policiesToSetMu sync.Mutex
	// policiesToSet tracks which policies need to be configured
	policiesToSet []dcgm.PolicyConfig

	// existingPoliciesOnStartup tracks what policies existed before we started
	// Used to determine if we should clear policies on close
	existingPoliciesOnStartup *dcgm.PolicyStatus

	started   bool
	startOnce sync.Once
}

// NewPolicyViolationCache creates a placeholder cache. Call SetupPolicyWatching() after components register policies.
func NewPolicyViolationCache(ctx context.Context, instance Instance) *PolicyViolationCache {
	cctx, ccancel := context.WithCancel(ctx)

	// Check what policies currently exist in DCGM
	var existingPolicies *dcgm.PolicyStatus
	if instance != nil && instance.DCGMExists() {
		existingPolicies = instance.GetExistingPolicies()
	}

	return &PolicyViolationCache{
		ctx:                       cctx,
		cancel:                    ccancel,
		instance:                  instance,
		subscribers:               make(map[string][]chan dcgm.PolicyViolation),
		policiesToSet:             make([]dcgm.PolicyConfig, 0),
		existingPoliciesOnStartup: existingPolicies,
	}
}

// RegisterPolicyToSet registers a policy configuration that needs to be set in DCGM.
// This should be called by components during initialization before SetupPolicyWatching().
func (c *PolicyViolationCache) RegisterPolicyToSet(config dcgm.PolicyConfig) {
	c.policiesToSetMu.Lock()
	defer c.policiesToSetMu.Unlock()

	c.policiesToSet = append(c.policiesToSet, config)

	log.Logger.Debugw("registered policy",
		"condition", config.Condition,
		"totalPolicies", len(c.policiesToSet))
}

// SetupPolicyWatching creates a single DCGM listener for all 7 policy types.
// This should be called once after component initialization.
func (c *PolicyViolationCache) SetupPolicyWatching() error {
	if c.instance == nil || !c.instance.DCGMExists() {
		log.Logger.Infow("DCGM not available, policy violation monitoring disabled")
		return nil
	}

	groupHandle := c.instance.GetGroupHandle()

	// Set policies that were registered by components
	c.policiesToSetMu.Lock()

	if len(c.policiesToSet) > 0 {
		log.Logger.Infow("setting DCGM policies", "numPolicies", len(c.policiesToSet))

		// Set all policies at once
		if err := dcgm.SetPolicyForGroup(groupHandle, c.policiesToSet...); err != nil {
			c.policiesToSetMu.Unlock()
			return fmt.Errorf("failed to set policies for group: %w", err)
		}

		// Extract policy conditions for watching
		var policyConditions []dcgm.PolicyCondition
		for _, config := range c.policiesToSet {
			policyConditions = append(policyConditions, config.Condition)
		}

		// Watch for violations on the policies we just set
		masterCh, err := dcgm.WatchPolicyViolationsForGroup(
			c.ctx,
			groupHandle,
			policyConditions...,
		)
		if err != nil {
			c.policiesToSetMu.Unlock()
			return fmt.Errorf("failed to watch policy violations: %w", err)
		}
		c.masterChannel = masterCh
		log.Logger.Infow("DCGM policy violation monitoring enabled", "numPolicies", len(c.policiesToSet))
	}
	c.policiesToSetMu.Unlock()

	return nil
}

// Start begins background violation distribution. Requires SetupPolicyWatching() to be called first.
func (c *PolicyViolationCache) Start() error {
	var startErr error
	c.startOnce.Do(func() {
		if c.masterChannel == nil {
			log.Logger.Debugw("no master channel, policy violation distribution disabled")
			return
		}

		c.started = true
		go c.distributeViolations()
		log.Logger.Debugw("policy violation distributor started")
	})
	return startErr
}

// Subscribe creates a buffered channel for receiving policy violations of a specific type.
// Components call this after SetupPolicyWatching() to get violations relevant to them.
func (c *PolicyViolationCache) Subscribe(policyName string) <-chan dcgm.PolicyViolation {
	ch := make(chan dcgm.PolicyViolation, 100)

	c.subscribersMu.Lock()
	defer c.subscribersMu.Unlock()

	c.subscribers[policyName] = append(c.subscribers[policyName], ch)
	log.Logger.Debugw("component subscribed to policy violations",
		"policyName", policyName,
		"totalSubscribers", len(c.subscribers[policyName]))

	return ch
}

// distributeViolations runs in a goroutine to receive violations from DCGM
// and route them to the appropriate component subscribers.
func (c *PolicyViolationCache) distributeViolations() {
	defer log.Logger.Debugw("policy violation distributor stopped")

	for {
		select {
		case <-c.ctx.Done():
			c.closeAllSubscribers()
			return

		case violation, ok := <-c.masterChannel:
			if !ok {
				log.Logger.Warnw("policy violation master channel closed")
				c.closeAllSubscribers()
				return
			}

			// Determine the policy type from the violation data
			policyType := c.detectPolicyType(violation.Data)
			if policyType == "" {
				log.Logger.Warnw("unknown policy violation data type",
					"dataType", fmt.Sprintf("%T", violation.Data),
					"timestamp", violation.Timestamp)
				continue
			}

			// Route to subscribers of this policy type
			c.subscribersMu.RLock()
			subscribers := c.subscribers[policyType]
			if len(subscribers) == 0 {
				log.Logger.Debugw("no subscribers for policy type",
					"policyType", policyType,
					"timestamp", violation.Timestamp)
			}

			for _, subscriberCh := range subscribers {
				select {
				case subscriberCh <- violation:
					// Successfully sent
				default:
					log.Logger.Warnw("subscriber channel full, dropping violation",
						"policyType", policyType,
						"timestamp", violation.Timestamp)
				}
			}
			c.subscribersMu.RUnlock()
		}
	}
}

// detectPolicyType examines the violation data and returns the corresponding policy name
func (c *PolicyViolationCache) detectPolicyType(data interface{}) string {
	switch data.(type) {
	case dcgm.PowerPolicyCondition:
		return "PowerPolicy"
	case dcgm.ThermalPolicyCondition:
		return "ThermalPolicy"
	case dcgm.DbePolicyCondition:
		return "DbePolicy"
	case dcgm.RetiredPagesPolicyCondition:
		return "MaxRtPgPolicy"
	case dcgm.PciPolicyCondition:
		return "PCIePolicy"
	case dcgm.NvlinkPolicyCondition:
		return "NvlinkPolicy"
	case dcgm.XidPolicyCondition:
		return "XidPolicy"
	default:
		return ""
	}
}

// closeAllSubscribers closes all subscriber channels when the cache shuts down
func (c *PolicyViolationCache) closeAllSubscribers() {
	c.subscribersMu.Lock()
	defer c.subscribersMu.Unlock()

	for policyType, channels := range c.subscribers {
		for _, ch := range channels {
			close(ch)
		}
		log.Logger.Debugw("closed subscriber channels", "policyType", policyType, "count", len(channels))
	}
}

// Close stops the distributor and cleans up resources
func (c *PolicyViolationCache) Close() error {
	c.cancel()

	// Only clear policies if:
	// 1. We registered policies to set (len(c.policiesToSet) > 0)
	// 2. There were no existing policies when we started
	hadExistingPolicies := c.existingPoliciesOnStartup != nil &&
		c.existingPoliciesOnStartup.Conditions != nil &&
		len(c.existingPoliciesOnStartup.Conditions) > 0

	shouldClearPolicies := len(c.policiesToSet) > 0 && !hadExistingPolicies

	if shouldClearPolicies && c.instance != nil && c.instance.DCGMExists() {
		groupHandle := c.instance.GetGroupHandle()
		if err := dcgm.ClearPolicyForGroup(groupHandle); err != nil {
			log.Logger.Warnw("failed to clear policies on close", "error", err)
			return err
		}
		log.Logger.Infow("cleared policies on close",
			"numPoliciesSet", len(c.policiesToSet),
			"hadExistingPolicies", hadExistingPolicies)
	}

	return nil
}
