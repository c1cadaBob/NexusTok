// 本文件是 relay/common 包中参数覆盖（Param Override）功能的单元测试集。
// 覆盖了各种操作模式（set、delete、trim_prefix、trim_suffix、replace、regex_replace、
// move、copy、prepend、append、ensure_prefix、ensure_suffix、trim_space、to_lower、to_upper、
// return_error、prune_objects、set_header、copy_header、move_header、pass_headers、sync_fields 等），
// 以及条件执行（conditions）、通配符路径（wildcard path）、上下文注入等高级功能。
package common

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	common2 "github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/types"

	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/setting/model_setting"
	"github.com/samber/lo"
)

// TestApplyParamOverrideTrimPrefix 测试 trim_prefix 操作：移除 model 字段值的 "openai/" 前缀。
func TestApplyParamOverrideTrimPrefix(t *testing.T) {
	// trim_prefix example:
	// {"operations":[{"path":"model","mode":"trim_prefix","value":"openai/"}]}
	input := []byte(`{"model":"openai/gpt-4","temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "model",
				"mode":  "trim_prefix",
				"value": "openai/",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4","temperature":0.7}`, string(out))
}

// TestApplyParamOverrideTrimSuffix 测试 trim_suffix 操作：移除 model 字段值的 "-latest" 后缀。
func TestApplyParamOverrideTrimSuffix(t *testing.T) {
	// trim_suffix example:
	// {"operations":[{"path":"model","mode":"trim_suffix","value":"-latest"}]}
	input := []byte(`{"model":"gpt-4-latest","temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "model",
				"mode":  "trim_suffix",
				"value": "-latest",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4","temperature":0.7}`, string(out))
}

// TestApplyParamOverrideTrimNoop 测试 trim_prefix 操作的空操作情况：当前缀不存在时不做任何修改。
func TestApplyParamOverrideTrimNoop(t *testing.T) {
	// trim_prefix no-op example:
	// {"operations":[{"path":"model","mode":"trim_prefix","value":"openai/"}]}
	input := []byte(`{"model":"gpt-4","temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "model",
				"mode":  "trim_prefix",
				"value": "openai/",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4","temperature":0.7}`, string(out))
}

