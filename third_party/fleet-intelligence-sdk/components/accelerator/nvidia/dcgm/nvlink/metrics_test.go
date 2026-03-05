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
		dcgm.DCGM_FI_DEV_NVLINK_COUNT_LINK_RECOVERY_EVENTS,
	}

	for _, field := range required {
		if _, ok := fieldSet[field]; !ok {
			t.Errorf("missing expected nvlink field: %d", field)
		}
	}
}
