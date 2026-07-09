// waffo_pancake.go 实现了 Waffo Pancake 支付服务的集成。
// 包括创建结账会话、Webhook 签名验证、RSA 密钥处理、
// 请求签名构建和 Webhook 事件解析等功能。
package service

import (
	"context"         // 请求上下文
	"crypto"          // 哈希算法标识
	"crypto/rsa"      // RSA 密码学操作
	"crypto/sha256"   // SHA-256 哈希
	"crypto/x509"     // 证书和密钥解析
	"encoding/base64" // Base64 编解码
	"encoding/pem"    // PEM 格式编解码
	"fmt"             // 格式化输出
	"math"            // 数学函数（Abs）
	"strconv"         // 字符串转换
	"strings"         // 字符串操作
	"time"            // 时间操作

	"github.com/c1cada/NexusTok/common"  // 公共工具：JSON 序列化等
	"github.com/c1cada/NexusTok/dto"     // 数据传输对象
	"github.com/c1cada/NexusTok/model"   // 数据模型：TopUp 等
	"github.com/c1cada/NexusTok/setting" // 系统配置
	pancake "github.com/waffo-com/waffo-pancake-sdk-go"
)

// Waffo Pancake 支付服务的常量配置
const (
	waffoPancakeDefaultTolerance   = 5 * time.Minute           // Webhook 时间戳容差（5分钟）
	defaultWaffoPancakeStoreName   = "nexustok-store"          // 管理端一键创建的默认店铺名
	defaultWaffoPancakeProductName = "nexustok-charge-product" // 管理端一键创建的钱包充值商品名
)

// WaffoPancakePriceSnapshot 结账价格快照
type WaffoPancakePriceSnapshot struct {
	Amount      string `json:"amount"`      // 金额
	TaxIncluded bool   `json:"taxIncluded"` // 是否含税
	TaxCategory string `json:"taxCategory"` // 税种分类
}

// WaffoPancakeCreateSessionParams 创建结账会话的请求参数
type WaffoPancakeCreateSessionParams struct {
	StoreID                 string                     `json:"storeId"`                           // 商店 ID，保留给旧调用方语义和日志，不再参与 SDK checkout
	ProductID               string                     `json:"productId"`                         // 产品 ID
	ProductType             string                     `json:"productType"`                       // 产品类型，当前运行时固定使用 onetime product
	Currency                string                     `json:"currency"`                          // 货币代码
	PriceSnapshot           *WaffoPancakePriceSnapshot `json:"priceSnapshot,omitempty"`           // 价格快照
	BuyerEmail              string                     `json:"buyerEmail,omitempty"`              // 买家邮箱
	BuyerIdentity           string                     `json:"buyerIdentity,omitempty"`           // NexusTok 侧稳定用户身份
	SuccessURL              string                     `json:"successUrl,omitempty"`              // 支付成功回调 URL
	ExpiresInSeconds        *int                       `json:"expiresInSeconds,omitempty"`        // 会话过期秒数
	OrderMerchantExternalID string                     `json:"orderMerchantExternalId,omitempty"` // NexusTok 本地 trade_no
}

// WaffoPancakeCheckoutSession 结账会话响应
type WaffoPancakeCheckoutSession struct {
	SessionID      string `json:"sessionId"`      // 会话 ID
	CheckoutURL    string `json:"checkoutUrl"`    // 结账页面 URL
	ExpiresAt      string `json:"expiresAt"`      // 过期时间
	OrderID        string `json:"orderId"`        // 上游订单 ID；Authenticated checkout 创建阶段通常为空
	Token          string `json:"token"`          // Authenticated checkout 的 buyer session token
	TokenExpiresAt string `json:"tokenExpiresAt"` // buyer session token 过期时间
}

// waffoPancakeAPIError API 错误响应
type waffoPancakeAPIError struct {
	Message string `json:"message"` // 错误消息
	Layer   string `json:"layer"`   // 错误层级
}

// waffoPancakeCreateSessionResponse 创建会话的 API 响应
type waffoPancakeCreateSessionResponse struct {
	Data   *WaffoPancakeCheckoutSession `json:"data"`   // 响应数据
	Errors []waffoPancakeAPIError       `json:"errors"` // 错误列表
}

// waffoPancakeWebhookData Webhook 事件数据
type waffoPancakeWebhookData struct {
	ID                            string          `json:"id"`                            // 事件数据 ID
	OrderID                       string          `json:"orderId"`                       // Pancake 上游订单 ID
	OrderMerchantExternalID       string          `json:"orderMerchantExternalId"`       // NexusTok 本地 trade_no
	MerchantProvidedBuyerIdentity string          `json:"merchantProvidedBuyerIdentity"` // NexusTok checkout 传入的稳定用户身份
	BuyerEmail                    string          `json:"buyerEmail"`                    // 买家邮箱
	Currency                      string          `json:"currency"`                      // 货币代码
	Amount                        dto.StringValue `json:"amount"`                        // 金额
	TaxAmount                     dto.StringValue `json:"taxAmount"`                     // 税额
	ProductName                   string          `json:"productName"`                   // 产品名称
}

