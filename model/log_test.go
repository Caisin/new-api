package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestFormatUserLogsStripsRetryPathDebugFields(t *testing.T) {
	logs := []*Log{
		{
			Other: common.MapToJsonStr(map[string]interface{}{
				"admin_info":             map[string]interface{}{"use_channel": []string{"11", "18"}},
				"reject_reason":          "debug-only",
				"retry_path":             []int{11, 18},
				"retry_path_text":        "11 -> 18",
				"attempt_count":          2,
				"skipped_model_channels": map[string]interface{}{"12": "policy_manual_disabled"},
				"kept_field":             "visible",
			}),
		},
	}

	formatUserLogs(logs, 0)

	otherMap, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	require.NotContains(t, otherMap, "admin_info")
	require.NotContains(t, otherMap, "reject_reason")
	require.NotContains(t, otherMap, "retry_path")
	require.NotContains(t, otherMap, "retry_path_text")
	require.NotContains(t, otherMap, "attempt_count")
	require.NotContains(t, otherMap, "skipped_model_channels")
	require.Equal(t, "visible", otherMap["kept_field"])
}
