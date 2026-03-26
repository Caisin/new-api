package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type CandidateChannel struct {
	Channel         *model.Channel
	ChannelID       int
	PolicyPriority  int64
	StateStatus     string
	PolicyApplied   bool
	FromLegacyRoute bool
}

type OrderedModelChannelCandidates struct {
	Group              string
	Model              string
	Candidates         []CandidateChannel
	PolicyRowCount     int
	UseLegacySelection bool
}

func BuildOrderedModelChannelCandidates(ctx *gin.Context, group string, modelName string) (*OrderedModelChannelCandidates, error) {
	_ = ctx

	result := &OrderedModelChannelCandidates{
		Group:      group,
		Model:      modelName,
		Candidates: make([]CandidateChannel, 0),
	}
	if group == "" || modelName == "" {
		return result, nil
	}

	effectiveModelKey, policies, err := getOrderedModelChannelPolicyLookup(modelName)
	if err != nil {
		return nil, err
	}
	result.PolicyRowCount = len(policies)
	if len(policies) == 0 {
		result.UseLegacySelection = true
		return result, nil
	}

	stateMap, err := model.CacheGetModelChannelStateMapByModel(effectiveModelKey)
	if err != nil {
		return nil, err
	}

	for _, policy := range policies {
		if !policy.ManualEnabled {
			continue
		}
		if !model.IsChannelEnabledForGroupModel(group, modelName, policy.ChannelId) {
			continue
		}

		stateStatus := model.ModelChannelStateStatusEnabled
		if state := stateMap[policy.ChannelId]; state != nil {
			stateStatus = state.Status
			if state.IsDisabled() {
				continue
			}
		}

		channel, err := model.CacheGetChannel(policy.ChannelId)
		if err != nil {
			return nil, err
		}
		if channel == nil || channel.Status != common.ChannelStatusEnabled {
			continue
		}

		result.Candidates = append(result.Candidates, CandidateChannel{
			Channel:        channel,
			ChannelID:      channel.Id,
			PolicyPriority: policy.Priority,
			StateStatus:    stateStatus,
			PolicyApplied:  true,
		})
	}

	return result, nil
}

func getOrderedModelChannelPolicyLookup(modelName string) (string, []model.ModelChannelPolicy, error) {
	policies, err := model.CacheGetModelChannelPoliciesByModel(modelName)
	if err != nil {
		return "", nil, err
	}
	if len(policies) > 0 {
		return modelName, policies, nil
	}

	normalizedModel := ratio_setting.FormatMatchingModelName(modelName)
	if normalizedModel == "" || normalizedModel == modelName {
		return modelName, policies, nil
	}

	policies, err = model.CacheGetModelChannelPoliciesByModel(normalizedModel)
	if err != nil {
		return "", nil, err
	}
	return normalizedModel, policies, nil
}
