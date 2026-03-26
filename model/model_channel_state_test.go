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
