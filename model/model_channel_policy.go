package model

type ModelChannelPolicy struct {
	Id            int64  `json:"id" gorm:"primaryKey"`
	Model         string `json:"model" gorm:"type:varchar(255);not null;uniqueIndex:idx_model_channel_policy_model_channel,priority:1"`
	ChannelId     int    `json:"channel_id" gorm:"not null;uniqueIndex:idx_model_channel_policy_model_channel,priority:2"`
	Priority      int64  `json:"priority" gorm:"bigint;not null;default:0"`
	ManualEnabled bool   `json:"manual_enabled" gorm:"not null;default:true"`
	CreatedAt     int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt     int64  `json:"updated_at" gorm:"bigint"`
}

func (ModelChannelPolicy) TableName() string {
	return "model_channel_policies"
}
