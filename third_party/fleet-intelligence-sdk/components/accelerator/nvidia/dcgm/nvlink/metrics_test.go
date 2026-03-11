package nvlink

import (
	"testing"

	dcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"
)

func TestNVLinkFieldsIncludeExpectedCounters(t *testing.T) {
	fieldSet := make(map[dcgm.Short]struct{}, len(nvlinkFields))
	for _, field := range nvlinkFields {
		fieldSet[field] = struct{}{}
	}

	required := []dcgm.Short{
		dcgm.DCGM_FI_DEV_NVLINK_BANDWIDTH_TOTAL,
		dcgm.DCGM_FI_DEV_NVLINK_ERROR_DL_CRC,
		dcgm.DCGM_FI_DEV_NVLINK_ERROR_DL_RECOVERY,
		dcgm.DCGM_FI_DEV_NVLINK_ERROR_DL_REPLAY,
		dcgm.DCGM_FI_DEV_NVLINK_COUNT_LINK_RECOVERY_SUCCESSFUL_EVENTS,
		dcgm.DCGM_FI_DEV_NVLINK_COUNT_LINK_RECOVERY_FAILED_EVENTS,
		dcgm.DCGM_FI_DEV_FABRIC_MANAGER_STATUS,
		dcgm.DCGM_FI_DEV_C2C_LINK_ERROR_REPLAY,
		dcgm.DCGM_FI_DEV_NVLINK_COUNT_RX_GENERAL_ERRORS,
		dcgm.DCGM_FI_DEV_NVLINK_COUNT_RX_MALFORMED_PACKET_ERRORS,
		dcgm.DCGM_FI_DEV_NVLINK_COUNT_RX_REMOTE_ERRORS,
		dcgm.DCGM_FI_DEV_NVLINK_COUNT_RX_SYMBOL_ERRORS,
		dcgm.DCGM_FI_DEV_NVLINK_COUNT_RX_BUFFER_OVERRUN_ERRORS,
		dcgm.DCGM_FI_DEV_NVLINK_COUNT_LOCAL_LINK_INTEGRITY_ERRORS,
		dcgm.DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS,
		dcgm.DCGM_FI_DEV_NVLINK_COUNT_EFFECTIVE_BER,
		dcgm.DCGM_FI_DEV_NVLINK_COUNT_SYMBOL_BER,
		dcgm.DCGM_FI_DEV_NVLINK_COUNT_TX_DISCARDS,
	}

	for _, field := range required {
		if _, ok := fieldSet[field]; !ok {
			t.Errorf("missing expected nvlink field: %d", field)
		}
	}
}