// TestApplyParamOverrideMixedLegacyAndOperations 测试传统覆盖与 operations 混合使用：两者同时生效。
func TestApplyParamOverrideMixedLegacyAndOperations(t *testing.T) {
	input := []byte(`{"model":"openai/gpt-4","temperature":0.7}`)
	override := map[string]interface{}{
		"temperature": 0.2,
		"top_p":       0.95,
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "model",
				"mode":  "trim_prefix",
				"value": "openai/",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4","temperature":0.2,"top_p":0.95}`, string(out))
}

// TestApplyParamOverrideMixedLegacyAndOperationsConflictPrefersOperations 测试冲突时 operations 优先于传统覆盖。
func TestApplyParamOverrideMixedLegacyAndOperationsConflictPrefersOperations(t *testing.T) {
	input := []byte(`{"model":"openai/gpt-4","temperature":0.7}`)
	override := map[string]interface{}{
		"model":       "legacy-model",
		"temperature": 0.2,
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "model",
				"mode":  "set",
				"value": "op-model",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"op-model","temperature":0.2}`, string(out))
}

// TestApplyParamOverrideTrimRequiresValue 测试 trim_prefix 操作缺少 value 时应返回错误。
func TestApplyParamOverrideTrimRequiresValue(t *testing.T) {
	// trim_prefix requires value example:
	// {"operations":[{"path":"model","mode":"trim_prefix"}]}
	input := []byte(`{"model":"gpt-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "model",
				"mode": "trim_prefix",
			},
		},
	}

	_, err := ApplyParamOverride(input, override, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestApplyParamOverrideReplace 测试 replace 操作：将 model 中的 "openai/" 替换为空字符串。
func TestApplyParamOverrideReplace(t *testing.T) {
	// replace example:
	// {"operations":[{"path":"model","mode":"replace","from":"openai/","to":""}]}
	input := []byte(`{"model":"openai/gpt-4o-mini","temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "model",
				"mode": "replace",
				"from": "openai/",
				"to":   "",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4o-mini","temperature":0.7}`, string(out))
}

// TestApplyParamOverrideRegexReplace 测试 regex_replace 操作：使用正则表达式替换 model 前缀。
func TestApplyParamOverrideRegexReplace(t *testing.T) {
	// regex_replace example:
	// {"operations":[{"path":"model","mode":"regex_replace","from":"^gpt-","to":"openai/gpt-"}]}
	input := []byte(`{"model":"gpt-4o-mini","temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "model",
				"mode": "regex_replace",
				"from": "^gpt-",
				"to":   "openai/gpt-",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"openai/gpt-4o-mini","temperature":0.7}`, string(out))
}

// TestApplyParamOverrideReplaceRequiresFrom 测试 replace 操作缺少 from 参数时应返回错误。
func TestApplyParamOverrideReplaceRequiresFrom(t *testing.T) {
	// replace requires from example:
	// {"operations":[{"path":"model","mode":"replace"}]}
	input := []byte(`{"model":"gpt-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "model",
				"mode": "replace",
			},
		},
	}

	_, err := ApplyParamOverride(input, override, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestApplyParamOverrideRegexReplaceRequiresPattern 测试 regex_replace 操作缺少 pattern 时应返回错误。
func TestApplyParamOverrideRegexReplaceRequiresPattern(t *testing.T) {
	// regex_replace requires from(pattern) example:
	// {"operations":[{"path":"model","mode":"regex_replace"}]}
	input := []byte(`{"model":"gpt-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "model",
				"mode": "regex_replace",
			},
		},
	}

	_, err := ApplyParamOverride(input, override, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestApplyParamOverrideDelete 测试 delete 操作：删除 temperature 字段。
func TestApplyParamOverrideDelete(t *testing.T) {
	input := []byte(`{"model":"gpt-4","temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "temperature",
				"mode": "delete",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("failed to unmarshal output JSON: %v", err)
	}
	if _, exists := got["temperature"]; exists {
		t.Fatalf("expected temperature to be deleted")
	}
}

// TestApplyParamOverrideDeleteWildcardPath 测试通配符路径的 delete 操作：批量删除 tools 数组中每个元素的 input_examples。
func TestApplyParamOverrideDeleteWildcardPath(t *testing.T) {
	input := []byte(`{"tools":[{"type":"bash","custom":{"input_examples":["a"],"other":1}},{"type":"code","custom":{"input_examples":["b"]}},{"type":"noop","custom":{"other":2}}]}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "tools.*.custom.input_examples",
				"mode": "delete",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"tools":[{"type":"bash","custom":{"other":1}},{"type":"code","custom":{}},{"type":"noop","custom":{"other":2}}]}`, string(out))
}

// TestApplyParamOverrideSetWildcardPath 测试通配符路径的 set 操作：批量设置 tools 数组中每个元素的 enabled 字段。
func TestApplyParamOverrideSetWildcardPath(t *testing.T) {
	input := []byte(`{"tools":[{"custom":{"tag":"A"}},{"custom":{"tag":"B"}},{"custom":{"tag":"C"}}]}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "tools.*.custom.enabled",
				"mode":  "set",
				"value": true,
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}

	var got struct {
		Tools []struct {
			Custom struct {
				Enabled bool `json:"enabled"`
			} `json:"custom"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("failed to unmarshal output JSON: %v", err)
	}

	if !lo.EveryBy(got.Tools, func(item struct {
		Custom struct {
			Enabled bool `json:"enabled"`
		} `json:"custom"`
	}) bool {
		return item.Custom.Enabled
	}) {
		t.Fatalf("expected wildcard set to enable all tools, got: %s", string(out))
	}
}

// TestApplyParamOverrideTrimSpaceWildcardPath 测试通配符路径的 trim_space 操作：批量去除 tools 中每个元素 name 字段的首尾空格。
func TestApplyParamOverrideTrimSpaceWildcardPath(t *testing.T) {
	input := []byte(`{"tools":[{"custom":{"name":" alpha "}},{"custom":{"name":" beta"}},{"custom":{"name":"gamma "}}]}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "tools.*.custom.name",
				"mode": "trim_space",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}

	var got struct {
		Tools []struct {
			Custom struct {
				Name string `json:"name"`
			} `json:"custom"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("failed to unmarshal output JSON: %v", err)
	}

	names := lo.Map(got.Tools, func(item struct {
		Custom struct {
			Name string `json:"name"`
		} `json:"custom"`
	}, _ int) string {
		return item.Custom.Name
	})
	if !reflect.DeepEqual(names, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("unexpected names after wildcard trim_space: %v", names)
	}
}

// TestApplyParamOverrideDeleteWildcardEqualsIndexedPaths 验证通配符路径 delete 操作与逐个索引路径操作的结果一致性。
func TestApplyParamOverrideDeleteWildcardEqualsIndexedPaths(t *testing.T) {
	input := []byte(`{"tools":[{"custom":{"input_examples":["a"],"other":1}},{"custom":{"input_examples":["b"],"other":2}},{"custom":{"input_examples":["c"],"other":3}}]}`)

	wildcardOverride := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "tools.*.custom.input_examples",
				"mode": "delete",
			},
		},
	}

	indexedOverride := map[string]interface{}{
		"operations": lo.Map(lo.Range(3), func(index int, _ int) interface{} {
			return map[string]interface{}{
				"path": fmt.Sprintf("tools.%d.custom.input_examples", index),
				"mode": "delete",
			}
		}),
	}

	wildcardOut, err := ApplyParamOverride(input, wildcardOverride, nil)
	if err != nil {
		t.Fatalf("wildcard ApplyParamOverride returned error: %v", err)
	}

	indexedOut, err := ApplyParamOverride(input, indexedOverride, nil)
	if err != nil {
		t.Fatalf("indexed ApplyParamOverride returned error: %v", err)
	}

	assertJSONEqual(t, string(indexedOut), string(wildcardOut))
}

// TestApplyParamOverrideSetWildcardKeepOrigin 测试通配符 set 操作的 keep_origin 选项：已有值的字段不被覆盖。
func TestApplyParamOverrideSetWildcardKeepOrigin(t *testing.T) {
	input := []byte(`{"tools":[{"custom":{"tag":"A"}},{"custom":{"tag":"B","enabled":false}},{"custom":{"tag":"C"}}]}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":        "tools.*.custom.enabled",
				"mode":        "set",
				"value":       true,
				"keep_origin": true,
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}

	var got struct {
		Tools []struct {
			Custom struct {
				Enabled bool `json:"enabled"`
			} `json:"custom"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("failed to unmarshal output JSON: %v", err)
	}

	enabledValues := lo.Map(got.Tools, func(item struct {
		Custom struct {
			Enabled bool `json:"enabled"`
		} `json:"custom"`
	}, _ int) bool {
		return item.Custom.Enabled
	})
	if !reflect.DeepEqual(enabledValues, []bool{true, false, true}) {
		t.Fatalf("unexpected enabled values after wildcard keep_origin set: %v", enabledValues)
	}
}

// TestApplyParamOverrideTrimSpaceMultiWildcardPath 测试多级通配符路径的 trim_space 操作。
func TestApplyParamOverrideTrimSpaceMultiWildcardPath(t *testing.T) {
	input := []byte(`{"tools":[{"custom":{"items":[{"name":" alpha "},{"name":" beta "}]}},{"custom":{"items":[{"name":" gamma"}]}}]}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "tools.*.custom.items.*.name",
				"mode": "trim_space",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}

	var got struct {
		Tools []struct {
			Custom struct {
				Items []struct {
					Name string `json:"name"`
				} `json:"items"`
			} `json:"custom"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("failed to unmarshal output JSON: %v", err)
	}

	names := lo.FlatMap(got.Tools, func(tool struct {
		Custom struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
		} `json:"custom"`
	}, _ int) []string {
		return lo.Map(tool.Custom.Items, func(item struct {
			Name string `json:"name"`
		}, _ int) string {
			return item.Name
		})
	})
	if !reflect.DeepEqual(names, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("unexpected names after multi wildcard trim_space: %v", names)
	}
}

// TestApplyParamOverrideSet 测试 set 操作：设置 temperature 为 0.1。
func TestApplyParamOverrideSet(t *testing.T) {
	input := []byte(`{"model":"gpt-4","temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "temperature",
				"mode":  "set",
				"value": 0.1,
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4","temperature":0.1}`, string(out))
}

// TestApplyParamOverrideSetWithDescriptionKeepsCompatibility 测试带 description 字段的 set 操作与不带 description 的行为一致性。
func TestApplyParamOverrideSetWithDescriptionKeepsCompatibility(t *testing.T) {
	input := []byte(`{"model":"gpt-4","temperature":0.7}`)
	overrideWithoutDesc := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "temperature",
				"mode":  "set",
				"value": 0.1,
			},
		},
	}
	overrideWithDesc := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"description": "set temperature for deterministic output",
				"path":        "temperature",
				"mode":        "set",
				"value":       0.1,
			},
		},
	}

	outWithoutDesc, err := ApplyParamOverride(input, overrideWithoutDesc, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride without description returned error: %v", err)
	}

	outWithDesc, err := ApplyParamOverride(input, overrideWithDesc, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride with description returned error: %v", err)
	}

	assertJSONEqual(t, string(outWithoutDesc), string(outWithDesc))
	assertJSONEqual(t, `{"model":"gpt-4","temperature":0.1}`, string(outWithDesc))
}

// TestApplyParamOverrideSetKeepOrigin 测试 set 操作的 keep_origin 选项：当字段已有值时不覆盖。
func TestApplyParamOverrideSetKeepOrigin(t *testing.T) {
	input := []byte(`{"model":"gpt-4","temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":        "temperature",
				"mode":        "set",
				"value":       0.1,
				"keep_origin": true,
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4","temperature":0.7}`, string(out))
}

// TestApplyParamOverrideMove 测试 move 操作：将 model 字段移动到 meta.model 路径下。
func TestApplyParamOverrideMove(t *testing.T) {
	input := []byte(`{"model":"gpt-4","meta":{"x":1}}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "move",
				"from": "model",
				"to":   "meta.model",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"meta":{"x":1,"model":"gpt-4"}}`, string(out))
}

// TestApplyParamOverrideMoveMissingSource 测试 move 操作源字段不存在时应返回错误。
func TestApplyParamOverrideMoveMissingSource(t *testing.T) {
	input := []byte(`{"meta":{"x":1}}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "move",
				"from": "model",
				"to":   "meta.model",
			},
		},
	}

	_, err := ApplyParamOverride(input, override, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestApplyParamOverridePrependAppendString 测试 prepend 和 append 操作的字符串拼接：在 model 前添加 "openai/"，后添加 "-latest"。
func TestApplyParamOverridePrependAppendString(t *testing.T) {
	input := []byte(`{"model":"gpt-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "model",
				"mode":  "prepend",
				"value": "openai/",
			},
			map[string]interface{}{
				"path":  "model",
				"mode":  "append",
				"value": "-latest",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"openai/gpt-4-latest"}`, string(out))
}

// TestApplyParamOverridePrependAppendArray 测试 prepend 和 append 操作的数组拼接。
func TestApplyParamOverridePrependAppendArray(t *testing.T) {
	input := []byte(`{"arr":[1,2]}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "arr",
				"mode":  "prepend",
				"value": 0,
			},
			map[string]interface{}{
				"path":  "arr",
				"mode":  "append",
				"value": []interface{}{3, 4},
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"arr":[0,1,2,3,4]}`, string(out))
}

// TestApplyParamOverrideAppendObjectMergeKeepOrigin 测试对象类型的 append 操作配合 keep_origin：已有键的值不被覆盖。
func TestApplyParamOverrideAppendObjectMergeKeepOrigin(t *testing.T) {
	input := []byte(`{"obj":{"a":1}}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":        "obj",
				"mode":        "append",
				"keep_origin": true,
				"value": map[string]interface{}{
					"a": 2,
					"b": 3,
				},
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"obj":{"a":1,"b":3}}`, string(out))
}

// TestApplyParamOverrideAppendObjectMergeOverride 测试对象类型的 append 操作默认覆盖已有键的值。
func TestApplyParamOverrideAppendObjectMergeOverride(t *testing.T) {
	input := []byte(`{"obj":{"a":1}}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "obj",
				"mode": "append",
				"value": map[string]interface{}{
					"a": 2,
					"b": 3,
				},
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"obj":{"a":2,"b":3}}`, string(out))
}

// TestApplyParamOverrideConditionORDefault 测试条件执行的默认 OR 逻辑：任一条件满足即执行操作。
func TestApplyParamOverrideConditionORDefault(t *testing.T) {
	input := []byte(`{"model":"gpt-4","temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "temperature",
				"mode":  "set",
				"value": 0.1,
				"conditions": []interface{}{
					map[string]interface{}{
						"path":  "model",
						"mode":  "prefix",
						"value": "gpt",
					},
					map[string]interface{}{
						"path":  "model",
						"mode":  "prefix",
						"value": "claude",
					},
				},
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4","temperature":0.1}`, string(out))
}

