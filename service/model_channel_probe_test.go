package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestProbeAutoDisabledModelChannelSuccessRestoresStateAndChannel(t *testing.T) {
	db := setupModelChannelCircuitTestDB(t)

	channel := &model.Channel{
		Id:      101,
		Name:    "probe-channel",
		Key:     "test-key",
		Status:  common.ChannelStatusAutoDisabled,
		AutoBan: common.GetPointer(1),
		Models:  "gpt-4.1",
		Group:   "default",
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	require.NoError(t, model.UpsertModelChannelState(&model.ModelChannelState{
		Model:               "gpt-4.1",
		ChannelId:           101,
		Status:              model.ModelChannelStateStatusAutoDisabled,
		ConsecutiveFailures: 3,
		LastFailureAt:       100,
		ProbeAfterAt:        50,
		DisabledAt:          100,
		DisableReason:       "model_rate_limited",
	}))
	model.InitChannelCache()

	restoreProbeExecutorForTest(t, func(ctx context.Context, modelName string, channel *model.Channel) ModelChannelProbeAttemptResult {
		return ModelChannelProbeAttemptResult{UsingKey: "test-key"}
	})

	result := ProbeModelChannelPair(context.Background(), "gpt-4.1", 101)
	require.True(t, result.Success)

	state, err := model.GetModelChannelState("gpt-4.1", 101)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, model.ModelChannelStateStatusEnabled, state.Status)
	require.Zero(t, state.ConsecutiveFailures)
	require.Zero(t, state.ProbeAfterAt)
	require.Zero(t, state.DisabledAt)

	reloadedChannel, err := model.GetChannelById(101, true)
	require.NoError(t, err)
	require.NotNil(t, reloadedChannel)
	require.Equal(t, common.ChannelStatusEnabled, reloadedChannel.Status)
}

