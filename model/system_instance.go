// Package model - system_instance.go
// 该文件定义系统实例心跳模型，用于多节点部署时展示每个节点的在线状态、
// 启动时间和运行资源快照。它是后续 SystemTask 调度观测的基础数据层。
package model

import (
	"github.com/c1cada/NexusTok/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SystemInstanceStatusOnline = "online"
	SystemInstanceStatusStale  = "stale"

	// SystemInstanceStaleAfterSeconds 表示多久未上报后将节点视为 stale。
	// 上报周期当前为 30 秒，因此 90 秒可以容忍一次短暂 GC、网络抖动或数据库慢查询。
	SystemInstanceStaleAfterSeconds int64 = 90
)

// SystemInstance 保存一个运行中服务节点的最近心跳。
//
// NodeName 是主键，来自 NODE_NAME 或主机名兜底；Info 使用 TEXT 保存 JSON，
// 避免 JSONB/JSON 列类型造成 SQLite、MySQL 5.7 和 PostgreSQL 的迁移差异。
type SystemInstance struct {
	NodeName   string `json:"node_name" gorm:"type:varchar(128);primaryKey"`
	Info       string `json:"info" gorm:"type:text"`
	StartedAt  int64  `json:"started_at" gorm:"bigint;index"`
	LastSeenAt int64  `json:"last_seen_at" gorm:"bigint;index"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt  int64  `json:"updated_at" gorm:"bigint;index"`
}

// SystemInstanceResponse 是前端展示使用的节点视图。
type SystemInstanceResponse struct {
	NodeName          string `json:"node_name"`
	Status            string `json:"status"`
	StaleAfterSeconds int64  `json:"stale_after_seconds"`
	StartedAt         int64  `json:"started_at"`
	LastSeenAt        int64  `json:"last_seen_at"`
	Info              any    `json:"info"`
}

// BeforeCreate 填充创建和更新时间戳，保持与项目内 Unix 秒时间戳风格一致。
func (instance *SystemInstance) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if instance.CreatedAt == 0 {
		instance.CreatedAt = now
	}
	if instance.UpdatedAt == 0 {
		instance.UpdatedAt = now
	}
	return nil
}

// UpsertSystemInstance 写入或刷新节点心跳。
//
// 该函数只使用 GORM OnConflict，不写数据库方言专用 SQL；冲突键为 node_name，
// 在 SQLite、MySQL 和 PostgreSQL 上均由 GORM 生成对应 upsert 语句。
func UpsertSystemInstance(nodeName string, info any, startedAt int64, lastSeenAt int64) error {
	infoText, err := marshalSystemInstanceInfo(info)
	if err != nil {
		return err
	}
	if lastSeenAt == 0 {
		lastSeenAt = common.GetTimestamp()
	}
	instance := &SystemInstance{
		NodeName:   nodeName,
		Info:       infoText,
		StartedAt:  startedAt,
		LastSeenAt: lastSeenAt,
		UpdatedAt:  lastSeenAt,
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "node_name"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"info",
			"started_at",
			"last_seen_at",
			"updated_at",
		}),
	}).Create(instance).Error
}

// ListSystemInstances 按最近心跳倒序列出所有节点。
func ListSystemInstances() ([]*SystemInstance, error) {
	var instances []*SystemInstance
	err := DB.Order("last_seen_at desc").Find(&instances).Error
	return instances, err
}

// ToResponse 将数据库记录转换成前端视图，并根据 now 判断在线状态。
func (instance *SystemInstance) ToResponse(now int64) SystemInstanceResponse {
	status := SystemInstanceStatusOnline
	if now-instance.LastSeenAt > SystemInstanceStaleAfterSeconds {
		status = SystemInstanceStatusStale
	}
	return SystemInstanceResponse{
		NodeName:          instance.NodeName,
		Status:            status,
		StaleAfterSeconds: SystemInstanceStaleAfterSeconds,
		StartedAt:         instance.StartedAt,
		LastSeenAt:        instance.LastSeenAt,
		Info:              decodeSystemInstanceInfo(instance.Info),
	}
}

func marshalSystemInstanceInfo(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	data, err := common.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeSystemInstanceInfo(data string) any {
	if data == "" {
		return nil
	}
	var value any
	if err := common.UnmarshalJsonStr(data, &value); err != nil {
		return data
	}
	return value
}
