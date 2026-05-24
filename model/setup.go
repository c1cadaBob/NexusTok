// Package model - setup.go
// 该文件定义了系统初始化（Setup）数据模型及相关操作
//
// 主要结构体：
// - Setup：系统初始化记录
//
// 核心功能：
// - 记录系统初始化状态和版本信息
// - 判断系统是否已完成初始化
// - Root 用户存在性检查
package model

type Setup struct {
	ID            uint   `json:"id" gorm:"primaryKey"`
	Version       string `json:"version" gorm:"type:varchar(50);not null"`
	InitializedAt int64  `json:"initialized_at" gorm:"type:bigint;not null"`
}

func GetSetup() *Setup {
	var setup Setup
	err := DB.First(&setup).Error
	if err != nil {
		return nil
	}
	return &setup
}
