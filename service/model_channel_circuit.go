package service

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	modelChannelRetryPathContextKey = "model_channel_retry_path"
	modelChannelSkippedContextKey   = "model_channel_skipped"
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
	PolicyLookupModel  string
	Candidates         []CandidateChannel
	SkippedChannels    map[int]string
	PolicyRowCount     int
	UseLegacySelection bool
}

func BuildOrderedModelChannelCandidates(ctx *gin.Context, group string, modelName string) (*OrderedModelChannelCandidates, error) {
	_ = ctx

	result := &OrderedModelChannelCandidates{
		Group:             group,
		Model:             modelName,
		PolicyLookupModel: modelName,
		Candidates:        make([]CandidateChannel, 0),
		SkippedChannels:   make(map[int]string),
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
	result.PolicyLookupModel = effectiveModelKey

	stateMap, err := model.CacheGetModelChannelStateMapByModel(effectiveModelKey)
	if err != nil {
		return nil, err
	}

	for _, policy := range policies {
		if !policy.ManualEnabled {
			result.SkippedChannels[policy.ChannelId] = "policy_manual_disabled"
			continue
		}
		channel, err := model.CacheGetChannel(policy.ChannelId)
		if err != nil {
			return nil, err
		}
		if channel == nil || channel.Status != common.ChannelStatusEnabled {
			result.SkippedChannels[policy.ChannelId] = "channel_disabled"
			continue
		}
		if !model.IsChannelEnabledForGroupModel(group, modelName, policy.ChannelId) {
			result.SkippedChannels[policy.ChannelId] = "group_model_unavailable"
			continue
		}

		stateStatus := model.ModelChannelStateStatusEnabled
		if state := stateMap[policy.ChannelId]; state != nil {
			stateStatus = state.Status
			if state.IsDisabled() {
				result.SkippedChannels[policy.ChannelId] = state.Status
				continue
			}
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

func ApplyModelChannelFailure(state *model.ModelChannelState, reasonType string, reason string, threshold int, probeAfterSeconds int64) (bool, error) {
	if state == nil {
		return true, nil
	}
	shouldRetryNext := applyModelChannelFailureState(state, reasonType, reason, threshold, probeAfterSeconds)
	return shouldRetryNext, model.UpsertModelChannelState(state)
}

func AppendRetryPath(ctx *gin.Context, channelID int) {
	if ctx == nil {
		return
	}
	path, _ := ctx.Get(modelChannelRetryPathContextKey)
	if ids, ok := path.([]int); ok {
		ids = append(ids, channelID)
		ctx.Set(modelChannelRetryPathContextKey, ids)
		return
	}
	ctx.Set(modelChannelRetryPathContextKey, []int{channelID})
}

func GetRetryPath(ctx *gin.Context) ([]int, string) {
	if ctx == nil {
		return nil, ""
	}
	path, _ := ctx.Get(modelChannelRetryPathContextKey)
	ids, ok := path.([]int)
	if !ok || len(ids) == 0 {
		return []int{}, ""
	}
	parts := make([]string, 0, len(ids))
	for _, channelID := range ids {
		parts = append(parts, strconv.Itoa(channelID))
	}
	return append([]int(nil), ids...), strings.Join(parts, " -> ")
}

func SetSkippedModelChannels(ctx *gin.Context, skipped map[int]string) {
	if ctx == nil {
		return
	}
	if len(skipped) == 0 {
		ctx.Set(modelChannelSkippedContextKey, map[int]string{})
		return
	}
	cloned := make(map[int]string, len(skipped))
	for channelID, reason := range skipped {
		cloned[channelID] = reason
	}
	ctx.Set(modelChannelSkippedContextKey, cloned)
}

func ResetModelChannelFailure(modelName string, channelID int) error {
	existing, err := model.GetModelChannelState(modelName, channelID)
	if err != nil || existing == nil {
		return err
	}
	if existing.Status == model.ModelChannelStateStatusManualDisabled {
		return nil
	}
	return model.MutateModelChannelState(modelName, channelID, func(state *model.ModelChannelState) error {
		if state.Status == model.ModelChannelStateStatusManualDisabled {
			return nil
		}
		state.Status = model.ModelChannelStateStatusEnabled
		state.ConsecutiveFailures = 0
		state.LastFailureAt = 0
		state.ProbeAfterAt = 0
		state.DisabledAt = 0
		state.DisableReason = ""
		return nil
	})
}

func RecordModelChannelFailure(modelName string, channelID int, apiErr *types.NewAPIError) error {
	if !ShouldTrackModelChannelFailure(apiErr) || modelName == "" || channelID == 0 {
		return nil
	}

	threshold := operation_setting.GetModelChannelCircuitFailureThreshold()
	probeAfterSeconds := int64(operation_setting.GetModelChannelCircuitProbeIntervalMinutes() * 60)
	reasonType, reason := classifyModelChannelFailure(apiErr)
	return model.MutateModelChannelState(modelName, channelID, func(state *model.ModelChannelState) error {
		applyModelChannelFailureState(state, reasonType, reason, threshold, probeAfterSeconds)
		return nil
	})
}

func ShouldTrackModelChannelFailure(apiErr *types.NewAPIError) bool {
	if apiErr == nil {
		return false
	}
	if apiErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	switch apiErr.GetErrorCode() {
	case types.ErrorCodeModelNotFound:
		return true
	default:
		return isUnsupportedModelError(apiErr)
	}
}

func classifyModelChannelFailure(apiErr *types.NewAPIError) (string, string) {
	if apiErr == nil {
		return "", ""
	}
	if apiErr.StatusCode == http.StatusTooManyRequests {
		return "model_rate_limited", apiErr.ErrorWithStatusCode()
	}
	switch apiErr.GetErrorCode() {
	case types.ErrorCodeModelNotFound:
		return "model_not_found", apiErr.ErrorWithStatusCode()
	default:
		if isUnsupportedModelError(apiErr) {
			return "model_not_supported", apiErr.ErrorWithStatusCode()
		}
		return "model_error", apiErr.ErrorWithStatusCode()
	}
}

func isUnsupportedModelError(apiErr *types.NewAPIError) bool {
	if apiErr == nil {
		return false
	}
	lowerMessage := strings.ToLower(apiErr.Error())
	if !strings.Contains(lowerMessage, "model") {
		return false
	}
	return strings.Contains(lowerMessage, "not supported") ||
		strings.Contains(lowerMessage, "unsupported") ||
		strings.Contains(lowerMessage, "does not support")
}

func applyModelChannelFailureState(state *model.ModelChannelState, reasonType string, reason string, threshold int, probeAfterSeconds int64) bool {
	now := common.GetTimestamp()
	if state.Status == "" {
		state.Status = model.ModelChannelStateStatusEnabled
	}
	state.LastFailureAt = now
	state.ConsecutiveFailures++
	if reasonType != "" {
		state.DisableReason = reasonType
	} else {
		state.DisableReason = reason
	}

	if threshold <= 0 {
		threshold = 1
	}
	if state.ConsecutiveFailures >= threshold {
		state.Status = model.ModelChannelStateStatusAutoDisabled
		state.DisabledAt = now
		if probeAfterSeconds > 0 {
			state.ProbeAfterAt = now + probeAfterSeconds
		} else {
			state.ProbeAfterAt = 0
		}
	} else {
		state.Status = model.ModelChannelStateStatusEnabled
		state.DisabledAt = 0
		state.ProbeAfterAt = 0
	}
	return true
}
