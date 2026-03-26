package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
)

type ModelChannelProbeResult struct {
	Model     string
	ChannelId int
	Success   bool
	Message   string
}

type ModelChannelProbeAttemptResult struct {
	UsingKey string
	Err      *types.NewAPIError
}

type ModelChannelProbeExecutor func(ctx context.Context, modelName string, channel *model.Channel) ModelChannelProbeAttemptResult

var (
	modelChannelProbeExecutor     ModelChannelProbeExecutor
	modelChannelProbeExecutorLock sync.RWMutex
	modelChannelProbeWorkerOnce   sync.Once
)

func RegisterModelChannelProbeExecutor(executor ModelChannelProbeExecutor) {
	modelChannelProbeExecutorLock.Lock()
	defer modelChannelProbeExecutorLock.Unlock()
	modelChannelProbeExecutor = executor
}

func getModelChannelProbeExecutor() ModelChannelProbeExecutor {
	modelChannelProbeExecutorLock.RLock()
	defer modelChannelProbeExecutorLock.RUnlock()
	return modelChannelProbeExecutor
}

func ProbeModelChannelPair(ctx context.Context, modelName string, channelID int) ModelChannelProbeResult {
	result := ModelChannelProbeResult{
		Model:     modelName,
		ChannelId: channelID,
	}
	if modelName == "" || channelID == 0 {
		result.Message = "model or channel_id is empty"
		return result
	}

	channel, err := model.CacheGetChannel(channelID)
	if err != nil || channel == nil {
		channel, err = model.GetChannelById(channelID, true)
		if err != nil {
			result.Message = err.Error()
			_ = ScheduleNextModelChannelProbe(modelName, channelID, result.Message)
			return result
		}
	}
	if channel == nil {
		result.Message = "channel not found"
		_ = ScheduleNextModelChannelProbe(modelName, channelID, result.Message)
		return result
	}

	executor := getModelChannelProbeExecutor()
	if executor == nil {
		result.Message = "model channel probe executor is not configured"
		_ = ScheduleNextModelChannelProbe(modelName, channelID, result.Message)
		return result
	}

	attempt := executor(ctx, modelName, channel)
	if attempt.Err == nil {
		if err := ResetModelChannelFailure(modelName, channelID); err != nil {
			result.Message = err.Error()
			return result
		}
		if channel.Status == common.ChannelStatusAutoDisabled {
			EnableChannel(channel.Id, attempt.UsingKey, channel.Name)
		}
		result.Success = true
		return result
	}

	result.Message = attempt.Err.Error()
	_ = ScheduleNextModelChannelProbe(modelName, channelID, result.Message)
	if ShouldDisableChannel(channel.Type, attempt.Err) && channel.GetAutoBan() {
		DisableChannel(*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, attempt.UsingKey, channel.GetAutoBan()), attempt.Err.ErrorWithStatusCode())
	}
	return result
}

func ProbeDueAutoDisabledModelChannels(ctx context.Context, now int64, limit int) ([]ModelChannelProbeResult, error) {
	states, err := model.ListDueAutoDisabledModelChannelStates(now, limit)
	if err != nil {
		return nil, err
	}
	results := make([]ModelChannelProbeResult, 0, len(states))
	for _, state := range states {
		results = append(results, ProbeModelChannelPair(ctx, state.Model, state.ChannelId))
	}
	return results, nil
}

func AutomaticallyProbeModelChannels() {
	if !common.IsMasterNode {
		return
	}
	modelChannelProbeWorkerOnce.Do(func() {
		for {
			interval := GetModelChannelProbeSleepInterval()
			time.Sleep(interval)
			results, err := ProbeDueAutoDisabledModelChannels(context.Background(), common.GetTimestamp(), 100)
			if err != nil {
				common.SysError(fmt.Sprintf("probe model channels failed: %v", err))
				continue
			}
			if len(results) > 0 {
				common.SysLog(fmt.Sprintf("probed %d auto-disabled model channels", len(results)))
			}
		}
	})
}

func GetModelChannelProbeSleepInterval() time.Duration {
	minutes := operation_setting.GetModelChannelCircuitProbeIntervalMinutes()
	if minutes <= 0 {
		minutes = operation_setting.DefaultModelChannelProbeIntervalMinutes
	}
	return time.Duration(minutes) * time.Minute
}
