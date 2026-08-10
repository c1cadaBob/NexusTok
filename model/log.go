// Package model - log.go
// 该文件定义了操作日志（Log）数据模型及相关操作
//
// 主要结构体：
// - Log：操作日志记录
// - RecordConsumeLogParams：消费日志记录参数
// - RecordTaskBillingLogParams：任务计费日志记录参数
// - Stat：统计信息（配额、RPM、TPM）
//
// 日志类型常量：
// - LogTypeTopup：充值日志
// - LogTypeConsume：消费日志
// - LogTypeManage：管理操作日志
// - LogTypeSystem：系统日志
// - LogTypeError：错误日志
// - LogTypeRefund：退款日志
// - LogTypeLogin：成功登录日志
//
// 核心功能：
// - 日志记录：支持多种类型的日志记录
// - 日志查询：支持按用户、模型、时间范围等条件查询
// - 统计分析：配额汇总、RPM（每分钟请求数）、TPM（每分钟 Token 数）
// - 日志清理：支持按时间戳删除旧日志
package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

// applyExplicitLogTextFilter 为日志查询追加文本过滤条件。
//
// value 为空时不追加过滤；包含 `%` 时视为用户显式请求模糊搜索，并走统一 LIKE 清洗；
// 不含 `%` 时使用等值匹配，避免模型名或用户名中的 `_` 被数据库误判为单字符通配符。
func applyExplicitLogTextFilter(tx *gorm.DB, column string, value string) (*gorm.DB, error) {
	if value == "" {
		return tx, nil
	}
	if strings.Contains(value, "%") {
		condition, pattern, err := buildLogLikeCondition(column, value)
		if err != nil {
			return nil, err
		}
		return tx.Where(condition, pattern), nil
	}
	return tx.Where(column+" = ?", value), nil
}

// buildLogLikeCondition 按日志库方言生成 LIKE 条件和安全搜索模式。
//
// SQLite/MySQL/PostgreSQL 使用 `!` 作为 ESCAPE 字符；未来 ClickHouse 日志库不支持同样的
// ESCAPE 子句，改用反斜杠转义 `_` 与反斜杠本身。两条路径都复用 validateLikePattern()
// 限制 `%` 通配符数量和关键词长度。
func buildLogLikeCondition(column string, value string) (string, string, error) {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		pattern, err := sanitizeClickHouseLikePattern(value)
		if err != nil {
			return "", "", err
		}
		return column + " LIKE ?", pattern, nil
	}

	pattern, err := sanitizeLikePattern(value)
	if err != nil {
		return "", "", err
	}
	return column + " LIKE ? ESCAPE '!'", pattern, nil
}

