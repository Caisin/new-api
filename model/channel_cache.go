package model

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var group2model2channels map[string]map[string][]int // enabled channel
var channelsIDM map[int]*Channel                     // all channels include disabled
var model2channelPolicies map[string][]ModelChannelPolicy
var model2channelStateMap map[string]map[int]*ModelChannelState
var channelSyncLock sync.RWMutex

// ChannelCacheStateForTest captures the in-memory channel cache state for tests.
type ChannelCacheStateForTest struct {
	Group2Model2Channels map[string]map[string][]int
	ChannelsByID         map[int]*Channel
	ModelChannelPolicies map[string][]ModelChannelPolicy
	ModelChannelStateMap map[string]map[int]*ModelChannelState
}

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
	}
	var abilities []*Ability
	DB.Find(&abilities)
	groups := make(map[string]bool)
	for _, ability := range abilities {
		groups[ability.Group] = true
	}
	newGroup2model2channels := make(map[string]map[string][]int)
	for group := range groups {
		newGroup2model2channels[group] = make(map[string][]int)
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		groups := strings.Split(channel.Group, ",")
		for _, group := range groups {
			models := strings.Split(channel.Models, ",")
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]int, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	var policies []ModelChannelPolicy
	DB.Order("priority DESC").Order("channel_id ASC").Find(&policies)
	newModel2channelPolicies := make(map[string][]ModelChannelPolicy)
	for _, policy := range policies {
		newModel2channelPolicies[policy.Model] = append(newModel2channelPolicies[policy.Model], policy)
	}

	var states []ModelChannelState
	DB.Find(&states)
	newModel2channelStateMap := make(map[string]map[int]*ModelChannelState)
	for i := range states {
		state := states[i]
		if _, ok := newModel2channelStateMap[state.Model]; !ok {
			newModel2channelStateMap[state.Model] = make(map[int]*ModelChannelState)
		}
		newModel2channelStateMap[state.Model][state.ChannelId] = &state
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	model2channelPolicies = newModel2channelPolicies
	model2channelStateMap = newModel2channelStateMap
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channelSyncLock.Unlock()
	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func GetRandomSatisfiedChannel(group string, model string, retry int) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannel(group, model, retry)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	// First, try to find channels with the exact model name.
	channels := group2model2channels[group][model]

	// If no channels found, try to find channels with the normalized model name.
	if len(channels) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channels = group2model2channels[group][normalizedModel]
	}

	if len(channels) == 0 {
		return nil, nil
	}

	if len(channels) == 1 {
		if channel, ok := channelsIDM[channels[0]]; ok {
			return channel, nil
		}
		return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channels[0])
	}

	uniquePriorities := make(map[int]bool)
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			uniquePriorities[int(channel.GetPriority())] = true
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}
	var sortedUniquePriorities []int
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(sortedUniquePriorities)))

	if retry >= len(uniquePriorities) {
		retry = len(uniquePriorities) - 1
	}
	targetPriority := int64(sortedUniquePriorities[retry])

	// get the priority for the given retry number
	var sumWeight = 0
	var targetChannels []*Channel
	for _, channelId := range channels {
		if channel, ok := channelsIDM[channelId]; ok {
			if channel.GetPriority() == targetPriority {
				sumWeight += channel.GetWeight()
				targetChannels = append(targetChannels, channel)
			}
		} else {
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
		}
	}

	if len(targetChannels) == 0 {
		return nil, errors.New(fmt.Sprintf("no channel found, group: %s, model: %s, priority: %d", group, model, targetPriority))
	}

	// smoothing factor and adjustment
	smoothingFactor := 1
	smoothingAdjustment := 0

	if sumWeight == 0 {
		// when all channels have weight 0, set sumWeight to the number of channels and set smoothing adjustment to 100
		// each channel's effective weight = 100
		sumWeight = len(targetChannels) * 100
		smoothingAdjustment = 100
	} else if sumWeight/len(targetChannels) < 10 {
		// when the average weight is less than 10, set smoothing factor to 100
		smoothingFactor = 100
	}

	// Calculate the total weight of all channels up to endIdx
	totalWeight := sumWeight * smoothingFactor

	// Generate a random value in the range [0, totalWeight)
	randomWeight := rand.Intn(totalWeight)

	// Find a channel based on its weight
	for _, channel := range targetChannels {
		randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
		if randomWeight < 0 {
			return channel, nil
		}
	}
	// return null if no channel is not found
	return nil, errors.New("channel not found")
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		// delete the channel from group2model2channels
		for group, model2channels := range group2model2channels {
			for model, channels := range model2channels {
				for i, channelId := range channels {
					if channelId == id {
						// remove the channel from the slice
						group2model2channels[group][model] = append(channels[:i], channels[i+1:]...)
						break
					}
				}
			}
		}
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel == nil {
		return
	}

	println("CacheUpdateChannel:", channel.Id, channel.Name, channel.Status, channel.ChannelInfo.MultiKeyPollingIndex)

	println("before:", channelsIDM[channel.Id].ChannelInfo.MultiKeyPollingIndex)
	channelsIDM[channel.Id] = channel
	println("after :", channelsIDM[channel.Id].ChannelInfo.MultiKeyPollingIndex)
}