// waffoPancakeWebhookEvent Webhook 事件对象
type waffoPancakeWebhookEvent struct {
	ID        string                  `json:"id"`        // 事件 ID
	Timestamp string                  `json:"timestamp"` // 事件时间戳
	EventType string                  `json:"eventType"` // 事件类型
	EventID   string                  `json:"eventId"`   // 事件唯一 ID
	StoreID   string                  `json:"storeId"`   // 商店 ID
	Mode      string                  `json:"mode"`      // 环境模式（test/prod）
	Data      waffoPancakeWebhookData `json:"data"`      // 事件数据
}

// NormalizedEventType 返回标准化的事件类型字符串
func (e *waffoPancakeWebhookEvent) NormalizedEventType() string {
	if e == nil {
		return ""
	}
	return e.EventType
}

// CreateWaffoPancakeCheckoutSession 创建 Waffo Pancake Authenticated 结账会话。
//
// 运行时 checkout 统一使用官方 SDK，并强制传入 NexusTok 本地 trade_no 与稳定
// buyer identity。Webhook 回调必须依赖这两个字段做订单归属校验，避免只靠邮箱
// 或上游 orderId 导致充值和订阅订单串单。
//
// 参数：
//   - ctx: 请求上下文
//   - params: 结账会话参数
//
// 返回：
//   - *WaffoPancakeCheckoutSession: 包含会话 ID、结账 URL 等
//   - error: 创建失败的错误信息
func CreateWaffoPancakeCheckoutSession(ctx context.Context, params *WaffoPancakeCreateSessionParams) (*WaffoPancakeCheckoutSession, error) {
	if params == nil {
		return nil, fmt.Errorf("missing checkout params")
	}
	if strings.TrimSpace(params.ProductID) == "" {
		return nil, fmt.Errorf("missing product id")
	}
	if strings.TrimSpace(params.BuyerIdentity) == "" {
		return nil, fmt.Errorf("missing buyer identity")
	}
	if strings.TrimSpace(params.OrderMerchantExternalID) == "" {
		return nil, fmt.Errorf("missing order merchant external id")
	}

	client, err := newWaffoPancakeRuntimeClient()
	if err != nil {
		return nil, err
	}

	currency := strings.ToUpper(strings.TrimSpace(params.Currency))
	if currency == "" {
		currency = "USD"
	}
	sdkParams := pancake.AuthenticatedCheckoutParams{
		CreateCheckoutSessionParams: pancake.CreateCheckoutSessionParams{
			ProductID:               strings.TrimSpace(params.ProductID),
			Currency:                currency,
			BuyerEmail:              optionalWaffoPancakeString(params.BuyerEmail),
			SuccessURL:              optionalWaffoPancakeString(params.SuccessURL),
			ExpiresInSeconds:        params.ExpiresInSeconds,
			OrderMerchantExternalID: optionalWaffoPancakeString(params.OrderMerchantExternalID),
		},
		BuyerIdentity: strings.TrimSpace(params.BuyerIdentity),
	}
	if params.PriceSnapshot != nil {
		taxCategory := strings.TrimSpace(params.PriceSnapshot.TaxCategory)
		if taxCategory == "" {
			taxCategory = "saas"
		}
		sdkParams.PriceSnapshot = &pancake.PriceInfo{
			Amount:      strings.TrimSpace(params.PriceSnapshot.Amount),
			TaxCategory: pancake.TaxCategory(taxCategory),
		}
	}

	session, err := client.Checkout.Authenticated.Create(ctx, sdkParams)
	if err != nil {
		return nil, fmt.Errorf("request Waffo Pancake authenticated checkout session: %w", err)
	}
	if session == nil || strings.TrimSpace(session.CheckoutURL) == "" || strings.TrimSpace(session.SessionID) == "" {
		return nil, fmt.Errorf("Waffo Pancake returned empty checkout session")
	}
	return &WaffoPancakeCheckoutSession{
		SessionID:      session.SessionID,
		CheckoutURL:    session.CheckoutURL,
		ExpiresAt:      session.ExpiresAt,
		Token:          session.Token,
		TokenExpiresAt: session.TokenExpiresAt,
	}, nil
}