// sanitizeClickHouseLikePattern 清洗 ClickHouse LIKE 搜索模式。
//
// ClickHouse 使用反斜杠处理 LIKE 中的特殊字符；这里只保留 `%` 作为用户显式通配符，
// `_` 始终按普通字符匹配，避免模型名如 `gpt_4` 被误扩展为 `gpt?4`。
func sanitizeClickHouseLikePattern(input string) (string, error) {
	input = strings.ReplaceAll(input, `\`, `\\`)
	input = strings.ReplaceAll(input, `_`, `\_`)

	if err := validateLikePattern(input); err != nil {
		return "", err
	}
	return input, nil
}

type Log struct {
	Id                int    `json:"id" gorm:"index:idx_created_at_id,priority:1;index:idx_user_id_id,priority:2"`
	UserId            int    `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:2;index:idx_created_at_type"`
	Type              int    `json:"type" gorm:"index:idx_created_at_type"`
	Content           string `json:"content"`
	Username          string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName         string `json:"token_name" gorm:"index;default:''"`
	ModelName         string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota             int    `json:"quota" gorm:"default:0"`
	PromptTokens      int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens  int    `json:"completion_tokens" gorm:"default:0"`
	UseTime           int    `json:"use_time" gorm:"default:0"`
	IsStream          bool   `json:"is_stream"`
	ChannelId         int    `json:"channel" gorm:"index"`
	ChannelName       string `json:"channel_name" gorm:"->"`
	TokenId           int    `json:"token_id" gorm:"default:0;index"`
	Group             string `json:"group" gorm:"index"`
	Ip                string `json:"ip" gorm:"index;default:''"`
	RequestId         string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	UpstreamRequestId string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_logs_upstream_request_id;default:''"`
	Other             string `json:"other"`
}

// 日志类型值会持久化到数据库，禁止使用 iota，避免新增类型时意外改变历史含义。
const (
	LogTypeUnknown = 0
	LogTypeTopup   = 1
	LogTypeConsume = 2
	LogTypeManage  = 3
	LogTypeSystem  = 4
	LogTypeError   = 5
	LogTypeRefund  = 6
	LogTypeLogin   = 7
)

// ensureLogRequestId 为新日志补齐 request_id。
//
// 已有 request_id 必须原样保留，便于 relay 链路把上游或请求上下文中的追踪 ID 写入日志；
// 空值才生成本地 ID，为后续 ClickHouse 排序、审计定位和跨表排障提供稳定键。
func ensureLogRequestId(log *Log) {
	if log != nil && log.RequestId == "" {
		log.RequestId = common.GetTimeString() + common.GetRandomString(8)
	}
}

// createLog 统一写入日志并保证 request_id 不为空。
//
// 该包装保持现有 LOG_DB 写入路径不变，只在入库前补齐追踪 ID，避免各个 Record*Log
// 调用点重复处理。
func createLog(log *Log) error {
	ensureLogRequestId(log)
	return LOG_DB.Create(log).Error
}

// clickHouseLogOrder 返回 ClickHouse 日志查询的稳定排序表达式。
//
// ClickHouse logs 表不依赖传统自增 id，因此排序使用 created_at 与 request_id 组合；
// prefix 用于兼容 `logs.` 这样的表名前缀。
func clickHouseLogOrder(prefix string) string {
	return prefix + "created_at desc, " + prefix + "request_id desc"
}

// assignDisplayLogIds 为分页结果生成前端展示序号。
//
// 该序号只用于页面展示，不代表数据库主键；ClickHouse 独立日志库同样依赖这个函数隐藏
// 不同日志库底层主键策略差异，让前端分页不暴露底层排序实现。
func assignDisplayLogIds(logs []*Log, startIdx int) {
	for i := range logs {
		logs[i].Id = startIdx + i + 1
	}
}

// formatUserLogs 格式化用户日志（移除管理员敏感字段，设置序号）。
//
// 参数：
//   - logs: 日志列表
//   - startIdx: 起始序号
func formatUserLogs(logs []*Log, startIdx int) {
	for i := range logs {
		logs[i].ChannelName = ""
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// 移除仅管理员可见的调试和审计字段，避免普通用户日志接口泄露内部操作者、
			// 路由模板、路径参数、节点状态等排障信息。
			delete(otherMap, "admin_info")
			delete(otherMap, "audit_info")
			// delete(otherMap, "reject_reason")
			delete(otherMap, "stream_status")
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
	}
	assignDisplayLogIds(logs, startIdx)
}

// GetLogByTokenId 根据 Token ID 获取最近的日志记录
//
// 参数：
//   - tokenId: Token ID
//
// 返回值：
//   - logs: 日志列表
//   - err: 查询失败时返回错误
func GetLogByTokenId(tokenId int) (logs []*Log, err error) {
	order := "id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("")
	}
	// Token 日志用于调用方查看本 Token 的 API 请求历史，不能混入充值、管理审计等
	// 非调用类型记录，避免“使用日志”语义与管理员页面保持不一致。
	err = LOG_DB.Model(&Log{}).
		Where("token_id = ? AND type = ?", tokenId, LogTypeConsume).
		Order(order).
		Limit(common.MaxRecentItems).
		Find(&logs).Error
	formatUserLogs(logs, 0)
	return logs, err
}

// RecordLog 记录操作日志
// 消费日志需要开启 LogConsumeEnabled 配置才会记录
//
// 参数：
//   - userId: 用户 ID
//   - logType: 日志类型（LogTypeTopup、LogTypeConsume 等）
//   - content: 日志内容
func RecordLog(userId int, logType int, content string) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	err := createLog(log)
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

// RecordLogWithAdminInfo 记录操作日志，并将管理员相关信息存入 Other.admin_info
//
// 参数：
//   - userId: 用户 ID
//   - logType: 日志类型
//   - content: 日志内容
//   - adminInfo: 管理员信息（如服务器 IP、节点名称等）
func RecordLogWithAdminInfo(userId int, logType int, content string, adminInfo map[string]interface{}) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	if len(adminInfo) > 0 {
		other := map[string]interface{}{
			"admin_info": adminInfo,
		}
		log.Other = common.MapToJsonStr(other)
	}
	if err := createLog(log); err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

