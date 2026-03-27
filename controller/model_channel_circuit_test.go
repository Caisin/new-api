package controller

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelChannelCircuitControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ModelChannelPolicy{}, &model.ModelChannelState{}))

	model.DB = db
	model.LOG_DB = db
	common.MemoryCacheEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func TestGetModelChannelCircuitDetailReturnsPolicyAndState(t *testing.T) {
	db := setupModelChannelCircuitControllerTestDB(t)

	channel := &model.Channel{
		Id:      101,
		Name:    "channel-101",
		Key:     "test-key",
		Status:  common.ChannelStatusEnabled,
		Models:  "gpt5.4",
		Group:   "default",
		AutoBan: common.GetPointer(1),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt5.4",
		ChannelId:     101,
		Priority:      300,
		ManualEnabled: true,
	}).Insert())
	require.NoError(t, model.UpsertModelChannelState(&model.ModelChannelState{
		Model:               "gpt5.4",
		ChannelId:           101,
		Status:              model.ModelChannelStateStatusAutoDisabled,
		ConsecutiveFailures: 3,
		DisableReason:       "model_not_supported",
	}))

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/model_channel_circuit/models/gpt5.4", nil, 1)
	ctx.Params = gin.Params{{Key: "model", Value: "gpt5.4"}}

	GetModelChannelCircuitDetail(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success)
	require.Contains(t, recorder.Body.String(), "\"model\":\"gpt5.4\"")
	require.Contains(t, recorder.Body.String(), "\"channel_id\":101")
	require.Contains(t, recorder.Body.String(), "\"status\":\"auto_disabled\"")
}

func TestGetModelChannelCircuitDetailReturnsMissingChannelRows(t *testing.T) {
	db := setupModelChannelCircuitControllerTestDB(t)

	channel := &model.Channel{
		Id:      111,
		Name:    "channel-111",
		Key:     "test-key",
		Status:  common.ChannelStatusEnabled,
		Models:  "gpt5.4",
		Group:   "default",
		AutoBan: common.GetPointer(1),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt5.4",
		ChannelId:     110,
		Priority:      400,
		ManualEnabled: true,
	}).Insert())
	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt5.4",
		ChannelId:     111,
		Priority:      300,
		ManualEnabled: true,
	}).Insert())

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/model_channel_circuit/models/gpt5.4", nil, 1)
	ctx.Params = gin.Params{{Key: "model", Value: "gpt5.4"}}

	GetModelChannelCircuitDetail(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success)
	require.Contains(t, recorder.Body.String(), "\"channel_missing\":true")
}

