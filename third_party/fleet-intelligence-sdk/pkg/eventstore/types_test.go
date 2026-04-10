// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package eventstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEvent_ToEvent_PreservesEventID(t *testing.T) {
	t.Parallel()

	ev := Event{
		Component: "component-a",
		EventID:   "123e4567-e89b-12d3-a456-426614174000",
		Time:      time.Unix(1710000000, 0).UTC(),
		Name:      "event-a",
		Type:      "Warning",
		Message:   "hello",
		ExtraInfo: map[string]string{
			"key": "value",
		},
	}

	apiEvent := ev.ToEvent()

	assert.Equal(t, ev.EventID, apiEvent.EventID)
	assert.Equal(t, "value", apiEvent.ExtraInfo["key"])
}

func TestEvent_ToEvent_EmptyEventID(t *testing.T) {
	t.Parallel()

	ev := Event{
		Component: "component-a",
		Time:      time.Unix(1710000000, 0).UTC(),
		Name:      "event-a",
		Type:      "Warning",
		Message:   "hello",
		ExtraInfo: map[string]string{
			"key": "value",
		},
	}

	apiEvent := ev.ToEvent()

	assert.Empty(t, apiEvent.EventID)
	assert.Equal(t, "value", apiEvent.ExtraInfo["key"])
}
