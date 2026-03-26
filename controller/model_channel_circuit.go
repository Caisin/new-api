package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetModelChannelCircuitModels(c *gin.Context) {
	items, err := service.GetModelChannelCircuitModels()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items)
}

func GetModelChannelCircuitDetail(c *gin.Context) {
	modelName := c.Param("model")
	detail, err := service.GetModelChannelCircuitDetail(modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, detail)
}

func UpdateModelChannelPolicies(c *gin.Context) {
	modelName := c.Param("model")
	var req dto.UpdateModelChannelPoliciesRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := service.SaveModelChannelPolicies(modelName, req.Channels); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{})
}

func EnableModelChannelCircuit(c *gin.Context) {
	modelName := c.Param("model")
	channelID, err := strconv.Atoi(c.Param("channel_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := service.EnableModelChannelCircuitPair(modelName, channelID); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{})
}

func DisableModelChannelCircuit(c *gin.Context) {
	modelName := c.Param("model")
	channelID, err := strconv.Atoi(c.Param("channel_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := service.DisableModelChannelCircuitPair(modelName, channelID); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{})
}

func ProbeModelChannelCircuit(c *gin.Context) {
	modelName := c.Param("model")
	channelID, err := strconv.Atoi(c.Param("channel_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := service.ProbeModelChannelCircuitPair(modelName, channelID)
	c.JSON(http.StatusOK, gin.H{
		"success": result.Success,
		"message": result.Message,
		"data":    result,
	})
}
