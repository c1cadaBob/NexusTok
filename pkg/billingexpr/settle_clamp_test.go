package billingexpr_test

import (
	"math"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestComputeTieredQuota_ClampOnOverflow 锁定计费安全不变量：
// 动态计费表达式如果因为异常配置或极端 token 数输出超大配额，结算结果必须饱和到
// int32 上限，不能在 int 转换时回绕成负数或给用户产生异常退款；同时 Clamp 事件必须
// 返回给调用方，供日志和管理审计记录。
func TestComputeTieredQuota_ClampOnOverflow(t *testing.T) {
	// exprOutput = p * 1e9 = 1e18；
	// quotaBeforeGroup = 1e18 / 1e6 * 5e5 = 5e17，远超 int32 上限。
	exprStr := `tier("base", p * 1000000000)`
	snap := &billingexpr.BillingSnapshot{
		BillingMode:  "tiered_expr",
		ExprString:   exprStr,
		ExprHash:     billingexpr.ExprHashString(exprStr),
		GroupRatio:   1.0,
		QuotaPerUnit: 500_000,
	}

	result, err := billingexpr.ComputeTieredQuota(snap, billingexpr.TokenParams{P: 1_000_000_000})
	require.NoError(t, err)

	assert.Equal(t, math.MaxInt32, result.ActualQuotaAfterGroup, "超大结算必须饱和到 int32 上限，不能回绕为负数")
	require.NotNil(t, result.Clamp, "饱和事件必须返回，便于后续审计记录")
	assert.Equal(t, common.QuotaClampOverflow, result.Clamp.Kind)
	assert.Equal(t, math.MaxInt32, result.Clamp.Clamped)
}

// TestComputeTieredQuota_NoClampInRange 验证普通计费范围不会误报 Clamp。
// 常规请求不应写入 quota_saturation 审计信息，避免管理日志出现噪音。
func TestComputeTieredQuota_NoClampInRange(t *testing.T) {
	exprStr := `tier("base", p * 2 + c * 10)`
	snap := &billingexpr.BillingSnapshot{
		BillingMode:  "tiered_expr",
		ExprString:   exprStr,
		ExprHash:     billingexpr.ExprHashString(exprStr),
		GroupRatio:   1.0,
		QuotaPerUnit: 500_000,
	}

	result, err := billingexpr.ComputeTieredQuota(snap, billingexpr.TokenParams{P: 1000, C: 500})
	require.NoError(t, err)

	assert.Equal(t, 3500, result.ActualQuotaAfterGroup)
	assert.Nil(t, result.Clamp, "正常范围内的结算不应报告 clamp")
}
