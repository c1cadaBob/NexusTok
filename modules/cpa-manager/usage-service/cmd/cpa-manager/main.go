// main - main.go
// CPA Manager 使用量采集服务的程序入口。
// 负责加载配置、初始化数据库存储、启动使用量采集管理器和 HTTP API 服务，
// 并处理操作系统的优雅关闭信号（SIGINT/SIGTERM）。
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/seakee/cpa-manager/usage-service/internal/collector"
	"github.com/seakee/cpa-manager/usage-service/internal/config"
	"github.com/seakee/cpa-manager/usage-service/internal/httpapi"
	"github.com/seakee/cpa-manager/usage-service/internal/store"
)

// main 是程序入口函数，按以下顺序完成初始化：
// 1. 加载配置（支持配置文件和环境变量）
// 2. 打开 SQLite 数据库连接
// 3. 创建采集管理器（Manager）
// 4. 注册操作系统中断信号以支持优雅关闭
// 5. 按优先级尝试从不同来源加载上游 CPA 连接配置并启动采集器
// 6. 启动 HTTP API 服务
// 7. 等待关闭信号后执行优雅关停
func main() {
	// 加载配置，支持配置文件和环境变量两种方式
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	// 打开 SQLite 数据库，自动创建目录和数据库文件
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	// 创建采集管理器实例，负责从上游 CPA 拉取使用量数据
	manager := collector.NewManager(cfg, db)
	// 注册 SIGINT 和 SIGTERM 信号，收到后通过 ctx 取消通知所有子组件退出
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 按优先级尝试三种配置来源来启动采集器：
	// 1. 配置文件/环境变量中直接指定的 CPA 上游地址和管理密钥
	// 2. 数据库中保存的 ManagerConfig 配置
	// 3. 数据库中保存的 Setup 配置（旧版兼容）
	if cfg.CPAUpstreamURL != "" && cfg.ManagementKey != "" {
		// 优先使用配置文件/环境变量中的设置
		manager.Start(ctx, collector.RuntimeConfig{
			CPAUpstreamURL: cfg.CPAUpstreamURL,
			ManagementKey:  cfg.ManagementKey,
			CollectorMode:  cfg.CollectorMode,
			Queue:          cfg.Queue,
			PopSide:        cfg.PopSide,
			BatchSize:      cfg.BatchSize,
			PollInterval:   cfg.PollInterval,
			TLSSkipVerify:  cfg.TLSSkipVerify,
		})
	} else if managerCfg, ok, err := db.LoadManagerConfig(ctx); err == nil && ok &&
		managerCfg.CPAConnection.CPABaseURL != "" && managerCfg.CPAConnection.ManagementKey != "" {
		// 其次尝试从数据库加载 ManagerConfig 配置
		if managerCollectorEnabled(managerCfg) {
			manager.Start(ctx, runtimeConfigFromManagerConfig(managerCfg, cfg))
		}
	} else if setup, ok, err := db.LoadSetup(ctx); err == nil && ok {
		// 最后尝试从数据库加载旧版 Setup 配置
		manager.Start(ctx, collector.RuntimeConfig{
			CPAUpstreamURL: setup.CPAUpstreamURL,
			ManagementKey:  setup.ManagementKey,
			CollectorMode:  cfg.CollectorMode,
			Queue:          setup.Queue,
			PopSide:        setup.PopSide,
			BatchSize:      cfg.BatchSize,
			PollInterval:   cfg.PollInterval,
			TLSSkipVerify:  cfg.TLSSkipVerify,
		})
	} else if err != nil {
		log.Printf("load setup: %v", err)
	}

	// 创建 HTTP 服务器，注册所有 API 路由
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.New(cfg, db, manager).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// 在独立 goroutine 中启动 HTTP 服务，避免阻塞主 goroutine
	go func() {
		log.Printf("cpa-manager listening on %s", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	// 阻塞等待关闭信号
	<-ctx.Done()
	// 收到信号后给予 10 秒宽限期完成正在处理的请求
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// 停止采集管理器
	manager.Stop()
	// 优雅关闭 HTTP 服务器
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// runtimeConfigFromManagerConfig 从数据库中存储的 ManagerConfig 构建采集器运行时配置。
// 参数 base 提供配置文件中的默认值作为回退，当 ManagerConfig 中某些字段未设置时使用 base 的对应值。
// 返回值用于启动采集管理器（Manager）。
func runtimeConfigFromManagerConfig(managerCfg store.ManagerConfig, base config.Config) collector.RuntimeConfig {
	// 将毫秒转换为 time.Duration，无效值回退到 base 配置
	pollInterval := time.Duration(managerCfg.Collector.PollIntervalMS) * time.Millisecond
	if pollInterval <= 0 {
		pollInterval = base.PollInterval
	}
	// 批次大小，无效值回退到 base 配置
	batchSize := managerCfg.Collector.BatchSize
	if batchSize <= 0 {
		batchSize = base.BatchSize
	}
	return collector.RuntimeConfig{
		CPAUpstreamURL: managerCfg.CPAConnection.CPABaseURL,
		ManagementKey:  managerCfg.CPAConnection.ManagementKey,
		CollectorMode:  valueOr(managerCfg.Collector.CollectorMode, base.CollectorMode),
		Queue:          valueOr(managerCfg.Collector.Queue, base.Queue),
		PopSide:        valueOr(managerCfg.Collector.PopSide, base.PopSide),
		BatchSize:      batchSize,
		PollInterval:   pollInterval,
		TLSSkipVerify:  managerCfg.Collector.TLSSkipVerify,
	}
}

// valueOr 返回 value；若 value 为空字符串则返回 fallback 作为默认值。
// 用于配置项的级联回退逻辑。
func valueOr(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// managerCollectorEnabled 判断 ManagerConfig 中的采集器是否处于启用状态。
// 当 Enabled 字段为 nil 时默认视为已启用（向后兼容）。
func managerCollectorEnabled(managerCfg store.ManagerConfig) bool {
	return managerCfg.Collector.Enabled == nil || *managerCfg.Collector.Enabled
}
