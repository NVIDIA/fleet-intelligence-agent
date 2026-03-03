package dcgm

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"

	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
)

// FieldValueCache manages centralized DCGM field watching and caching.
// Polls field values in the background and provides cached results to components.
type FieldValueCache struct {
	ctx      context.Context
	cancel   context.CancelFunc
	instance Instance

	mu         sync.RWMutex
	values     map[uint]map[dcgm.Short]dcgm.FieldValue_v1 // deviceID -> fieldID -> value
	lastUpdate time.Time
	lastError  error

	fieldGroupID   dcgm.FieldHandle
	pollInterval   time.Duration
	started        bool
	startOnce      sync.Once
	registrationMu sync.Mutex
}

// NewFieldValueCache creates a placeholder cache. Call SetupFieldWatching() after components register fields.
func NewFieldValueCache(ctx context.Context, instance Instance, pollInterval time.Duration) *FieldValueCache {
	cctx, ccancel := context.WithCancel(ctx)

	return &FieldValueCache{
		ctx:          cctx,
		cancel:       ccancel,
		instance:     instance,
		values:       make(map[uint]map[dcgm.Short]dcgm.FieldValue_v1),
		pollInterval: pollInterval,
	}
}

// SetupFieldWatching creates the field group and starts DCGM watching for all registered fields.
// For tests, use SetupFieldWatchingWithName to provide a unique name.
func (fc *FieldValueCache) SetupFieldWatching() error {
	return fc.SetupFieldWatchingWithName("gpud-gpu-fields")
}

// SetupFieldWatchingWithName creates the field group with a custom name.
// This is useful for tests to avoid naming conflicts when running in parallel.
func (fc *FieldValueCache) SetupFieldWatchingWithName(fieldGroupName string) error {
	fc.registrationMu.Lock()
	defer fc.registrationMu.Unlock()

	if fc.instance == nil || !fc.instance.DCGMExists() {
		log.Logger.Debugw("DCGM not available, skipping field watching setup")
		return nil
	}

	watchedFields := fc.instance.GetWatchedFields()
	if len(watchedFields) == 0 {
		log.Logger.Debugw("no fields registered, skipping field watching setup")
		return nil
	}

	fieldGroupID, err := dcgm.FieldGroupCreate(fieldGroupName, watchedFields)
	if err != nil {
		setupErr := fmt.Errorf("failed to create DCGM field group: %w", err)
		// Store error so GetResult() returns it to components
		fc.mu.Lock()
		fc.lastError = setupErr
		fc.mu.Unlock()
		return setupErr
	}
	fc.fieldGroupID = fieldGroupID

	updateFreqMicroseconds := int64(fc.pollInterval / time.Microsecond)
	maxKeepAge := fc.pollInterval.Seconds() * 2
	maxKeepSamples := int32(3)

	err = dcgm.WatchFieldsWithGroupEx(fieldGroupID, fc.instance.GetGroupHandle(),
		updateFreqMicroseconds, maxKeepAge, maxKeepSamples)
	if err != nil {
		dcgm.FieldGroupDestroy(fieldGroupID)
		setupErr := fmt.Errorf("failed to set up DCGM field watching: %w", err)
		// Store error so GetResult() returns it to components
		fc.mu.Lock()
		fc.lastError = setupErr
		fc.mu.Unlock()
		return setupErr
	}

	log.Logger.Infow("set up DCGM field watching with centralized field group",
		"updateFreq", fc.pollInterval,
		"maxKeepAge", maxKeepAge,
		"numFields", len(watchedFields))

	return nil
}

// Start begins background polling. Requires SetupFieldWatching() to be called first.
func (fc *FieldValueCache) Start() error {
	var startErr error
	fc.startOnce.Do(func() {
		fc.registrationMu.Lock()
		defer fc.registrationMu.Unlock()

		if fc.instance == nil || !fc.instance.DCGMExists() {
			log.Logger.Debugw("no fields or DCGM unavailable, skipping polling")
			fc.started = true
			return
		}

		watchedFields := fc.instance.GetWatchedFields()
		if len(watchedFields) == 0 {
			log.Logger.Debugw("no fields or DCGM unavailable, skipping polling")
			fc.started = true
			return
		}

		fc.started = true

		if err := fc.Poll(); err != nil {
			log.Logger.Warnw("initial poll failed", "error", err)
		}

		go fc.pollLoop()

		log.Logger.Infow("field cache polling started", "interval", fc.pollInterval, "fields", len(watchedFields))
	})

	return startErr
}