// VerifyConfiguredWaffoPancakeWebhook 验证 Waffo Pancake Webhook 请求的签名。
// 自动从 payload 中检测环境（test/prod），并使用对应的公钥验证。
//
// 参数：
//   - payload: Webhook 请求体（JSON 字符串）
//   - signatureHeader: X-Waffo-Signature 请求头值
//
// 返回：
//   - *waffoPancakeWebhookEvent: 解析后的 Webhook 事件
//   - error: 验证失败的错误信息
func VerifyConfiguredWaffoPancakeWebhook(payload string, signatureHeader string) (*waffoPancakeWebhookEvent, error) {
	environment := resolveWaffoPancakeWebhookEnvironment(payload)
	return verifyWaffoPancakeWebhook(payload, signatureHeader, environment)
}

// WaffoPancakeBuyerIdentityFromUserID 生成 Pancake Authenticated checkout 使用的稳定买家身份。
//
// Webhook 解析充值和订阅订单时会用同一个函数计算期望值，因此这里的格式一旦变更，
// 必须同时兼容历史 pending 订单，避免付款成功后因为身份不匹配而无法入账。
func WaffoPancakeBuyerIdentityFromUserID(userID int) string {
	return fmt.Sprintf("nexustok-user-%d", userID)
}

// ResolveWaffoPancakeTradeNo 从 Webhook 事件中解析订单号。
// 验证订单号在数据库中存在且支付方式为 Waffo Pancake。
//
// 参数：
//   - event: Webhook 事件对象
//
// 返回：
//   - string: 订单号（trade_no）
//   - error: 解析或验证失败的错误信息
func ResolveWaffoPancakeTradeNo(event *waffoPancakeWebhookEvent) (string, error) {
	if event == nil {
		return "", fmt.Errorf("missing webhook event")
	}

	if tradeNo := strings.TrimSpace(event.Data.OrderMerchantExternalID); tradeNo != "" {
		topUp := model.GetTopUpByTradeNo(tradeNo)
		if topUp != nil && topUp.PaymentProvider == model.PaymentProviderWaffoPancake {
			if err := validateWaffoPancakeBuyerIdentity(event.Data.MerchantProvidedBuyerIdentity, topUp.UserId, tradeNo); err != nil {
				return "", err
			}
			return tradeNo, nil
		}
		return "", fmt.Errorf("waffo pancake order not found for tradeNo=%s", tradeNo)
	}

	if tradeNo := strings.TrimSpace(event.Data.OrderID); tradeNo != "" {
		// 兼容迁移前创建的旧 checkout：旧路径没有 orderMerchantExternalId，只能用上游 OrderID 反查。
		// 该 fallback 只用于钱包充值；订阅订单必须走外部单号，避免权益发放串单。
		topUp := model.GetTopUpByTradeNo(tradeNo)
		if topUp != nil && topUp.PaymentProvider == model.PaymentProviderWaffoPancake {
			return tradeNo, nil
		}
		return "", fmt.Errorf("waffo pancake order not found for webhook orderId=%s", tradeNo)
	}

	return "", fmt.Errorf("missing webhook orderMerchantExternalId")
}

// ResolveWaffoPancakeSubscriptionTradeNo 从 Webhook 事件中解析订阅订单号。
//
// 订阅不提供旧 OrderID fallback：只有 checkout 创建时写入的本地 trade_no 才能完成
// SubscriptionOrder，确保订阅权益不会因为上游订单 ID 或邮箱变化而串单。
func ResolveWaffoPancakeSubscriptionTradeNo(event *waffoPancakeWebhookEvent) (string, error) {
	if event == nil {
		return "", fmt.Errorf("missing webhook event")
	}
	tradeNo := strings.TrimSpace(event.Data.OrderMerchantExternalID)
	if tradeNo == "" {
		return "", fmt.Errorf("missing webhook orderMerchantExternalId")
	}
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	if order == nil || order.PaymentProvider != model.PaymentProviderWaffoPancake {
		return "", fmt.Errorf("waffo pancake subscription order not found for tradeNo=%s", tradeNo)
	}
	if err := validateWaffoPancakeBuyerIdentity(event.Data.MerchantProvidedBuyerIdentity, order.UserId, tradeNo); err != nil {
		return "", err
	}
	return tradeNo, nil
}

func validateWaffoPancakeBuyerIdentity(actualIdentity string, userID int, tradeNo string) error {
	expectedIdentity := WaffoPancakeBuyerIdentityFromUserID(userID)
	if strings.TrimSpace(actualIdentity) != expectedIdentity {
		return fmt.Errorf(
			"waffo pancake buyer identity mismatch for tradeNo=%s: expected=%q actual=%q",
			tradeNo,
			expectedIdentity,
			strings.TrimSpace(actualIdentity),
		)
	}
	return nil
}