// OperationAuditLogParams 描述一条管理操作审计日志。
//
// 设计约束：
//   - UserId 必须是实际操作者，而不是被操作资源的用户 ID；目标资源应放入 Params。
//   - Params 只保存语言无关、非敏感的结构化参数，供前端本地化和后续排障使用。
//   - AdminInfo 写入 Other.admin_info，仅管理员日志视图可见。
//   - AuditInfo 写入 Other.audit_info，仅管理员日志视图可见，用于保存兜底中间件捕获的路由、
//     状态码、业务 success 结果和路径参数；不要放请求体或密钥类字段。
type OperationAuditLogParams struct {
	UserId    int
	Content   string
	Ip        string
	Action    string
	Params    map[string]interface{}
	AdminInfo map[string]interface{}
	AuditInfo map[string]interface{}
}

// LoginLogParams 描述一条成功登录审计日志。
//
// 设计约束：
//   - 仅记录成功登录，不记录失败用户名或失败原因，避免制造账号枚举信号。
//   - Extra 只放普通用户也可见的轻量元数据，例如 login_method、user_agent。
//   - Content 是导出和旧前端兜底文本；前端优先读取 Other.op 和 Extra 中的结构化字段。
type LoginLogParams struct {
	UserId   int
	Username string
	Content  string
	Ip       string
	Action   string
	Params   map[string]interface{}
	Extra    map[string]interface{}
}

// buildOperationAuditOpField 构建语言无关的操作描述。
// 前端可以依据 action 和 params 渲染本地化文案；Content 只作为导出、旧前端或解析失败时的兜底文本。
func buildOperationAuditOpField(action string, params map[string]interface{}) map[string]interface{} {
	op := map[string]interface{}{
		"action": action,
	}
	if len(params) > 0 {
		op["params"] = params
	}
	return op
}

// RecordLoginLog 记录用户成功登录审计日志。
//
// 该函数复用 logs 表和 Other 文本 JSON 字段，不新增数据库结构；写入失败只记录系统日志，
// 不影响登录主流程。调用方应传入已知用户名，避免在登录成功路径中额外查询。
func RecordLoginLog(params LoginLogParams) {
	if params.Action == "" {
		params.Action = "login"
	}
	other := map[string]interface{}{}
	for key, value := range params.Extra {
		other[key] = value
	}
	other["op"] = buildOperationAuditOpField(params.Action, params.Params)
	log := &Log{
		UserId:    params.UserId,
		Username:  params.Username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeLogin,
		Content:   params.Content,
		Ip:        params.Ip,
		Other:     common.MapToJsonStr(other),
	}
	if err := createLog(log); err != nil {
		common.SysLog("failed to record login log: " + err.Error())
	}
}

// RecordOperationAuditLog 记录管理操作审计日志。
//
// 该函数复用 logs 表和 LogTypeManage，不新增数据库表或 JSON 类型列，确保 SQLite、
// MySQL 和 PostgreSQL 均按现有文本 JSON 方式兼容。调用方必须保证传入的 Params、
// AdminInfo 和 AuditInfo 已经去除敏感字段；本函数不会读取请求体，也不会做深度脱敏。
func RecordOperationAuditLog(params OperationAuditLogParams) {
	if params.Action == "" {
		params.Action = "generic"
	}
	username, _ := GetUsernameById(params.UserId, false)
	other := map[string]interface{}{
		"op": buildOperationAuditOpField(params.Action, params.Params),
	}
	if len(params.AdminInfo) > 0 {
		other["admin_info"] = params.AdminInfo
	}
	if len(params.AuditInfo) > 0 {
		other["audit_info"] = params.AuditInfo
	}
	log := &Log{
		UserId:    params.UserId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeManage,
		Content:   params.Content,
		Ip:        params.Ip,
		Other:     common.MapToJsonStr(other),
	}
	if err := createLog(log); err != nil {
		common.SysLog("failed to record operation audit log: " + err.Error())
	}
}

// RecordTopupLog 记录充值日志
// 包含服务器 IP、节点名称、调用者 IP、支付方式等管理员信息
//
// 参数：
//   - userId: 用户 ID
//   - content: 日志内容
//   - callerIp: 调用者 IP 地址
//   - paymentMethod: 支付方式
//   - callbackPaymentMethod: 回调支付方式
func RecordTopupLog(userId int, content string, callerIp string, paymentMethod string, callbackPaymentMethod string) {
	RecordTopupLogWithAdminInfo(userId, content, callerIp, paymentMethod, callbackPaymentMethod, nil)
}

