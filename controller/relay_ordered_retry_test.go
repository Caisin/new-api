package controller

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type errReadCloser struct {
	err error
}

func (r *errReadCloser) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r *errReadCloser) Close() error {
	return nil
}

func TestShouldRetryRelayAttemptAllowsOrderedModelScoped400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	retryParam := &service.RetryParam{
		Ctx: ctx,
		OrderedCandidates: &service.OrderedModelChannelCandidates{
			Model:              "gpt-4.1",
			PolicyLookupModel:  "gpt-4.1",
			UseLegacySelection: false,
		},
	}
	unsupportedModelErr := types.NewOpenAIError(errors.New("model gpt-4.1 is not supported by this provider"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)

	require.True(t, shouldRetryRelayAttempt(ctx, retryParam, unsupportedModelErr, 1))
	require.False(t, shouldRetryRelayAttempt(ctx, retryParam, unsupportedModelErr, 0))
	require.False(t, shouldRetryRelayAttempt(ctx, &service.RetryParam{Ctx: ctx}, unsupportedModelErr, 1))
}

func TestShouldRetryRelayAttemptRespectsChannelAffinitySkipRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("channel_affinity_skip_retry_on_failure", true)

	retryParam := &service.RetryParam{
		Ctx: ctx,
		OrderedCandidates: &service.OrderedModelChannelCandidates{
			Model:              "gpt-4.1",
			PolicyLookupModel:  "gpt-4.1",
			UseLegacySelection: false,
		},
	}
	rateLimitedErr := types.NewOpenAIError(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)

	require.False(t, shouldRetryRelayAttempt(ctx, retryParam, rateLimitedErr, 1))
}

func TestShouldUseOrderedModelChannelRoutingIgnoresUninitializedChannelMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	relayInfo := &relaycommon.RelayInfo{
		TokenGroup:      "default",
		OriginModelName: "gpt-4.1",
	}

	require.True(t, shouldUseOrderedModelChannelRouting(ctx, relayInfo))

	ctx.Set("specific_channel_id", 123)
	require.False(t, shouldUseOrderedModelChannelRouting(ctx, relayInfo))
}

func TestGetChannelUsesOrderedCandidatesBeforeChannelMetaInit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("channel_id", 99)
	ctx.Set("channel_type", 1)
	ctx.Set("channel_name", "legacy-selected")

	retryParam := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-4.1",
		Retry:      serviceRetryPointer(0),
		OrderedCandidates: &service.OrderedModelChannelCandidates{
			Model:             "gpt-4.1",
			PolicyLookupModel: "gpt-4.1",
			Candidates: []service.CandidateChannel{
				{
					Channel: &model.Channel{Id: 12, Name: "ordered-channel", Type: 2},
				},
			},
		},
	}
	relayInfo := &relaycommon.RelayInfo{
		TokenGroup:      "default",
		OriginModelName: "gpt-4.1",
	}

	channel, err := getChannel(ctx, relayInfo, retryParam)
	require.Nil(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 12, channel.Id)
	require.Equal(t, "ordered-channel", channel.Name)
	require.Equal(t, 12, ctx.GetInt("channel_id"))
	require.Equal(t, "ordered-channel", ctx.GetString("channel_name"))
	require.Equal(t, "gpt-4.1", ctx.GetString("original_model"))
}

func TestPrepareRelayAttemptOnlyRecordsActualAttempts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("body read failure does not record retry path", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		ctx.Request.Body = &errReadCloser{err: errors.New("boom")}

		err := prepareRelayAttempt(ctx, 11)
		require.NotNil(t, err)

		pathIDs, pathText := service.GetRetryPath(ctx)
		require.Empty(t, pathIDs)
		require.Empty(t, pathText)
	})

	t.Run("body ready records retry path", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", io.NopCloser(strings.NewReader(`{"model":"gpt-4.1"}`)))

		err := prepareRelayAttempt(ctx, 12)
		require.Nil(t, err)

		pathIDs, pathText := service.GetRetryPath(ctx)
		require.Equal(t, []int{12}, pathIDs)
		require.Equal(t, "12", pathText)
	})
}

func serviceRetryPointer(v int) *int {
	return &v
}