func TestGetModelChannelCircuitModelsIncludesAbilityOnlyModelForBootstrap(t *testing.T) {
	db := setupModelChannelCircuitControllerTestDB(t)

	channel := &model.Channel{
		Id:       121,
		Name:     "channel-121",
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		Models:   "gpt-bootstrap-controller",
		Group:    "default",
		Priority: common.GetPointer(int64(30)),
		AutoBan:  common.GetPointer(1),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/model_channel_circuit/models", nil, 1)

	GetModelChannelCircuitModels(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success)
	require.Contains(t, recorder.Body.String(), "\"model\":\"gpt-bootstrap-controller\"")
	require.Contains(t, recorder.Body.String(), "\"bootstrap_needed\":true")
}

func TestGetModelChannelCircuitDetailReturnsBootstrapDraftFromAbilities(t *testing.T) {
	db := setupModelChannelCircuitControllerTestDB(t)

	channelA := &model.Channel{
		Id:       131,
		Name:     "channel-131",
		Key:      "test-key-a",
		Status:   common.ChannelStatusEnabled,
		Models:   "gpt-bootstrap-controller-detail",
		Group:    "default",
		Priority: common.GetPointer(int64(10)),
		AutoBan:  common.GetPointer(1),
	}
	channelB := &model.Channel{
		Id:       132,
		Name:     "channel-132",
		Key:      "test-key-b",
		Status:   common.ChannelStatusEnabled,
		Models:   "gpt-bootstrap-controller-detail",
		Group:    "vip",
		Priority: common.GetPointer(int64(30)),
		AutoBan:  common.GetPointer(1),
	}
	require.NoError(t, db.Create(channelA).Error)
	require.NoError(t, db.Create(channelB).Error)
	require.NoError(t, channelA.AddAbilities(nil))
	require.NoError(t, channelB.AddAbilities(nil))

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/model_channel_circuit/models/gpt-bootstrap-controller-detail", nil, 1)
	ctx.Params = gin.Params{{Key: "model", Value: "gpt-bootstrap-controller-detail"}}

	GetModelChannelCircuitDetail(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success)
	require.Contains(t, recorder.Body.String(), "\"bootstrap_needed\":true")
	require.Contains(t, recorder.Body.String(), "\"channel_id\":132")
	require.Contains(t, recorder.Body.String(), "\"priority\":30")
}

func TestEnableModelChannelCircuitBootstrapsPoliciesFromAbilities(t *testing.T) {
	db := setupModelChannelCircuitControllerTestDB(t)

	channel := &model.Channel{
		Id:       141,
		Name:     "channel-141",
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		Models:   "gpt-bootstrap-enable-controller",
		Group:    "default",
		Priority: common.GetPointer(int64(30)),
		AutoBan:  common.GetPointer(1),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/model_channel_circuit/models/gpt-bootstrap-enable-controller/channel/141/enable", nil, 1)
	ctx.Params = gin.Params{
		{Key: "model", Value: "gpt-bootstrap-enable-controller"},
		{Key: "channel_id", Value: "141"},
	}

	EnableModelChannelCircuit(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success)

	policy, err := model.GetModelChannelPolicy("gpt-bootstrap-enable-controller", 141)
	require.NoError(t, err)
	require.NotNil(t, policy)
}

func TestUpdateModelChannelPoliciesReordersAndCreatesMissingRows(t *testing.T) {
	setupModelChannelCircuitControllerTestDB(t)

	body := map[string]any{
		"channels": []map[string]any{
			{
				"channel_id":     101,
				"priority":       300,
				"manual_enabled": true,
			},
			{
				"channel_id":     102,
				"priority":       200,
				"manual_enabled": false,
			},
		},
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/model_channel_circuit/models/gpt5.4/policies", body, 1)
	ctx.Params = gin.Params{{Key: "model", Value: "gpt5.4"}}

	UpdateModelChannelPolicies(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success)

	policies, err := model.GetModelChannelPoliciesByModel("gpt5.4")
	require.NoError(t, err)
	require.Len(t, policies, 2)
	require.Equal(t, int64(300), policies[0].Priority)
	require.Equal(t, 101, policies[0].ChannelId)
	require.Equal(t, int64(200), policies[1].Priority)
	require.Equal(t, 102, policies[1].ChannelId)
	require.False(t, policies[1].ManualEnabled)
}

func TestEnableModelChannelCircuitRevertsManualDisabledState(t *testing.T) {
	db := setupModelChannelCircuitControllerTestDB(t)

	channel := &model.Channel{
		Id:      201,
		Name:    "channel-201",
		Key:     "test-key",
		Status:  common.ChannelStatusEnabled,
		Models:  "gpt5.4",
		Group:   "default",
		AutoBan: common.GetPointer(1),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt5.4",
		ChannelId:     201,
		Priority:      200,
		ManualEnabled: true,
	}).Insert())
	require.NoError(t, model.UpsertModelChannelState(&model.ModelChannelState{
		Model:         "gpt5.4",
		ChannelId:     201,
		Status:        model.ModelChannelStateStatusManualDisabled,
		DisabledAt:    123,
		DisableReason: "manual_disabled",
	}))

	disableCtx, disableRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/model_channel_circuit/models/gpt5.4/channel/201/enable", nil, 1)
	disableCtx.Params = gin.Params{
		{Key: "model", Value: "gpt5.4"},
		{Key: "channel_id", Value: "201"},
	}
	EnableModelChannelCircuit(disableCtx)

	response := decodeAPIResponse(t, disableRecorder)
	require.True(t, response.Success)

	state, err := model.GetModelChannelState("gpt5.4", 201)
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, model.ModelChannelStateStatusEnabled, state.Status)
	require.Zero(t, state.DisabledAt)
	require.Zero(t, state.ProbeAfterAt)
	require.Empty(t, state.DisableReason)
}

func TestUpdateModelChannelPoliciesRejectsEmptyChannels(t *testing.T) {
	db := setupModelChannelCircuitControllerTestDB(t)

	require.NoError(t, (&model.ModelChannelPolicy{
		Model:         "gpt5.4",
		ChannelId:     301,
		Priority:      100,
		ManualEnabled: true,
	}).Insert())

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/model_channel_circuit/models/gpt5.4/policies", map[string]any{}, 1)
	ctx.Params = gin.Params{{Key: "model", Value: "gpt5.4"}}

	UpdateModelChannelPolicies(ctx)

	response := decodeAPIResponse(t, recorder)
	require.False(t, response.Success)

	policies, err := model.GetModelChannelPoliciesByModel("gpt5.4")
	require.NoError(t, err)
	require.Len(t, policies, 1)
	require.Equal(t, 301, policies[0].ChannelId)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NotNil(t, sqlDB)
}

func TestProbeModelChannelCircuitRejectsUnconfiguredPair(t *testing.T) {
	setupModelChannelCircuitControllerTestDB(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/model_channel_circuit/models/gpt5.4/channel/999/probe", nil, 1)
	ctx.Params = gin.Params{
		{Key: "model", Value: "gpt5.4"},
		{Key: "channel_id", Value: "999"},
	}

	ProbeModelChannelCircuit(ctx)

	require.Contains(t, recorder.Body.String(), "\"success\":false")
	require.Contains(t, recorder.Body.String(), "policy")
}
