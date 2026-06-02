// Copyright 2024 Lepton AI Inc
// Source: https://github.com/leptonai/gpud

package sxid

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/NVIDIA/fleet-intelligence-sdk/api/v1"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/eventstore"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/log"
)

const (
	healthStateHealthy   = 0
	healthStateDegraded  = 1
	healthStateUnhealthy = 2
)

func translateToStateHealth(health int) apiv1.HealthStateType {
	switch health {
	case healthStateHealthy:
		return apiv1.HealthStateTypeHealthy
	case healthStateDegraded:
		return apiv1.HealthStateTypeDegraded
	case healthStateUnhealthy:
		return apiv1.HealthStateTypeUnhealthy
	default:
		return apiv1.HealthStateTypeHealthy
	}
}

const rebootThreshold = 2

// sxidHealthStateExtraInfoEntry is one SXID occurrence exported in HealthState.ExtraInfo["data"].
type sxidHealthStateExtraInfoEntry struct {
	GPUUUID    string `json:"gpu_uuid,omitempty"`
	DeviceUUID string `json:"device_uuid,omitempty"`
	SXid       uint64 `json:"sxid"`
	Time       string `json:"time,omitempty"`
	Message    string `json:"message,omitempty"`
	EventType  string `json:"event_type,omitempty"`
}

// evolveHealthyState resolves the state of the SXID error component.
// note: assume events are sorted by time in descending order
func evolveHealthyState(events eventstore.Events, now time.Time) (ret apiv1.HealthState) {
	defer func() {
		log.Logger.Debugf("EvolveHealthyState: %v", ret)
	}()
	var lastSuggestedAction *apiv1.SuggestedActions
	var lastSXidErr *sxidErrorEventDetail
	lastHealth := healthStateHealthy
	sxidRebootMap := make(map[uint64]int)
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		log.Logger.Debugf("EvolveHealthyState: event: %v %v %+v %+v %+v", event.Time, event.Name, lastSuggestedAction, sxidRebootMap, lastSXidErr)
		if event.Name == EventNameErrorSXid {
			resolvedEvent := resolveSXIDEvent(event)
			var currSXidErr sxidErrorEventDetail
			if err := json.Unmarshal([]byte(resolvedEvent.ExtraInfo[EventKeyErrorSXidData]), &currSXidErr); err != nil {
				log.Logger.Errorf("failed to unmarshal event %s %s extra info: %s", resolvedEvent.Name, resolvedEvent.Message, err)
				continue
			}

			currEvent := healthStateHealthy
			switch resolvedEvent.Type {
			case string(apiv1.EventTypeCritical):
				currEvent = healthStateDegraded
			case string(apiv1.EventTypeFatal):
				currEvent = healthStateUnhealthy
			}
			if currEvent < lastHealth {
				continue
			}
			lastHealth = currEvent
			lastSXidErr = &currSXidErr
			if currSXidErr.SuggestedActionsByGPUd != nil && len(currSXidErr.SuggestedActionsByGPUd.RepairActions) > 0 {
				if currSXidErr.SuggestedActionsByGPUd.RepairActions[0] == apiv1.RepairActionTypeRebootSystem {
					if count, ok := sxidRebootMap[currSXidErr.SXid]; !ok {
						sxidRebootMap[currSXidErr.SXid] = 0
					} else if count >= rebootThreshold {
						currSXidErr.SuggestedActionsByGPUd.RepairActions[0] = apiv1.RepairActionTypeHardwareInspection
					}
				}
				lastSXidErr.SuggestedActionsByGPUd.RepairActions = lastSXidErr.SuggestedActionsByGPUd.RepairActions[:1]
				lastSuggestedAction = currSXidErr.SuggestedActionsByGPUd
			}
		} else if event.Name == "reboot" {
			if lastSuggestedAction != nil && len(lastSuggestedAction.RepairActions) > 0 && (lastSuggestedAction.RepairActions[0] == apiv1.RepairActionTypeRebootSystem || lastSuggestedAction.RepairActions[0] == apiv1.RepairActionTypeCheckUserAppAndGPU) {
				lastHealth = healthStateHealthy
				lastSuggestedAction = nil
				lastSXidErr = nil
			}
			for v, count := range sxidRebootMap {
				sxidRebootMap[v] = count + 1
			}
		}
	}
	since := now.Add(-DefaultMetadataLookback)
	window := collectSXIDsInLookback(events, since)

	state := apiv1.HealthState{
		Time:             metav1.NewTime(now),
		Component:        Name,
		Name:             StateNameErrorSXid,
		Health:           translateToStateHealth(lastHealth),
		Reason:           formatSXIDHealthStateReason(lastSXidErr, window),
		SuggestedActions: lastSuggestedAction,
	}
	if len(window) > 0 {
		if dataJSON, err := json.Marshal(window); err == nil {
			state.ExtraInfo = map[string]string{"data": string(dataJSON)}
		} else {
			log.Logger.Errorw("failed to marshal SXID lookback extra_info", "error", err)
		}
	}
	return state
}

// collectSXIDsInLookback returns every error_sxid event in [since, now] (option A: no reboot filtering).
func collectSXIDsInLookback(events eventstore.Events, since time.Time) []sxidHealthStateExtraInfoEntry {
	var entries []sxidHealthStateExtraInfoEntry
	for _, event := range events {
		if event.Name != EventNameErrorSXid {
			continue
		}
		if event.Time.Before(since) {
			continue
		}
		resolvedEvent := resolveSXIDEvent(event)
		var sxidErr sxidErrorEventDetail
		if err := json.Unmarshal([]byte(resolvedEvent.ExtraInfo[EventKeyErrorSXidData]), &sxidErr); err != nil {
			log.Logger.Errorf("failed to unmarshal lookback SXID event: %s", err)
			continue
		}
		if sxidErr.Time.IsZero() {
			sxidErr.Time = metav1.NewTime(event.Time)
		}
		entries = append(entries, sxidHealthStateExtraInfoEntryFromDetail(&sxidErr, resolvedEvent.Type, resolvedEvent.Message))
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Time < entries[j].Time
	})
	return entries
}