// newWaffoPancakeRuntimeClient 使用已保存配置创建运行时 checkout client。
//
// 与管理端临时凭证 client 分开命名，避免后续误把尚未保存的私钥用于用户支付。
func newWaffoPancakeRuntimeClient() (*pancake.Client, error) {
	return newWaffoPancakeAdminClientFromCreds(setting.WaffoPancakeMerchantID, setting.WaffoPancakePrivateKey)
}

// newWaffoPancakeAdminClientFromCreds 使用传入凭证创建 Pancake SDK client。
//
// 管理端 catalog/Store/Product 创建会传入临时或已保存凭证；运行时 checkout 只通过
// newWaffoPancakeRuntimeClient 使用已保存凭证，避免尚未保存的私钥参与用户支付。
func newWaffoPancakeAdminClientFromCreds(merchantID string, privateKey string) (*pancake.Client, error) {
	merchantID = strings.TrimSpace(merchantID)
	privateKey = strings.TrimSpace(privateKey)
	if merchantID == "" || privateKey == "" {
		return nil, fmt.Errorf("merchant id and private key are required")
	}
	return pancake.New(pancake.Config{
		MerchantID: merchantID,
		PrivateKey: privateKey,
	})
}

// WaffoPancakeCatalogProduct 是 catalog 中可绑定的一次性商品。
type WaffoPancakeCatalogProduct struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// WaffoPancakeCatalogStore 是 Pancake Store 及其 active OnetimeProduct 列表。
type WaffoPancakeCatalogStore struct {
	ID              string                       `json:"id"`
	Name            string                       `json:"name"`
	Status          string                       `json:"status"`
	ProdEnabled     bool                         `json:"prodEnabled"`
	OnetimeProducts []WaffoPancakeCatalogProduct `json:"onetimeProducts"`
}

// WaffoPancakeCatalog 是管理端 catalog 响应。
type WaffoPancakeCatalog struct {
	Stores []WaffoPancakeCatalogStore `json:"stores"`
}

// ListWaffoPancakeCatalog 查询 Pancake 账号下的 Store 与 active OnetimeProduct。
//
// 该接口既作为“凭证是否可用”的验证入口，也为前端 Store/Product 下拉提供数据。
// 当前使用 limit=100，与 new-api-main 行为一致；若真实商户目录超过上限，后续再
// 加 offset 分页，避免本轮引入复杂分页状态。
func ListWaffoPancakeCatalog(ctx context.Context, merchantID string, privateKey string) (*WaffoPancakeCatalog, error) {
	client, err := newWaffoPancakeAdminClientFromCreds(merchantID, privateKey)
	if err != nil {
		return nil, err
	}

	type queryShape struct {
		Stores []WaffoPancakeCatalogStore `json:"stores"`
	}
	resp, err := pancake.GraphQLQuery[queryShape](ctx, client, pancake.GraphQLParams{
		Query: `query {
			stores(limit: 100) {
				id
				name
				status
				prodEnabled
				onetimeProducts {
					id
					name
					status
				}
			}
		}`,
	})
	if err != nil {
		return nil, fmt.Errorf("query Waffo Pancake catalog: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("waffo pancake catalog query returned %d errors: %s", len(resp.Errors), resp.Errors[0].Message)
	}

	stores := resp.Data.Stores
	for i := range stores {
		active := stores[i].OnetimeProducts[:0]
		for _, product := range stores[i].OnetimeProducts {
			if strings.EqualFold(strings.TrimSpace(product.Status), "active") {
				active = append(active, product)
			}
		}
		stores[i].OnetimeProducts = active
	}
	return &WaffoPancakeCatalog{Stores: stores}, nil
}

// WaffoPancakePairResult 是一键创建 Store + OnetimeProduct 的结果。
//
// OrphanStore 为 true 表示 Store 已创建但 Product 创建或发布失败，前端应展示
// StoreID 供管理员到 Pancake 后台接管或稍后重试。
type WaffoPancakePairResult struct {
	StoreID     string `json:"store_id"`
	StoreName   string `json:"store_name"`
	ProductID   string `json:"product_id"`
	ProductName string `json:"product_name"`
	OrphanStore bool   `json:"orphan_store,omitempty"`
}

// CreateWaffoPancakePrimaryStore 创建 NexusTok 默认 Waffo Pancake Store。
func CreateWaffoPancakePrimaryStore(ctx context.Context, merchantID string, privateKey string) (string, error) {
	client, err := newWaffoPancakeAdminClientFromCreds(merchantID, privateKey)
	if err != nil {
		return "", err
	}
	storeRes, err := client.Stores.Create(ctx, pancake.CreateStoreParams{
		Name: defaultWaffoPancakeStoreName,
	})
	if err != nil {
		return "", fmt.Errorf("create Waffo Pancake store: %w", err)
	}
	return storeRes.Store.ID, nil
}

// CreateWaffoPancakeProductForPlan 创建并发布订阅套餐专属的一次性商品。
//
// 这里有意使用 OnetimeProduct，而不是 Pancake 自动续费的 SubscriptionProduct：
// NexusTok 当前订阅模型只表达一次购买后的内部有效期和额度，并没有外部续费、
// 取消、逾期、退款撤销权益等生命周期处理。先把 plan 级 product 绑定原生化，
// 后续如接入用户侧 Pancake 订阅支付，再单独评审 checkout 元数据和 webhook
// 订单分流，避免上游自动扣款但本地权益没有续期。
func CreateWaffoPancakeProductForPlan(ctx context.Context, merchantID string, privateKey string, storeID string, name string, amount string, returnURL string) (string, error) {
	storeID = strings.TrimSpace(storeID)
	if storeID == "" {
		return "", fmt.Errorf("store id is required to create a product")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("plan name is required")
	}
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return "", fmt.Errorf("plan price is required")
	}
	client, err := newWaffoPancakeAdminClientFromCreds(merchantID, privateKey)
	if err != nil {
		return "", err
	}
	productRes, err := client.OnetimeProducts.Create(ctx, pancake.CreateOnetimeProductParams{
		StoreID: storeID,
		Name:    name,
		Prices: pancake.Prices{
			"USD": {
				Amount:      amount,
				TaxCategory: pancake.TaxCategory("saas"),
			},
		},
		SuccessURL: optionalWaffoPancakeString(strings.TrimSpace(returnURL)),
	})
	if err != nil {
		return "", fmt.Errorf("create Waffo Pancake plan product: %w", err)
	}
	productID := productRes.Product.ID
	if _, err := client.OnetimeProducts.Publish(ctx, pancake.PublishOnetimeProductParams{ID: productID}); err != nil {
		return "", fmt.Errorf("publish Waffo Pancake plan product: %w", err)
	}
	return productID, nil
}

