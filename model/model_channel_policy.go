package model

import (
	"errors"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

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
	originalID := policy.Id
	originalCreatedAt := policy.CreatedAt
	originalUpdatedAt := policy.UpdatedAt
	now := common.GetTimestamp()
	policy.CreatedAt = now
	policy.UpdatedAt = now

	originalPriority := policy.Priority
	originalManualEnabled := policy.ManualEnabled

	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("id").Create(policy).Error; err != nil {
			return err
		}

		return tx.Model(&ModelChannelPolicy{}).Where("id = ?", policy.Id).Updates(map[string]interface{}{
			"priority":       originalPriority,
			"manual_enabled": originalManualEnabled,
			"created_at":     policy.CreatedAt,
			"updated_at":     policy.UpdatedAt,
		}).Error
	}); err != nil {
		policy.Id = originalID
		policy.CreatedAt = originalCreatedAt
		policy.UpdatedAt = originalUpdatedAt
		policy.Priority = originalPriority
		policy.ManualEnabled = originalManualEnabled
		return err
	}

	policy.Priority = originalPriority
	policy.ManualEnabled = originalManualEnabled
	return nil
}

func (policy *ModelChannelPolicy) Update() error {
	policy.UpdatedAt = common.GetTimestamp()
	query := DB.Model(&ModelChannelPolicy{})
	if policy.Id > 0 {
		query = query.Where("id = ?", policy.Id)
	} else {
		query = query.Where("model = ? AND channel_id = ?", policy.Model, policy.ChannelId)
	}
	return query.Select("priority", "manual_enabled", "updated_at").Updates(policy).Error
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

func GetModelChannelPoliciesByChannelID(channelID int) ([]ModelChannelPolicy, error) {
	policies := make([]ModelChannelPolicy, 0)
	if channelID == 0 {
		return policies, nil
	}
	err := DB.Where("channel_id = ?", channelID).
		Order("priority DESC").
		Order("model ASC").
		Find(&policies).Error
	return policies, err
}

func GetModelChannelPolicy(modelName string, channelID int) (*ModelChannelPolicy, error) {
	if modelName == "" || channelID == 0 {
		return nil, nil
	}
	var policy ModelChannelPolicy
	err := DB.Where("model = ? AND channel_id = ?", modelName, channelID).First(&policy).Error
	if err == nil {
		return &policy, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, err
}

func ReplaceModelChannelPolicies(modelName string, policies []ModelChannelPolicy) error {
	if modelName == "" {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("model = ?", modelName).Delete(&ModelChannelPolicy{}).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		for i := range policies {
			policy := policies[i]
			policy.Model = modelName
			policy.Id = 0
			policy.CreatedAt = now
			policy.UpdatedAt = now
			originalPriority := policy.Priority
			originalManualEnabled := policy.ManualEnabled
			if err := tx.Omit("id").Create(&policy).Error; err != nil {
				return err
			}
			if err := tx.Model(&ModelChannelPolicy{}).Where("id = ?", policy.Id).Updates(map[string]interface{}{
				"priority":       originalPriority,
				"manual_enabled": originalManualEnabled,
				"created_at":     policy.CreatedAt,
				"updated_at":     policy.UpdatedAt,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func ListModelChannelPolicyModels() ([]string, error) {
	models := make([]string, 0)
	if err := DB.Model(&ModelChannelPolicy{}).Distinct("model").Pluck("model", &models).Error; err != nil {
		return nil, err
	}
	sort.Strings(models)
	return models, nil
}
