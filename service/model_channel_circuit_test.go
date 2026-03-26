package service

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelChannelCircuitTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ModelChannelPolicy{}, &model.ModelChannelState{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)

	oldDB := model.DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldCacheState := model.SnapshotChannelCacheStateForTest()

	model.DB = db
	common.MemoryCacheEnabled = true
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		model.RestoreChannelCacheStateForTest(oldCacheState)
	})

	return db
}

func createModelCircuitTestChannel(t *testing.T, db *gorm.DB, id int, group string, modelName string, status int, channelPriority int64, weight uint) {
	t.Helper()

	channel := &model.Channel{
		Id:       id,
		Name:     fmt.Sprintf("channel-%d", id),
		Key:      fmt.Sprintf("key-%d", id),
		Group:    group,
		Models:   modelName,
		Status:   status,
		Priority: &channelPriority,
		Weight:   &weight,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
}

func TestBuildOrderedModelChannelCandidatesUsesPolicyOrderAndFiltersBlockedChannels(t *testing.T) {
	db := setupModelChannelCircuitTestDB(t)

	createModelCircuitTestChannel(t, db, 11, "default", "gpt-4.1", common.ChannelStatusEnabled, 10, 1)
	createModelCircuitTestChannel(t, db, 12, "default", "gpt-4.1", common.ChannelStatusEnabled, 20, 1)
	createModelCircuitTestChannel(t, db, 13, "default", "gpt-4.1", common.ChannelStatusEnabled, 30, 1)
	createModelCircuitTestChannel(t, db, 14, "default", "gpt-4.1", common.ChannelStatusEnabled, 40, 1)
	createModelCircuitTestChannel(t, db, 15, "vip", "gpt-4.1", common.ChannelStatusEnabled, 50, 1)
	createModelCircuitTestChannel(t, db, 16, "default", "gpt-4.1", common.ChannelStatusManuallyDisabled, 60, 1)
	createModelCircuitTestChannel(t, db, 17, "default", "gpt-4.1", common.ChannelStatusEnabled, 999, 1)

	policies := []model.ModelChannelPolicy{
		{Model: "gpt-4.1", ChannelId: 11, Priority: 100, ManualEnabled: true},
		{Model: "gpt-4.1", ChannelId: 12, Priority: 90, ManualEnabled: false},
		{Model: "gpt-4.1", ChannelId: 13, Priority: 80, ManualEnabled: true},
		{Model: "gpt-4.1", ChannelId: 14, Priority: 70, ManualEnabled: true},
		{Model: "gpt-4.1", ChannelId: 15, Priority: 60, ManualEnabled: true},
		{Model: "gpt-4.1", ChannelId: 16, Priority: 50, ManualEnabled: true},
		{Model: "gpt-4.1", ChannelId: 17, Priority: 40, ManualEnabled: true},
	}
	for i := range policies {
		model.DB = db
		require.NoError(t, policies[i].Insert())
	}

	require.NoError(t, db.Create(&model.ModelChannelState{
		Model:     "gpt-4.1",
		ChannelId: 13,
		Status:    model.ModelChannelStateStatusAutoDisabled,
	}).Error)
	require.NoError(t, db.Create(&model.ModelChannelState{
		Model:     "gpt-4.1",
		ChannelId: 14,
		Status:    model.ModelChannelStateStatusManualDisabled,
	}).Error)
	require.NoError(t, db.Create(&model.ModelChannelState{
		Model:     "gpt-4.1",
		ChannelId: 17,
		Status:    model.ModelChannelStateStatusEnabled,
	}).Error)

	model.InitChannelCache()

	result, err := BuildOrderedModelChannelCandidates(nil, "default", "gpt-4.1")
	require.NoError(t, err)
	require.False(t, result.UseLegacySelection)
	require.Equal(t, 7, result.PolicyRowCount)
	require.Len(t, result.Candidates, 2)
	require.Equal(t, "policy_manual_disabled", result.SkippedChannels[12])
	require.Equal(t, model.ModelChannelStateStatusAutoDisabled, result.SkippedChannels[13])
	require.Equal(t, model.ModelChannelStateStatusManualDisabled, result.SkippedChannels[14])
	require.Equal(t, "group_model_unavailable", result.SkippedChannels[15])
	require.Equal(t, "channel_disabled", result.SkippedChannels[16])

	require.Equal(t, 11, result.Candidates[0].ChannelID)
	require.Equal(t, int64(100), result.Candidates[0].PolicyPriority)
	require.Equal(t, model.ModelChannelStateStatusEnabled, result.Candidates[0].StateStatus)
	require.Equal(t, 17, result.Candidates[1].ChannelID)
	require.Equal(t, int64(40), result.Candidates[1].PolicyPriority)
	require.Equal(t, model.ModelChannelStateStatusEnabled, result.Candidates[1].StateStatus)
	require.Equal(t, 11, result.Candidates[0].Channel.Id)
	require.Equal(t, 17, result.Candidates[1].Channel.Id)
}

func TestBuildOrderedModelChannelCandidatesFallsBackWhenModelHasNoPolicyRows(t *testing.T) {
	db := setupModelChannelCircuitTestDB(t)

	createModelCircuitTestChannel(t, db, 21, "default", "gpt-no-policy", common.ChannelStatusEnabled, 50, 5)
	createModelCircuitTestChannel(t, db, 22, "default", "gpt-no-policy", common.ChannelStatusEnabled, 10, 10)

	model.InitChannelCache()

	result, err := BuildOrderedModelChannelCandidates(nil, "default", "gpt-no-policy")
	require.NoError(t, err)
	require.True(t, result.UseLegacySelection)
	require.Equal(t, "gpt-no-policy", result.PolicyLookupModel)
	require.Equal(t, 0, result.PolicyRowCount)
	require.Empty(t, result.Candidates)
}

func TestBuildOrderedModelChannelCandidatesKeepsRequestedLookupModelWhenFallingBackWithoutPolicies(t *testing.T) {
	setupModelChannelCircuitTestDB(t)

	result, err := BuildOrderedModelChannelCandidates(nil, "default", "gpt-4o-gizmo-special")
	require.NoError(t, err)
	require.True(t, result.UseLegacySelection)
	require.Equal(t, "gpt-4o-gizmo-special", result.Model)
	require.Equal(t, "gpt-4o-gizmo-special", result.PolicyLookupModel)
	require.Equal(t, 0, result.PolicyRowCount)
	require.Empty(t, result.Candidates)
}

func TestBuildOrderedModelChannelCandidatesUsesNormalizedPolicyAndStateModelKey(t *testing.T) {
	db := setupModelChannelCircuitTestDB(t)

	createModelCircuitTestChannel(t, db, 41, "default", "gpt-4o-gizmo-*", common.ChannelStatusEnabled, 10, 1)
	createModelCircuitTestChannel(t, db, 42, "default", "gpt-4o-gizmo-*", common.ChannelStatusEnabled, 20, 1)

	model.DB = db
	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-4o-gizmo-*",
		ChannelId:     41,
		Priority:      100,
		ManualEnabled: true,
	}).Insert())
	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-4o-gizmo-*",
		ChannelId:     42,
		Priority:      90,
		ManualEnabled: true,
	}).Insert())
	require.NoError(t, db.Create(&model.ModelChannelState{
		Model:     "gpt-4o-gizmo-*",
		ChannelId: 42,
		Status:    model.ModelChannelStateStatusAutoDisabled,
	}).Error)

	model.InitChannelCache()

	result, err := BuildOrderedModelChannelCandidates(nil, "default", "gpt-4o-gizmo-special")
	require.NoError(t, err)
	require.False(t, result.UseLegacySelection)
	require.Equal(t, "gpt-4o-gizmo-special", result.Model)
	require.Equal(t, "gpt-4o-gizmo-*", result.PolicyLookupModel)
	require.Equal(t, 2, result.PolicyRowCount)
	require.Len(t, result.Candidates, 1)
	require.Equal(t, 41, result.Candidates[0].ChannelID)
	require.Equal(t, int64(100), result.Candidates[0].PolicyPriority)
}

