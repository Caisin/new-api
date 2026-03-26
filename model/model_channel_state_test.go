package model

import (
	"reflect"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelChannelStateSchemaMatchesTask1Spec(t *testing.T) {
	typ := reflect.TypeOf(ModelChannelState{})

	for _, name := range []string{
		"Model",
		"ChannelId",
		"Status",
		"ConsecutiveFailures",
		"LastFailureAt",
		"ProbeAfterAt",
		"DisabledAt",
		"DisableReason",
	} {
		_, ok := typ.FieldByName(name)
		require.True(t, ok, "expected field %s", name)
	}

	_, hasGroup := typ.FieldByName("Group")
	require.False(t, hasGroup, "field Group should not exist")
}

func TestModelChannelStateHasUniqueKeyOnModelAndChannelId(t *testing.T) {
	typ := reflect.TypeOf(ModelChannelState{})
	modelField, ok := typ.FieldByName("Model")
	require.True(t, ok)
	channelField, ok := typ.FieldByName("ChannelId")
	require.True(t, ok)

	modelTag := modelField.Tag.Get("gorm")
	channelTag := channelField.Tag.Get("gorm")

	require.True(t, strings.Contains(modelTag, "uniqueIndex:idx_model_channel_state_model_channel"))
	require.True(t, strings.Contains(channelTag, "uniqueIndex:idx_model_channel_state_model_channel"))
	require.True(t, strings.Contains(modelTag, "priority:1"))
	require.True(t, strings.Contains(channelTag, "priority:2"))
}

func TestIsValidModelChannelStateStatus(t *testing.T) {
	require.True(t, IsValidModelChannelStateStatus(ModelChannelStateStatusEnabled))
	require.True(t, IsValidModelChannelStateStatus(ModelChannelStateStatusAutoDisabled))
	require.True(t, IsValidModelChannelStateStatus(ModelChannelStateStatusManualDisabled))
	require.False(t, IsValidModelChannelStateStatus(""))
	require.False(t, IsValidModelChannelStateStatus("disabled"))
}

func TestModelChannelStateIsDisabled(t *testing.T) {
	require.False(t, (&ModelChannelState{Status: ModelChannelStateStatusEnabled}).IsDisabled())
	require.True(t, (&ModelChannelState{Status: ModelChannelStateStatusAutoDisabled}).IsDisabled())
	require.True(t, (&ModelChannelState{Status: ModelChannelStateStatusManualDisabled}).IsDisabled())
}

func TestModelChannelStateUniqueModelChannelConstraint(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ModelChannelState{}))

	first := ModelChannelState{
		Model:     "gpt-4.1",
		ChannelId: 100,
		Status:    ModelChannelStateStatusEnabled,
	}
	require.NoError(t, db.Create(&first).Error)

	dup := ModelChannelState{
		Model:     "gpt-4.1",
		ChannelId: 100,
		Status:    ModelChannelStateStatusEnabled,
	}
	require.Error(t, db.Create(&dup).Error)
}

func TestGetModelChannelStateMapByModelQueriesDatabase(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ModelChannelState{}))

	oldDB := DB
	DB = db
	t.Cleanup(func() {
		DB = oldDB
	})

	require.NoError(t, db.Create(&ModelChannelState{
		Model:     "gpt-4.1",
		ChannelId: 301,
		Status:    ModelChannelStateStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&ModelChannelState{
		Model:     "gpt-4.1",
		ChannelId: 302,
		Status:    ModelChannelStateStatusAutoDisabled,
	}).Error)
	require.NoError(t, db.Create(&ModelChannelState{
		Model:     "gpt-4o",
		ChannelId: 303,
		Status:    ModelChannelStateStatusManualDisabled,
	}).Error)

	stateMap, err := GetModelChannelStateMapByModel("gpt-4.1")
	require.NoError(t, err)
	require.Len(t, stateMap, 2)
	require.Equal(t, ModelChannelStateStatusEnabled, stateMap[301].Status)
	require.Equal(t, ModelChannelStateStatusAutoDisabled, stateMap[302].Status)
	_, exists := stateMap[303]
	require.False(t, exists)
}

func TestCacheGetModelChannelStateMapByModelRefreshesViaInitChannelCache(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ModelChannelPolicy{}, &ModelChannelState{}))

	setupChannelCacheBackedTest(t, db)

	require.NoError(t, db.Create(&ModelChannelState{
		Model:     "gpt-cache",
		ChannelId: 401,
		Status:    ModelChannelStateStatusEnabled,
	}).Error)

	InitChannelCache()

	stateMap, err := CacheGetModelChannelStateMapByModel("gpt-cache")
	require.NoError(t, err)
	require.Len(t, stateMap, 1)
	require.Equal(t, ModelChannelStateStatusEnabled, stateMap[401].Status)

	require.NoError(t, db.Create(&ModelChannelState{
		Model:     "gpt-cache",
		ChannelId: 402,
		Status:    ModelChannelStateStatusManualDisabled,
	}).Error)

	InitChannelCache()
	stateMap, err = CacheGetModelChannelStateMapByModel("gpt-cache")
	require.NoError(t, err)
	require.Len(t, stateMap, 2)
	require.Equal(t, ModelChannelStateStatusEnabled, stateMap[401].Status)
	require.Equal(t, ModelChannelStateStatusManualDisabled, stateMap[402].Status)
}

func TestCacheGetModelChannelStateMapByModelReturnsDefensiveCopy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ModelChannelPolicy{}, &ModelChannelState{}))

	setupChannelCacheBackedTest(t, db)

	require.NoError(t, db.Create(&ModelChannelState{
		Model:     "gpt-cache",
		ChannelId: 401,
		Status:    ModelChannelStateStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&ModelChannelState{
		Model:     "gpt-cache",
		ChannelId: 402,
		Status:    ModelChannelStateStatusManualDisabled,
	}).Error)

	InitChannelCache()

	stateMap, err := CacheGetModelChannelStateMapByModel("gpt-cache")
	require.NoError(t, err)
	require.Len(t, stateMap, 2)

	stateMap[401].Status = ModelChannelStateStatusAutoDisabled
	delete(stateMap, 402)
	stateMap[999] = &ModelChannelState{
		Model:     "gpt-cache",
		ChannelId: 999,
		Status:    ModelChannelStateStatusManualDisabled,
	}

	cachedStateMap, err := CacheGetModelChannelStateMapByModel("gpt-cache")
	require.NoError(t, err)
	require.Len(t, cachedStateMap, 2)
	require.Equal(t, ModelChannelStateStatusEnabled, cachedStateMap[401].Status)
	require.Equal(t, ModelChannelStateStatusManualDisabled, cachedStateMap[402].Status)
	_, exists := cachedStateMap[999]
	require.False(t, exists)
}
