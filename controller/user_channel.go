package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// maskAPIKey returns a masked version of the API key for safe display
func maskAPIKey(key string) string {
	lines := strings.Split(key, "\n")
	maskedLines := make([]string, 0, len(lines))
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if len(trimmed) <= 10 {
			maskedLines = append(maskedLines, "sk-******")
		} else {
			maskedLines = append(maskedLines, trimmed[:6]+"******"+trimmed[len(trimmed)-4:])
		}
	}
	return strings.Join(maskedLines, "\n")
}

// AddUserChannel handles creating a channel by a normal user (sharing an idle API key).
func AddUserChannel(c *gin.Context) {
	addChannelRequest := AddChannelRequest{}
	err := c.ShouldBindJSON(&addChannelRequest)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if addChannelRequest.Channel == nil {
		common.ApiError(c, fmt.Errorf("channel is required"))
		return
	}

	userId := c.GetInt("id")
	addChannelRequest.Channel.UserId = &userId

	// Default to "default" shared pool if empty or set to user_shared
	group := strings.TrimSpace(addChannelRequest.Channel.Group)
	if group == "" || group == "user_shared" {
		addChannelRequest.Channel.Group = "default"
	}

	priority := int64(0)
	addChannelRequest.Channel.Priority = &priority

	// Ensure weight is at least 1 so weighted selection can pick this channel
	weight := uint(1)
	if addChannelRequest.Channel.Weight != nil && *addChannelRequest.Channel.Weight > 0 {
		weight = *addChannelRequest.Channel.Weight
	}
	addChannelRequest.Channel.Weight = &weight

	if err := validateChannel(addChannelRequest.Channel, true); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	addChannelRequest.Channel.CreatedTime = common.GetTimestamp()
	keys := make([]string, 0)

	switch addChannelRequest.Mode {
	case "multi_to_single":
		addChannelRequest.Channel.ChannelInfo.IsMultiKey = true
		addChannelRequest.Channel.ChannelInfo.MultiKeyMode = addChannelRequest.MultiKeyMode
		cleanKeys := make([]string, 0)
		for key := range strings.SplitSeq(addChannelRequest.Channel.Key, "\n") {
			key = strings.TrimSpace(key)
			if key != "" {
				cleanKeys = append(cleanKeys, key)
			}
		}
		if len(cleanKeys) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Key 不能为空",
			})
			return
		}
		addChannelRequest.Channel.ChannelInfo.MultiKeySize = len(cleanKeys)
		addChannelRequest.Channel.Key = strings.Join(cleanKeys, "\n")
		keys = []string{addChannelRequest.Channel.Key}
	case "batch":
		for key := range strings.SplitSeq(addChannelRequest.Channel.Key, "\n") {
			key = strings.TrimSpace(key)
			if key != "" {
				keys = append(keys, key)
			}
		}
	default:
		keys = []string{strings.TrimSpace(addChannelRequest.Channel.Key)}
	}

	if len(keys) == 0 || (len(keys) == 1 && keys[0] == "") {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Key 不能为空",
		})
		return
	}

	channels := make([]model.Channel, 0, len(keys))
	for _, key := range keys {
		localChannel := *addChannelRequest.Channel
		localChannel.Key = key
		if addChannelRequest.BatchAddSetKeyPrefix2Name && len(keys) > 1 {
			prefix := key
			if len(key) > 8 {
				prefix = key[:8]
			}
			localChannel.Name = fmt.Sprintf("%s %s", localChannel.Name, prefix)
		}
		channels = append(channels, localChannel)
	}

	// Use BatchInsertChannels to ensure abilities are registered in the routing table
	err = model.BatchInsertChannels(channels)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	model.InitChannelCache()
	recordManageAudit(c, "channel.create_user", map[string]any{
		"mode":   addChannelRequest.Mode,
		"type":   addChannelRequest.Channel.Type,
		"name":   addChannelRequest.Channel.Name,
		"models": addChannelRequest.Channel.Models,
		"count":  len(channels),
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "共享渠道添加成功，已接入公共服务池！",
	})
}

// GetUserChannels returns paginated channels owned by the current user.
func GetUserChannels(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	channelData := make([]*model.Channel, 0)

	query := model.DB.Model(&model.Channel{}).Where("user_id = ?", userId)

	if typeParam, has := c.GetQuery("type"); has {
		if parsed, err := strconv.Atoi(typeParam); err == nil && parsed >= 0 {
			query = query.Where("type = ?", parsed)
		}
	}
	if nameParam, has := c.GetQuery("name"); has && nameParam != "" {
		query = query.Where("name LIKE ?", "%"+nameParam+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	err := query.Order("id desc").Limit(pageInfo.PageSize).Offset(pageInfo.GetStartIdx()).Find(&channelData).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Mask API keys before returning to client
	for _, ch := range channelData {
		ch.Key = maskAPIKey(ch.Key)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items": channelData,
			"total": total,
		},
		"total": total,
	})
}

