package operation_setting

import "testing"

import "github.com/stretchr/testify/require"

func TestModelChannelCircuitSettingDefaults(t *testing.T) {
	require.Equal(t, 3, DefaultModelChannelFailureThreshold)
	require.Equal(t, 5, DefaultModelChannelProbeIntervalMinutes)

	s := GetModelChannelCircuitSetting()
	require.Equal(t, DefaultModelChannelFailureThreshold, s.FailureThreshold)
	require.Equal(t, DefaultModelChannelProbeIntervalMinutes, s.ProbeIntervalMinutes)
}

func TestModelChannelCircuitSettingFallbackGetters(t *testing.T) {
	s := GetModelChannelCircuitSetting()
	original := *s
	t.Cleanup(func() {
		*s = original
	})

	s.FailureThreshold = 0
	s.ProbeIntervalMinutes = 0

	require.Equal(t, DefaultModelChannelFailureThreshold, GetModelChannelCircuitFailureThreshold())
	require.Equal(t, DefaultModelChannelProbeIntervalMinutes, GetModelChannelCircuitProbeIntervalMinutes())
}
