package model

const (
	ModelChannelStateStatusEnabled        = "enabled"
	ModelChannelStateStatusAutoDisabled   = "auto_disabled"
	ModelChannelStateStatusManualDisabled = "manual_disabled"
)

type ModelChannelState struct {
	Id                  int64  `json:"id" gorm:"primaryKey"`
	Model               string `json:"model" gorm:"type:varchar(255);not null;uniqueIndex:idx_model_channel_state_model_channel,priority:1"`
	ChannelId           int    `json:"channel_id" gorm:"not null;uniqueIndex:idx_model_channel_state_model_channel,priority:2"`
	Status              string `json:"status" gorm:"type:varchar(32);not null;default:'enabled';index"`
	ConsecutiveFailures int    `json:"consecutive_failures" gorm:"not null;default:0"`
	LastFailureAt       int64  `json:"last_failure_at" gorm:"bigint;default:0"`
	ProbeAfterAt        int64  `json:"probe_after_at" gorm:"bigint;default:0"`
	DisabledAt          int64  `json:"disabled_at" gorm:"bigint;default:0"`
	DisableReason       string `json:"disable_reason" gorm:"type:text"`
	CreatedAt           int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt           int64  `json:"updated_at" gorm:"bigint"`
}

func (ModelChannelState) TableName() string {
	return "model_channel_states"
}

func IsValidModelChannelStateStatus(status string) bool {
	switch status {
	case ModelChannelStateStatusEnabled, ModelChannelStateStatusAutoDisabled, ModelChannelStateStatusManualDisabled:
		return true
	default:
		return false
	}
}

func (s *ModelChannelState) IsDisabled() bool {
	return s.Status == ModelChannelStateStatusAutoDisabled || s.Status == ModelChannelStateStatusManualDisabled
}
