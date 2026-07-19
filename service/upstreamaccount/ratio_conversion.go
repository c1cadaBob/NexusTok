package upstreamaccount

// RatioConversionConfig 表示供应商充值到账换算配置。
//
// 这两个值只描述“管理员从上游供应商购买额度”的成本关系，不是 NexusTok 面向
// 用户的充值倍率，也不会写入系统 TopupGroupRatio。只有两个值同时大于 0 时才
// 参与换算；否则实际倍率等同于上游返回的有效倍率。
type RatioConversionConfig struct {
	PaidCNY           float64 `json:"paid_cny,omitempty"`
	PlatformUSDCredit float64 `json:"platform_usd_credit,omitempty"`
}

// Enabled 判断充值换算配置是否完整有效。
func (config RatioConversionConfig) Enabled() bool {
	return config.PaidCNY > 0 && config.PlatformUSDCredit > 0
}

// RatioConversionSnapshot 是写入快照与账号同步元数据的倍率换算结果。
type RatioConversionSnapshot struct {
	PaidCNY           float64 `json:"paid_cny,omitempty"`
	PlatformUSDCredit float64 `json:"platform_usd_credit,omitempty"`
	Enabled           bool    `json:"enabled"`
}

// conversionSnapshot 将请求中的换算配置归一化为可展示和可持久化的快照。
func conversionSnapshot(config RatioConversionConfig) *RatioConversionSnapshot {
	if !config.Enabled() {
		return nil
	}
	return &RatioConversionSnapshot{
		PaidCNY:           config.PaidCNY,
		PlatformUSDCredit: config.PlatformUSDCredit,
		Enabled:           true,
	}
}

// applyRatioConversion 把供应商充值关系换算为真实成本倍率。
//
// 公式：倍率换算 = 上游倍率 / (平台到账美刀额度 / 实付人民币)
// 等价于：倍率换算 = 上游倍率 * 实付人民币 / 平台到账美刀额度。
func applyRatioConversion(upstreamRatio float64, config RatioConversionConfig) float64 {
	if upstreamRatio <= 0 {
		upstreamRatio = 1
	}
	if !config.Enabled() {
		return upstreamRatio
	}
	return upstreamRatio * config.PaidCNY / config.PlatformUSDCredit
}

// ApplyRatioConversion 为快照中的每个密钥写入有效上游倍率和倍率换算值。
func ApplyRatioConversion(snapshot *Snapshot, config RatioConversionConfig) {
	if snapshot == nil {
		return
	}
	snapshot.RatioConversion = conversionSnapshot(config)
	for i := range snapshot.Keys {
		upstreamRatio := EffectiveKeyRatio(snapshot.Keys[i])
		snapshot.Keys[i].EffectiveRatio = upstreamRatio
		snapshot.Keys[i].RatioConversion = applyRatioConversion(upstreamRatio, config)
	}
}

// ApplyExistingRatioConversion 根据快照中已经保存的换算配置重新计算密钥倍率。
func ApplyExistingRatioConversion(snapshot *Snapshot) {
	if snapshot == nil || snapshot.RatioConversion == nil || !snapshot.RatioConversion.Enabled {
		ApplyRatioConversion(snapshot, RatioConversionConfig{})
		return
	}
	ApplyRatioConversion(snapshot, RatioConversionConfig{
		PaidCNY:           snapshot.RatioConversion.PaidCNY,
		PlatformUSDCredit: snapshot.RatioConversion.PlatformUSDCredit,
	})
}

// applySnapshotRatioConversionForRequest 在创建/刷新落库前确定本次同步使用的倍率换算。
//
// 前端通常在预览时传入充值换算配置，快照中已经带有 `RatioConversion`；但管理员也
// 可能在预览后调整数值再保存。请求中两个数值同时有效时优先使用请求配置，否则沿用
// 快照已有配置；两者都没有时回退为上游有效倍率，保持旧同步快照可继续落库。
func applySnapshotRatioConversionForRequest(snapshot *Snapshot, config RatioConversionConfig) {
	if snapshot == nil {
		return
	}
	if config.Enabled() {
		ApplyRatioConversion(snapshot, config)
		ApplySuggestions(snapshot)
		return
	}
	if snapshot.RatioConversion != nil {
		ApplyExistingRatioConversion(snapshot)
		ApplySuggestions(snapshot)
		return
	}
	ApplyExistingRatioConversion(snapshot)
}

// EffectiveKeyRatio 返回一个同步密钥的有效上游倍率。
//
// 优先使用上游密钥所属分组倍率；如果没有分组倍率，则对可用模型倍率取平均；
// 都没有时使用 1，表示没有成本差异信息。
func EffectiveKeyRatio(key SyncedKey) float64 {
	if key.GroupRatio != nil && *key.GroupRatio > 0 {
		return *key.GroupRatio
	}
	if len(key.ModelRatios) > 0 {
		var sum float64
		var count float64
		for _, ratio := range key.ModelRatios {
			if ratio <= 0 {
				continue
			}
			sum += ratio
			count++
		}
		if count > 0 {
			return sum / count
		}
	}
	if key.EffectiveRatio > 0 {
		return key.EffectiveRatio
	}
	return 1
}

// ConvertedKeyRatio 返回已换算的真实成本倍率；旧数据没有该字段时按有效上游倍率回退。
func ConvertedKeyRatio(key SyncedKey) float64 {
	if key.RatioConversion > 0 {
		return key.RatioConversion
	}
	return EffectiveKeyRatio(key)
}