// TestApplyParamOverrideConditionAND 测试条件执行的 AND 逻辑：所有条件都满足才执行操作。
func TestApplyParamOverrideConditionAND(t *testing.T) {
	input := []byte(`{"model":"gpt-4","temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "temperature",
				"mode":  "set",
				"value": 0.1,
				"logic": "AND",
				"conditions": []interface{}{
					map[string]interface{}{
						"path":  "model",
						"mode":  "prefix",
						"value": "gpt",
					},
					map[string]interface{}{
						"path":  "temperature",
						"mode":  "gt",
						"value": 0.5,
					},
				},
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4","temperature":0.1}`, string(out))
}

// TestApplyParamOverrideConditionInvert 测试条件的 invert（取反）选项：条件满足时反而不执行操作。
func TestApplyParamOverrideConditionInvert(t *testing.T) {
	input := []byte(`{"model":"gpt-4","temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "temperature",
				"mode":  "set",
				"value": 0.1,
				"conditions": []interface{}{
					map[string]interface{}{
						"path":   "model",
						"mode":   "prefix",
						"value":  "gpt",
						"invert": true,
					},
				},
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4","temperature":0.7}`, string(out))
}

// TestApplyParamOverrideConditionPassMissingKey 测试 pass_missing_key 选项：当条件引用的字段不存在时视为条件通过。
func TestApplyParamOverrideConditionPassMissingKey(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "temperature",
				"mode":  "set",
				"value": 0.1,
				"conditions": []interface{}{
					map[string]interface{}{
						"path":             "model",
						"mode":             "prefix",
						"value":            "gpt",
						"pass_missing_key": true,
					},
				},
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.1}`, string(out))
}

// TestApplyParamOverrideConditionFromContext 测试从上下文（ctx）中读取条件值：请求体中没有 model 字段，但上下文中有。
func TestApplyParamOverrideConditionFromContext(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "temperature",
				"mode":  "set",
				"value": 0.1,
				"conditions": []interface{}{
					map[string]interface{}{
						"path":  "model",
						"mode":  "prefix",
						"value": "gpt",
					},
				},
			},
		},
	}
	ctx := map[string]interface{}{
		"model": "gpt-4",
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.1}`, string(out))
}

// TestApplyParamOverrideNegativeIndexPath 测试负数索引路径：arr.-1 表示数组最后一个元素。
func TestApplyParamOverrideNegativeIndexPath(t *testing.T) {
	input := []byte(`{"arr":[{"model":"a"},{"model":"b"}]}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "arr.-1.model",
				"mode":  "set",
				"value": "c",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"arr":[{"model":"a"},{"model":"c"}]}`, string(out))
}

