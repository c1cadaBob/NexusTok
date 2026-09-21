/*
Copyright (C) 2023-2026 c1cadaBob

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@c1cadabob.dev
*/
package service

import (
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/constant"
	"github.com/c1cadaBob/NexusTok/logger"
	"github.com/c1cadaBob/NexusTok/model"
	relaycommon "github.com/c1cadaBob/NexusTok/relay/common"

	"github.com/gin-gonic/gin"
)

func routingKeyFirstLatencyMs(info *relaycommon.RelayInfo) int64 {
	if info == nil {
		return 0
	}
	if info.HasSendResponse() {
		return info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
	}
	return time.Since(info.StartTime).Milliseconds()
}

func recordSelectedRoutingKeyHealth(ctx *gin.Context, info *relaycommon.RelayInfo, success bool) {
	routingKeyID := common.GetContextKeyInt(ctx, constant.ContextKeyRoutingKeyId)
	if routingKeyID <= 0 {
		return
	}
	latencyMs := int64(0)
	if success {
		latencyMs = routingKeyFirstLatencyMs(info)
	}
	err := model.RecordRoutingKeyHealthSample(
		uint(routingKeyID),
		common.GetContextKeyInt(ctx, constant.ContextKeyChannelId),
		common.GetContextKeyString(ctx, constant.ContextKeyRoutingKeySource),
		success,
		latencyMs,
		common.GetTimestamp(),
	)
	if err != nil {
		logger.LogWarn(ctx, "记录路由密钥健康样本失败: "+err.Error())
	}
}

func TouchSelectedUpstreamKeyLastUsed(ctx *gin.Context, info *relaycommon.RelayInfo) {
	routingKeyID := common.GetContextKeyInt(ctx, constant.ContextKeyRoutingKeyId)
	if routingKeyID > 0 {
		if err := model.TouchRoutingKeyLastUsed(uint(routingKeyID), common.GetTimestamp()); err != nil {
			logger.LogWarn(ctx, "更新路由密钥最后使用时间失败: "+err.Error())
		}
		recordSelectedRoutingKeyHealth(ctx, info, true)
		return
	}
	keyID := common.GetContextKeyInt(ctx, constant.ContextKeyUpstreamKeyId)
	if keyID <= 0 {
		return
	}
	if err := model.UpdateUpstreamKeyLastUsed(uint(keyID), common.GetTimestamp()); err != nil {
		logger.LogWarn(ctx, "更新上游密钥最后使用时间失败: "+err.Error())
	}
}

func RecordSelectedRoutingKeyFailure(ctx *gin.Context, info *relaycommon.RelayInfo) {
	recordSelectedRoutingKeyHealth(ctx, info, false)
}