// buildTopupAdminInfo 构造充值日志的管理员审计信息。
// extraAdminInfo 会覆盖同名字段，便于异常路径附加 quota_saturation 等结构化信息；
// 常规调用方传 nil 时保持历史日志结构不变。
func buildTopupAdminInfo(callerIp string, paymentMethod string, callbackPaymentMethod string, extraAdminInfo map[string]interface{}) map[string]interface{} {
	adminInfo := map[string]interface{}{
		"server_ip":               common.GetIp(),
		"node_name":               common.NodeName,
		"caller_ip":               callerIp,
		"payment_method":          paymentMethod,
		"callback_payment_method": callbackPaymentMethod,
		"version":                 common.Version,
	}
	for key, value := range extraAdminInfo {
		adminInfo[key] = value
	}
	return adminInfo
}

// RecordTopupLogWithAdminInfo 记录充值日志，并允许调用方附加充值路径专属管理员审计信息。
// 基础 admin_info 会始终包含节点、调用者 IP 和支付方式；extraAdminInfo 只用于
// 额度饱和等少数异常场景，普通用户日志视图会移除 admin_info，避免暴露内部细节。
func RecordTopupLogWithAdminInfo(userId int, content string, callerIp string, paymentMethod string, callbackPaymentMethod string, extraAdminInfo map[string]interface{}) {
	username, _ := GetUsernameById(userId, false)
	adminInfo := buildTopupAdminInfo(callerIp, paymentMethod, callbackPaymentMethod, extraAdminInfo)
	other := map[string]interface{}{
		"admin_info": adminInfo,
	}
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   content,
		Ip:        callerIp,
		Other:     common.MapToJsonStr(other),
	}
	err := createLog(log)
	if err != nil {
		common.SysLog("failed to record topup log: " + err.Error())
	}
}

// RecordErrorLog 记录错误日志
// 包含请求上下文、渠道信息、模型信息等详细信息
//
// 参数：
//   - c: Gin 上下文
//   - userId: 用户 ID
//   - channelId: 渠道 ID
//   - modelName: 模型名称
//   - tokenName: Token 名称
//   - content: 错误内容
//   - tokenId: Token ID
//   - useTimeSeconds: 使用时间（秒）
//   - isStream: 是否为流式请求
//   - group: 用户组
//   - other: 其他信息
func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, content))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	otherStr := common.MapToJsonStr(other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     0,
		CompletionTokens: 0,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId:         requestId,
		UpstreamRequestId: upstreamRequestId,
		Other:             otherStr,
	}
	err := createLog(log)
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