// TestApplyParamOverrideRegexReplaceInvalidPattern 测试无效正则表达式时应返回错误。
func TestApplyParamOverrideRegexReplaceInvalidPattern(t *testing.T) {
	// regex_replace invalid pattern example:
	// {"operations":[{"path":"model","mode":"regex_replace","from":"(","to":"x"}]}
	input := []byte(`{"model":"gpt-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "model",
				"mode": "regex_replace",
				"from": "(",
				"to":   "x",
			},
		},
	}

	_, err := ApplyParamOverride(input, override, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestApplyParamOverrideCopy 测试 copy 操作：将 model 字段复制到 original_model。
func TestApplyParamOverrideCopy(t *testing.T) {
	// copy example:
	// {"operations":[{"mode":"copy","from":"model","to":"original_model"}]}
	input := []byte(`{"model":"gpt-4","temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "copy",
				"from": "model",
				"to":   "original_model",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4","original_model":"gpt-4","temperature":0.7}`, string(out))
}

// TestApplyParamOverrideCopyMissingSource 测试 copy 操作源字段不存在时应返回错误。
func TestApplyParamOverrideCopyMissingSource(t *testing.T) {
	// copy missing source example:
	// {"operations":[{"mode":"copy","from":"model","to":"original_model"}]}
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "copy",
				"from": "model",
				"to":   "original_model",
			},
		},
	}

	_, err := ApplyParamOverride(input, override, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestApplyParamOverrideCopyRequiresFromTo 测试 copy 操作缺少 from/to 参数时应返回错误。
func TestApplyParamOverrideCopyRequiresFromTo(t *testing.T) {
	// copy requires from/to example:
	// {"operations":[{"mode":"copy"}]}
	input := []byte(`{"model":"gpt-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "copy",
			},
		},
	}

	_, err := ApplyParamOverride(input, override, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestApplyParamOverrideEnsurePrefix 测试 ensure_prefix 操作：当 model 缺少 "openai/" 前缀时自动添加。
func TestApplyParamOverrideEnsurePrefix(t *testing.T) {
	// ensure_prefix example:
	// {"operations":[{"path":"model","mode":"ensure_prefix","value":"openai/"}]}
	input := []byte(`{"model":"gpt-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "model",
				"mode":  "ensure_prefix",
				"value": "openai/",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"openai/gpt-4"}`, string(out))
}

// TestApplyParamOverrideEnsurePrefixNoop 测试 ensure_prefix 操作的空操作：当前缀已存在时不重复添加。
func TestApplyParamOverrideEnsurePrefixNoop(t *testing.T) {
	// ensure_prefix no-op example:
	// {"operations":[{"path":"model","mode":"ensure_prefix","value":"openai/"}]}
	input := []byte(`{"model":"openai/gpt-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "model",
				"mode":  "ensure_prefix",
				"value": "openai/",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"openai/gpt-4"}`, string(out))
}

// TestApplyParamOverrideEnsureSuffix 测试 ensure_suffix 操作：当 model 缺少 "-latest" 后缀时自动添加。
func TestApplyParamOverrideEnsureSuffix(t *testing.T) {
	// ensure_suffix example:
	// {"operations":[{"path":"model","mode":"ensure_suffix","value":"-latest"}]}
	input := []byte(`{"model":"gpt-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "model",
				"mode":  "ensure_suffix",
				"value": "-latest",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4-latest"}`, string(out))
}

// TestApplyParamOverrideEnsureSuffixNoop 测试 ensure_suffix 操作的空操作：当后缀已存在时不重复添加。
func TestApplyParamOverrideEnsureSuffixNoop(t *testing.T) {
	// ensure_suffix no-op example:
	// {"operations":[{"path":"model","mode":"ensure_suffix","value":"-latest"}]}
	input := []byte(`{"model":"gpt-4-latest"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "model",
				"mode":  "ensure_suffix",
				"value": "-latest",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4-latest"}`, string(out))
}

// TestApplyParamOverrideEnsureRequiresValue 测试 ensure_prefix 操作缺少 value 时应返回错误。
func TestApplyParamOverrideEnsureRequiresValue(t *testing.T) {
	// ensure_prefix requires value example:
	// {"operations":[{"path":"model","mode":"ensure_prefix"}]}
	input := []byte(`{"model":"gpt-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "model",
				"mode": "ensure_prefix",
			},
		},
	}

	_, err := ApplyParamOverride(input, override, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestApplyParamOverrideTrimSpace 测试 trim_space 操作：去除 model 字段值的首尾空格和换行符。
func TestApplyParamOverrideTrimSpace(t *testing.T) {
	// trim_space example:
	// {"operations":[{"path":"model","mode":"trim_space"}]}
	input := []byte("{\"model\":\"  gpt-4 \\n\"}")
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "model",
				"mode": "trim_space",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4"}`, string(out))
}

// TestApplyParamOverrideToLower 测试 to_lower 操作：将 model 字段值转为小写。
func TestApplyParamOverrideToLower(t *testing.T) {
	// to_lower example:
	// {"operations":[{"path":"model","mode":"to_lower"}]}
	input := []byte(`{"model":"GPT-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "model",
				"mode": "to_lower",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4"}`, string(out))
}

// TestApplyParamOverrideToUpper 测试 to_upper 操作：将 model 字段值转为大写。
func TestApplyParamOverrideToUpper(t *testing.T) {
	// to_upper example:
	// {"operations":[{"path":"model","mode":"to_upper"}]}
	input := []byte(`{"model":"gpt-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "model",
				"mode": "to_upper",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"GPT-4"}`, string(out))
}

// TestApplyParamOverrideReturnError 测试 return_error 操作：在满足条件时强制返回指定的错误响应。
func TestApplyParamOverrideReturnError(t *testing.T) {
	input := []byte(`{"model":"gemini-2.5-pro"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "return_error",
				"value": map[string]interface{}{
					"message":     "forced bad request by param override",
					"status_code": 422,
					"code":        "forced_bad_request",
					"type":        "invalid_request_error",
					"skip_retry":  true,
				},
				"conditions": []interface{}{
					map[string]interface{}{
						"path":  "retry.is_retry",
						"mode":  "full",
						"value": true,
					},
				},
			},
		},
	}
	ctx := map[string]interface{}{
		"retry": map[string]interface{}{
			"index":    1,
			"is_retry": true,
		},
	}

	_, err := ApplyParamOverride(input, override, ctx)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	returnErr, ok := AsParamOverrideReturnError(err)
	if !ok {
		t.Fatalf("expected ParamOverrideReturnError, got %T: %v", err, err)
	}
	if returnErr.StatusCode != 422 {
		t.Fatalf("expected status 422, got %d", returnErr.StatusCode)
	}
	if returnErr.Code != "forced_bad_request" {
		t.Fatalf("expected code forced_bad_request, got %s", returnErr.Code)
	}
	if !returnErr.SkipRetry {
		t.Fatalf("expected skip_retry true")
	}
}

// TestApplyParamOverridePruneObjectsByTypeString 测试 prune_objects 操作：按 type 字符串值递归删除匹配的对象。
func TestApplyParamOverridePruneObjectsByTypeString(t *testing.T) {
	input := []byte(`{
		"messages":[
			{"role":"assistant","content":[
				{"type":"output_text","text":"a"},
				{"type":"redacted_thinking","text":"secret"},
				{"type":"tool_call","name":"tool_a"}
			]},
			{"role":"assistant","content":[
				{"type":"output_text","text":"b"},
				{"type":"wrapper","parts":[
					{"type":"redacted_thinking","text":"secret2"},
					{"type":"output_text","text":"c"}
				]}
			]}
		]
	}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode":  "prune_objects",
				"value": "redacted_thinking",
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{
		"messages":[
			{"role":"assistant","content":[
				{"type":"output_text","text":"a"},
				{"type":"tool_call","name":"tool_a"}
			]},
			{"role":"assistant","content":[
				{"type":"output_text","text":"b"},
				{"type":"wrapper","parts":[
					{"type":"output_text","text":"c"}
				]}
			]}
		]
	}`, string(out))
}

// TestApplyParamOverridePruneObjectsWhereAndPath 测试 prune_objects 操作的 where + path 组合：在指定路径下按条件删除对象。
func TestApplyParamOverridePruneObjectsWhereAndPath(t *testing.T) {
	input := []byte(`{
		"a":{"items":[{"type":"redacted_thinking","id":1},{"type":"output_text","id":2}]},
		"b":{"items":[{"type":"redacted_thinking","id":3},{"type":"output_text","id":4}]}
	}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path": "a",
				"mode": "prune_objects",
				"value": map[string]interface{}{
					"where": map[string]interface{}{
						"type": "redacted_thinking",
					},
				},
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{
		"a":{"items":[{"type":"output_text","id":2}]},
		"b":{"items":[{"type":"redacted_thinking","id":3},{"type":"output_text","id":4}]}
	}`, string(out))
}

// TestApplyParamOverrideNormalizeThinkingSignatureUnsupported 测试不支持的 normalize_thinking_signature 操作应返回错误。
func TestApplyParamOverrideNormalizeThinkingSignatureUnsupported(t *testing.T) {
	input := []byte(`{"items":[{"type":"redacted_thinking"}]}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "normalize_thinking_signature",
			},
		},
	}

	_, err := ApplyParamOverride(input, override, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestApplyParamOverrideConditionFromRetryAndLastErrorContext 测试基于重试索引和上一次错误的条件执行。
func TestApplyParamOverrideConditionFromRetryAndLastErrorContext(t *testing.T) {
	info := &RelayInfo{
		RetryIndex: 1,
		LastError: types.WithOpenAIError(types.OpenAIError{
			Message: "invalid thinking signature",
			Type:    "invalid_request_error",
			Code:    "bad_thought_signature",
		}, 400),
	}
	ctx := BuildParamOverrideContext(info)

	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "temperature",
				"mode":  "set",
				"value": 0.1,
				"logic": "AND",
				"conditions": []interface{}{
					map[string]interface{}{
						"path":  "is_retry",
						"mode":  "full",
						"value": true,
					},
					map[string]interface{}{
						"path":  "last_error.code",
						"mode":  "contains",
						"value": "thought_signature",
					},
				},
			},
		},
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.1}`, string(out))
}

// TestApplyParamOverrideConditionFromRequestHeaders 测试基于请求头的条件执行。
func TestApplyParamOverrideConditionFromRequestHeaders(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "temperature",
				"mode":  "set",
				"value": 0.1,
				"conditions": []interface{}{
					map[string]interface{}{
						"path":  "request_headers.authorization",
						"mode":  "contains",
						"value": "Bearer ",
					},
				},
			},
		},
	}
	ctx := map[string]interface{}{
		"request_headers": map[string]interface{}{
			"authorization": "Bearer token-123",
		},
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.1}`, string(out))
}

