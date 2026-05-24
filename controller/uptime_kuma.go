// Package controller - uptime_kuma.go
// 该文件实现了 Uptime Kuma 状态监控的 API 控制器
//
// Uptime Kuma 是一个开源的服务状态监控工具
// 功能包括：
// - 从多个 Uptime Kuma 实例获取监控数据
// - 并行请求多个监控组
// - 返回服务可用性和状态信息
//
// 主要 API：
// - GetUptimeKumaStatus：获取所有监控组的状态信息
package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/setting/console_setting"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

// 常量定义
const (
	requestTimeout   = 30 * time.Second // 整体请求超时时间
	httpTimeout      = 10 * time.Second // 单个 HTTP 请求超时时间
	uptimeKeySuffix  = "_24"            // 24 小时可用性键后缀
	apiStatusPath    = "/api/status-page/"       // Uptime Kuma 状态页面 API 路径
	apiHeartbeatPath = "/api/status-page/heartbeat/" // Uptime Kuma 心跳 API 路径
)

// Monitor 监控项结构体
type Monitor struct {
	Name   string  `json:"name"`              // 监控项名称
	Uptime float64 `json:"uptime"`            // 24 小时可用性（百分比）
	Status int     `json:"status"`            // 当前状态（0: 异常, 1: 正常, 2: 暂停, 3: 待定）
	Group  string  `json:"group,omitempty"`   // 监控组名称
}

// UptimeGroupResult 监控组结果结构体
type UptimeGroupResult struct {
	CategoryName string    `json:"categoryName"` // 分类名称
	Monitors     []Monitor `json:"monitors"`     // 监控项列表
}

// getAndDecode 发送 HTTP GET 请求并解码响应
//
// 通用的 HTTP 请求工具函数，支持上下文超时控制
//
// 参数：
//   - ctx: 请求上下文
//   - client: HTTP 客户端
//   - url: 请求 URL
//   - dest: 目标结构体指针
func getAndDecode(ctx context.Context, client *http.Client, url string, dest interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("non-200 status")
	}

	return json.NewDecoder(resp.Body).Decode(dest)
}

// fetchGroupData 获取单个监控组的数据
//
// 并行请求状态页面和心跳数据，然后合并结果
// 参数：
//   - ctx: 请求上下文
//   - client: HTTP 客户端
//   - groupConfig: 监控组配置（包含 url、slug、categoryName）
//
// 返回：
//   - UptimeGroupResult: 监控组结果
func fetchGroupData(ctx context.Context, client *http.Client, groupConfig map[string]interface{}) UptimeGroupResult {
	url, _ := groupConfig["url"].(string)
	slug, _ := groupConfig["slug"].(string)
	categoryName, _ := groupConfig["categoryName"].(string)

	result := UptimeGroupResult{
		CategoryName: categoryName,
		Monitors:     []Monitor{},
	}

	if url == "" || slug == "" {
		return result
	}

	baseURL := strings.TrimSuffix(url, "/")

	var statusData struct {
		PublicGroupList []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			MonitorList []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"monitorList"`
		} `json:"publicGroupList"`
	}

	var heartbeatData struct {
		HeartbeatList map[string][]struct {
			Status int `json:"status"`
		} `json:"heartbeatList"`
		UptimeList map[string]float64 `json:"uptimeList"`
	}

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return getAndDecode(gCtx, client, baseURL+apiStatusPath+slug, &statusData)
	})
	g.Go(func() error {
		return getAndDecode(gCtx, client, baseURL+apiHeartbeatPath+slug, &heartbeatData)
	})

	if g.Wait() != nil {
		return result
	}

	for _, pg := range statusData.PublicGroupList {
		if len(pg.MonitorList) == 0 {
			continue
		}

		for _, m := range pg.MonitorList {
			monitor := Monitor{
				Name:  m.Name,
				Group: pg.Name,
			}

			monitorID := strconv.Itoa(m.ID)

			if uptime, exists := heartbeatData.UptimeList[monitorID+uptimeKeySuffix]; exists {
				monitor.Uptime = uptime
			}

			if heartbeats, exists := heartbeatData.HeartbeatList[monitorID]; exists && len(heartbeats) > 0 {
				monitor.Status = heartbeats[0].Status
			}

			result.Monitors = append(result.Monitors, monitor)
		}
	}

	return result
}

// GetUptimeKumaStatus 获取所有监控组的状态信息
//
// 流程：
// 1. 从配置获取监控组列表
// 2. 并行请求每个监控组的数据
// 3. 合并结果返回
//
// 返回：
//   - success: 是否成功
//   - data: 监控组结果列表
func GetUptimeKumaStatus(c *gin.Context) {
	groups := console_setting.GetUptimeKumaGroups()
	if len(groups) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": []UptimeGroupResult{}})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	client := &http.Client{Timeout: httpTimeout}
	results := make([]UptimeGroupResult, len(groups))

	g, gCtx := errgroup.WithContext(ctx)
	for i, group := range groups {
		i, group := i, group
		g.Go(func() error {
			results[i] = fetchGroupData(gCtx, client, group)
			return nil
		})
	}

	g.Wait()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": results})
}
