// Package service 提供业务逻辑层服务
// 本文件为 Waffo Pancake 支付集成的单元测试
package service

import (
	"fmt"     // 格式化输出
	"strings" // 字符串操作
	"testing" // 测试框架
	"time"    // 时间处理

	"github.com/c1cada/NexusTok/common"   // 公共工具包
	"github.com/c1cada/NexusTok/model"    // 数据模型
	"github.com/c1cada/NexusTok/setting"  // 配置管理
	"github.com/glebarez/sqlite"          // SQLite 驱动
	"github.com/stretchr/testify/require" // 测试断言
	"gorm.io/gorm"                        // ORM 框架
)

// setupWaffoPancakeTestDB 设置 Waffo Pancake 测试数据库
// 创建内存 SQLite 数据库并自动迁移所需的表结构
//
// 参数：
//   - t: 测试实例
//
// 返回值：
//   - *gorm.DB: 初始化完成的数据库连接
func setupWaffoPancakeTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	// 配置使用 SQLite 数据库
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	// 创建内存数据库（使用测试名称作为唯一标识，避免并行测试冲突）
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	// 设置全局数据库实例
	model.DB = db
	model.LOG_DB = db

	// 自动迁移用户、充值表和订阅订单表
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.SubscriptionOrder{}))

	// 注册清理函数，测试结束后关闭数据库连接
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

// TestWaffoPancakeCreateSessionResponseParsesDocumentedPayload 测试 Waffo Pancake 创建会话响应的 JSON 解析
// 验证官方文档中的标准响应格式能被正确解析
func TestWaffoPancakeCreateSessionResponseParsesDocumentedPayload(t *testing.T) {
	var result waffoPancakeCreateSessionResponse
	// 解析官方文档示例的 JSON 响应
	err := common.Unmarshal([]byte(`{
		"data": {
			"sessionId": "cs_550e8400-e29b-41d4-a716-446655440000",
			"checkoutUrl": "https://checkout.waffo.ai/my-store-abc123/checkout/cs_550e8400-e29b-41d4-a716-446655440000",
			"expiresAt": "2026-01-22T10:30:00.000Z"
		}
	}`), &result)
	require.NoError(t, err)
	require.NotNil(t, result.Data)
	require.Equal(t, "cs_550e8400-e29b-41d4-a716-446655440000", result.Data.SessionID)
	require.Empty(t, result.Data.OrderID) // 创建会话响应中不应包含 OrderID
}