// TestApplyParamOverrideSetHeaderAndUseInLaterCondition 测试 set_header 操作设置的头信息可在后续条件中引用。
func TestApplyParamOverrideSetHeaderAndUseInLaterCondition(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode":  "set_header",
				"path":  "X-Debug-Mode",
				"value": "enabled",
			},
			map[string]interface{}{
				"path":  "temperature",
				"mode":  "set",
				"value": 0.1,
				"conditions": []interface{}{
					map[string]interface{}{
						"path":  "header_override.x-debug-mode",
						"mode":  "full",
						"value": "enabled",
					},
				},
			},
		},
	}

	out, err := ApplyParamOverride(input, override, nil)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.1}`, string(out))
}

// TestApplyParamOverrideCopyHeaderFromRequestHeaders 测试 copy_header 操作从请求头复制到 header_override。
func TestApplyParamOverrideCopyHeaderFromRequestHeaders(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "copy_header",
				"from": "Authorization",
				"to":   "X-Upstream-Auth",
			},
			map[string]interface{}{
				"path":  "temperature",
				"mode":  "set",
				"value": 0.1,
				"conditions": []interface{}{
					map[string]interface{}{
						"path":  "header_override.x-upstream-auth",
						"mode":  "contains",
						"value": "Bearer ",
					},
				},
			},
		},
	}
	ctx := map[string]interface{}{
		"request_headers": map[string]interface{}{
			"authorization": "Bearer token-123",
		},
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.1}`, string(out))
}

// TestApplyParamOverridePassHeadersSkipsMissingHeaders 测试 pass_headers 操作跳过不存在的请求头。
func TestApplyParamOverridePassHeadersSkipsMissingHeaders(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode":  "pass_headers",
				"value": []interface{}{"X-Codex-Beta-Features", "Session_id"},
			},
		},
	}
	ctx := map[string]interface{}{
		"request_headers": map[string]interface{}{
			"session_id": "sess-123",
		},
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.7}`, string(out))

	headers, ok := ctx["header_override"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected header_override context map")
	}
	if headers["session_id"] != "sess-123" {
		t.Fatalf("expected session_id to be passed, got: %v", headers["session_id"])
	}
	if _, exists := headers["x-codex-beta-features"]; exists {
		t.Fatalf("expected missing header to be skipped")
	}
}

// TestApplyParamOverrideCopyHeaderSkipsMissingSource 测试 copy_header 操作在源头不存在时跳过。
func TestApplyParamOverrideCopyHeaderSkipsMissingSource(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "copy_header",
				"from": "X-Missing-Header",
				"to":   "X-Upstream-Auth",
			},
		},
	}
	ctx := map[string]interface{}{
		"request_headers": map[string]interface{}{
			"authorization": "Bearer token-123",
		},
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.7}`, string(out))

	headers, ok := ctx["header_override"].(map[string]interface{})
	if !ok {
		return
	}
	if _, exists := headers["x-upstream-auth"]; exists {
		t.Fatalf("expected X-Upstream-Auth to be skipped when source header is missing")
	}
}

// TestApplyParamOverrideMoveHeaderSkipsMissingSource 测试 move_header 操作在源头不存在时跳过。
func TestApplyParamOverrideMoveHeaderSkipsMissingSource(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "move_header",
				"from": "X-Missing-Header",
				"to":   "X-Upstream-Auth",
			},
		},
	}
	ctx := map[string]interface{}{
		"request_headers": map[string]interface{}{
			"authorization": "Bearer token-123",
		},
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.7}`, string(out))

	headers, ok := ctx["header_override"].(map[string]interface{})
	if !ok {
		return
	}
	if _, exists := headers["x-upstream-auth"]; exists {
		t.Fatalf("expected X-Upstream-Auth to be skipped when source header is missing")
	}
}

// TestApplyParamOverrideSyncFieldsHeaderToJSON 测试 sync_fields 操作：从请求头同步值到 JSON body。
func TestApplyParamOverrideSyncFieldsHeaderToJSON(t *testing.T) {
	input := []byte(`{"model":"gpt-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "sync_fields",
				"from": "header:session_id",
				"to":   "json:prompt_cache_key",
			},
		},
	}
	ctx := map[string]interface{}{
		"request_headers": map[string]interface{}{
			"session_id": "sess-123",
		},
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4","prompt_cache_key":"sess-123"}`, string(out))
}

// TestApplyParamOverrideSyncFieldsJSONToHeader 测试 sync_fields 操作：从 JSON body 同步值到请求头。
func TestApplyParamOverrideSyncFieldsJSONToHeader(t *testing.T) {
	input := []byte(`{"model":"gpt-4","prompt_cache_key":"cache-abc"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "sync_fields",
				"from": "header:session_id",
				"to":   "json:prompt_cache_key",
			},
		},
	}
	ctx := map[string]interface{}{}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4","prompt_cache_key":"cache-abc"}`, string(out))

	headers, ok := ctx["header_override"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected header_override context map")
	}
	if headers["session_id"] != "cache-abc" {
		t.Fatalf("expected session_id to be synced from prompt_cache_key, got: %v", headers["session_id"])
	}
}

// TestApplyParamOverrideSyncFieldsNoChangeWhenBothExist 测试 sync_fields 操作：两侧都有值时不进行同步。
func TestApplyParamOverrideSyncFieldsNoChangeWhenBothExist(t *testing.T) {
	input := []byte(`{"model":"gpt-4","prompt_cache_key":"cache-body"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "sync_fields",
				"from": "header:session_id",
				"to":   "json:prompt_cache_key",
			},
		},
	}
	ctx := map[string]interface{}{
		"request_headers": map[string]interface{}{
			"session_id": "cache-header",
		},
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-4","prompt_cache_key":"cache-body"}`, string(out))

	headers, _ := ctx["header_override"].(map[string]interface{})
	if headers != nil {
		if _, exists := headers["session_id"]; exists {
			t.Fatalf("expected no override when both sides already have value")
		}
	}
}