// GetUserChannel retrieves a single channel by ID for the owner.
func GetUserChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	userId := c.GetInt("id")
	channel, err := model.GetChannelById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if channel.UserId == nil || *channel.UserId != userId {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "无权访问此渠道",
		})
		return
	}

	// Mask the key
	channel.Key = maskAPIKey(channel.Key)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channel,
	})
}

// UpdateUserChannel handles updating an existing channel owned by the user.
func UpdateUserChannel(c *gin.Context) {
	channel := PatchChannel{}
	rawBody, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := common.Unmarshal(rawBody, &channel); err != nil {
		common.ApiError(c, err)
		return
	}
	var requestData map[string]any
	if err := common.Unmarshal(rawBody, &requestData); err != nil {
		common.ApiError(c, err)
		return
	}
	if _, ok := requestData["status"]; ok {
		common.ApiError(c, fmt.Errorf("status cannot be updated here"))
		return
	}
	clearChannelReadOnlyFields(&channel, requestData)

	originalChannel, err := model.GetChannelById(channel.Id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	userId := c.GetInt("id")
	if originalChannel.UserId == nil || *originalChannel.UserId != userId {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "forbidden",
		})
		return
	}

	// Preserve owner and routing constraints
	channel.UserId = originalChannel.UserId
	if channel.Group == "" {
		channel.Group = originalChannel.Group
	}
	channel.Priority = originalChannel.Priority
	if channel.Weight == nil || *channel.Weight == 0 {
		channel.Weight = originalChannel.Weight
	}

	// If key is empty or masked, don't overwrite the original key
	if channel.Key == "" || strings.Contains(channel.Key, "******") {
		channel.Key = originalChannel.Key
	}

	if err := channel.Update(); err != nil {
		common.ApiError(c, err)
		return
	}

	model.InitChannelCache()
	common.ApiSuccess(c, nil)
}

// UpdateUserChannelStatus allows users to enable/disable their contributed channel.
func UpdateUserChannelStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	userId := c.GetInt("id")
	originalChannel, err := model.GetChannelById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if originalChannel.UserId == nil || *originalChannel.UserId != userId {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "forbidden",
		})
		return
	}

	req := ChannelStatusRequest{}
	if err := c.ShouldBindJSON(&req); err != nil || !isManageableChannelStatus(req.Status) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	changed := model.UpdateChannelStatus(id, "", req.Status, "user manual operation")
	if changed {
		model.InitChannelCache()
		if req.Status != common.ChannelStatusEnabled {
			closeActiveChannelWebSockets([]int{id})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    changed,
	})
}

// DeleteUserChannel removes a channel owned by the user.
func DeleteUserChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	originalChannel, err := model.GetChannelById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	userId := c.GetInt("id")
	if originalChannel.UserId == nil || *originalChannel.UserId != userId {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "forbidden",
		})
		return
	}

	_, err = model.BatchDeleteChannels([]int{id})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	common.ApiSuccess(c, nil)
}

// TestUserChannel tests connectivity of a user-contributed channel.
func TestUserChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	userId := c.GetInt("id")
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if channel.UserId == nil || *channel.UserId != userId {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "forbidden",
		})
		return
	}

	testModel := c.Query("model")
	endpointType := c.Query("endpoint_type")
	isStream, _ := strconv.ParseBool(c.Query("stream"))

	tik := time.Now()
	requestCtx := context.Background()
	if c.Request != nil {
		requestCtx = c.Request.Context()
	}

	result := testChannel(requestCtx, channel, userId, testModel, endpointType, isStream)
	if result.localErr != nil {
		resp := gin.H{
			"success": false,
			"message": result.localErr.Error(),
			"time":    0.0,
		}
		if result.newAPIError != nil {
			resp["error_code"] = result.newAPIError.GetErrorCode()
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	go channel.UpdateResponseTime(milliseconds)
	consumedTime := float64(milliseconds) / 1000.0

	if result.newAPIError != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":    false,
			"message":    result.newAPIError.Error(),
			"time":       consumedTime,
			"error_code": result.newAPIError.GetErrorCode(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"time":    consumedTime,
	})
}