func TestBuildOrderedModelChannelCandidatesDoesNotFallbackWhenPoliciesExistButNoCandidatesRemain(t *testing.T) {
	db := setupModelChannelCircuitTestDB(t)

	createModelCircuitTestChannel(t, db, 31, "default", "gpt-blocked", common.ChannelStatusEnabled, 10, 1)
	createModelCircuitTestChannel(t, db, 32, "vip", "gpt-blocked", common.ChannelStatusEnabled, 20, 1)

	model.DB = db
	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-blocked",
		ChannelId:     31,
		Priority:      100,
		ManualEnabled: false,
	}).Insert())
	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-blocked",
		ChannelId:     32,
		Priority:      90,
		ManualEnabled: true,
	}).Insert())

	model.InitChannelCache()

	result, err := BuildOrderedModelChannelCandidates(nil, "default", "gpt-blocked")
	require.NoError(t, err)
	require.False(t, result.UseLegacySelection)
	require.Equal(t, 2, result.PolicyRowCount)
	require.Equal(t, "policy_manual_disabled", result.SkippedChannels[31])
	require.Equal(t, "group_model_unavailable", result.SkippedChannels[32])
	require.Empty(t, result.Candidates)
}

func TestBuildOrderedModelChannelCandidatesSkipsMissingPolicyChannel(t *testing.T) {
	db := setupModelChannelCircuitTestDB(t)

	createModelCircuitTestChannel(t, db, 61, "default", "gpt-missing", common.ChannelStatusEnabled, 20, 1)

	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-missing",
		ChannelId:     60,
		Priority:      100,
		ManualEnabled: true,
	}).Insert())
	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-missing",
		ChannelId:     61,
		Priority:      90,
		ManualEnabled: true,
	}).Insert())

	model.InitChannelCache()

	result, err := BuildOrderedModelChannelCandidates(nil, "default", "gpt-missing")
	require.NoError(t, err)
	require.Len(t, result.Candidates, 1)
	require.Equal(t, 61, result.Candidates[0].ChannelID)
	require.Equal(t, "channel_missing", result.SkippedChannels[60])
}