// CreateWaffoPancakePrimaryProduct 创建并发布钱包充值使用的一次性商品。
//
// 商品种子价格固定为 1.00 USD；真实充值金额仍由当前 checkout 请求中的
// PriceSnapshot 覆盖，因此不会改变用户侧任意金额充值语义。
func CreateWaffoPancakePrimaryProduct(ctx context.Context, merchantID string, privateKey string, storeID string, returnURL string) (string, error) {
	storeID = strings.TrimSpace(storeID)
	if storeID == "" {
		return "", fmt.Errorf("store id is required to create a product")
	}
	client, err := newWaffoPancakeAdminClientFromCreds(merchantID, privateKey)
	if err != nil {
		return "", err
	}
	productRes, err := client.OnetimeProducts.Create(ctx, pancake.CreateOnetimeProductParams{
		StoreID: storeID,
		Name:    defaultWaffoPancakeProductName,
		Prices: pancake.Prices{
			"USD": {
				Amount:      "1.00",
				TaxCategory: pancake.TaxCategory("saas"),
			},
		},
		SuccessURL: optionalWaffoPancakeString(strings.TrimSpace(returnURL)),
	})
	if err != nil {
		return "", fmt.Errorf("create Waffo Pancake product: %w", err)
	}
	productID := productRes.Product.ID
	if _, err := client.OnetimeProducts.Publish(ctx, pancake.PublishOnetimeProductParams{ID: productID}); err != nil {
		return "", fmt.Errorf("publish Waffo Pancake product: %w", err)
	}
	return productID, nil
}

// CreateWaffoPancakePrimaryPair 一键创建默认 Store 与钱包充值商品。
func CreateWaffoPancakePrimaryPair(ctx context.Context, merchantID string, privateKey string, returnURL string) (*WaffoPancakePairResult, error) {
	storeID, err := CreateWaffoPancakePrimaryStore(ctx, merchantID, privateKey)
	if err != nil {
		return nil, err
	}
	productID, err := CreateWaffoPancakePrimaryProduct(ctx, merchantID, privateKey, storeID, returnURL)
	if err != nil {
		return &WaffoPancakePairResult{
			StoreID:     storeID,
			StoreName:   defaultWaffoPancakeStoreName,
			OrphanStore: true,
		}, fmt.Errorf("store created at %s but product creation failed: %w", storeID, err)
	}
	return &WaffoPancakePairResult{
		StoreID:     storeID,
		StoreName:   defaultWaffoPancakeStoreName,
		ProductID:   productID,
		ProductName: defaultWaffoPancakeProductName,
	}, nil
}

func optionalWaffoPancakeString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

