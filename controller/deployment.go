// Package controller - deployment.go
// 该文件实现了 io.net GPU 容器部署管理的 API 控制器
//
// 通过 io.net Enterprise API 管理 GPU 容器部署，支持：
// - 部署创建/更新/删除/延期
// - 硬件类型查询
// - 可用位置查询
// - 价格估算
// - 容器日志查看
// - 集群名称管理
//
// 主要 API：
// - GetAllDeployments：获取所有部署列表
// - CreateDeployment：创建新部署
// - GetDeployment：获取部署详情
// - UpdateDeployment：更新部署配置
// - DeleteDeployment：删除部署
// - GetHardwareTypes：获取可用硬件类型
// - GetPriceEstimation：获取价格估算
package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/pkg/ionet"
	"github.com/gin-gonic/gin"
)

// getIoAPIKey 获取 io.net API 密钥
//
// 从系统配置中读取 io.net 的启用状态和 API 密钥
//
// 返回值：
//   - string: API 密钥
//   - bool: 是否成功获取（false 表示未启用或密钥为空）
func getIoAPIKey(c *gin.Context) (string, bool) {
	common.OptionMapRWMutex.RLock()
	enabled := common.OptionMap["model_deployment.ionet.enabled"] == "true"
	apiKey := common.OptionMap["model_deployment.ionet.api_key"]
	common.OptionMapRWMutex.RUnlock()
	if !enabled || strings.TrimSpace(apiKey) == "" {
		common.ApiErrorMsg(c, "io.net model deployment is not enabled or api key missing")
		return "", false
	}
	return apiKey, true
}

// GetModelDeploymentSettings 获取模型部署配置信息
//
// 返回 io.net 的启用状态、API 密钥配置状态和连接可用性
func GetModelDeploymentSettings(c *gin.Context) {
	common.OptionMapRWMutex.RLock()
	enabled := common.OptionMap["model_deployment.ionet.enabled"] == "true"
	hasAPIKey := strings.TrimSpace(common.OptionMap["model_deployment.ionet.api_key"]) != ""
	common.OptionMapRWMutex.RUnlock()

	common.ApiSuccess(c, gin.H{
		"provider":    "io.net",
		"enabled":     enabled,
		"configured":  hasAPIKey,
		"can_connect": enabled && hasAPIKey,
	})
}

// getIoClient 获取 io.net 标准客户端
func getIoClient(c *gin.Context) (*ionet.Client, bool) {
	apiKey, ok := getIoAPIKey(c)
	if !ok {
		return nil, false
	}
	return ionet.NewClient(apiKey), true
}

// getIoEnterpriseClient 获取 io.net 企业版客户端
func getIoEnterpriseClient(c *gin.Context) (*ionet.Client, bool) {
	apiKey, ok := getIoAPIKey(c)
	if !ok {
		return nil, false
	}
	return ionet.NewEnterpriseClient(apiKey), true
}

// TestIoNetConnection 测试 io.net API 连接
//
// 支持使用请求中的 API 密钥或系统配置的密钥进行测试
func TestIoNetConnection(c *gin.Context) {
	var req struct {
		APIKey string `json:"api_key"`
	}

	rawBody, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(bytes.TrimSpace(rawBody)) > 0 {
		if err := json.Unmarshal(rawBody, &req); err != nil {
			common.ApiErrorMsg(c, "invalid request payload")
			return
		}
	}

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		common.OptionMapRWMutex.RLock()
		storedKey := strings.TrimSpace(common.OptionMap["model_deployment.ionet.api_key"])
		common.OptionMapRWMutex.RUnlock()
		if storedKey == "" {
			common.ApiErrorMsg(c, "api_key is required")
			return
		}
		apiKey = storedKey
	}

	client := ionet.NewEnterpriseClient(apiKey)
	result, err := client.GetMaxGPUsPerContainer()
	if err != nil {
		if apiErr, ok := err.(*ionet.APIError); ok {
			message := strings.TrimSpace(apiErr.Message)
			if message == "" {
				message = "failed to validate api key"
			}
			common.ApiErrorMsg(c, message)
			return
		}
		common.ApiError(c, err)
		return
	}

	totalHardware := 0
	totalAvailable := 0
	if result != nil {
		totalHardware = len(result.Hardware)
		totalAvailable = result.Total
		if totalAvailable == 0 {
			for _, hw := range result.Hardware {
				totalAvailable += hw.Available
			}
		}
	}

	common.ApiSuccess(c, gin.H{
		"hardware_count":  totalHardware,
		"total_available": totalAvailable,
	})
}