func TestApplyModelChannelFailureRetriesImmediatelyBeforeThreshold(t *testing.T) {
	setupModelChannelCircuitTestDB(t)

	state := &model.ModelChannelState{
		Model:               "gpt-4.1",
		ChannelId:           11,
		Status:              model.ModelChannelStateStatusEnabled,
		ConsecutiveFailures: 1,
	}

	shouldRetryNext, err := ApplyModelChannelFailure(state, "model_rate_limited", "429", 3, 300)
	require.NoError(t, err)
	require.True(t, shouldRetryNext)
	require.Equal(t, model.ModelChannelStateStatusEnabled, state.Status)
	require.Equal(t, 2, state.ConsecutiveFailures)
	require.Positive(t, state.LastFailureAt)
	require.Zero(t, state.ProbeAfterAt)
	require.Equal(t, "model_rate_limited", state.DisableReason)

	saved, err := model.GetModelChannelState("gpt-4.1", 11)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, model.ModelChannelStateStatusEnabled, saved.Status)
	require.Equal(t, 2, saved.ConsecutiveFailures)
}

func TestApplyModelChannelFailureDisablesAtThresholdAndSchedulesProbe(t *testing.T) {
	setupModelChannelCircuitTestDB(t)

	state := &model.ModelChannelState{
		Model:               "gpt-4.1",
		ChannelId:           12,
		Status:              model.ModelChannelStateStatusEnabled,
		ConsecutiveFailures: 2,
	}

	shouldRetryNext, err := ApplyModelChannelFailure(state, "model_rate_limited", "429", 3, 300)
	require.NoError(t, err)
	require.True(t, shouldRetryNext)
	require.Equal(t, model.ModelChannelStateStatusAutoDisabled, state.Status)
	require.Equal(t, 3, state.ConsecutiveFailures)
	require.Positive(t, state.DisabledAt)
	require.Greater(t, state.ProbeAfterAt, state.LastFailureAt)

	saved, err := model.GetModelChannelState("gpt-4.1", 12)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, model.ModelChannelStateStatusAutoDisabled, saved.Status)
	require.Equal(t, 3, saved.ConsecutiveFailures)
}

func TestRecordRetryPathAppendsActualAttemptedChannels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	AppendRetryPath(ctx, 11)
	AppendRetryPath(ctx, 18)
	AppendRetryPath(ctx, 24)

	pathIDs, pathText := GetRetryPath(ctx)
	require.Equal(t, []int{11, 18, 24}, pathIDs)
	require.Equal(t, "11 -> 18 -> 24", pathText)
}