// SaveWaffoPancakeConfig 原子保存 Waffo Pancake 管理端配置。
//
// 该函数承接支付设置页的一次性保存动作，避免 Merchant、Store、Product、
// 回跳地址和价格参数被多次 PUT 分批写入后出现半成功状态。启用支付时会校验
// 运行所需的核心字段和当前环境的 webhook 公钥；未启用时允许管理员保存空绑定，
// 便于先关闭网关再清理配置。私钥和 webhook 公钥不回显，输入为空表示保留已有值。
func SaveWaffoPancakeConfig(enabled bool, sandbox bool, merchantID string, privateKey string, webhookPublicKey string, webhookTestKey string, storeID string, productID string, returnURL string, currency string, unitPrice float64, minTopUp int) error {
	merchantID = strings.TrimSpace(merchantID)
	storeID = strings.TrimSpace(storeID)
	productID = strings.TrimSpace(productID)
	privateKey = strings.TrimSpace(privateKey)
	webhookPublicKey = strings.TrimSpace(webhookPublicKey)
	webhookTestKey = strings.TrimSpace(webhookTestKey)
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		currency = "USD"
	}
	if minTopUp < 1 {
		minTopUp = 1
	}

	if enabled {
		if merchantID == "" {
			return fmt.Errorf("merchant id is required")
		}
		if storeID == "" {
			return fmt.Errorf("store id is required")
		}
		if productID == "" {
			return fmt.Errorf("product id is required")
		}
		if strings.TrimSpace(privateKey) == "" && strings.TrimSpace(setting.WaffoPancakePrivateKey) == "" {
			return fmt.Errorf("private key is required")
		}
		if unitPrice <= 0 {
			return fmt.Errorf("unit price must be greater than 0")
		}
		if sandbox {
			if webhookTestKey == "" && strings.TrimSpace(setting.WaffoPancakeWebhookTestKey) == "" {
				return fmt.Errorf("sandbox webhook public key is required")
			}
		} else if webhookPublicKey == "" && strings.TrimSpace(setting.WaffoPancakeWebhookPublicKey) == "" {
			return fmt.Errorf("production webhook public key is required")
		}
	}

	values := map[string]string{
		"WaffoPancakeEnabled":    strconv.FormatBool(enabled),
		"WaffoPancakeSandbox":    strconv.FormatBool(sandbox),
		"WaffoPancakeMerchantID": merchantID,
		"WaffoPancakeStoreID":    storeID,
		"WaffoPancakeProductID":  productID,
		"WaffoPancakeReturnURL":  strings.TrimSpace(returnURL),
		"WaffoPancakeCurrency":   currency,
		"WaffoPancakeUnitPrice":  strconv.FormatFloat(unitPrice, 'f', -1, 64),
		"WaffoPancakeMinTopUp":   strconv.Itoa(minTopUp),
	}
	if privateKey != "" {
		values["WaffoPancakePrivateKey"] = privateKey
	}
	if webhookPublicKey != "" {
		values["WaffoPancakeWebhookPublicKey"] = webhookPublicKey
	}
	if webhookTestKey != "" {
		values["WaffoPancakeWebhookTestKey"] = webhookTestKey
	}

	if err := model.UpdateOptionsBulk(values); err != nil {
		return fmt.Errorf("persist Waffo Pancake config: %w", err)
	}
	return nil
}

// normalizeRSAPrivateKey 标准化 RSA 私钥格式为 PEM 编码
func normalizeRSAPrivateKey(raw string) (string, error) {
	return normalizePEMKey(raw, "PRIVATE KEY", "RSA PRIVATE KEY")
}

// normalizeRSAPublicKey 标准化 RSA 公钥格式为 PEM 编码
func normalizeRSAPublicKey(raw string) (string, error) {
	return normalizePEMKey(raw, "PUBLIC KEY", "RSA PUBLIC KEY")
}

