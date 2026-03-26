package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

const (
	DefaultModelChannelFailureThreshold     = 3
	DefaultModelChannelProbeIntervalMinutes = 5
)

type ModelChannelCircuitSetting struct {
	FailureThreshold     int `json:"failure_threshold"`
	ProbeIntervalMinutes int `json:"probe_interval_minutes"`
}

var modelChannelCircuitSetting = ModelChannelCircuitSetting{
	FailureThreshold:     DefaultModelChannelFailureThreshold,
	ProbeIntervalMinutes: DefaultModelChannelProbeIntervalMinutes,
}

func init() {
	config.GlobalConfig.Register("model_channel_circuit_setting", &modelChannelCircuitSetting)
}

func GetModelChannelCircuitSetting() *ModelChannelCircuitSetting {
	return &modelChannelCircuitSetting
}

func GetModelChannelCircuitFailureThreshold() int {
	if modelChannelCircuitSetting.FailureThreshold <= 0 {
		return DefaultModelChannelFailureThreshold
	}
	return modelChannelCircuitSetting.FailureThreshold
}

func GetModelChannelCircuitProbeIntervalMinutes() int {
	if modelChannelCircuitSetting.ProbeIntervalMinutes <= 0 {
		return DefaultModelChannelProbeIntervalMinutes
	}
	return modelChannelCircuitSetting.ProbeIntervalMinutes
}