func TestShouldTrackModelChannelFailureTreatsUnsupportedModelAsModelScoped(t *testing.T) {
	apiErr := types.NewOpenAIError(errors.New("model gpt-4.1 is not supported by this provider"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)

	require.True(t, ShouldTrackModelChannelFailure(apiErr))
	reasonType, reason := classifyModelChannelFailure(apiErr)
	require.Equal(t, "model_not_supported", reasonType)
	require.Contains(t, reason, "not supported")
}

func TestRecordModelChannelFailureCountsConcurrentFailures(t *testing.T) {
	setupModelChannelCircuitTestDB(t)

	apiErr := types.NewOpenAIError(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)
	var wg sync.WaitGroup
	errCh := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- RecordModelChannelFailure("gpt-4.1", 19, apiErr)
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	saved, err := model.GetModelChannelState("gpt-4.1", 19)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, 5, saved.ConsecutiveFailures)
	require.Equal(t, model.ModelChannelStateStatusAutoDisabled, saved.Status)
}

func TestResetModelChannelFailurePreservesManualDisabled(t *testing.T) {
	setupModelChannelCircuitTestDB(t)

	require.NoError(t, model.UpsertModelChannelState(&model.ModelChannelState{
		Model:               "gpt-4.1",
		ChannelId:           20,
		Status:              model.ModelChannelStateStatusManualDisabled,
		ConsecutiveFailures: 4,
		LastFailureAt:       123,
		ProbeAfterAt:        456,
		DisabledAt:          789,
		DisableReason:       "manual",
	}))

	require.NoError(t, ResetModelChannelFailure("gpt-4.1", 20))

	saved, err := model.GetModelChannelState("gpt-4.1", 20)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, model.ModelChannelStateStatusManualDisabled, saved.Status)
	require.Equal(t, 4, saved.ConsecutiveFailures)
	require.Equal(t, int64(123), saved.LastFailureAt)
	require.Equal(t, int64(456), saved.ProbeAfterAt)
	require.Equal(t, int64(789), saved.DisabledAt)
	require.Equal(t, "manual", saved.DisableReason)
}

func TestSaveModelChannelPoliciesRefreshesMemoryCacheAndRemovesStaleStates(t *testing.T) {
	db := setupModelChannelCircuitTestDB(t)

	createModelCircuitTestChannel(t, db, 71, "default", "gpt-save", common.ChannelStatusEnabled, 10, 1)
	createModelCircuitTestChannel(t, db, 72, "default", "gpt-save", common.ChannelStatusEnabled, 20, 1)

	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-save",
		ChannelId:     71,
		Priority:      100,
		ManualEnabled: true,
	}).Insert())
	require.NoError(t, model.UpsertModelChannelState(&model.ModelChannelState{
		Model:     "gpt-save",
		ChannelId: 71,
		Status:    model.ModelChannelStateStatusManualDisabled,
	}))

	model.InitChannelCache()

	require.NoError(t, SaveModelChannelPolicies("gpt-save", []dto.ModelChannelPolicyItem{
		{
			ChannelId:     72,
			Priority:      300,
			ManualEnabled: false,
		},
	}))

	policies, err := model.CacheGetModelChannelPoliciesByModel("gpt-save")
	require.NoError(t, err)
	require.Len(t, policies, 1)
	require.Equal(t, 72, policies[0].ChannelId)
	require.False(t, policies[0].ManualEnabled)

	state, err := model.GetModelChannelState("gpt-save", 71)
	require.NoError(t, err)
	require.Nil(t, state)
}

func TestGetModelChannelCircuitDetailKeepsMissingChannelPolicyRows(t *testing.T) {
	db := setupModelChannelCircuitTestDB(t)

	createModelCircuitTestChannel(t, db, 81, "default", "gpt-detail", common.ChannelStatusEnabled, 10, 1)

	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-detail",
		ChannelId:     80,
		Priority:      200,
		ManualEnabled: true,
	}).Insert())
	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-detail",
		ChannelId:     81,
		Priority:      100,
		ManualEnabled: true,
	}).Insert())
	require.NoError(t, model.UpsertModelChannelState(&model.ModelChannelState{
		Model:               "gpt-detail",
		ChannelId:           80,
		Status:              model.ModelChannelStateStatusAutoDisabled,
		ConsecutiveFailures: 3,
		LastFailureAt:       123,
		DisableReason:       "model_not_found",
	}))

	detail, err := GetModelChannelCircuitDetail("gpt-detail")
	require.NoError(t, err)
	require.Len(t, detail.Channels, 2)
	require.Equal(t, 80, detail.Channels[0].ChannelId)
	require.True(t, detail.Channels[0].ChannelMissing)
	require.Equal(t, int64(123), detail.Channels[0].LastFailureAt)
	require.Equal(t, model.ModelChannelStateStatusAutoDisabled, detail.Channels[0].Status)
}

