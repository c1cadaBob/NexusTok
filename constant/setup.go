// Package constant - setup.go
// 该文件定义了系统初始化状态标志
package constant

// Setup 系统是否已完成初始化设置
//
// 在首次运行时，系统需要进行初始化设置（创建管理员账户、配置基础参数等）
// 此变量在初始化完成后设置为 true
//
// 用于：
// - 检测系统是否需要初始化
// - 防止重复初始化
// - 前端展示初始化引导页面
var Setup = false