type RecordConsumeLogParams struct {
	ChannelId        int                    `json:"channel_id"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	ModelName        string                 `json:"model_name"`
	TokenName        string                 `json:"token_name"`
	Quota            int                    `json:"quota"`
	Content          string                 `json:"content"`
	TokenId          int                    `json:"token_id"`
	UseTimeSeconds   int                    `json:"use_time_seconds"`
	IsStream         bool                   `json:"is_stream"`
	Group            string                 `json:"group"`
	Other            map[string]interface{} `json:"other"`
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	logger.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, params=%s", userId, common.GetJsonString(params)))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	otherStr := common.MapToJsonStr(params.Other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId:         requestId,
		UpstreamRequestId: upstreamRequestId,
		Other:             otherStr,
	}
	err := createLog(log)
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
	if common.DataExportEnabled {
		gopool.Go(func() {
			LogQuotaData(QuotaDataLogParams{
				UserID:    userId,
				Username:  username,
				NodeName:  common.NodeName,
				TokenID:   params.TokenId,
				UseGroup:  params.Group,
				ChannelID: params.ChannelId,
				ModelName: params.ModelName,
				Quota:     params.Quota,
				CreatedAt: common.GetTimestamp(),
				TokenUsed: params.PromptTokens + params.CompletionTokens,
			})
		})
	}
}

type RecordTaskBillingLogParams struct {
	UserId    int
	LogType   int
	Content   string
	ChannelId int
	ModelName string
	Quota     int
	TokenId   int
	Group     string
	Other     map[string]interface{}
}

func RecordTaskBillingLog(params RecordTaskBillingLogParams) {
	if params.LogType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(params.UserId, false)
	tokenName := ""
	if params.TokenId > 0 {
		if token, err := GetTokenById(params.TokenId); err == nil {
			tokenName = token.Name
		}
	}
	log := &Log{
		UserId:    params.UserId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      params.LogType,
		Content:   params.Content,
		TokenName: tokenName,
		ModelName: params.ModelName,
		Quota:     params.Quota,
		ChannelId: params.ChannelId,
		TokenId:   params.TokenId,
		Group:     params.Group,
		Other:     common.MapToJsonStr(params.Other),
	}
	err := createLog(log)
	if err != nil {
		common.SysLog("failed to record task billing log: " + err.Error())
	}
}

// GetAllLogs 保留按单一日志类型查询的通用能力，供内部兼容调用使用。
//
// 面向页面的消费日志与审计日志必须使用 GetConsumeLogs、GetAuditLogs，避免调用点
// 重新开放“所有日志类型”而把两类页面语义混在一起。
func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	var logTypes []int
	if logType != LogTypeUnknown {
		logTypes = []int{logType}
	}
	return getAllLogsByTypes(logTypes, startTimestamp, endTimestamp, modelName, username, tokenName, startIdx, num, channel, group, requestId, upstreamRequestId)
}

// GetConsumeLogs 查询 API 调用产生的消费日志。
//
// 使用日志和跨用户用量查看均依赖该入口，类型在模型层固定，避免客户端 query 参数
// 篡改或未来控制器遗漏筛选时暴露管理、充值等不属于 API 调用的记录。
func GetConsumeLogs(startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	return getAllLogsByTypes(
		[]int{LogTypeConsume},
		startTimestamp,
		endTimestamp,
		modelName,
		username,
		tokenName,
		startIdx,
		num,
		channel,
		group,
		requestId,
		upstreamRequestId,
	)
}

// GetAuditLogs 查询管理员操作和成功登录审计记录。
//
// 两种类型均使用结构化 Other 字段记录操作者、操作参数和登录元数据。查询范围在模型层
// 固定，既避免 API 调用日志混入审计页，也避免后续页面误把充值、系统或错误日志当审计证据。
func GetAuditLogs(startTimestamp int64, endTimestamp int64, username string, startIdx int, num int, requestId string) (logs []*Log, total int64, err error) {
	return getAllLogsByTypes(
		[]int{LogTypeManage, LogTypeLogin},
		startTimestamp,
		endTimestamp,
		"",
		username,
		"",
		startIdx,
		num,
		0,
		"",
		requestId,
		"",
	)
}

func getAllLogsByTypes(logTypes []int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	tx := LOG_DB
	switch len(logTypes) {
	case 1:
		tx = tx.Where("logs.type = ?", logTypes[0])
	case 2:
		tx = tx.Where("logs.type IN ?", logTypes)
	case 0:
		// 空切片仅供兼容旧调用，表示不额外限制类型。面向页面的入口不会走此分支。
	default:
		tx = tx.Where("logs.type IN ?", logTypes)
	}

	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, 0, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", username); err != nil {
		return nil, 0, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if upstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", upstreamRequestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	order := "logs.created_at desc, logs.id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("logs.")
	}
	err = tx.Order(order).Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if common.MemoryCacheEnabled {
			// 优先从内存缓存读取渠道名称，减少后台日志列表的数据库查询。
			for _, channelId := range channelIds.Items() {
				if cacheChannel, err := CacheGetChannel(channelId); err == nil {
					channels = append(channels, struct {
						Id   int    `gorm:"column:id"`
						Name string `gorm:"column:name"`
					}{
						Id:   channelId,
						Name: cacheChannel.Name,
					})
				}
			}
		} else {
			// 未启用缓存时批量查询渠道名称，避免按日志逐条查询。
			if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
				return logs, total, err
			}
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, total, err
}

const logSearchCountLimit = 10000

// GetUserConsumeLogs 查询当前用户的 API 调用消费日志。
//
// 用户可见的使用日志只表达请求使用量，因此即使调用方携带旧版 type 参数，也不能通过
// 该入口查看管理、充值或系统类型日志。
func GetUserConsumeLogs(userId int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	return GetUserLogs(
		userId,
		LogTypeConsume,
		startTimestamp,
		endTimestamp,
		modelName,
		tokenName,
		startIdx,
		num,
		group,
		requestId,
		upstreamRequestId,
	)
}

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB.Where("logs.user_id = ?", userId)
	} else {
		tx = LOG_DB.Where("logs.user_id = ? and logs.type = ?", userId, logType)
	}

	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, 0, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if upstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", upstreamRequestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Limit(logSearchCountLimit).Count(&total).Error
	if err != nil {
		common.SysError("failed to count user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	order := "logs.id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("logs.")
	}
	err = tx.Order(order).Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		common.SysError("failed to search user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}

	formatUserLogs(logs, startIdx)
	return logs, total, err
}

type Stat struct {
	Quota int `json:"quota"`
	Rpm   int `json:"rpm"`
	Tpm   int `json:"tpm"`
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) (stat Stat, err error) {
	tx := LOG_DB.Table("logs").Select("COALESCE(sum(quota), 0) quota")

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, COALESCE(sum(prompt_tokens), 0) + COALESCE(sum(completion_tokens), 0) tpm")

	if tx, err = applyExplicitLogTextFilter(tx, "username", username); err != nil {
		return stat, err
	}
	if rpmTpmQuery, err = applyExplicitLogTextFilter(rpmTpmQuery, "username", username); err != nil {
		return stat, err
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
		rpmTpmQuery = rpmTpmQuery.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if tx, err = applyExplicitLogTextFilter(tx, "model_name", modelName); err != nil {
		return stat, err
	}
	if rpmTpmQuery, err = applyExplicitLogTextFilter(rpmTpmQuery, "model_name", modelName); err != nil {
		return stat, err
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
		rpmTpmQuery = rpmTpmQuery.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
		rpmTpmQuery = rpmTpmQuery.Where(logGroupCol+" = ?", group)
	}

	tx = tx.Where("type = ?", LogTypeConsume)
	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume)

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	// 执行查询
	if err := tx.Scan(&stat).Error; err != nil {
		common.SysError("failed to query log stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	if err := rpmTpmQuery.Scan(&stat).Error; err != nil {
		common.SysError("failed to query rpm/tpm stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}

	return stat, nil
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := LOG_DB.Table("logs").Select("COALESCE(sum(prompt_tokens), 0) + COALESCE(sum(completion_tokens), 0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	var total int64 = 0

	for {
		if nil != ctx.Err() {
			return total, ctx.Err()
		}

		rowsAffected, err := DeleteOldLogBatch(ctx, targetTimestamp, limit)
		if nil != err {
			return total, err
		}

		total += rowsAffected

		if rowsAffected < int64(limit) {
			break
		}
	}

	return total, nil
}

// CountOldLog 统计目标时间戳之前的日志数量。
//
// SystemTask 日志清理任务用该函数初始化和刷新进度。查询使用 LOG_DB.WithContext，
// 让后台任务在租约丢失或服务关闭时可以尽快响应 context cancellation。
func CountOldLog(ctx context.Context, targetTimestamp int64) (int64, error) {
	var total int64
	err := LOG_DB.WithContext(ctx).Model(&Log{}).Where("created_at < ?", targetTimestamp).Count(&total).Error
	if err != nil {
		return 0, err
	}
	return total, nil
}

// DeleteOldLogBatch 删除一批目标时间戳之前的日志。
//
// limit 小于等于 0 时回退到 100，保持旧版 DeleteOldLog 的批量粒度。这里不使用
// 数据库方言专用 SQL，避免破坏 SQLite、MySQL 和 PostgreSQL 三库兼容性。
func DeleteOldLogBatch(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		// ClickHouse 的 DELETE 是表 mutation，会重写数据分区；如果像传统数据库一样
		// 按 id 小批量删除，会产生大量 mutation 并拖慢后台合并。这里先统计数量，再用
		// 一次同步 mutation 删除全部过期日志，让 SystemTask 的进度循环可以一次完成。
		total, err := CountOldLog(ctx, targetTimestamp)
		if err != nil {
			return 0, err
		}
		if total == 0 {
			return 0, nil
		}
		if err := LOG_DB.WithContext(ctx).Exec(
			"ALTER TABLE logs DELETE WHERE created_at < ? SETTINGS mutations_sync = 1",
			targetTimestamp,
		).Error; err != nil {
			return 0, err
		}
		return total, nil
	}

	var ids []int
	if err := LOG_DB.WithContext(ctx).
		Model(&Log{}).
		Where("created_at < ?", targetTimestamp).
		Order("id asc").
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := LOG_DB.WithContext(ctx).Where("id IN ?", ids).Delete(&Log{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
