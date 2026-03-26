package dto

type ModelChannelPolicyItem struct {
	ChannelId     int   `json:"channel_id"`
	Priority      int64 `json:"priority"`
	ManualEnabled bool  `json:"manual_enabled"`
}

type UpdateModelChannelPoliciesRequest struct {
	Channels []ModelChannelPolicyItem `json:"channels"`
}

type ModelChannelCircuitModelSummary struct {
	Model             string `json:"model"`
	PolicyCount       int    `json:"policy_count"`
	AutoDisabledCount int    `json:"auto_disabled_count"`
	ManualDisabled    int    `json:"manual_disabled_count"`
}

type ModelChannelCircuitChannelDetail struct {
	ChannelId           int    `json:"channel_id"`
	ChannelName         string `json:"channel_name"`
	ChannelType         int    `json:"channel_type"`
	ChannelStatus       int    `json:"channel_status"`
	ChannelMissing      bool   `json:"channel_missing"`
	Priority            int64  `json:"priority"`
	ManualEnabled       bool   `json:"manual_enabled"`
	Status              string `json:"status"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	LastFailureAt       int64  `json:"last_failure_at"`
	ProbeAfterAt        int64  `json:"probe_after_at"`
	DisabledAt          int64  `json:"disabled_at"`
	DisableReason       string `json:"disable_reason"`
}

type ModelChannelCircuitDetail struct {
	Model    string                             `json:"model"`
	Channels []ModelChannelCircuitChannelDetail `json:"channels"`
}
