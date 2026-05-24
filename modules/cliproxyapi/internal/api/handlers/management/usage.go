// management - usage.go
// 使用统计队列记录获取端点。
// 该模块提供从使用统计队列中弹出记录的 HTTP 接口，
// 支持通过 count 查询参数指定弹出的记录数量。
package management

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/redisqueue"
)

// usageQueueRecord 表示使用统计队列中的一条记录。
// 底层为原始 JSON 字节，MarshalJSON 时会验证 JSON 有效性，
// 有效则原样输出，无效则作为字符串输出。
type usageQueueRecord []byte

// MarshalJSON 实现 json.Marshaler 接口。
// 如果记录内容是有效的 JSON，直接输出原始字节；
// 否则将其作为字符串值输出，确保输出始终为有效 JSON。
func (r usageQueueRecord) MarshalJSON() ([]byte, error) {
	if json.Valid(r) {
		return append([]byte(nil), r...), nil
	}
	return json.Marshal(string(r))
}

// GetUsageQueue 从使用统计队列中弹出指定数量的记录。
// 通过查询参数 count 指定弹出数量，默认为 1。
// 返回的记录按先进先出（FIFO）顺序排列。
func (h *Handler) GetUsageQueue(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handler unavailable"})
		return
	}

	count, errCount := parseUsageQueueCount(c.Query("count"))
	if errCount != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": errCount.Error()})
		return
	}

	items := redisqueue.PopOldest(count)
	records := make([]usageQueueRecord, 0, len(items))
	for _, item := range items {
		records = append(records, usageQueueRecord(append([]byte(nil), item...)))
	}

	c.JSON(http.StatusOK, records)
}

// parseUsageQueueCount 解析使用统计队列的弹出数量参数。
// 空值默认为 1，非正整数返回错误。
func parseUsageQueueCount(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 1, nil
	}
	count, errCount := strconv.Atoi(value)
	if errCount != nil || count <= 0 {
		return 0, errors.New("count must be a positive integer")
	}
	return count, nil
}