// GetUserChannelStats calculates cumulative contribution statistics for the current user.
func GetUserChannelStats(c *gin.Context) {
	userId := c.GetInt("id")

	var totalChannels int64
	var enabledChannels int64
	var totalUsedQuota int64

	model.DB.Model(&model.Channel{}).Where("user_id = ?", userId).Count(&totalChannels)
	model.DB.Model(&model.Channel{}).Where("user_id = ? AND status = ?", userId, common.ChannelStatusEnabled).Count(&enabledChannels)
	model.DB.Model(&model.Channel{}).Where("user_id = ?", userId).Select("COALESCE(SUM(used_quota), 0)").Scan(&totalUsedQuota)

	// Revenue share ratio is 80%
	totalEarnedQuota := int64(float64(totalUsedQuota) * 0.8)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total_channels":     totalChannels,
			"enabled_channels":   enabledChannels,
			"total_used_quota":   totalUsedQuota,
			"total_earned_quota": totalEarnedQuota,
			"share_ratio":        0.8,
		},
	})
}

// FixUserChannelsAbilities recalculates and rebuilds ability mappings.
func FixUserChannelsAbilities(c *gin.Context) {
	success, fails, err := model.FixAbility()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"success": success,
			"fails":   fails,
		},
	})
}

// BatchDeleteUserChannels deletes multiple channels owned by the user.
func BatchDeleteUserChannels(c *gin.Context) {
	req := struct {
		Ids []int `json:"ids"`
	}{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if len(req.Ids) == 0 {
		common.ApiSuccess(c, 0)
		return
	}
	userId := c.GetInt("id")
	var userOwnedIds []int
	model.DB.Model(&model.Channel{}).Where("id IN ? AND user_id = ?", req.Ids, userId).Pluck("id", &userOwnedIds)
	if len(userOwnedIds) == 0 {
		common.ApiSuccess(c, 0)
		return
	}
	deleted, err := model.BatchDeleteChannels(userOwnedIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	common.ApiSuccess(c, deleted)
}

// BatchUpdateUserChannelStatus toggles status for multiple channels owned by the user.
func BatchUpdateUserChannelStatus(c *gin.Context) {
	req := struct {
		Ids    []int `json:"ids"`
		Status int   `json:"status"`
	}{}
	if err := c.ShouldBindJSON(&req); err != nil || !isManageableChannelStatus(req.Status) {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(req.Ids) == 0 {
		common.ApiSuccess(c, 0)
		return
	}
	userId := c.GetInt("id")
	var userOwnedIds []int
	model.DB.Model(&model.Channel{}).Where("id IN ? AND user_id = ?", req.Ids, userId).Pluck("id", &userOwnedIds)
	changedCount := 0
	for _, id := range userOwnedIds {
		if model.UpdateChannelStatus(id, "", req.Status, "user batch operation") {
			changedCount++
		}
	}
	if changedCount > 0 {
		model.InitChannelCache()
	}
	common.ApiSuccess(c, changedCount)
}

// DeleteDisabledUserChannels deletes all disabled channels owned by the user.
func DeleteDisabledUserChannels(c *gin.Context) {
	userId := c.GetInt("id")
	var disabledIds []int
	model.DB.Model(&model.Channel{}).Where("user_id = ? AND status IN (?, ?)", userId, common.ChannelStatusAutoDisabled, common.ChannelStatusManuallyDisabled).Pluck("id", &disabledIds)
	if len(disabledIds) == 0 {
		common.ApiSuccess(c, 0)
		return
	}
	deleted, err := model.BatchDeleteChannels(disabledIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	common.ApiSuccess(c, deleted)
}

// SearchUserChannels handles keyword search on user's channels.
func SearchUserChannels(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	keyword := strings.TrimSpace(c.Query("keyword"))
	modelKeyword := strings.TrimSpace(c.Query("model"))

	query := model.DB.Model(&model.Channel{}).Where("user_id = ?", userId)
	if keyword != "" {
		query = query.Where("name LIKE ? OR id = ?", "%"+keyword+"%", common.String2Int(keyword))
	}
	if modelKeyword != "" {
		query = query.Where("models LIKE ?", "%"+modelKeyword+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	channelData := make([]*model.Channel, 0)
	err := query.Order("id desc").Limit(pageInfo.PageSize).Offset(pageInfo.GetStartIdx()).Find(&channelData).Error
	if err != nil {
		common.ApiError(c, err)
		return
	}

	for _, ch := range channelData {
		ch.Key = maskAPIKey(ch.Key)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items": channelData,
			"total": total,
		},
		"total": total,
	})
}