func CacheGetModelChannelPoliciesByModel(modelName string) ([]ModelChannelPolicy, error) {
	if !common.MemoryCacheEnabled {
		return GetModelChannelPoliciesByModel(modelName)
	}
	policies := make([]ModelChannelPolicy, 0)
	if modelName == "" {
		return policies, nil
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	if model2channelPolicies == nil {
		return policies, nil
	}

	cachedPolicies, ok := model2channelPolicies[modelName]
	if !ok || len(cachedPolicies) == 0 {
		return policies, nil
	}

	policies = make([]ModelChannelPolicy, len(cachedPolicies))
	copy(policies, cachedPolicies)
	return policies, nil
}

func CacheGetModelChannelStateMapByModel(modelName string) (map[int]*ModelChannelState, error) {
	if !common.MemoryCacheEnabled {
		return GetModelChannelStateMapByModel(modelName)
	}
	stateMap := make(map[int]*ModelChannelState)
	if modelName == "" {
		return stateMap, nil
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	if model2channelStateMap == nil {
		return stateMap, nil
	}

	cachedStateMap, ok := model2channelStateMap[modelName]
	if !ok || len(cachedStateMap) == 0 {
		return stateMap, nil
	}

	stateMap = make(map[int]*ModelChannelState, len(cachedStateMap))
	for channelID, state := range cachedStateMap {
		if state == nil {
			continue
		}
		stateCopy := *state
		stateMap[channelID] = &stateCopy
	}
	return stateMap, nil
}

func SnapshotChannelCacheStateForTest() ChannelCacheStateForTest {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	return ChannelCacheStateForTest{
		Group2Model2Channels: cloneGroupModelChannels(group2model2channels),
		ChannelsByID:         cloneChannelsByID(channelsIDM),
		ModelChannelPolicies: cloneModelChannelPolicies(model2channelPolicies),
		ModelChannelStateMap: cloneModelChannelStateMap(model2channelStateMap),
	}
}

func RestoreChannelCacheStateForTest(state ChannelCacheStateForTest) {
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()

	group2model2channels = cloneGroupModelChannels(state.Group2Model2Channels)
	channelsIDM = cloneChannelsByID(state.ChannelsByID)
	model2channelPolicies = cloneModelChannelPolicies(state.ModelChannelPolicies)
	model2channelStateMap = cloneModelChannelStateMap(state.ModelChannelStateMap)
}

func cloneGroupModelChannels(src map[string]map[string][]int) map[string]map[string][]int {
	if src == nil {
		return nil
	}
	dst := make(map[string]map[string][]int, len(src))
	for group, modelMap := range src {
		if modelMap == nil {
			dst[group] = nil
			continue
		}
		clonedModelMap := make(map[string][]int, len(modelMap))
		for modelName, channelIDs := range modelMap {
			if channelIDs == nil {
				clonedModelMap[modelName] = nil
				continue
			}
			clonedIDs := make([]int, len(channelIDs))
			copy(clonedIDs, channelIDs)
			clonedModelMap[modelName] = clonedIDs
		}
		dst[group] = clonedModelMap
	}
	return dst
}

func cloneChannelsByID(src map[int]*Channel) map[int]*Channel {
	if src == nil {
		return nil
	}
	dst := make(map[int]*Channel, len(src))
	for id, channel := range src {
		if channel == nil {
			dst[id] = nil
			continue
		}
		channelCopy := *channel
		if channel.Keys != nil {
			channelCopy.Keys = append([]string(nil), channel.Keys...)
		}
		channelCopy.ChannelInfo.MultiKeyStatusList = cloneIntMap(channel.ChannelInfo.MultiKeyStatusList)
		channelCopy.ChannelInfo.MultiKeyDisabledReason = cloneStringMap(channel.ChannelInfo.MultiKeyDisabledReason)
		channelCopy.ChannelInfo.MultiKeyDisabledTime = cloneInt64Map(channel.ChannelInfo.MultiKeyDisabledTime)
		dst[id] = &channelCopy
	}
	return dst
}

func cloneModelChannelPolicies(src map[string][]ModelChannelPolicy) map[string][]ModelChannelPolicy {
	if src == nil {
		return nil
	}
	dst := make(map[string][]ModelChannelPolicy, len(src))
	for modelName, policies := range src {
		if policies == nil {
			dst[modelName] = nil
			continue
		}
		clonedPolicies := make([]ModelChannelPolicy, len(policies))
		copy(clonedPolicies, policies)
		dst[modelName] = clonedPolicies
	}
	return dst
}

func cloneModelChannelStateMap(src map[string]map[int]*ModelChannelState) map[string]map[int]*ModelChannelState {
	if src == nil {
		return nil
	}
	dst := make(map[string]map[int]*ModelChannelState, len(src))
	for modelName, stateMap := range src {
		if stateMap == nil {
			dst[modelName] = nil
			continue
		}
		clonedStateMap := make(map[int]*ModelChannelState, len(stateMap))
		for channelID, state := range stateMap {
			if state == nil {
				clonedStateMap[channelID] = nil
				continue
			}
			stateCopy := *state
			clonedStateMap[channelID] = &stateCopy
		}
		dst[modelName] = clonedStateMap
	}
	return dst
}

func cloneIntMap(src map[int]int) map[int]int {
	if src == nil {
		return nil
	}
	dst := make(map[int]int, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneStringMap(src map[int]string) map[int]string {
	if src == nil {
		return nil
	}
	dst := make(map[int]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneInt64Map(src map[int]int64) map[int]int64 {
	if src == nil {
		return nil
	}
	dst := make(map[int]int64, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