// normalizePEMKey 将密钥标准化为 PEM 格式。
// 支持三种输入格式：
//   - 完整 PEM 块：直接重新编码
//   - 纯 Base64 (PKCS#8)：解码后检测格式并编码为 PEM
//   - 纯 Base64 (PKCS#1)：解码后编码为对应 PEM 类型
//
// 参数：
//   - raw: 原始密钥字符串
//   - pkcs8Type: PKCS#8 格式的 PEM 类型标签
//   - pkcs1Type: PKCS#1 格式的 PEM 类型标签
//
// 返回：
//   - string: 标准化后的 PEM 格式密钥
//   - error: 解析失败的错误信息
func normalizePEMKey(raw string, pkcs8Type string, pkcs1Type string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("%s is empty", strings.ToLower(pkcs8Type))
	}

	// 处理转义换行符
	normalized := strings.TrimSpace(strings.ReplaceAll(raw, `\n`, "\n"))
	// 输入已是 PEM 格式时，解码后重新编码以标准化
	if strings.Contains(normalized, "BEGIN ") {
		block, _ := pem.Decode([]byte(normalized))
		if block == nil {
			return "", fmt.Errorf("invalid PEM encoded %s", strings.ToLower(pkcs8Type))
		}
		return string(pem.EncodeToMemory(block)), nil
	}

	// 输入为纯 Base64 时，解码为 DER
	der, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(normalized, "\n", ""))
	if err != nil {
		return "", fmt.Errorf("invalid base64 encoded %s: %w", strings.ToLower(pkcs8Type), err)
	}

	// 自动检测 DER 格式：PKCS#8 或 PKCS#1
	pemType := pkcs8Type
	if pkcs8Type == "PRIVATE KEY" {
		if _, err := x509.ParsePKCS8PrivateKey(der); err != nil {
			if _, err := x509.ParsePKCS1PrivateKey(der); err == nil {
				pemType = pkcs1Type
			} else {
				return "", fmt.Errorf("invalid RSA private key")
			}
		}
	} else {
		if _, err := x509.ParsePKIXPublicKey(der); err != nil {
			if _, err := x509.ParsePKCS1PublicKey(der); err == nil {
				pemType = pkcs1Type
			} else {
				return "", fmt.Errorf("invalid RSA public key")
			}
		}
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: pemType, Bytes: der})), nil
}