// TestApplyParamOverrideSyncFieldsInvalidTarget 测试 sync_fields 操作的无效目标前缀应返回错误。
func TestApplyParamOverrideSyncFieldsInvalidTarget(t *testing.T) {
	input := []byte(`{"model":"gpt-4"}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "sync_fields",
				"from": "foo:session_id",
				"to":   "json:prompt_cache_key",
			},
		},
	}

	_, err := ApplyParamOverride(input, override, nil)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// TestApplyParamOverrideSetHeaderKeepOrigin 测试 set_header 操作的 keep_origin 选项。
func TestApplyParamOverrideSetHeaderKeepOrigin(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode":        "set_header",
				"path":        "X-Feature-Flag",
				"value":       "new-value",
				"keep_origin": true,
			},
		},
	}
	ctx := map[string]interface{}{
		"header_override": map[string]interface{}{
			"x-feature-flag": "legacy-value",
		},
	}

	_, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	headers, ok := ctx["header_override"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected header_override context map")
	}
	if headers["x-feature-flag"] != "legacy-value" {
		t.Fatalf("expected keep_origin to preserve old value, got: %v", headers["x-feature-flag"])
	}
}

// TestApplyParamOverrideSetHeaderMapRewritesCommaSeparatedHeader 测试 set_header 操作的 map 值模式：重写逗号分隔的 header 值。
func TestApplyParamOverrideSetHeaderMapRewritesCommaSeparatedHeader(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "set_header",
				"path": "anthropic-beta",
				"value": map[string]interface{}{
					"advanced-tool-use-2025-11-20": nil,
					"computer-use-2025-01-24":      "computer-use-2025-01-24",
				},
			},
		},
	}
	ctx := map[string]interface{}{
		"request_headers": map[string]interface{}{
			"anthropic-beta": "advanced-tool-use-2025-11-20, computer-use-2025-01-24",
		},
	}

	_, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}

	headers, ok := ctx["header_override"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected header_override context map")
	}
	if headers["anthropic-beta"] != "computer-use-2025-01-24" {
		t.Fatalf("expected anthropic-beta to keep only mapped value, got: %v", headers["anthropic-beta"])
	}
}

// TestApplyParamOverrideSetHeaderMapDeleteWholeHeaderWhenAllTokensCleared 测试当所有 token 都被设为 nil 时整个 header 被删除。
func TestApplyParamOverrideSetHeaderMapDeleteWholeHeaderWhenAllTokensCleared(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "set_header",
				"path": "anthropic-beta",
				"value": map[string]interface{}{
					"advanced-tool-use-2025-11-20": nil,
					"computer-use-2025-01-24":      nil,
				},
			},
		},
	}
	ctx := map[string]interface{}{
		"header_override": map[string]interface{}{
			"anthropic-beta": "advanced-tool-use-2025-11-20,computer-use-2025-01-24",
		},
	}

	_, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}

	headers, ok := ctx["header_override"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected header_override context map")
	}
	if _, exists := headers["anthropic-beta"]; exists {
		t.Fatalf("expected anthropic-beta to be deleted when all mapped values are null")
	}
}

// TestApplyParamOverrideSetHeaderMapAppendsTokens 测试 $append 操作：追加新的 header token 并去重。
func TestApplyParamOverrideSetHeaderMapAppendsTokens(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "set_header",
				"path": "anthropic-beta",
				"value": map[string]interface{}{
					"$append": []interface{}{"context-1m-2025-08-07", "computer-use-2025-01-24"},
				},
			},
		},
	}
	ctx := map[string]interface{}{
		"header_override": map[string]interface{}{
			"anthropic-beta": "computer-use-2025-01-24",
		},
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.7}`, string(out))

	headers, ok := ctx["header_override"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected header_override context map")
	}
	if headers["anthropic-beta"] != "computer-use-2025-01-24,context-1m-2025-08-07" {
		t.Fatalf("expected anthropic-beta to append new token without duplicates, got: %v", headers["anthropic-beta"])
	}
}

// TestApplyParamOverrideSetHeaderMapAppendsTokensWhenHeaderMissing 测试 $append 操作在 header 不存在时创建新 header。
func TestApplyParamOverrideSetHeaderMapAppendsTokensWhenHeaderMissing(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "set_header",
				"path": "anthropic-beta",
				"value": map[string]interface{}{
					"$append": []interface{}{"context-1m-2025-08-07", "computer-use-2025-01-24"},
				},
			},
		},
	}

	ctx := map[string]interface{}{}
	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.7}`, string(out))

	headers, ok := ctx["header_override"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected header_override context map")
	}
	if headers["anthropic-beta"] != "context-1m-2025-08-07,computer-use-2025-01-24" {
		t.Fatalf("expected anthropic-beta to be created from appended tokens, got: %v", headers["anthropic-beta"])
	}
}

// TestApplyParamOverrideSetHeaderMapKeepOnlyDeclaredDropsUndeclaredTokens 测试 $keep_only_declared 选项：移除未声明的 token。
func TestApplyParamOverrideSetHeaderMapKeepOnlyDeclaredDropsUndeclaredTokens(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "set_header",
				"path": "anthropic-beta",
				"value": map[string]interface{}{
					"computer-use-2025-01-24": "computer-use-2025-01-24",
					"$append":                 []interface{}{"context-1m-2025-08-07"},
					"$keep_only_declared":     true,
				},
			},
		},
	}
	ctx := map[string]interface{}{
		"header_override": map[string]interface{}{
			"anthropic-beta": "advanced-tool-use-2025-11-20,computer-use-2025-01-24",
		},
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.7}`, string(out))

	headers, ok := ctx["header_override"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected header_override context map")
	}
	if headers["anthropic-beta"] != "computer-use-2025-01-24,context-1m-2025-08-07" {
		t.Fatalf("expected anthropic-beta to keep only declared tokens, got: %v", headers["anthropic-beta"])
	}
}

// TestApplyParamOverrideSetHeaderMapKeepOnlyDeclaredDeletesHeaderWhenNothingDeclaredMatches 测试 $keep_only_declared 当无声明 token 匹配时删除整个 header。
func TestApplyParamOverrideSetHeaderMapKeepOnlyDeclaredDeletesHeaderWhenNothingDeclaredMatches(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"mode": "set_header",
				"path": "anthropic-beta",
				"value": map[string]interface{}{
					"computer-use-2025-01-24": "computer-use-2025-01-24",
					"$keep_only_declared":     true,
				},
			},
		},
	}
	ctx := map[string]interface{}{
		"header_override": map[string]interface{}{
			"anthropic-beta": "advanced-tool-use-2025-11-20",
		},
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.7}`, string(out))

	headers, ok := ctx["header_override"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected header_override context map")
	}
	if _, exists := headers["anthropic-beta"]; exists {
		t.Fatalf("expected anthropic-beta to be deleted when no declared tokens remain, got: %v", headers["anthropic-beta"])
	}
}

// TestApplyParamOverrideConditionsObjectShorthand 测试 conditions 的对象简写语法。
func TestApplyParamOverrideConditionsObjectShorthand(t *testing.T) {
	input := []byte(`{"temperature":0.7}`)
	override := map[string]interface{}{
		"operations": []interface{}{
			map[string]interface{}{
				"path":  "temperature",
				"mode":  "set",
				"value": 0.1,
				"logic": "AND",
				"conditions": map[string]interface{}{
					"is_retry":               true,
					"last_error.status_code": 400.0,
				},
			},
		},
	}
	ctx := map[string]interface{}{
		"is_retry": true,
		"last_error": map[string]interface{}{
			"status_code": 400.0,
		},
	}

	out, err := ApplyParamOverride(input, override, ctx)
	if err != nil {
		t.Fatalf("ApplyParamOverride returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.1}`, string(out))
}