// TestResolveWaffoPancakeTradeNo_UsesWebhookOrderIDWhenLocalOrderExists 测试当本地订单存在时使用 Webhook 的 OrderID
// 验证迁移前旧 checkout 的 Webhook 回调仍可用 OrderID 匹配本地充值订单。
func TestResolveWaffoPancakeTradeNo_UsesWebhookOrderIDWhenLocalOrderExists(t *testing.T) {
	db := setupWaffoPancakeTestDB(t)

	// 创建待支付的充值订单
	topUp := &model.TopUp{
		UserId:          1,
		Amount:          10,
		Money:           29,
		TradeNo:         "ORD_5dXBtmF2HLlHfbPNm0Wcnz",
		PaymentMethod:   model.PaymentMethodWaffoPancake,
		PaymentProvider: model.PaymentProviderWaffoPancake,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(topUp).Error)

	// 使用 Webhook 回调的 OrderID 解析交易号
	tradeNo, err := ResolveWaffoPancakeTradeNo(&waffoPancakeWebhookEvent{
		Data: waffoPancakeWebhookData{
			OrderID: "ORD_5dXBtmF2HLlHfbPNm0Wcnz",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "ORD_5dXBtmF2HLlHfbPNm0Wcnz", tradeNo)
}

// TestResolveWaffoPancakeTradeNo_UsesExternalIDAndBuyerIdentity 测试新 checkout 外部单号解析。
// 新路径必须使用 orderMerchantExternalId 匹配本地 trade_no，并校验 buyer identity 防串单。
func TestResolveWaffoPancakeTradeNo_UsesExternalIDAndBuyerIdentity(t *testing.T) {
	db := setupWaffoPancakeTestDB(t)

	topUp := &model.TopUp{
		UserId:          42,
		Amount:          10,
		Money:           29,
		TradeNo:         "WAFFO_PANCAKE-42-123456-abc123",
		PaymentMethod:   model.PaymentMethodWaffoPancake,
		PaymentProvider: model.PaymentProviderWaffoPancake,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(topUp).Error)

	tradeNo, err := ResolveWaffoPancakeTradeNo(&waffoPancakeWebhookEvent{
		Data: waffoPancakeWebhookData{
			OrderID:                       "ORD_remote",
			OrderMerchantExternalID:       topUp.TradeNo,
			MerchantProvidedBuyerIdentity: WaffoPancakeBuyerIdentityFromUserID(topUp.UserId),
		},
	})
	require.NoError(t, err)
	require.Equal(t, topUp.TradeNo, tradeNo)

	tradeNo, err = ResolveWaffoPancakeTradeNo(&waffoPancakeWebhookEvent{
		Data: waffoPancakeWebhookData{
			OrderID:                       "ORD_remote",
			OrderMerchantExternalID:       topUp.TradeNo,
			MerchantProvidedBuyerIdentity: WaffoPancakeBuyerIdentityFromUserID(99),
		},
	})
	require.Error(t, err)
	require.Empty(t, tradeNo)
}

// TestResolveWaffoPancakeSubscriptionTradeNo_RequiresExternalIDAndIdentity 测试订阅订单解析。
// 订阅不接受旧 OrderID fallback，必须使用外部单号和稳定 buyer identity。
func TestResolveWaffoPancakeSubscriptionTradeNo_RequiresExternalIDAndIdentity(t *testing.T) {
	db := setupWaffoPancakeTestDB(t)

	order := &model.SubscriptionOrder{
		UserId:          42,
		PlanId:          7,
		Money:           9.99,
		TradeNo:         "WAFFO_PANCAKE_SUB-42-123456-abc123",
		PaymentMethod:   model.PaymentMethodWaffoPancake,
		PaymentProvider: model.PaymentProviderWaffoPancake,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(order).Error)

	tradeNo, err := ResolveWaffoPancakeSubscriptionTradeNo(&waffoPancakeWebhookEvent{
		Data: waffoPancakeWebhookData{
			OrderID:                       "ORD_remote",
			OrderMerchantExternalID:       order.TradeNo,
			MerchantProvidedBuyerIdentity: WaffoPancakeBuyerIdentityFromUserID(order.UserId),
		},
	})
	require.NoError(t, err)
	require.Equal(t, order.TradeNo, tradeNo)

	tradeNo, err = ResolveWaffoPancakeSubscriptionTradeNo(&waffoPancakeWebhookEvent{
		Data: waffoPancakeWebhookData{
			OrderID: "ORD_remote",
		},
	})
	require.Error(t, err)
	require.Empty(t, tradeNo)
}

// TestResolveWaffoPancakeTradeNo_FailsWhenWebhookOrderIDIsUnknown 测试当 Webhook OrderID 未知时解析失败
// 验证当 Webhook 回调的 OrderID 在本地不存在时返回错误
func TestResolveWaffoPancakeTradeNo_FailsWhenWebhookOrderIDIsUnknown(t *testing.T) {
	db := setupWaffoPancakeTestDB(t)

	// 创建测试用户
	user := &model.User{
		Id:       42,
		Email:    "buyer@example.com",
		Username: "buyer",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(user).Error)

	// 创建待支付的充值订单（使用不同的交易号）
	topUp := &model.TopUp{
		UserId:          user.Id,
		Amount:          10,
		Money:           29,
		TradeNo:         "WAFFO_PANCAKE-42-123456-abc123",
		PaymentMethod:   model.PaymentMethodWaffoPancake,
		PaymentProvider: model.PaymentProviderWaffoPancake,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(topUp).Error)

	// 使用未知的 OrderID 尝试解析，应返回错误
	tradeNo, err := ResolveWaffoPancakeTradeNo(&waffoPancakeWebhookEvent{
		Data: waffoPancakeWebhookData{
			OrderID:    "ORD_unknown",
			BuyerEmail: user.Email,
			Amount:     "29.00",
		},
	})
	require.Error(t, err)
	require.Empty(t, tradeNo)
}

// TestResolveWaffoPancakeWebhookEnvironment 测试 Webhook 环境解析逻辑
// 验证不同 mode 值和 sandbox 配置下的环境解析行为
func TestResolveWaffoPancakeWebhookEnvironment(t *testing.T) {
	// 保存原始 sandbox 配置，测试结束后恢复
	originalSandbox := setting.WaffoPancakeSandbox
	t.Cleanup(func() {
		setting.WaffoPancakeSandbox = originalSandbox
	})

	// 定义测试用例：不同 mode 值和 sandbox 配置的组合
	testCases := []struct {
		name     string // 测试用例名称
		payload  string // Webhook 请求体 JSON
		expected string // 期望的环境值
		sandbox  bool   // sandbox 配置
	}{
		{
			name:     "test mode",
			payload:  `{"mode":"test"}`,
			expected: "test",
		},
		{
			name:     "prod mode",
			payload:  `{"mode":"prod"}`,
			expected: "prod",
		},
		{
			name:     "missing mode falls back to sandbox",
			payload:  `{}`,
			expected: "test",
			sandbox:  true, // sandbox 为 true 时，缺失 mode 回退到 test
		},
		{
			name:     "invalid mode falls back to prod",
			payload:  `{"mode":"staging"}`,
			expected: "prod", // 无效的 mode 值回退到 prod
		},
	}

	// 遍历执行所有测试用例
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setting.WaffoPancakeSandbox = tc.sandbox
			environment := resolveWaffoPancakeWebhookEnvironment(tc.payload)
			require.Equal(t, tc.expected, environment)
		})
	}
}
