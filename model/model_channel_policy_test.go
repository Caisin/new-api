package model

import (
	"reflect"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type channelCacheTestState struct {
	db                    *gorm.DB
	memoryCacheEnabled    bool
	group2model2channels  map[string]map[string][]int
	channelsIDM           map[int]*Channel
	model2channelPolicies map[string][]ModelChannelPolicy
	model2channelStateMap map[string]map[int]*ModelChannelState
}

func setupChannelCacheBackedTest(t *testing.T, db *gorm.DB) {
	t.Helper()

	channelSyncLock.RLock()
	state := channelCacheTestState{
		db:                    DB,
		memoryCacheEnabled:    common.MemoryCacheEnabled,
		group2model2channels:  group2model2channels,
		channelsIDM:           channelsIDM,
		model2channelPolicies: model2channelPolicies,
		model2channelStateMap: model2channelStateMap,
	}
	channelSyncLock.RUnlock()

	DB = db
	common.MemoryCacheEnabled = true

	t.Cleanup(func() {
		DB = state.db
		common.MemoryCacheEnabled = state.memoryCacheEnabled

		channelSyncLock.Lock()
		group2model2channels = state.group2model2channels
		channelsIDM = state.channelsIDM
		model2channelPolicies = state.model2channelPolicies
		model2channelStateMap = state.model2channelStateMap
		channelSyncLock.Unlock()
	})
}

func TestModelChannelPolicySchemaMatchesTask1Spec(t *testing.T) {
	typ := reflect.TypeOf(ModelChannelPolicy{})

	for _, name := range []string{"Model", "ChannelId", "Priority", "ManualEnabled"} {
		_, ok := typ.FieldByName(name)
		require.True(t, ok, "expected field %s", name)
	}

	for _, name := range []string{"Group", "FailureThreshold", "ProbeIntervalSeconds"} {
		_, ok := typ.FieldByName(name)
		require.False(t, ok, "field %s should not exist", name)
	}
}

func TestModelChannelPolicyHasUniqueKeyOnModelAndChannelId(t *testing.T) {
	typ := reflect.TypeOf(ModelChannelPolicy{})
	modelField, ok := typ.FieldByName("Model")
	require.True(t, ok)
	channelField, ok := typ.FieldByName("ChannelId")
	require.True(t, ok)

	modelTag := modelField.Tag.Get("gorm")
	channelTag := channelField.Tag.Get("gorm")

	require.True(t, strings.Contains(modelTag, "uniqueIndex:idx_model_channel_policy_model_channel"))
	require.True(t, strings.Contains(channelTag, "uniqueIndex:idx_model_channel_policy_model_channel"))

	require.True(t, strings.Contains(modelTag, "priority:1"))
	require.True(t, strings.Contains(channelTag, "priority:2"))
}

func TestModelChannelPolicyUniqueModelChannelConstraint(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ModelChannelPolicy{}))

	first := ModelChannelPolicy{
		Model:         "gpt-4.1",
		ChannelId:     99,
		Priority:      10,
		ManualEnabled: true,
	}
	require.NoError(t, db.Create(&first).Error)

	dup := ModelChannelPolicy{
		Model:         "gpt-4.1",
		ChannelId:     99,
		Priority:      20,
		ManualEnabled: false,
	}
	require.Error(t, db.Create(&dup).Error)
}

func TestGetModelChannelPoliciesByModelQueriesDatabase(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ModelChannelPolicy{}))

	oldDB := DB
	DB = db
	t.Cleanup(func() {
		DB = oldDB
	})

	require.NoError(t, db.Create(&ModelChannelPolicy{
		Model:         "gpt-4.1",
		ChannelId:     101,
		Priority:      5,
		ManualEnabled: true,
	}).Error)
	require.NoError(t, db.Create(&ModelChannelPolicy{
		Model:         "gpt-4.1",
		ChannelId:     102,
		Priority:      20,
		ManualEnabled: false,
	}).Error)
	require.NoError(t, db.Create(&ModelChannelPolicy{
		Model:         "gpt-4o",
		ChannelId:     103,
		Priority:      99,
		ManualEnabled: true,
	}).Error)

	policies, err := GetModelChannelPoliciesByModel("gpt-4.1")
	require.NoError(t, err)
	require.Len(t, policies, 2)
	require.Equal(t, int64(20), policies[0].Priority)
	require.Equal(t, 102, policies[0].ChannelId)
	require.Equal(t, int64(5), policies[1].Priority)
	require.Equal(t, 101, policies[1].ChannelId)
}

func TestCacheGetModelChannelPoliciesByModelRefreshesViaInitChannelCache(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ModelChannelPolicy{}, &ModelChannelState{}))

	setupChannelCacheBackedTest(t, db)

	require.NoError(t, db.Create(&ModelChannelPolicy{
		Model:         "gpt-cache",
		ChannelId:     201,
		Priority:      3,
		ManualEnabled: true,
	}).Error)

	InitChannelCache()

	policies, err := CacheGetModelChannelPoliciesByModel("gpt-cache")
	require.NoError(t, err)
	require.Len(t, policies, 1)
	require.Equal(t, 201, policies[0].ChannelId)
	require.Equal(t, int64(3), policies[0].Priority)

	require.NoError(t, db.Create(&ModelChannelPolicy{
		Model:         "gpt-cache",
		ChannelId:     202,
		Priority:      9,
		ManualEnabled: false,
	}).Error)

	InitChannelCache()
	policies, err = CacheGetModelChannelPoliciesByModel("gpt-cache")
	require.NoError(t, err)
	require.Len(t, policies, 2)
	require.Equal(t, 202, policies[0].ChannelId)
	require.Equal(t, int64(9), policies[0].Priority)
	require.Equal(t, 201, policies[1].ChannelId)
	require.Equal(t, int64(3), policies[1].Priority)
}

func TestCacheGetModelChannelPoliciesByModelReturnsDefensiveCopy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ModelChannelPolicy{}, &ModelChannelState{}))

	setupChannelCacheBackedTest(t, db)

	require.NoError(t, db.Create(&ModelChannelPolicy{
		Model:         "gpt-cache",
		ChannelId:     201,
		Priority:      3,
		ManualEnabled: true,
	}).Error)
	require.NoError(t, db.Create(&ModelChannelPolicy{
		Model:         "gpt-cache",
		ChannelId:     202,
		Priority:      9,
		ManualEnabled: false,
	}).Error)

	InitChannelCache()

	policies, err := CacheGetModelChannelPoliciesByModel("gpt-cache")
	require.NoError(t, err)
	require.Len(t, policies, 2)

	policies[0].ChannelId = 999
	policies[0].Priority = -1
	policies[1] = ModelChannelPolicy{
		Model:         "gpt-cache",
		ChannelId:     998,
		Priority:      1,
		ManualEnabled: true,
	}
	policies = append(policies, ModelChannelPolicy{
		Model:         "gpt-cache",
		ChannelId:     997,
		Priority:      0,
		ManualEnabled: false,
	})

	cachedPolicies, err := CacheGetModelChannelPoliciesByModel("gpt-cache")
	require.NoError(t, err)
	require.Len(t, cachedPolicies, 2)

	policiesByChannelID := make(map[int]ModelChannelPolicy, len(cachedPolicies))
	for _, policy := range cachedPolicies {
		policiesByChannelID[policy.ChannelId] = policy
	}

	require.Equal(t, int64(9), policiesByChannelID[202].Priority)
	require.Equal(t, int64(3), policiesByChannelID[201].Priority)
}