// TestApplyParamOverrideWithRelayInfoSyncRuntimeHeaders 测试 ApplyParamOverrideWithRelayInfo 的运行时头同步功能。
func TestApplyParamOverrideWithRelayInfoSyncRuntimeHeaders(t *testing.T) {
	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ParamOverride: map[string]interface{}{
				"operations": []interface{}{
					map[string]interface{}{
						"mode":  "set_header",
						"path":  "X-Injected-By-Param-Override",
						"value": "enabled",
					},
					map[string]interface{}{
						"mode": "delete_header",
						"path": "X-Delete-Me",
					},
				},
			},
			HeadersOverride: map[string]interface{}{
				"X-Delete-Me": "legacy",
				"X-Keep-Me":   "keep",
			},
		},
	}

	input := []byte(`{"temperature":0.7}`)
	out, err := ApplyParamOverrideWithRelayInfo(input, info)
	if err != nil {
		t.Fatalf("ApplyParamOverrideWithRelayInfo returned error: %v", err)
	}
	assertJSONEqual(t, `{"temperature":0.7}`, string(out))

	if !info.UseRuntimeHeadersOverride {
		t.Fatalf("expected runtime header override to be enabled")
	}
	if info.RuntimeHeadersOverride["x-keep-me"] != "keep" {
		t.Fatalf("expected x-keep-me header to be preserved, got: %v", info.RuntimeHeadersOverride["x-keep-me"])
	}
	if info.RuntimeHeadersOverride["x-injected-by-param-override"] != "enabled" {
		t.Fatalf("expected x-injected-by-param-override header to be set, got: %v", info.RuntimeHeadersOverride["x-injected-by-param-override"])
	}
	if _, exists := info.RuntimeHeadersOverride["x-delete-me"]; exists {
		t.Fatalf("expected x-delete-me header to be deleted")
	}
}

// TestApplyParamOverrideWithRelayInfoMixedLegacyAndOperations 测试 RelayInfo 中传统覆盖与 operations 混合使用。
func TestApplyParamOverrideWithRelayInfoMixedLegacyAndOperations(t *testing.T) {
	info := &RelayInfo{
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
		},
		ChannelMeta: &ChannelMeta{
			ParamOverride: map[string]interface{}{
				"temperature": 0.2,
				"operations": []interface{}{
					map[string]interface{}{
						"mode":  "pass_headers",
						"value": []interface{}{"Originator"},
					},
				},
			},
			HeadersOverride: map[string]interface{}{
				"X-Static": "legacy-static",
			},
		},
	}

	out, err := ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-5","temperature":0.7}`), info)
	if err != nil {
		t.Fatalf("ApplyParamOverrideWithRelayInfo returned error: %v", err)
	}
	assertJSONEqual(t, `{"model":"gpt-5","temperature":0.2}`, string(out))

	if !info.UseRuntimeHeadersOverride {
		t.Fatalf("expected runtime header override to be enabled")
	}
	if info.RuntimeHeadersOverride["x-static"] != "legacy-static" {
		t.Fatalf("expected x-static to be preserved, got: %v", info.RuntimeHeadersOverride["x-static"])
	}
	if info.RuntimeHeadersOverride["originator"] != "Codex CLI" {
		t.Fatalf("expected originator header to be passed, got: %v", info.RuntimeHeadersOverride["originator"])
	}
}

// TestApplyParamOverrideWithRelayInfoMoveAndCopyHeaders 测试通过 RelayInfo 执行 move_header 和 copy_header 操作。
func TestApplyParamOverrideWithRelayInfoMoveAndCopyHeaders(t *testing.T) {
	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ParamOverride: map[string]interface{}{
				"operations": []interface{}{
					map[string]interface{}{
						"mode": "move_header",
						"from": "X-Legacy-Trace",
						"to":   "X-Trace",
					},
					map[string]interface{}{
						"mode": "copy_header",
						"from": "X-Trace",
						"to":   "X-Trace-Backup",
					},
				},
			},
			HeadersOverride: map[string]interface{}{
				"X-Legacy-Trace": "trace-123",
			},
		},
	}

	input := []byte(`{"temperature":0.7}`)
	_, err := ApplyParamOverrideWithRelayInfo(input, info)
	if err != nil {
		t.Fatalf("ApplyParamOverrideWithRelayInfo returned error: %v", err)
	}
	if _, exists := info.RuntimeHeadersOverride["x-legacy-trace"]; exists {
		t.Fatalf("expected source header to be removed after move")
	}
	if info.RuntimeHeadersOverride["x-trace"] != "trace-123" {
		t.Fatalf("expected x-trace to be set, got: %v", info.RuntimeHeadersOverride["x-trace"])
	}
	if info.RuntimeHeadersOverride["x-trace-backup"] != "trace-123" {
		t.Fatalf("expected x-trace-backup to be copied, got: %v", info.RuntimeHeadersOverride["x-trace-backup"])
	}
}

// TestApplyParamOverrideWithRelayInfoSetHeaderMapRewritesAnthropicBeta 测试通过 RelayInfo 重写 anthropic-beta header。
func TestApplyParamOverrideWithRelayInfoSetHeaderMapRewritesAnthropicBeta(t *testing.T) {
	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ParamOverride: map[string]interface{}{
				"operations": []interface{}{
					map[string]interface{}{
						"mode": "set_header",
						"path": "anthropic-beta",
						"value": map[string]interface{}{
							"advanced-tool-use-2025-11-20": nil,
							"computer-use-2025-01-24":      "computer-use-2025-01-24",
						},
					},
				},
			},
			HeadersOverride: map[string]interface{}{
				"anthropic-beta": "advanced-tool-use-2025-11-20, computer-use-2025-01-24",
			},
		},
	}

	_, err := ApplyParamOverrideWithRelayInfo([]byte(`{"temperature":0.7}`), info)
	if err != nil {
		t.Fatalf("ApplyParamOverrideWithRelayInfo returned error: %v", err)
	}

	if !info.UseRuntimeHeadersOverride {
		t.Fatalf("expected runtime header override to be enabled")
	}
	if info.RuntimeHeadersOverride["anthropic-beta"] != "computer-use-2025-01-24" {
		t.Fatalf("expected anthropic-beta to be rewritten, got: %v", info.RuntimeHeadersOverride["anthropic-beta"])
	}
}

// TestGetEffectiveHeaderOverrideUsesRuntimeOverrideAsFinalResult 测试运行时头覆盖作为最终结果使用。
func TestGetEffectiveHeaderOverrideUsesRuntimeOverrideAsFinalResult(t *testing.T) {
	info := &RelayInfo{
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]interface{}{
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &ChannelMeta{
			HeadersOverride: map[string]interface{}{
				"X-Static":  "static-value",
				"X-Deleted": "should-not-exist",
			},
		},
	}

	effective := GetEffectiveHeaderOverride(info)
	if effective["x-runtime"] != "runtime-only" {
		t.Fatalf("expected x-runtime from runtime override, got: %v", effective["x-runtime"])
	}
	if _, exists := effective["x-static"]; exists {
		t.Fatalf("expected runtime override to be final and not merge channel headers")
	}
}

