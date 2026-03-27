package service

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
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
			if model.IsChannelNotFoundError(err) {
				result.SkippedChannels[policy.ChannelId] = "channel_missing"
				continue
			}
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
	err := model.MutateModelChannelState(modelName, channelID, func(state *model.ModelChannelState) error {
		applyModelChannelFailureState(state, reasonType, reason, threshold, probeAfterSeconds)
		return nil
	})
	if err != nil {
		return err
	}
	return MaybeEscalateChannelDisable(channelID)
}

func ScheduleNextModelChannelProbe(modelName string, channelID int, reason string) error {
	if modelName == "" || channelID == 0 {
		return nil
	}
	probeAfterSeconds := int64(operation_setting.GetModelChannelCircuitProbeIntervalMinutes() * 60)
	if probeAfterSeconds <= 0 {
		probeAfterSeconds = int64(operation_setting.DefaultModelChannelProbeIntervalMinutes * 60)
	}
	return model.MutateModelChannelState(modelName, channelID, func(state *model.ModelChannelState) error {
		if state.Status == model.ModelChannelStateStatusManualDisabled {
			return nil
		}
		now := common.GetTimestamp()
		state.Status = model.ModelChannelStateStatusAutoDisabled
		state.LastFailureAt = now
		if state.DisabledAt == 0 {
			state.DisabledAt = now
		}
		state.ProbeAfterAt = now + probeAfterSeconds
		if reason != "" {
			state.DisableReason = reason
		}
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

func GetModelChannelCircuitModels() ([]dto.ModelChannelCircuitModelSummary, error) {
	policyModelNames, err := model.ListModelChannelPolicyModels()
	if err != nil {
		return nil, err
	}
	modelNameSet := make(map[string]struct{}, len(policyModelNames))
	for _, modelName := range policyModelNames {
		modelNameSet[modelName] = struct{}{}
	}
	for _, modelName := range model.GetEnabledModels() {
		if modelName == "" {
			continue
		}
		modelNameSet[modelName] = struct{}{}
	}
	modelNames := make([]string, 0, len(modelNameSet))
	for modelName := range modelNameSet {
		modelNames = append(modelNames, modelName)
	}
	sort.Strings(modelNames)

	items := make([]dto.ModelChannelCircuitModelSummary, 0, len(modelNames))
	for _, modelName := range modelNames {
		policies, err := model.GetModelChannelPoliciesByModel(modelName)
		if err != nil {
			return nil, err
		}
		stateMap, err := model.GetModelChannelStateMapByModel(modelName)
		if err != nil {
			return nil, err
		}
		summary := dto.ModelChannelCircuitModelSummary{
			Model:           modelName,
			PolicyCount:     len(policies),
			BootstrapNeeded: len(policies) == 0,
		}
		policyChannelIDs := make(map[int]struct{}, len(policies))
		manualDisabledChannels := make(map[int]struct{})
		for _, policy := range policies {
			policyChannelIDs[policy.ChannelId] = struct{}{}
			if !policy.ManualEnabled {
				manualDisabledChannels[policy.ChannelId] = struct{}{}
			}
		}
		for _, state := range stateMap {
			if _, ok := policyChannelIDs[state.ChannelId]; !ok {
				continue
			}
			switch state.Status {
			case model.ModelChannelStateStatusAutoDisabled:
				summary.AutoDisabledCount++
			case model.ModelChannelStateStatusManualDisabled:
				manualDisabledChannels[state.ChannelId] = struct{}{}
			}
		}
		summary.ManualDisabled = len(manualDisabledChannels)
		items = append(items, summary)
	}
	return items, nil
}

func GetModelChannelCircuitDetail(modelName string) (*dto.ModelChannelCircuitDetail, error) {
	policies, err := model.GetModelChannelPoliciesByModel(modelName)
	if err != nil {
		return nil, err
	}
	stateMap, err := model.GetModelChannelStateMapByModel(modelName)
	if err != nil {
		return nil, err
	}
	detail := &dto.ModelChannelCircuitDetail{
		Model:           modelName,
		BootstrapNeeded: len(policies) == 0,
		Channels:        make([]dto.ModelChannelCircuitChannelDetail, 0, len(policies)),
	}
	if len(policies) == 0 {
		bootstrapChannels, err := buildBootstrapModelChannelCircuitDetail(modelName)
		if err != nil {
			return nil, err
		}
		if len(bootstrapChannels) == 0 {
			detail.BootstrapNeeded = false
			return detail, nil
		}
		detail.Channels = bootstrapChannels
		return detail, nil
	}
	for _, policy := range policies {
		item := dto.ModelChannelCircuitChannelDetail{
			ChannelId:     policy.ChannelId,
			Priority:      policy.Priority,
			ManualEnabled: policy.ManualEnabled,
			Status:        model.ModelChannelStateStatusEnabled,
		}
		channel, err := model.GetChannelById(policy.ChannelId, true)
		if err != nil {
			if model.IsChannelNotFoundError(err) {
				item.ChannelMissing = true
			} else {
				return nil, err
			}
		} else if channel != nil {
			item.ChannelName = channel.Name
			item.ChannelType = channel.Type
			item.ChannelStatus = channel.Status
		}
		if state := stateMap[policy.ChannelId]; state != nil {
			item.Status = state.Status
			item.ConsecutiveFailures = state.ConsecutiveFailures
			item.LastFailureAt = state.LastFailureAt
			item.ProbeAfterAt = state.ProbeAfterAt
			item.DisabledAt = state.DisabledAt
			item.DisableReason = state.DisableReason
		}
		detail.Channels = append(detail.Channels, item)
	}
	return detail, nil
}

func buildBootstrapModelChannelCircuitDetail(modelName string) ([]dto.ModelChannelCircuitChannelDetail, error) {
	channels, err := getBootstrapChannelsForModel(modelName)
	if err != nil {
		return nil, err
	}
	items := make([]dto.ModelChannelCircuitChannelDetail, 0, len(channels))
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		priority := int64(0)
		if channel.Priority != nil {
			priority = *channel.Priority
		}
		items = append(items, dto.ModelChannelCircuitChannelDetail{
			ChannelId:     channel.Id,
			ChannelName:   channel.Name,
			ChannelType:   channel.Type,
			ChannelStatus: channel.Status,
			Priority:      priority,
			ManualEnabled: true,
			Status:        model.ModelChannelStateStatusEnabled,
		})
	}
	return items, nil
}

func getBootstrapChannelsForModel(modelName string) ([]*model.Channel, error) {
	channelIDs, err := model.GetEnabledChannelIDsByModel(modelName)
	if err != nil {
		return nil, err
	}
	if len(channelIDs) == 0 {
		return []*model.Channel{}, nil
	}
	channels, err := model.GetChannelsByIds(channelIDs)
	if err != nil {
		return nil, err
	}
	sort.Slice(channels, func(i, j int) bool {
		leftPriority := int64(0)
		rightPriority := int64(0)
		if channels[i] != nil && channels[i].Priority != nil {
			leftPriority = *channels[i].Priority
		}
		if channels[j] != nil && channels[j].Priority != nil {
			rightPriority = *channels[j].Priority
		}
		if leftPriority == rightPriority {
			leftID := 0
			rightID := 0
			if channels[i] != nil {
				leftID = channels[i].Id
			}
			if channels[j] != nil {
				rightID = channels[j].Id
			}
			return leftID < rightID
		}
		return leftPriority > rightPriority
	})
	return channels, nil
}

func bootstrapModelChannelPoliciesFromAbilities(modelName string, requiredChannelID int) error {
	if modelName == "" {
		return fmt.Errorf("model-channel policy not found")
	}
	existingPolicies, err := model.GetModelChannelPoliciesByModel(modelName)
	if err != nil {
		return err
	}
	if len(existingPolicies) > 0 {
		return nil
	}
	channels, err := getBootstrapChannelsForModel(modelName)
	if err != nil {
		return err
	}
	if len(channels) == 0 {
		return fmt.Errorf("model-channel policy not found")
	}
	policies := make([]model.ModelChannelPolicy, 0, len(channels))
	requiredFound := requiredChannelID == 0
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		if channel.Id == requiredChannelID {
			requiredFound = true
		}
		priority := int64(0)
		if channel.Priority != nil {
			priority = *channel.Priority
		}
		policies = append(policies, model.ModelChannelPolicy{
			Model:         modelName,
			ChannelId:     channel.Id,
			Priority:      priority,
			ManualEnabled: true,
		})
	}
	if !requiredFound {
		return fmt.Errorf("model-channel policy not found")
	}
	if len(policies) == 0 {
		return fmt.Errorf("model-channel policy not found")
	}
	if err := model.ReplaceModelChannelPolicies(modelName, policies); err != nil {
		return err
	}
	model.InitChannelCache()
	return nil
}

func SaveModelChannelPolicies(modelName string, items []dto.ModelChannelPolicyItem) error {
	if len(items) == 0 {
		return fmt.Errorf("channels 不能为空")
	}
	policies := make([]model.ModelChannelPolicy, 0, len(items))
	for _, item := range items {
		if item.ChannelId == 0 {
			continue
		}
		policies = append(policies, model.ModelChannelPolicy{
			Model:         modelName,
			ChannelId:     item.ChannelId,
			Priority:      item.Priority,
			ManualEnabled: item.ManualEnabled,
		})
	}
	if len(policies) == 0 {
		return fmt.Errorf("channels 不能为空")
	}
	if err := model.ReplaceModelChannelPoliciesAndCleanupStates(modelName, policies); err != nil {
		return err
	}
	model.InitChannelCache()
	return nil
}

func EnableModelChannelCircuitPair(modelName string, channelID int) error {
	if _, _, err := getRequiredModelChannelPolicyAndChannel(modelName, channelID); err != nil {
		return err
	}
	return model.MutateModelChannelState(modelName, channelID, func(state *model.ModelChannelState) error {
		state.Status = model.ModelChannelStateStatusEnabled
		state.ConsecutiveFailures = 0
		state.LastFailureAt = 0
		state.ProbeAfterAt = 0
		state.DisabledAt = 0
		state.DisableReason = ""
		return nil
	})
}

func DisableModelChannelCircuitPair(modelName string, channelID int) error {
	if _, _, err := getRequiredModelChannelPolicyAndChannel(modelName, channelID); err != nil {
		return err
	}
	return model.MutateModelChannelState(modelName, channelID, func(state *model.ModelChannelState) error {
		state.Status = model.ModelChannelStateStatusManualDisabled
		state.DisabledAt = common.GetTimestamp()
		state.ProbeAfterAt = 0
		state.DisableReason = "manual_disabled"
		return nil
	})
}

func ProbeModelChannelCircuitPair(modelName string, channelID int) ModelChannelProbeResult {
	result := ModelChannelProbeResult{
		Model:     modelName,
		ChannelId: channelID,
	}
	if _, _, err := getRequiredModelChannelPolicyAndChannel(modelName, channelID); err != nil {
		result.Message = err.Error()
		return result
	}
	return ProbeModelChannelPair(context.Background(), modelName, channelID)
}

func getRequiredModelChannelPolicyAndChannel(modelName string, channelID int) (*model.ModelChannelPolicy, *model.Channel, error) {
	policy, err := model.GetModelChannelPolicy(modelName, channelID)
	if err != nil {
		return nil, nil, err
	}
	if policy == nil {
		if err := bootstrapModelChannelPoliciesFromAbilities(modelName, channelID); err != nil {
			return nil, nil, err
		}
		policy, err = model.GetModelChannelPolicy(modelName, channelID)
		if err != nil {
			return nil, nil, err
		}
		if policy == nil {
			return nil, nil, fmt.Errorf("model-channel policy not found")
		}
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		if model.IsChannelNotFoundError(err) {
			return nil, nil, fmt.Errorf("model-channel pair channel not found")
		}
		return nil, nil, err
	}
	return policy, channel, nil
}