// Stop stops polling and destroys the field group.
func (fc *FieldValueCache) Stop() {
	fc.cancel()

	if fc.instance != nil && fc.instance.DCGMExists() && fc.fieldGroupID.GetHandle() != 0 {
		if err := dcgm.FieldGroupDestroy(fc.fieldGroupID); err != nil {
			log.Logger.Warnw("failed to destroy field group", "error", err)
		}
	}
}

func (fc *FieldValueCache) pollLoop() {
	ticker := time.NewTicker(fc.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-fc.ctx.Done():
			return
		case <-ticker.C:
			if err := fc.Poll(); err != nil {
				log.Logger.Warnw("field polling failed", "error", err)
			}
		}
	}
}

// Poll queries DCGM and updates the cache. Used by background polling and scan mode.
func (fc *FieldValueCache) Poll() error {
	if fc.instance == nil {
		return fmt.Errorf("DCGM instance is nil")
	}

	if !fc.instance.DCGMExists() {
		return fmt.Errorf("DCGM library not loaded")
	}

	watchedFields := fc.instance.GetWatchedFields()
	if len(watchedFields) == 0 {
		return fmt.Errorf("no fields to watch")
	}

	devices := fc.instance.GetDevices()
	newValues := make(map[uint]map[dcgm.Short]dcgm.FieldValue_v1)
	var pollErr error

	for _, device := range devices {
		fieldValues, err := dcgm.GetLatestValuesForFields(device.ID, watchedFields)
		if err != nil {
			// Check for fatal errors that require restart
			if IsRestartRequired(err) {
				log.Logger.Errorw("DCGM fatal error, exiting for restart",
					"component", "field_cache",
					"deviceID", device.ID,
					"error", err,
					"action", "systemd/k8s will restart agent and recreate DCGM resources")
				os.Exit(1)
			}

			// Check if this is a transient error (benign, don't store)
			if IsTransientError(err) {
				log.Logger.Infow("DCGM transient error, will retry",
					"component", "field_cache",
					"deviceID", device.ID,
					"error", err)
				continue // Skip this device, try next
			}

			// Store error with priority: unhealthy > unknown
			// Wrap error with device context for better debugging
			if pollErr == nil {
				// No error stored yet, store this one
				pollErr = fmt.Errorf("device %d: %w", device.ID, err)
			} else if IsUnhealthyAPIError(err) && !IsUnhealthyAPIError(pollErr) {
				// Replace unknown error with unhealthy error (higher priority)
				pollErr = fmt.Errorf("device %d: %w", device.ID, err)
			}

			// Continue polling other devices regardless of error type
			continue
		}

		deviceValues := make(map[dcgm.Short]dcgm.FieldValue_v1)
		for _, fieldValue := range fieldValues {
			deviceValues[fieldValue.FieldID] = fieldValue
		}
		newValues[device.ID] = deviceValues
	}

	fc.mu.Lock()
	fc.values = newValues
	fc.lastUpdate = time.Now()
	fc.lastError = pollErr
	fc.mu.Unlock()

	log.Logger.Debugw("field values cached", "devices", len(newValues), "fields", len(watchedFields))
	if pollErr != nil {
		return fmt.Errorf("DCGM error during field polling: %w", pollErr)
	}
	return nil
}

// DeviceFieldValues represents field values for a single device with metadata.
type DeviceFieldValues struct {
	DeviceID uint
	UUID     string
	Values   []dcgm.FieldValue_v1
}

// GetResult returns field values for all devices. Primary API for components.
func (fc *FieldValueCache) GetResult(fields []dcgm.Short) ([]DeviceFieldValues, error) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	// Check lastError first - it contains more specific error information
	if fc.lastError != nil {
		return nil, fc.lastError
	}

	if fc.instance == nil {
		return nil, fmt.Errorf("DCGM instance not available")
	}

	devices := fc.instance.GetDevices()
	result := make([]DeviceFieldValues, 0, len(devices))

	for _, device := range devices {
		deviceValues, exists := fc.values[device.ID]
		if !exists {
			continue
		}

		fieldValues := make([]dcgm.FieldValue_v1, 0, len(fields))
		for _, fieldID := range fields {
			if fieldValue, exists := deviceValues[fieldID]; exists {
				if isSentinel := CheckSentinel(fieldValue,
					"deviceID", device.ID,
					"uuid", device.UUID,
				); isSentinel {
					continue
				}
				fieldValues = append(fieldValues, fieldValue)
			}
		}

		if len(fieldValues) > 0 {
			result = append(result, DeviceFieldValues{
				DeviceID: device.ID,
				UUID:     device.UUID,
				Values:   fieldValues,
			})
		}
	}

	return result, nil
}