// TestRemoveDisabledFieldsSkipWhenChannelPassThroughEnabled 测试渠道级 pass-through 启用时跳过字段过滤。
func TestRemoveDisabledFieldsSkipWhenChannelPassThroughEnabled(t *testing.T) {
	input := `{
		"service_tier":"flex",
		"safety_identifier":"user-123",
		"store":true,
		"stream_options":{"include_obfuscation":false}
	}`
	settings := dto.ChannelOtherSettings{}

	out, err := RemoveDisabledFields([]byte(input), settings, true)
	if err != nil {
		t.Fatalf("RemoveDisabledFields returned error: %v", err)
	}
	assertJSONEqual(t, input, string(out))
}

// TestRemoveDisabledFieldsSkipWhenGlobalPassThroughEnabled 测试全局 pass-through 启用时跳过字段过滤。
func TestRemoveDisabledFieldsSkipWhenGlobalPassThroughEnabled(t *testing.T) {
	original := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = true
	t.Cleanup(func() {
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = original
	})

	input := `{
		"service_tier":"flex",
		"safety_identifier":"user-123",
		"stream_options":{"include_obfuscation":false}
	}`
	settings := dto.ChannelOtherSettings{}

	out, err := RemoveDisabledFields([]byte(input), settings, false)
	if err != nil {
		t.Fatalf("RemoveDisabledFields returned error: %v", err)
	}
	assertJSONEqual(t, input, string(out))
}

// TestRemoveDisabledFieldsDefaultFiltering 测试默认的禁用字段过滤行为。
func TestRemoveDisabledFieldsDefaultFiltering(t *testing.T) {
	input := `{
		"service_tier":"flex",
		"inference_geo":"eu",
		"speed":"fast",
		"cache_control":{"type":"ephemeral"},
		"safety_identifier":"user-123",
		"store":true,
		"stream_options":{"include_obfuscation":false}
	}`
	settings := dto.ChannelOtherSettings{}

	out, err := RemoveDisabledFields([]byte(input), settings, false)
	if err != nil {
		t.Fatalf("RemoveDisabledFields returned error: %v", err)
	}
	assertJSONEqual(t, `{"cache_control":{"type":"ephemeral"},"store":true}`, string(out))
}

// TestRemoveDisabledFieldsAllowInferenceGeo 测试允许 inference_geo 字段通过过滤。
func TestRemoveDisabledFieldsAllowInferenceGeo(t *testing.T) {
	input := `{
		"inference_geo":"eu",
		"store":true
	}`
	settings := dto.ChannelOtherSettings{
		AllowInferenceGeo: true,
	}

	out, err := RemoveDisabledFields([]byte(input), settings, false)
	if err != nil {
		t.Fatalf("RemoveDisabledFields returned error: %v", err)
	}
	assertJSONEqual(t, `{"inference_geo":"eu","store":true}`, string(out))
}

// TestRemoveDisabledFieldsAllowSpeed 测试允许 speed 字段通过过滤。
func TestRemoveDisabledFieldsAllowSpeed(t *testing.T) {
	input := `{
		"speed":"fast",
		"store":true
	}`
	settings := dto.ChannelOtherSettings{
		AllowSpeed: true,
	}

	out, err := RemoveDisabledFields([]byte(input), settings, false)
	if err != nil {
		t.Fatalf("RemoveDisabledFields returned error: %v", err)
	}
	assertJSONEqual(t, `{"speed":"fast","store":true}`, string(out))
}

// TestApplyParamOverrideWithRelayInfoRecordsOperationAuditInDebugMode 测试调试模式下记录所有操作审计日志。
func TestApplyParamOverrideWithRelayInfoRecordsOperationAuditInDebugMode(t *testing.T) {
	originalDebugEnabled := common2.DebugEnabled
	common2.DebugEnabled = true
	t.Cleanup(func() {
		common2.DebugEnabled = originalDebugEnabled
	})

	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ParamOverride: map[string]interface{}{
				"operations": []interface{}{
					map[string]interface{}{
						"mode": "copy",
						"from": "metadata.target_model",
						"to":   "model",
					},
					map[string]interface{}{
						"mode":  "set",
						"path":  "service_tier",
						"value": "flex",
					},
					map[string]interface{}{
						"mode":  "set",
						"path":  "temperature",
						"value": 0.1,
					},
				},
			},
		},
	}

	out, err := ApplyParamOverrideWithRelayInfo([]byte(`{
		"model":"gpt-4.1",
		"temperature":0.7,
		"metadata":{"target_model":"gpt-4.1-mini"}
	}`), info)
	if err != nil {
		t.Fatalf("ApplyParamOverrideWithRelayInfo returned error: %v", err)
	}
	assertJSONEqual(t, `{
		"model":"gpt-4.1-mini",
		"temperature":0.1,
		"service_tier":"flex",
		"metadata":{"target_model":"gpt-4.1-mini"}
	}`, string(out))

	expected := []string{
		"copy metadata.target_model -> model",
		"set service_tier = flex",
		"set temperature = 0.1",
	}
	if !reflect.DeepEqual(info.ParamOverrideAudit, expected) {
		t.Fatalf("unexpected param override audit, got %#v", info.ParamOverrideAudit)
	}
}

// TestApplyParamOverrideWithRelayInfoRecordsOnlyKeyOperationsWhenDebugDisabled 测试非调试模式下仅记录关键操作审计日志。
func TestApplyParamOverrideWithRelayInfoRecordsOnlyKeyOperationsWhenDebugDisabled(t *testing.T) {
	originalDebugEnabled := common2.DebugEnabled
	common2.DebugEnabled = false
	t.Cleanup(func() {
		common2.DebugEnabled = originalDebugEnabled
	})

	info := &RelayInfo{
		ChannelMeta: &ChannelMeta{
			ParamOverride: map[string]interface{}{
				"operations": []interface{}{
					map[string]interface{}{
						"mode": "copy",
						"from": "metadata.target_model",
						"to":   "model",
					},
					map[string]interface{}{
						"mode":  "set",
						"path":  "temperature",
						"value": 0.1,
					},
				},
			},
		},
	}

	_, err := ApplyParamOverrideWithRelayInfo([]byte(`{
		"model":"gpt-4.1",
		"temperature":0.7,
		"metadata":{"target_model":"gpt-4.1-mini"}
	}`), info)
	if err != nil {
		t.Fatalf("ApplyParamOverrideWithRelayInfo returned error: %v", err)
	}

	expected := []string{
		"copy metadata.target_model -> model",
	}
	if !reflect.DeepEqual(info.ParamOverrideAudit, expected) {
		t.Fatalf("unexpected param override audit, got %#v", info.ParamOverrideAudit)
	}
}

// assertJSONEqual 辅助函数：比较两个 JSON 字符串是否语义相等（忽略键顺序和空白差异）。
func assertJSONEqual(t *testing.T, want, got string) {
	t.Helper()

	var wantObj interface{}
	var gotObj interface{}

	if err := json.Unmarshal([]byte(want), &wantObj); err != nil {
		t.Fatalf("failed to unmarshal want JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(got), &gotObj); err != nil {
		t.Fatalf("failed to unmarshal got JSON: %v", err)
	}

	if !reflect.DeepEqual(wantObj, gotObj) {
		t.Fatalf("json not equal\nwant: %s\ngot:  %s", want, got)
	}
}