func TestProbeAutoDisabledModelChannelFailureSchedulesNextProbe(t *testing.T) {
	db := setupModelChannelCircuitTestDB(t)

	channel := &model.Channel{
		Id:      102,
		Name:    "probe-channel",
		Key:     "test-key",
		Status:  common.ChannelStatusEnabled,
		AutoBan: common.GetPointer(1),
		Models:  "gpt-4.1",
		Group:   "default",
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	require.NoError(t, model.UpsertModelChannelState(&model.ModelChannelState{
		Model:               "gpt-4.1",
		ChannelId:           102,
		Status:              model.ModelChannelStateStatusAutoDisabled,
		ConsecutiveFailures: 3,
		LastFailureAt:       100,
		ProbeAfterAt:        50,
		DisabledAt:          100,
		DisableReason:       "model_not_supported",
	}))
	model.InitChannelCache()

	restoreProbeExecutorForTest(t, func(ctx context.Context, modelName string, channel *model.Channel) ModelChannelProbeAttemptResult {
		return ModelChannelProbeAttemptResult{
			Err: types.NewOpenAIError(errors.New("still failing"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests),
		}
	})

	result := ProbeModelChannelPair(context.Background(), "gpt-4.1", 102)
	require.False(t, result.Success)
	require.Contains(t, result.Message, "still failing")

	state, err := model.GetModelChannelState("gpt-4.1", 102)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, model.ModelChannelStateStatusAutoDisabled, state.Status)
	require.Greater(t, state.ProbeAfterAt, int64(50))
	require.Equal(t, "still failing", state.DisableReason)
}

func TestProbeModelChannelPairFallsBackToDatabaseWhenChannelCacheMisses(t *testing.T) {
	db := setupModelChannelCircuitTestDB(t)

	channel := &model.Channel{
		Id:      104,
		Name:    "probe-db-fallback",
		Key:     "test-key",
		Status:  common.ChannelStatusEnabled,
		AutoBan: common.GetPointer(1),
		Models:  "gpt-4.1",
		Group:   "default",
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	require.NoError(t, model.UpsertModelChannelState(&model.ModelChannelState{
		Model:               "gpt-4.1",
		ChannelId:           104,
		Status:              model.ModelChannelStateStatusAutoDisabled,
		ConsecutiveFailures: 3,
		LastFailureAt:       100,
		ProbeAfterAt:        50,
		DisabledAt:          100,
		DisableReason:       "model_rate_limited",
	}))

	restoreProbeExecutorForTest(t, func(ctx context.Context, modelName string, channel *model.Channel) ModelChannelProbeAttemptResult {
		return ModelChannelProbeAttemptResult{UsingKey: "test-key"}
	})

	result := ProbeModelChannelPair(context.Background(), "gpt-4.1", 104)
	require.True(t, result.Success)

	state, err := model.GetModelChannelState("gpt-4.1", 104)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, model.ModelChannelStateStatusEnabled, state.Status)
}

func TestMaybeEscalateChannelDisableWhenAllConfiguredModelsDisabled(t *testing.T) {
	db := setupModelChannelCircuitTestDB(t)

	channel := &model.Channel{
		Id:      103,
		Name:    "escalate-channel",
		Key:     "test-key",
		Status:  common.ChannelStatusEnabled,
		AutoBan: common.GetPointer(1),
		Models:  "gpt-4.1,gpt-4.1-mini",
		Group:   "default",
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-4.1",
		ChannelId:     103,
		Priority:      100,
		ManualEnabled: true,
	}).Insert())
	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-4.1-mini",
		ChannelId:     103,
		Priority:      90,
		ManualEnabled: true,
	}).Insert())
	require.NoError(t, model.UpsertModelChannelState(&model.ModelChannelState{
		Model:     "gpt-4.1",
		ChannelId: 103,
		Status:    model.ModelChannelStateStatusAutoDisabled,
	}))
	require.NoError(t, model.UpsertModelChannelState(&model.ModelChannelState{
		Model:     "gpt-4.1-mini",
		ChannelId: 103,
		Status:    model.ModelChannelStateStatusManualDisabled,
	}))
	model.InitChannelCache()

	require.NoError(t, MaybeEscalateChannelDisable(103))

	reloadedChannel, err := model.GetChannelById(103, true)
	require.NoError(t, err)
	require.NotNil(t, reloadedChannel)
	require.Equal(t, common.ChannelStatusAutoDisabled, reloadedChannel.Status)
}

func TestShouldDisableChannelSkipsModelScopedFailures(t *testing.T) {
	oldAutoDisable := common.AutomaticDisableChannelEnabled
	oldRanges := append([]operation_setting.StatusCodeRange(nil), operation_setting.AutomaticDisableStatusCodeRanges...)
	oldKeywords := append([]string(nil), operation_setting.AutomaticDisableKeywords...)
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 400, End: 429}}
	operation_setting.AutomaticDisableKeywords = []string{"rate limited", "model missing", "not supported"}
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = oldAutoDisable
		operation_setting.AutomaticDisableStatusCodeRanges = oldRanges
		operation_setting.AutomaticDisableKeywords = oldKeywords
	})

	rateLimitedErr := types.NewOpenAIError(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)
	modelNotFoundErr := types.NewOpenAIError(errors.New("model missing"), types.ErrorCodeModelNotFound, http.StatusNotFound)
	unsupportedModelErr := types.NewOpenAIError(errors.New("model gpt-4.1 is not supported by this provider"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)

	require.False(t, ShouldDisableChannel(0, rateLimitedErr))
	require.False(t, ShouldDisableChannel(0, modelNotFoundErr))
	require.False(t, ShouldDisableChannel(0, unsupportedModelErr))
}

func restoreProbeExecutorForTest(t *testing.T, executor ModelChannelProbeExecutor) {
	t.Helper()

	oldExecutor := modelChannelProbeExecutor
	modelChannelProbeExecutor = executor
	t.Cleanup(func() {
		modelChannelProbeExecutor = oldExecutor
	})
}
