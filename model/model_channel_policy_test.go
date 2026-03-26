package model

import (
	"reflect"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

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