// requireDeploymentID 从路径参数中获取部署 ID
func requireDeploymentID(c *gin.Context) (string, bool) {
	deploymentID := strings.TrimSpace(c.Param("id"))
	if deploymentID == "" {
		common.ApiErrorMsg(c, "deployment ID is required")
		return "", false
	}
	return deploymentID, true
}

// requireContainerID 从路径参数中获取容器 ID
func requireContainerID(c *gin.Context) (string, bool) {
	containerID := strings.TrimSpace(c.Param("container_id"))
	if containerID == "" {
		common.ApiErrorMsg(c, "container ID is required")
		return "", false
	}
	return containerID, true
}

// mapIoNetDeployment 将 io.net 部署对象转换为 API 响应格式
//
// 包含硬件信息、时间剩余、完成百分比等详细信息
func mapIoNetDeployment(d ionet.Deployment) map[string]interface{} {
	var created int64
	if d.CreatedAt.IsZero() {
		created = time.Now().Unix()
	} else {
		created = d.CreatedAt.Unix()
	}

	timeRemainingHours := d.ComputeMinutesRemaining / 60
	timeRemainingMins := d.ComputeMinutesRemaining % 60
	var timeRemaining string
	if timeRemainingHours > 0 {
		timeRemaining = fmt.Sprintf("%d hour %d minutes", timeRemainingHours, timeRemainingMins)
	} else if timeRemainingMins > 0 {
		timeRemaining = fmt.Sprintf("%d minutes", timeRemainingMins)
	} else {
		timeRemaining = "completed"
	}

	hardwareInfo := fmt.Sprintf("%s %s x%d", d.BrandName, d.HardwareName, d.HardwareQuantity)

	return map[string]interface{}{
		"id":                        d.ID,
		"deployment_name":           d.Name,
		"container_name":            d.Name,
		"status":                    strings.ToLower(d.Status),
		"type":                      "Container",
		"time_remaining":            timeRemaining,
		"time_remaining_minutes":    d.ComputeMinutesRemaining,
		"hardware_info":             hardwareInfo,
		"hardware_name":             d.HardwareName,
		"brand_name":                d.BrandName,
		"hardware_quantity":         d.HardwareQuantity,
		"completed_percent":         d.CompletedPercent,
		"compute_minutes_served":    d.ComputeMinutesServed,
		"compute_minutes_remaining": d.ComputeMinutesRemaining,
		"created_at":                created,
		"updated_at":                created,
		"model_name":                "",
		"model_version":             "",
		"instance_count":            d.HardwareQuantity,
		"resource_config": map[string]interface{}{
			"cpu":    "",
			"memory": "",
			"gpu":    strconv.Itoa(d.HardwareQuantity),
		},
		"description": "",
		"provider":    "io.net",
	}
}

// computeStatusCounts 计算各状态的部署数量统计
func computeStatusCounts(total int, deployments []ionet.Deployment) map[string]int64 {
	counts := map[string]int64{
		"all": int64(total),
	}

	for _, status := range []string{"running", "completed", "failed", "deployment requested", "termination requested", "destroyed"} {
		counts[status] = 0
	}

	for _, d := range deployments {
		status := strings.ToLower(strings.TrimSpace(d.Status))
		counts[status] = counts[status] + 1
	}

	return counts
}

// GetAllDeployments 获取所有部署列表
//
// 支持分页和状态过滤
func GetAllDeployments(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	status := c.Query("status")
	opts := &ionet.ListDeploymentsOptions{
		Status:    strings.ToLower(strings.TrimSpace(status)),
		Page:      pageInfo.GetPage(),
		PageSize:  pageInfo.GetPageSize(),
		SortBy:    "created_at",
		SortOrder: "desc",
	}

	dl, err := client.ListDeployments(opts)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	items := make([]map[string]interface{}, 0, len(dl.Deployments))
	for _, d := range dl.Deployments {
		items = append(items, mapIoNetDeployment(d))
	}

	data := gin.H{
		"page":          pageInfo.GetPage(),
		"page_size":     pageInfo.GetPageSize(),
		"total":         dl.Total,
		"items":         items,
		"status_counts": computeStatusCounts(dl.Total, dl.Deployments),
	}
	common.ApiSuccess(c, data)
}