func TestGetModelChannelCircuitModelsCountsPolicyManualDisable(t *testing.T) {
	db := setupModelChannelCircuitTestDB(t)

	createModelCircuitTestChannel(t, db, 91, "default", "gpt-summary", common.ChannelStatusEnabled, 10, 1)
	createModelCircuitTestChannel(t, db, 92, "default", "gpt-summary", common.ChannelStatusEnabled, 20, 1)

	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-summary",
		ChannelId:     91,
		Priority:      100,
		ManualEnabled: false,
	}).Insert())
	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-summary",
		ChannelId:     92,
		Priority:      90,
		ManualEnabled: true,
	}).Insert())
	require.NoError(t, model.UpsertModelChannelState(&model.ModelChannelState{
		Model:     "gpt-summary",
		ChannelId: 92,
		Status:    model.ModelChannelStateStatusManualDisabled,
	}))
	require.NoError(t, model.UpsertModelChannelState(&model.ModelChannelState{
		Model:     "gpt-summary",
		ChannelId: 999,
		Status:    model.ModelChannelStateStatusAutoDisabled,
	}))

	items, err := GetModelChannelCircuitModels()
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, 0, items[0].AutoDisabledCount)
	require.Equal(t, 2, items[0].ManualDisabled)
}

func TestEnableModelChannelCircuitPairRejectsUnknownPolicy(t *testing.T) {
	setupModelChannelCircuitTestDB(t)

	err := EnableModelChannelCircuitPair("gpt-unknown", 999)
	require.Error(t, err)
	require.Contains(t, err.Error(), "policy")
}

func TestEnableModelChannelCircuitPairRejectsMissingChannel(t *testing.T) {
	setupModelChannelCircuitTestDB(t)

	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-missing-channel",
		ChannelId:     1001,
		Priority:      100,
		ManualEnabled: true,
	}).Insert())

	err := EnableModelChannelCircuitPair("gpt-missing-channel", 1001)
	require.Error(t, err)
	require.Contains(t, err.Error(), "channel")
}

func TestProbeModelChannelCircuitPairRejectsUnknownPolicy(t *testing.T) {
	setupModelChannelCircuitTestDB(t)

	result := ProbeModelChannelCircuitPair("gpt-unknown", 999)
	require.False(t, result.Success)
	require.Contains(t, result.Message, "policy")
}

func TestProbeModelChannelCircuitPairRejectsMissingChannel(t *testing.T) {
	setupModelChannelCircuitTestDB(t)

	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt-missing-channel",
		ChannelId:     1002,
		Priority:      100,
		ManualEnabled: true,
	}).Insert())

	result := ProbeModelChannelCircuitPair("gpt-missing-channel", 1002)
	require.False(t, result.Success)
	require.Contains(t, result.Message, "channel")
}

func TestSetupModelChannelCircuitTestDBRestoresChannelCacheGlobals(t *testing.T) {
	sentinel := model.ChannelCacheStateForTest{
		Group2Model2Channels: map[string]map[string][]int{
			"sentinel-group": {
				"sentinel-model": {99},
			},
		},
		ChannelsByID: map[int]*model.Channel{
			99: {
				Id:     99,
				Name:   "sentinel-channel",
				Status: common.ChannelStatusEnabled,
			},
		},
		ModelChannelPolicies: map[string][]model.ModelChannelPolicy{
			"sentinel-model": {
				{
					Model:         "sentinel-model",
					ChannelId:     99,
					Priority:      7,
					ManualEnabled: false,
				},
			},
		},
		ModelChannelStateMap: map[string]map[int]*model.ModelChannelState{
			"sentinel-model": {
				99: {
					Model:     "sentinel-model",
					ChannelId: 99,
					Status:    model.ModelChannelStateStatusManualDisabled,
				},
			},
		},
	}
	model.RestoreChannelCacheStateForTest(sentinel)

	t.Run("isolated cache mutation", func(t *testing.T) {
		db := setupModelChannelCircuitTestDB(t)
		createModelCircuitTestChannel(t, db, 11, "default", "gpt-4.1", common.ChannelStatusEnabled, 10, 1)
		model.InitChannelCache()

		current := model.SnapshotChannelCacheStateForTest()
		require.NotEqual(t, sentinel, current)
	})

	require.Equal(t, sentinel, model.SnapshotChannelCacheStateForTest())
}
