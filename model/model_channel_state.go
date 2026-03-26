package model

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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

func GetModelChannelStateMapByModel(modelName string) (map[int]*ModelChannelState, error) {
	stateMap := make(map[int]*ModelChannelState)
	if modelName == "" {
		return stateMap, nil
	}

	var states []ModelChannelState
	err := DB.Where("model = ?", modelName).Find(&states).Error
	if err != nil {
		return nil, err
	}
	for i := range states {
		state := states[i]
		stateMap[state.ChannelId] = &state
	}
	return stateMap, nil
}

func ListDueAutoDisabledModelChannelStates(now int64, limit int) ([]ModelChannelState, error) {
	states := make([]ModelChannelState, 0)
	if now <= 0 {
		now = common.GetTimestamp()
	}
	query := DB.Where("status = ? AND probe_after_at > 0 AND probe_after_at <= ?", ModelChannelStateStatusAutoDisabled, now).
		Order("probe_after_at ASC").
		Order("id ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&states).Error
	return states, err
}

func GetModelChannelState(modelName string, channelID int) (*ModelChannelState, error) {
	if modelName == "" || channelID == 0 {
		return nil, nil
	}

	var state ModelChannelState
	err := DB.Where("model = ? AND channel_id = ?", modelName, channelID).First(&state).Error
	if err == nil {
		return &state, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, err
}

func UpsertModelChannelState(state *ModelChannelState) error {
	if state == nil || state.Model == "" || state.ChannelId == 0 {
		return nil
	}

	now := common.GetTimestamp()
	existing, err := GetModelChannelState(state.Model, state.ChannelId)
	if err != nil {
		return err
	}

	if existing == nil {
		if state.Status == "" {
			state.Status = ModelChannelStateStatusEnabled
		}
		state.CreatedAt = now
		state.UpdatedAt = now
		if err := DB.Create(state).Error; err != nil {
			return err
		}
		CacheUpdateModelChannelState(state)
		return nil
	}

	state.Id = existing.Id
	if state.CreatedAt == 0 {
		state.CreatedAt = existing.CreatedAt
	}
	if state.Status == "" {
		state.Status = existing.Status
	}
	state.UpdatedAt = now
	err = DB.Model(&ModelChannelState{}).Where("id = ?", state.Id).
		Select("model", "channel_id", "status", "consecutive_failures", "last_failure_at", "probe_after_at", "disabled_at", "disable_reason", "created_at", "updated_at").
		Updates(state).Error
	if err != nil {
		return err
	}
	CacheUpdateModelChannelState(state)
	return nil
}

func MutateModelChannelState(modelName string, channelID int, mutate func(state *ModelChannelState) error) error {
	if modelName == "" || channelID == 0 || mutate == nil {
		return nil
	}

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		var updatedState *ModelChannelState
		err := DB.Transaction(func(tx *gorm.DB) error {
			var state ModelChannelState
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("model = ? AND channel_id = ?", modelName, channelID).
				First(&state).Error
			if err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				state = ModelChannelState{
					Model:     modelName,
					ChannelId: channelID,
					Status:    ModelChannelStateStatusEnabled,
				}
				if err := mutate(&state); err != nil {
					return err
				}
				if state.Status == "" {
					state.Status = ModelChannelStateStatusEnabled
				}
				now := common.GetTimestamp()
				state.CreatedAt = now
				state.UpdatedAt = now
				if err := tx.Omit("id").Create(&state).Error; err != nil {
					return err
				}
				stateCopy := state
				updatedState = &stateCopy
				return nil
			}

			if err := mutate(&state); err != nil {
				return err
			}
			if state.Status == "" {
				state.Status = ModelChannelStateStatusEnabled
			}
			state.UpdatedAt = common.GetTimestamp()
			if err := tx.Model(&ModelChannelState{}).Where("id = ?", state.Id).
				Select("model", "channel_id", "status", "consecutive_failures", "last_failure_at", "probe_after_at", "disabled_at", "disable_reason", "created_at", "updated_at").
				Updates(&state).Error; err != nil {
				return err
			}
			stateCopy := state
			updatedState = &stateCopy
			return nil
		})
		if err == nil {
			CacheUpdateModelChannelState(updatedState)
			return nil
		}
		lastErr = err
		if !isRetryableModelChannelStateMutationErr(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return lastErr
}

func isRetryableModelChannelStateMutationErr(err error) bool {
	if err == nil {
		return false
	}
	lowerErr := strings.ToLower(err.Error())
	return strings.Contains(lowerErr, "duplicate") ||
		strings.Contains(lowerErr, "unique constraint") ||
		strings.Contains(lowerErr, "database is locked") ||
		strings.Contains(lowerErr, "database table is locked") ||
		strings.Contains(lowerErr, "deadlocked")
}