// SearchDeployments 搜索部署
//
// 支持按关键词搜索部署名称
func SearchDeployments(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	keyword := strings.TrimSpace(c.Query("keyword"))

	dl, err := client.ListDeployments(&ionet.ListDeploymentsOptions{
		Status:    status,
		Page:      pageInfo.GetPage(),
		PageSize:  pageInfo.GetPageSize(),
		SortBy:    "created_at",
		SortOrder: "desc",
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}

	filtered := make([]ionet.Deployment, 0, len(dl.Deployments))
	if keyword == "" {
		filtered = dl.Deployments
	} else {
		kw := strings.ToLower(keyword)
		for _, d := range dl.Deployments {
			if strings.Contains(strings.ToLower(d.Name), kw) {
				filtered = append(filtered, d)
			}
		}
	}

	items := make([]map[string]interface{}, 0, len(filtered))
	for _, d := range filtered {
		items = append(items, mapIoNetDeployment(d))
	}

	total := dl.Total
	if keyword != "" {
		total = len(filtered)
	}

	data := gin.H{
		"page":      pageInfo.GetPage(),
		"page_size": pageInfo.GetPageSize(),
		"total":     total,
		"items":     items,
	}
	common.ApiSuccess(c, data)
}

// GetDeployment 获取部署详情
//
// 路径参数：
//   - id: 部署 ID
func GetDeployment(c *gin.Context) {
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	details, err := client.GetDeployment(deploymentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	data := map[string]interface{}{
		"id":              details.ID,
		"deployment_name": details.ID,
		"model_name":      "",
		"model_version":   "",
		"status":          strings.ToLower(details.Status),
		"instance_count":  details.TotalContainers,
		"hardware_id":     details.HardwareID,
		"resource_config": map[string]interface{}{
			"cpu":    "",
			"memory": "",
			"gpu":    strconv.Itoa(details.TotalGPUs),
		},
		"created_at":                details.CreatedAt.Unix(),
		"updated_at":                details.CreatedAt.Unix(),
		"description":               "",
		"amount_paid":               details.AmountPaid,
		"completed_percent":         details.CompletedPercent,
		"gpus_per_container":        details.GPUsPerContainer,
		"total_gpus":                details.TotalGPUs,
		"total_containers":          details.TotalContainers,
		"hardware_name":             details.HardwareName,
		"brand_name":                details.BrandName,
		"compute_minutes_served":    details.ComputeMinutesServed,
		"compute_minutes_remaining": details.ComputeMinutesRemaining,
		"locations":                 details.Locations,
		"container_config":          details.ContainerConfig,
	}

	common.ApiSuccess(c, data)
}

// UpdateDeploymentName 更新部署名称
//
// 更新前会检查名称是否可用
func UpdateDeploymentName(c *gin.Context) {
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	updateReq := &ionet.UpdateClusterNameRequest{
		Name: strings.TrimSpace(req.Name),
	}

	if updateReq.Name == "" {
		common.ApiErrorMsg(c, "deployment name cannot be empty")
		return
	}

	available, err := client.CheckClusterNameAvailability(updateReq.Name)
	if err != nil {
		common.ApiError(c, fmt.Errorf("failed to check name availability: %w", err))
		return
	}

	if !available {
		common.ApiErrorMsg(c, "deployment name is not available, please choose a different name")
		return
	}

	resp, err := client.UpdateClusterName(deploymentID, updateReq)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	data := gin.H{
		"status":  resp.Status,
		"message": resp.Message,
		"id":      deploymentID,
		"name":    updateReq.Name,
	}
	common.ApiSuccess(c, data)
}

// UpdateDeployment 更新部署配置
//
// 支持更新容器数量、GPU 配置等参数
func UpdateDeployment(c *gin.Context) {
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	var req ionet.UpdateDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	resp, err := client.UpdateDeployment(deploymentID, &req)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	data := gin.H{
		"status":        resp.Status,
		"deployment_id": resp.DeploymentID,
	}
	common.ApiSuccess(c, data)
}

// ExtendDeployment 延长部署时长
//
// 增加部署的计算时间
func ExtendDeployment(c *gin.Context) {
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	var req ionet.ExtendDurationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	details, err := client.ExtendDeployment(deploymentID, &req)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	data := mapIoNetDeployment(ionet.Deployment{
		ID:                      details.ID,
		Status:                  details.Status,
		Name:                    deploymentID,
		CompletedPercent:        float64(details.CompletedPercent),
		HardwareQuantity:        details.TotalGPUs,
		BrandName:               details.BrandName,
		HardwareName:            details.HardwareName,
		ComputeMinutesServed:    details.ComputeMinutesServed,
		ComputeMinutesRemaining: details.ComputeMinutesRemaining,
		CreatedAt:               details.CreatedAt,
	})

	common.ApiSuccess(c, data)
}

// DeleteDeployment 删除（终止）部署
//
// 提交终止请求，实际终止可能需要时间
func DeleteDeployment(c *gin.Context) {
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	resp, err := client.DeleteDeployment(deploymentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	data := gin.H{
		"status":        resp.Status,
		"deployment_id": resp.DeploymentID,
		"message":       "Deployment termination requested successfully",
	}
	common.ApiSuccess(c, data)
}

// CreateDeployment 创建新的 GPU 容器部署
func CreateDeployment(c *gin.Context) {
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	var req ionet.DeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	resp, err := client.DeployContainer(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	data := gin.H{
		"deployment_id": resp.DeploymentID,
		"status":        resp.Status,
		"message":       "Deployment created successfully",
	}
	common.ApiSuccess(c, data)
}

// GetHardwareTypes 获取可用的硬件类型列表
//
// 返回 GPU 型号、品牌和可用数量
func GetHardwareTypes(c *gin.Context) {
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	hardwareTypes, totalAvailable, err := client.ListHardwareTypes()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	data := gin.H{
		"hardware_types":  hardwareTypes,
		"total":           len(hardwareTypes),
		"total_available": totalAvailable,
	}
	common.ApiSuccess(c, data)
}

// GetLocations 获取可用的部署位置列表
func GetLocations(c *gin.Context) {
	client, ok := getIoClient(c)
	if !ok {
		return
	}

	locationsResp, err := client.ListLocations()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	total := locationsResp.Total
	if total == 0 {
		total = len(locationsResp.Locations)
	}

	data := gin.H{
		"locations": locationsResp.Locations,
		"total":     total,
	}
	common.ApiSuccess(c, data)
}

// GetAvailableReplicas 获取指定硬件的可用副本数
//
// 查询参数：
//   - hardware_id: 硬件 ID
//   - gpu_count: GPU 数量
func GetAvailableReplicas(c *gin.Context) {
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	hardwareIDStr := c.Query("hardware_id")
	gpuCountStr := c.Query("gpu_count")

	if hardwareIDStr == "" {
		common.ApiErrorMsg(c, "hardware_id parameter is required")
		return
	}

	hardwareID, err := strconv.Atoi(hardwareIDStr)
	if err != nil || hardwareID <= 0 {
		common.ApiErrorMsg(c, "invalid hardware_id parameter")
		return
	}

	gpuCount := 1
	if gpuCountStr != "" {
		if parsed, err := strconv.Atoi(gpuCountStr); err == nil && parsed > 0 {
			gpuCount = parsed
		}
	}

	replicas, err := client.GetAvailableReplicas(hardwareID, gpuCount)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, replicas)
}

// GetPriceEstimation 获取部署价格估算
func GetPriceEstimation(c *gin.Context) {
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	var req ionet.PriceEstimationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	priceResp, err := client.GetPriceEstimation(&req)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, priceResp)
}

// CheckClusterNameAvailability 检查集群名称是否可用
//
// 查询参数：
//   - name: 集群名称
func CheckClusterNameAvailability(c *gin.Context) {
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	clusterName := strings.TrimSpace(c.Query("name"))
	if clusterName == "" {
		common.ApiErrorMsg(c, "name parameter is required")
		return
	}

	available, err := client.CheckClusterNameAvailability(clusterName)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	data := gin.H{
		"available": available,
		"name":      clusterName,
	}
	common.ApiSuccess(c, data)
}

// GetDeploymentLogs 获取部署容器日志
//
// 支持日志级别过滤、流过滤、时间范围和游标分页
func GetDeploymentLogs(c *gin.Context) {
	client, ok := getIoClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	containerID := c.Query("container_id")
	if containerID == "" {
		common.ApiErrorMsg(c, "container_id parameter is required")
		return
	}
	level := c.Query("level")
	stream := c.Query("stream")
	cursor := c.Query("cursor")
	limitStr := c.Query("limit")
	follow := c.Query("follow") == "true"

	var limit int = 100
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
			if limit > 1000 {
				limit = 1000
			}
		}
	}

	opts := &ionet.GetLogsOptions{
		Level:  level,
		Stream: stream,
		Limit:  limit,
		Cursor: cursor,
		Follow: follow,
	}

	if startTime := c.Query("start_time"); startTime != "" {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			opts.StartTime = &t
		}
	}
	if endTime := c.Query("end_time"); endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			opts.EndTime = &t
		}
	}

	rawLogs, err := client.GetContainerLogsRaw(deploymentID, containerID, opts)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, rawLogs)
}