// signWaffoPancakeRequest 对 Waffo Pancake API 请求进行 RSA 签名。
// 流程：构建规范化请求 -> SHA-256 哈希 -> RSA PKCS1v15 签名 -> Base64 编码。
//
// 参数：
//   - method: HTTP 方法
//   - path: 请求路径
//   - timestamp: 时间戳
//   - body: 请求体
//   - privateKeyPEM: PEM 格式的 RSA 私钥
//
// 返回：
//   - string: Base64 编码的签名
//   - error: 签名失败的错误信息
func signWaffoPancakeRequest(method string, path string, timestamp string, body string, privateKeyPEM string) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("invalid RSA private key PEM")
	}

	// 根据 PEM 类型解析私钥
	var privateKey *rsa.PrivateKey
	switch block.Type {
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("parse PKCS#8 private key: %w", err)
		}
		parsed, ok := key.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("private key is not RSA")
		}
		privateKey = parsed
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("parse PKCS#1 private key: %w", err)
		}
		privateKey = key
	default:
		return "", fmt.Errorf("unsupported private key type: %s", block.Type)
	}

	// 构建规范化请求并计算 SHA-256 哈希
	canonicalRequest := buildWaffoPancakeCanonicalRequest(method, path, timestamp, body)
	digest := sha256.Sum256([]byte(canonicalRequest))
	// RSA PKCS1v15 签名
	signature, err := rsa.SignPKCS1v15(nil, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign Waffo Pancake request: %w", err)
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// buildWaffoPancakeCanonicalRequest 构建规范化请求字符串用于签名。
// 格式为 "METHOD\nPATH\nTIMESTAMP\nBODY_HASH"。
//
// 参数：
//   - method: HTTP 方法
//   - path: 请求路径
//   - timestamp: 时间戳
//   - body: 请求体
//
// 返回：
//   - string: 规范化请求字符串
func buildWaffoPancakeCanonicalRequest(method string, path string, timestamp string, body string) string {
	bodyHash := sha256.Sum256([]byte(body))
	return fmt.Sprintf(
		"%s\n%s\n%s\n%s",
		strings.ToUpper(method),
		path,
		timestamp,
		base64.StdEncoding.EncodeToString(bodyHash[:]),
	)
}

// verifyWaffoPancakeWebhook 验证 Waffo Pancake Webhook 的签名。
// 从签名头中提取时间戳和签名值，验证时间戳在容差范围内，然后验证 RSA 签名。
//
// 参数：
//   - payload: Webhook 请求体
//   - signatureHeader: X-Waffo-Signature 请求头
//   - environment: 环境标识（test/prod）
//
// 返回：
//   - *waffoPancakeWebhookEvent: 解析后的事件
//   - error: 验证失败的错误信息
func verifyWaffoPancakeWebhook(payload string, signatureHeader string, environment string) (*waffoPancakeWebhookEvent, error) {
	if signatureHeader == "" {
		return nil, fmt.Errorf("missing X-Waffo-Signature header")
	}

	// 解析签名头中的时间戳和签名值
	timestampPart, signaturePart := parseWaffoPancakeSignatureHeader(signatureHeader)
	if timestampPart == "" || signaturePart == "" {
		return nil, fmt.Errorf("malformed X-Waffo-Signature header")
	}

	// 验证时间戳在容差范围内（防止重放攻击）
	timestampMs, err := strconv.ParseInt(timestampPart, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp in X-Waffo-Signature header")
	}
	if math.Abs(float64(time.Now().UnixMilli()-timestampMs)) > float64(waffoPancakeDefaultTolerance.Milliseconds()) {
		return nil, fmt.Errorf("webhook timestamp outside tolerance window")
	}

	// 构建签名输入并使用对应环境的公钥验证
	signatureInput := fmt.Sprintf("%s.%s", timestampPart, payload)
	if err := verifyWaffoPancakeWebhookWithKey(signatureInput, signaturePart, resolveWaffoPancakeWebhookPublicKey(environment)); err != nil {
		return nil, fmt.Errorf("invalid webhook signature")
	}

	// 解析 Webhook 事件
	var event waffoPancakeWebhookEvent
	if err := common.Unmarshal([]byte(payload), &event); err != nil {
		return nil, fmt.Errorf("parse Waffo Pancake webhook payload: %w", err)
	}
	return &event, nil
}

// parseWaffoPancakeSignatureHeader 解析 Waffo Pancake 的签名头。
// 格式为 "t=<timestamp>,v1=<signature>"。
//
// 参数：
//   - header: X-Waffo-Signature 头值
//
// 返回：
//   - string: 时间戳部分
//   - string: 签名部分
func parseWaffoPancakeSignatureHeader(header string) (string, string) {
	var timestampPart string
	var signaturePart string
	for _, pair := range strings.Split(header, ",") {
		key, value, found := strings.Cut(strings.TrimSpace(pair), "=")
		if !found {
			continue
		}
		switch key {
		case "t":
			timestampPart = value
		case "v1":
			signaturePart = value
		}
	}
	return timestampPart, signaturePart
}

// resolveWaffoPancakeWebhookEnvironment 从 Webhook payload 中检测环境。
// 优先从 payload 的 mode 字段读取，其次使用全局沙箱配置。
func resolveWaffoPancakeWebhookEnvironment(payload string) string {
	var envelope struct {
		Mode string `json:"mode"`
	}
	if err := common.Unmarshal([]byte(payload), &envelope); err != nil {
		if setting.WaffoPancakeSandbox {
			return "test"
		}
		return "prod"
	}

	switch strings.ToLower(strings.TrimSpace(envelope.Mode)) {
	case "test":
		return "test"
	case "prod":
		return "prod"
	default:
		if setting.WaffoPancakeSandbox {
			return "test"
		}
		return "prod"
	}
}

// resolveWaffoPancakeWebhookPublicKey 根据环境选择对应的 Webhook 验证公钥。
func resolveWaffoPancakeWebhookPublicKey(environment string) string {
	if environment == "prod" {
		return strings.TrimSpace(setting.WaffoPancakeWebhookPublicKey)
	}
	return strings.TrimSpace(setting.WaffoPancakeWebhookTestKey)
}

// verifyWaffoPancakeWebhookWithKey 使用指定公钥验证 Webhook 签名。
// 流程：标准化公钥 -> 解析 PEM -> 计算 SHA-256 哈希 -> RSA PKCS1v15 验签。
//
// 参数：
//   - signatureInput: 签名输入（"timestamp.payload"）
//   - signaturePart: Base64 编码的签名值
//   - rawPublicKey: 原始公钥字符串
//
// 返回：
//   - error: 验证失败的错误信息
func verifyWaffoPancakeWebhookWithKey(signatureInput string, signaturePart string, rawPublicKey string) error {
	// 标准化公钥为 PEM 格式
	publicKeyPEM, err := normalizeRSAPublicKey(rawPublicKey)
	if err != nil {
		return err
	}

	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return fmt.Errorf("invalid RSA public key PEM")
	}

	// 根据 PEM 类型解析公钥
	var publicKey *rsa.PublicKey
	switch block.Type {
	case "PUBLIC KEY":
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("parse PKIX public key: %w", err)
		}
		parsed, ok := key.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("public key is not RSA")
		}
		publicKey = parsed
	case "RSA PUBLIC KEY":
		key, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("parse PKCS#1 public key: %w", err)
		}
		publicKey = key
	default:
		return fmt.Errorf("unsupported public key type: %s", block.Type)
	}

	// Base64 解码签名
	signature, err := base64.StdEncoding.DecodeString(signaturePart)
	if err != nil {
		return fmt.Errorf("decode webhook signature: %w", err)
	}

	// 计算签名输入的 SHA-256 哈希并验证签名
	digest := sha256.Sum256([]byte(signatureInput))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature); err != nil {
		return fmt.Errorf("verify webhook signature: %w", err)
	}
	return nil
}