func sxidHealthStateExtraInfoEntryFromDetail(sxidErr *sxidErrorEventDetail, eventType, message string) sxidHealthStateExtraInfoEntry {
	entry := sxidHealthStateExtraInfoEntry{
		DeviceUUID: sxidErr.DeviceUUID,
		GPUUUID:    sxidErr.DeviceUUID,
		SXid:       sxidErr.SXid,
		Message:    message,
		EventType:  eventType,
	}
	if entry.Message == "" {
		if sxidDetail, ok := GetDetail(int(sxidErr.SXid)); ok {
			entry.Message = fmt.Sprintf("SXID %d(%s) detected on %s", sxidErr.SXid, sxidDetail.Name, sxidErr.DeviceUUID)
		} else {
			entry.Message = fmt.Sprintf("SXID %d detected on %s", sxidErr.SXid, sxidErr.DeviceUUID)
		}
	}
	if !sxidErr.Time.IsZero() {
		entry.Time = sxidErr.Time.UTC().Format(time.RFC3339)
	}
	return entry
}

func formatSXIDHealthStateReason(lastSXidErr *sxidErrorEventDetail, window []sxidHealthStateExtraInfoEntry) string {
	if len(window) == 0 {
		if lastSXidErr == nil {
			return "SXIDComponent is healthy"
		}
		if sxidDetail, ok := GetDetail(int(lastSXidErr.SXid)); ok {
			return fmt.Sprintf("SXID %d(%s) detected on %s", lastSXidErr.SXid, sxidDetail.Name, lastSXidErr.DeviceUUID)
		}
		return fmt.Sprintf("SXID %d detected on %s", lastSXidErr.SXid, lastSXidErr.DeviceUUID)
	}
	if len(window) == 1 {
		return window[0].Message
	}
	return fmt.Sprintf("%d SXID error(s) in the last minute; latest: %s", len(window), window[len(window)-1].Message)
}

func resolveSXIDEvent(event eventstore.Event) eventstore.Event {
	ret := event
	if event.ExtraInfo == nil {
		return ret
	}

	rawData := event.ExtraInfo[EventKeyErrorSXidData]
	var sxidErr sxidErrorEventDetail
	if err := json.Unmarshal([]byte(rawData), &sxidErr); err == nil && sxidErr.SXid != 0 {
		if sxidErr.Time.IsZero() {
			sxidErr.Time = metav1.NewTime(event.Time)
		}
		if detail, ok := GetDetail(int(sxidErr.SXid)); ok && event.Type == "" {
			ret.Type = string(detail.EventType)
		}
		if ret.Message == "" {
			if detail, ok := GetDetail(int(sxidErr.SXid)); ok {
				ret.Message = fmt.Sprintf("SXID %d(%s) detected on %s", sxidErr.SXid, detail.Name, sxidErr.DeviceUUID)
			} else {
				ret.Message = fmt.Sprintf("SXID %d detected on %s", sxidErr.SXid, sxidErr.DeviceUUID)
			}
		}
		raw, _ := json.Marshal(sxidErr)
		if ret.ExtraInfo == nil {
			ret.ExtraInfo = make(map[string]string)
		}
		ret.ExtraInfo[EventKeyErrorSXidData] = string(raw)
		return ret
	}

	if currSXid, err := strconv.Atoi(rawData); err == nil {
		detail, ok := GetDetail(currSXid)
		if !ok {
			return ret
		}
		ret.Type = string(detail.EventType)

		var fatalType string
		if detail.AlwaysFatal {
			fatalType = " [Always Fatal]"
		} else if detail.PotentialFatal {
			fatalType = " [Potential Fatal]"
		} else {
			fatalType = " [Info]"
		}
		ret.Message = fmt.Sprintf("%s SXID %d(%s) detected on %s", fatalType, currSXid, detail.Name, event.ExtraInfo[EventKeyDeviceUUID])

		sxidErr := sxidErrorEventDetail{
			Time:                   metav1.NewTime(event.Time),
			DataSource:             "kmsg",
			DeviceUUID:             event.ExtraInfo[EventKeyDeviceUUID],
			SXid:                   uint64(currSXid),
			SuggestedActionsByGPUd: detail.SuggestedActionsByGPUd,
		}
		raw, _ := json.Marshal(sxidErr)

		if ret.ExtraInfo == nil {
			ret.ExtraInfo = make(map[string]string)
		}
		ret.ExtraInfo[EventKeyErrorSXidData] = string(raw)
	}
	return ret
}

// sxidErrorEventDetail represents an SXid error from kmsg.
type sxidErrorEventDetail struct {
	// Time is the time of the event.
	Time metav1.Time `json:"time"`

	// DataSource is the source of the data.
	DataSource string `json:"data_source"`

	// DeviceUUID is the UUID of the device that has the error.
	DeviceUUID string `json:"device_uuid"`

	// SXid is the corresponding SXid from the raw event.
	// The monitoring component can use this SXid to decide its own action.
	SXid uint64 `json:"sxid"`

	// SuggestedActionsByGPUd are the suggested actions for the error.
	SuggestedActionsByGPUd *apiv1.SuggestedActions `json:"suggested_actions_by_gpud,omitempty"`
}