// ListDeploymentContainers 列出部署的所有容器
func ListDeploymentContainers(c *gin.Context) {
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	containers, err := client.ListContainers(deploymentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	items := make([]map[string]interface{}, 0)
	if containers != nil {
		items = make([]map[string]interface{}, 0, len(containers.Workers))
		for _, ctr := range containers.Workers {
			events := make([]map[string]interface{}, 0, len(ctr.ContainerEvents))
			for _, event := range ctr.ContainerEvents {
				events = append(events, map[string]interface{}{
					"time":    event.Time.Unix(),
					"message": event.Message,
				})
			}

			items = append(items, map[string]interface{}{
				"container_id":       ctr.ContainerID,
				"device_id":          ctr.DeviceID,
				"status":             strings.ToLower(strings.TrimSpace(ctr.Status)),
				"hardware":           ctr.Hardware,
				"brand_name":         ctr.BrandName,
				"created_at":         ctr.CreatedAt.Unix(),
				"uptime_percent":     ctr.UptimePercent,
				"gpus_per_container": ctr.GPUsPerContainer,
				"public_url":         ctr.PublicURL,
				"events":             events,
			})
		}
	}

	response := gin.H{
		"total":      0,
		"containers": items,
	}
	if containers != nil {
		response["total"] = containers.Total
	}

	common.ApiSuccess(c, response)
}

// GetContainerDetails 获取容器详细信息
//
// 包含状态、硬件信息、运行时间和事件列表
func GetContainerDetails(c *gin.Context) {
	client, ok := getIoEnterpriseClient(c)
	if !ok {
		return
	}

	deploymentID, ok := requireDeploymentID(c)
	if !ok {
		return
	}

	containerID, ok := requireContainerID(c)
	if !ok {
		return
	}

	details, err := client.GetContainerDetails(deploymentID, containerID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if details == nil {
		common.ApiErrorMsg(c, "container details not found")
		return
	}

	events := make([]map[string]interface{}, 0, len(details.ContainerEvents))
	for _, event := range details.ContainerEvents {
		events = append(events, map[string]interface{}{
			"time":    event.Time.Unix(),
			"message": event.Message,
		})
	}

	data := gin.H{
		"deployment_id":      deploymentID,
		"container_id":       details.ContainerID,
		"device_id":          details.DeviceID,
		"status":             strings.ToLower(strings.TrimSpace(details.Status)),
		"hardware":           details.Hardware,
		"brand_name":         details.BrandName,
		"created_at":         details.CreatedAt.Unix(),
		"uptime_percent":     details.UptimePercent,
		"gpus_per_container": details.GPUsPerContainer,
		"public_url":         details.PublicURL,
		"events":             events,
	}

	common.ApiSuccess(c, data)
}
