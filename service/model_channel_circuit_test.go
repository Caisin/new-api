package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelChannelCircuitTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ModelChannelPolicy{}, &model.ModelChannelState{}))

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
	require.Empty(t, result.Candidates)
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
