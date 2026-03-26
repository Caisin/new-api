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

func (policy *ModelChannelPolicy) Insert() error {
	values := map[string]interface{}{
		"model":          policy.Model,
		"channel_id":     policy.ChannelId,
		"priority":       policy.Priority,
		"manual_enabled": policy.ManualEnabled,
		"created_at":     policy.CreatedAt,
		"updated_at":     policy.UpdatedAt,
	}
	return DB.Model(&ModelChannelPolicy{}).Create(values).Error
}

func (policy *ModelChannelPolicy) Update() error {
	updates := map[string]interface{}{
		"priority":       policy.Priority,
		"manual_enabled": policy.ManualEnabled,
		"updated_at":     policy.UpdatedAt,
	}
	query := DB.Model(&ModelChannelPolicy{})
	if policy.Id > 0 {
		query = query.Where("id = ?", policy.Id)
	} else {
		query = query.Where("model = ? AND channel_id = ?", policy.Model, policy.ChannelId)
	}
	return query.Updates(updates).Error
}

func GetModelChannelPoliciesByModel(modelName string) ([]ModelChannelPolicy, error) {
	policies := make([]ModelChannelPolicy, 0)
	if modelName == "" {
		return policies, nil
	}
	err := DB.Where("model = ?", modelName).
		Order("priority DESC").
		Order("channel_id ASC").
		Find(&policies).Error
	return policies, err
}
