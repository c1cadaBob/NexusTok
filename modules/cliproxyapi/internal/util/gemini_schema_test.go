// util - gemini_schema_test.go
// 该文件包含 CleanJSONSchemaForAntigravity 和 CleanJSONSchemaForGemini 函数的单元测试，
// 验证 JSON Schema 清洗逻辑的正确性，包括 const 转 enum、类型扁平化、anyOf/oneOf 展开、
// allOf 合并、$ref 处理、枚举提示、格式字段移除、数值枚举转字符串、空 schema 占位符、
// 扩展字段移除等功能。
package util

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

// TestCleanJSONSchemaForAntigravity_ConstToEnum 测试将 JSON Schema 中的 const 关键字
// 转换为 enum 数组的功能，确保 Antigravity API 兼容性。
func TestCleanJSONSchemaForAntigravity_ConstToEnum(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"kind": {
				"type": "string",
				"const": "InsightVizNode"
			}
		}
	}`

	expected := `{
		"type": "object",
		"properties": {
			"kind": {
				"type": "string",
				"enum": ["InsightVizNode"]
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)
	compareJSON(t, expected, result)
}

// TestCleanJSONSchemaForAntigravity_TypeFlattening_Nullable 测试类型数组扁平化逻辑，
// 验证 ["string", "null"] 类型会被简化为 "string" 并添加 "(nullable)" 描述提示，
// 同时从 required 数组中移除可空字段。
func TestCleanJSONSchemaForAntigravity_TypeFlattening_Nullable(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"name": {
				"type": ["string", "null"]
			},
			"other": {
				"type": "string"
			}
		},
		"required": ["name", "other"]
	}`

	expected := `{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "(nullable)"
			},
			"other": {
				"type": "string"
			}
		},
		"required": ["other"]
	}`

	result := CleanJSONSchemaForAntigravity(input)
	compareJSON(t, expected, result)
}

// TestCleanJSONSchemaForAntigravity_ConstraintsToDescription 测试约束关键字移除逻辑，
// 验证 minItems、minLength 等不被 Antigravity 支持的约束会被移除并作为提示信息
// 迁移到 description 字段中。
func TestCleanJSONSchemaForAntigravity_ConstraintsToDescription(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"tags": {
				"type": "array",
				"description": "List of tags",
				"minItems": 1
			},
			"name": {
				"type": "string",
				"description": "User name",
				"minLength": 3
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	// minItems should be REMOVED and moved to description
	if strings.Contains(result, `"minItems"`) {
		t.Errorf("minItems keyword should be removed")
	}
	if !strings.Contains(result, "minItems: 1") {
		t.Errorf("minItems hint missing in description")
	}

	// minLength should be moved to description
	if !strings.Contains(result, "minLength: 3") {
		t.Errorf("minLength hint missing in description")
	}
	if strings.Contains(result, `"minLength":`) || strings.Contains(result, `"minLength" :`) {
		t.Errorf("minLength keyword should be removed")
	}
}

// TestCleanJSONSchemaForAntigravity_AnyOfFlattening_SmartSelection 测试 anyOf 展开的智能选择逻辑，
// 验证在 null 和 object 类型之间会选择 object 作为主类型，
// 并添加 "Accepts: null | object" 的描述提示。
func TestCleanJSONSchemaForAntigravity_AnyOfFlattening_SmartSelection(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"query": {
				"anyOf": [
					{ "type": "null" },
					{
						"type": "object",
						"properties": {
							"kind": { "type": "string" }
						}
					}
				]
			}
		}
	}`

	expected := `{
		"type": "object",
		"properties": {
			"query": {
				"type": "object",
				"description": "Accepts: null | object",
				"properties": {
					"_": { "type": "boolean" },
					"kind": { "type": "string" }
				},
				"required": ["_"]
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)
	compareJSON(t, expected, result)
}

// TestCleanJSONSchemaForAntigravity_OneOfFlattening 测试 oneOf 展开逻辑，
// 验证在 string 和 integer 类型之间选择 string 作为主类型，
// 并添加 "Accepts: string | integer" 的描述提示。
func TestCleanJSONSchemaForAntigravity_OneOfFlattening(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"config": {
				"oneOf": [
					{ "type": "string" },
					{ "type": "integer" }
				]
			}
		}
	}`

	expected := `{
		"type": "object",
		"properties": {
			"config": {
				"type": "string",
				"description": "Accepts: string | integer"
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)
	compareJSON(t, expected, result)
}

// TestCleanJSONSchemaForAntigravity_AllOfMerging 测试 allOf 合并逻辑，
// 验证多个 allOf 分支中的 properties 和 required 会被正确合并到父级对象中。
func TestCleanJSONSchemaForAntigravity_AllOfMerging(t *testing.T) {
	input := `{
		"type": "object",
		"allOf": [
			{
				"properties": {
					"a": { "type": "string" }
				},
				"required": ["a"]
			},
			{
				"properties": {
					"b": { "type": "integer" }
				},
				"required": ["b"]
			}
		]
	}`

	expected := `{
		"type": "object",
		"properties": {
			"a": { "type": "string" },
			"b": { "type": "integer" }
		},
		"required": ["a", "b"]
	}`

	result := CleanJSONSchemaForAntigravity(input)
	compareJSON(t, expected, result)
}

// TestCleanJSONSchemaForAntigravity_RefHandling 测试 $ref 引用处理逻辑，
// 验证 $ref 会被转换为带有 "See: TypeName" 描述提示的占位对象。
func TestCleanJSONSchemaForAntigravity_RefHandling(t *testing.T) {
	input := `{
		"definitions": {
			"User": {
				"type": "object",
				"properties": {
					"name": { "type": "string" }
				}
			}
		},
		"type": "object",
		"properties": {
			"customer": { "$ref": "#/definitions/User" }
		}
	}`

	// After $ref is converted to placeholder object, empty schema placeholder is also added
	expected := `{
		"type": "object",
		"properties": {
			"customer": {
				"type": "object",
				"description": "See: User",
				"properties": {
					"reason": {
						"type": "string",
						"description": "Brief explanation of why you are calling this tool"
					}
				},
				"required": ["reason"]
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)
	compareJSON(t, expected, result)
}

// TestCleanJSONSchemaForAntigravity_RefHandling_DescriptionEscaping 测试 $ref 处理时
// description 中特殊字符（双引号、换行符）的转义是否正确保留。
func TestCleanJSONSchemaForAntigravity_RefHandling_DescriptionEscaping(t *testing.T) {
	input := `{
		"definitions": {
			"User": {
				"type": "object",
				"properties": {
					"name": { "type": "string" }
				}
			}
		},
		"type": "object",
		"properties": {
			"customer": {
				"description": "He said \"hi\"\\nsecond line",
				"$ref": "#/definitions/User"
			}
		}
	}`

	// After $ref is converted, empty schema placeholder is also added
	expected := `{
		"type": "object",
		"properties": {
			"customer": {
				"type": "object",
				"description": "He said \"hi\"\\nsecond line (See: User)",
				"properties": {
					"reason": {
						"type": "string",
						"description": "Brief explanation of why you are calling this tool"
					}
				},
				"required": ["reason"]
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)
	compareJSON(t, expected, result)
}

// TestCleanJSONSchemaForAntigravity_CyclicRefDefaults 测试循环 $ref 引用的处理逻辑，
// 验证当 schema 存在自引用（如 Node 引用自身）时不会导致无限递归，
// 并能正确生成带有类型和描述提示的结果。
func TestCleanJSONSchemaForAntigravity_CyclicRefDefaults(t *testing.T) {
	input := `{
		"definitions": {
			"Node": {
				"type": "object",
				"properties": {
					"child": { "$ref": "#/definitions/Node" }
				}
			}
		},
		"$ref": "#/definitions/Node"
	}`

	result := CleanJSONSchemaForAntigravity(input)

	var resMap map[string]interface{}
	json.Unmarshal([]byte(result), &resMap)

	if resMap["type"] != "object" {
		t.Errorf("Expected type: object, got: %v", resMap["type"])
	}

	desc, ok := resMap["description"].(string)
	if !ok || !strings.Contains(desc, "Node") {
		t.Errorf("Expected description hint containing 'Node', got: %v", resMap["description"])
	}
}

// TestCleanJSONSchemaForAntigravity_RequiredCleanup 测试 required 数组清理逻辑，
// 验证当 required 中包含不存在于 properties 中的字段名时，这些无效字段会被移除。
func TestCleanJSONSchemaForAntigravity_RequiredCleanup(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"a": {"type": "string"},
			"b": {"type": "string"}
		},
		"required": ["a", "b", "c"]
	}`

	expected := `{
		"type": "object",
		"properties": {
			"a": {"type": "string"},
			"b": {"type": "string"}
		},
		"required": ["a", "b"]
	}`

	result := CleanJSONSchemaForAntigravity(input)
	compareJSON(t, expected, result)
}

// TestCleanJSONSchemaForAntigravity_AllOfMerging_DotKeys 测试包含点号的属性名在
// allOf 合并时是否能正确处理，确保 sjson 路径转义不会产生伪键。
func TestCleanJSONSchemaForAntigravity_AllOfMerging_DotKeys(t *testing.T) {
	input := `{
		"type": "object",
		"allOf": [
			{
				"properties": {
					"my.param": { "type": "string" }
				},
				"required": ["my.param"]
			},
			{
				"properties": {
					"b": { "type": "integer" }
				},
				"required": ["b"]
			}
		]
	}`

	expected := `{
		"type": "object",
		"properties": {
			"my.param": { "type": "string" },
			"b": { "type": "integer" }
		},
		"required": ["my.param", "b"]
	}`

	result := CleanJSONSchemaForAntigravity(input)
	compareJSON(t, expected, result)
}

// TestCleanJSONSchemaForAntigravity_PropertyNameCollision 测试属性名与 schema 关键字冲突的情况，
// 验证名为 "pattern" 的属性不会被误认为是约束关键字而被移除。
func TestCleanJSONSchemaForAntigravity_PropertyNameCollision(t *testing.T) {
	// A tool has an argument named "pattern" - should NOT be treated as a constraint
	input := `{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "The regex pattern"
			}
		},
		"required": ["pattern"]
	}`

	expected := `{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "The regex pattern"
			}
		},
		"required": ["pattern"]
	}`

	result := CleanJSONSchemaForAntigravity(input)
	compareJSON(t, expected, result)

	var resMap map[string]interface{}
	json.Unmarshal([]byte(result), &resMap)
	props, _ := resMap["properties"].(map[string]interface{})
	if _, ok := props["description"]; ok {
		t.Errorf("Invalid 'description' property injected into properties map")
	}
}

// TestCleanJSONSchemaForAntigravity_DotKeys 测试包含点号的属性名在 $ref 处理时
// 是否能正确转义，确保 sjson 路径操作不会因点号而产生错误分割。
func TestCleanJSONSchemaForAntigravity_DotKeys(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"my.param": {
				"type": "string",
				"$ref": "#/definitions/MyType"
			}
		},
		"definitions": {
			"MyType": { "type": "string" }
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	var resMap map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resMap); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	props, ok := resMap["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing")
	}

	if val, ok := props["my.param"]; !ok {
		t.Fatalf("Key 'my.param' is missing. Result: %s", result)
	} else {
		valMap, _ := val.(map[string]interface{})
		if _, hasRef := valMap["$ref"]; hasRef {
			t.Errorf("Key 'my.param' still contains $ref")
		}
		if _, ok := props["my"]; ok {
			t.Errorf("Artifact key 'my' created by sjson splitting")
		}
	}
}

// TestCleanJSONSchemaForAntigravity_AnyOfAlternativeHints 测试 anyOf 展开后是否正确
// 生成包含所有备选类型的 "Accepts:" 描述提示。
func TestCleanJSONSchemaForAntigravity_AnyOfAlternativeHints(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"value": {
				"anyOf": [
					{ "type": "string" },
					{ "type": "integer" },
					{ "type": "null" }
				]
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	if !strings.Contains(result, "Accepts:") {
		t.Errorf("Expected alternative types hint, got: %s", result)
	}
	if !strings.Contains(result, "string") || !strings.Contains(result, "integer") {
		t.Errorf("Expected all alternative types in hint, got: %s", result)
	}
}

// TestCleanJSONSchemaForAntigravity_NullableHint 测试可空类型提示是否正确追加到
// 已有 description 中，并保留原始描述文本。
func TestCleanJSONSchemaForAntigravity_NullableHint(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"name": {
				"type": ["string", "null"],
				"description": "User name"
			}
		},
		"required": ["name"]
	}`

	result := CleanJSONSchemaForAntigravity(input)

	if !strings.Contains(result, "(nullable)") {
		t.Errorf("Expected nullable hint, got: %s", result)
	}
	if !strings.Contains(result, "User name") {
		t.Errorf("Expected original description to be preserved, got: %s", result)
	}
}

// TestCleanJSONSchemaForAntigravity_TypeFlattening_Nullable_DotKey 测试包含点号的属性名在
// 类型扁平化和可空处理时是否能正确工作。
func TestCleanJSONSchemaForAntigravity_TypeFlattening_Nullable_DotKey(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"my.param": {
				"type": ["string", "null"]
			},
			"other": {
				"type": "string"
			}
		},
		"required": ["my.param", "other"]
	}`

	expected := `{
		"type": "object",
		"properties": {
			"my.param": {
				"type": "string",
				"description": "(nullable)"
			},
			"other": {
				"type": "string"
			}
		},
		"required": ["other"]
	}`

	result := CleanJSONSchemaForAntigravity(input)
	compareJSON(t, expected, result)
}

// TestCleanJSONSchemaForAntigravity_EnumHint 测试枚举值提示生成功能，
// 验证当 enum 值数量在 2-10 之间时会生成 "Allowed: ..." 格式的描述提示。
func TestCleanJSONSchemaForAntigravity_EnumHint(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"status": {
				"type": "string",
				"enum": ["active", "inactive", "pending"],
				"description": "Current status"
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	if !strings.Contains(result, "Allowed:") {
		t.Errorf("Expected enum values hint, got: %s", result)
	}
	if !strings.Contains(result, "active") || !strings.Contains(result, "inactive") {
		t.Errorf("Expected enum values in hint, got: %s", result)
	}
}

// TestCleanJSONSchemaForAntigravity_AdditionalPropertiesHint 测试 additionalProperties 为 false 时
// 是否正确生成 "No extra properties allowed" 描述提示。
func TestCleanJSONSchemaForAntigravity_AdditionalPropertiesHint(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"name": { "type": "string" }
		},
		"additionalProperties": false
	}`

	result := CleanJSONSchemaForAntigravity(input)

	if !strings.Contains(result, "No extra properties allowed") {
		t.Errorf("Expected additionalProperties hint, got: %s", result)
	}
}

// TestCleanJSONSchemaForAntigravity_AnyOfFlattening_PreservesDescription 测试 anyOf 展开时
// 父级和子级的 description 是否被正确合并保留。
func TestCleanJSONSchemaForAntigravity_AnyOfFlattening_PreservesDescription(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"config": {
				"description": "Parent desc",
				"anyOf": [
					{ "type": "string", "description": "Child desc" },
					{ "type": "integer" }
				]
			}
		}
	}`

	expected := `{
		"type": "object",
		"properties": {
			"config": {
				"type": "string",
				"description": "Parent desc (Child desc) (Accepts: string | integer)"
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)
	compareJSON(t, expected, result)
}

// TestCleanJSONSchemaForAntigravity_SingleEnumNoHint 测试单值枚举不生成 "Allowed:" 提示的逻辑，
// 验证当 enum 只有一个值时不添加提示（因为选择是显而易见的）。
func TestCleanJSONSchemaForAntigravity_SingleEnumNoHint(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"kind": {
				"type": "string",
				"enum": ["fixed"]
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	if strings.Contains(result, "Allowed:") {
		t.Errorf("Single value enum should not add Allowed hint, got: %s", result)
	}
}

// TestCleanJSONSchemaForAntigravity_MultipleNonNullTypes 测试多个非 null 类型的扁平化逻辑，
// 验证 type 数组包含多种类型时生成 "Accepts: ..." 描述提示。
func TestCleanJSONSchemaForAntigravity_MultipleNonNullTypes(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"value": {
				"type": ["string", "integer", "boolean"]
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	if !strings.Contains(result, "Accepts:") {
		t.Errorf("Expected multiple types hint, got: %s", result)
	}
	if !strings.Contains(result, "string") || !strings.Contains(result, "integer") || !strings.Contains(result, "boolean") {
		t.Errorf("Expected all types in hint, got: %s", result)
	}
}

// compareJSON 是测试辅助函数，将两个 JSON 字符串反序列化为 map 后进行深度比较，
// 若不相等则输出格式化的差异信息。
func compareJSON(t *testing.T, expectedJSON, actualJSON string) {
	var expMap, actMap map[string]interface{}
	errExp := json.Unmarshal([]byte(expectedJSON), &expMap)
	errAct := json.Unmarshal([]byte(actualJSON), &actMap)

	if errExp != nil || errAct != nil {
		t.Fatalf("JSON Unmarshal error. Exp: %v, Act: %v", errExp, errAct)
	}

	if !reflect.DeepEqual(expMap, actMap) {
		expBytes, _ := json.MarshalIndent(expMap, "", "  ")
		actBytes, _ := json.MarshalIndent(actMap, "", "  ")
		t.Errorf("JSON mismatch:\nExpected:\n%s\n\nActual:\n%s", string(expBytes), string(actBytes))
	}
}

// ============================================================================
// Empty Schema Placeholder Tests
// ============================================================================

// TestCleanJSONSchemaForAntigravity_EmptySchemaPlaceholder 测试空对象 schema 的占位符添加逻辑，
// 验证没有 properties 的 object 类型会自动添加 "reason" 占位属性。
func TestCleanJSONSchemaForAntigravity_EmptySchemaPlaceholder(t *testing.T) {
	// Empty object schema with no properties should get a placeholder
	input := `{
		"type": "object"
	}`

	result := CleanJSONSchemaForAntigravity(input)

	// Should have placeholder property added
	if !strings.Contains(result, `"reason"`) {
		t.Errorf("Empty schema should have 'reason' placeholder property, got: %s", result)
	}
	if !strings.Contains(result, `"required"`) {
		t.Errorf("Empty schema should have 'required' with 'reason', got: %s", result)
	}
}

// TestCleanJSONSchemaForAntigravity_EmptyPropertiesPlaceholder 测试空 properties 对象的占位符添加逻辑，
// 验证 properties 为空对象时也会自动添加 "reason" 占位属性。
func TestCleanJSONSchemaForAntigravity_EmptyPropertiesPlaceholder(t *testing.T) {
	// Object with empty properties object
	input := `{
		"type": "object",
		"properties": {}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	// Should have placeholder property added
	if !strings.Contains(result, `"reason"`) {
		t.Errorf("Empty properties should have 'reason' placeholder, got: %s", result)
	}
}

// TestCleanJSONSchemaForAntigravity_NonEmptySchemaUnchanged 测试非空 schema 不会被添加占位符，
// 验证已有 properties 的 object 类型保持不变。
func TestCleanJSONSchemaForAntigravity_NonEmptySchemaUnchanged(t *testing.T) {
	// Schema with properties should NOT get placeholder
	input := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"}
		},
		"required": ["name"]
	}`

	result := CleanJSONSchemaForAntigravity(input)

	// Should NOT have placeholder property
	if strings.Contains(result, `"reason"`) {
		t.Errorf("Non-empty schema should NOT have 'reason' placeholder, got: %s", result)
	}
	// Original properties should be preserved
	if !strings.Contains(result, `"name"`) {
		t.Errorf("Original property 'name' should be preserved, got: %s", result)
	}
}

// TestCleanJSONSchemaForAntigravity_NestedEmptySchema 测试嵌套空对象 schema 的占位符添加逻辑，
// 验证数组 items 中的空 object 也会被正确处理。
func TestCleanJSONSchemaForAntigravity_NestedEmptySchema(t *testing.T) {
	// Nested empty object in items should also get placeholder
	input := `{
		"type": "object",
		"properties": {
			"items": {
				"type": "array",
				"items": {
					"type": "object"
				}
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	// Nested empty object should also get placeholder
	// Check that the nested object has a reason property
	parsed := gjson.Parse(result)
	nestedProps := parsed.Get("properties.items.items.properties")
	if !nestedProps.Exists() || !nestedProps.Get("reason").Exists() {
		t.Errorf("Nested empty object should have 'reason' placeholder, got: %s", result)
	}
}

// TestCleanJSONSchemaForAntigravity_EmptySchemaWithDescription 测试带有 description 的空 schema，
// 验证 description 被保留的同时仍会添加占位属性。
func TestCleanJSONSchemaForAntigravity_EmptySchemaWithDescription(t *testing.T) {
	// Empty schema with description should preserve description and add placeholder
	input := `{
		"type": "object",
		"description": "An empty object"
	}`

	result := CleanJSONSchemaForAntigravity(input)

	// Should have both description and placeholder
	if !strings.Contains(result, `"An empty object"`) {
		t.Errorf("Description should be preserved, got: %s", result)
	}
	if !strings.Contains(result, `"reason"`) {
		t.Errorf("Empty schema should have 'reason' placeholder, got: %s", result)
	}
}

// ============================================================================
// Format field handling (ad-hoc patch removal)
// ============================================================================

// TestCleanJSONSchemaForAntigravity_FormatFieldRemoval 测试 format 字段移除逻辑，
// 验证 "format": "uri" 等格式字段被移除并转换为 "format: uri" 的描述提示。
func TestCleanJSONSchemaForAntigravity_FormatFieldRemoval(t *testing.T) {
	// format:"uri" should be removed and added as hint
	input := `{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"format": "uri",
				"description": "A URL"
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	// format should be removed
	if strings.Contains(result, `"format"`) {
		t.Errorf("format field should be removed, got: %s", result)
	}
	// hint should be added to description
	if !strings.Contains(result, "format: uri") {
		t.Errorf("format hint should be added to description, got: %s", result)
	}
	// original description should be preserved
	if !strings.Contains(result, "A URL") {
		t.Errorf("Original description should be preserved, got: %s", result)
	}
}

// TestCleanJSONSchemaForAntigravity_FormatFieldNoDescription 测试无 description 时 format 字段的处理，
// 验证即使没有原始 description 也会正确生成 format 提示。
func TestCleanJSONSchemaForAntigravity_FormatFieldNoDescription(t *testing.T) {
	// format without description should create description with hint
	input := `{
		"type": "object",
		"properties": {
			"email": {
				"type": "string",
				"format": "email"
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	// format should be removed
	if strings.Contains(result, `"format"`) {
		t.Errorf("format field should be removed, got: %s", result)
	}
	// hint should be added
	if !strings.Contains(result, "format: email") {
		t.Errorf("format hint should be added, got: %s", result)
	}
}

// TestCleanJSONSchemaForAntigravity_MultipleFormats 测试多个 format 字段的批量处理，
// 验证 uri、email、date-time 等多种格式字段都被正确移除并生成对应提示。
func TestCleanJSONSchemaForAntigravity_MultipleFormats(t *testing.T) {
	// Multiple format fields should all be handled
	input := `{
		"type": "object",
		"properties": {
			"url": {"type": "string", "format": "uri"},
			"email": {"type": "string", "format": "email"},
			"date": {"type": "string", "format": "date-time"}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	// All format fields should be removed
	if strings.Contains(result, `"format"`) {
		t.Errorf("All format fields should be removed, got: %s", result)
	}
	// All hints should be added
	if !strings.Contains(result, "format: uri") {
		t.Errorf("uri format hint should be added, got: %s", result)
	}
	if !strings.Contains(result, "format: email") {
		t.Errorf("email format hint should be added, got: %s", result)
	}
	if !strings.Contains(result, "format: date-time") {
		t.Errorf("date-time format hint should be added, got: %s", result)
	}
}

// TestCleanJSONSchemaForAntigravity_NumericEnumToString 测试数值枚举值转字符串的逻辑，
// 验证 Gemini API 要求的 enum 值类型限制（仅允许字符串类型）。
func TestCleanJSONSchemaForAntigravity_NumericEnumToString(t *testing.T) {
	// Gemini API requires enum values to be strings, not numbers
	input := `{
		"type": "object",
		"properties": {
			"priority": {"type": "integer", "enum": [0, 1, 2]},
			"level": {"type": "number", "enum": [1.5, 2.5, 3.5]},
			"status": {"type": "string", "enum": ["active", "inactive"]}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	// Numeric enum values should be converted to strings
	if strings.Contains(result, `"enum":[0,1,2]`) {
		t.Errorf("Integer enum values should be converted to strings, got: %s", result)
	}
	if strings.Contains(result, `"enum":[1.5,2.5,3.5]`) {
		t.Errorf("Float enum values should be converted to strings, got: %s", result)
	}
	// Should contain string versions
	if !strings.Contains(result, `"0"`) || !strings.Contains(result, `"1"`) || !strings.Contains(result, `"2"`) {
		t.Errorf("Integer enum values should be converted to string format, got: %s", result)
	}
	// String enum values should remain unchanged
	if !strings.Contains(result, `"active"`) || !strings.Contains(result, `"inactive"`) {
		t.Errorf("String enum values should remain unchanged, got: %s", result)
	}
}

// TestCleanJSONSchemaForAntigravity_BooleanEnumToString 测试布尔枚举值转字符串的逻辑，
// 验证 true/false 枚举值被转换为 "true"/"false" 字符串形式。
func TestCleanJSONSchemaForAntigravity_BooleanEnumToString(t *testing.T) {
	// Boolean enum values should also be converted to strings
	input := `{
		"type": "object",
		"properties": {
			"enabled": {"type": "boolean", "enum": [true, false]}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	// Boolean enum values should be converted to strings
	if strings.Contains(result, `"enum":[true,false]`) {
		t.Errorf("Boolean enum values should be converted to strings, got: %s", result)
	}
	// Should contain string versions "true" and "false"
	if !strings.Contains(result, `"true"`) || !strings.Contains(result, `"false"`) {
		t.Errorf("Boolean enum values should be converted to string format, got: %s", result)
	}
}

// TestCleanJSONSchemaForGemini_RemovesGeminiUnsupportedMetadataFields 测试 Gemini 专用的 schema 清洗逻辑，
// 验证 $schema、$id、prefill、enumTitles、patternProperties 等 Gemini 不支持的字段被正确移除，
// 同时保留作为属性名的 "$id" 字段。
func TestCleanJSONSchemaForGemini_RemovesGeminiUnsupportedMetadataFields(t *testing.T) {
	input := `{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"$id": "root-schema",
		"type": "object",
		"properties": {
			"payload": {
				"type": "object",
				"prefill": "hello",
				"properties": {
					"mode": {
						"type": "string",
						"enum": ["a", "b"],
						"enumTitles": ["A", "B"]
					}
				},
				"patternProperties": {
					"^x-": {"type": "string"}
				}
			},
			"$id": {
				"type": "string",
				"description": "property name should not be removed"
			}
		}
	}`

	expected := `{
		"type": "object",
		"properties": {
			"payload": {
				"type": "object",
				"properties": {
					"mode": {
						"type": "string",
						"enum": ["a", "b"],
						"description": "Allowed: a, b"
					}
				}
			},
			"$id": {
				"type": "string",
				"description": "property name should not be removed"
			}
		}
	}`

	result := CleanJSONSchemaForGemini(input)
	compareJSON(t, expected, result)
}

// TestRemoveExtensionFields 测试 x-* 扩展字段的移除逻辑，涵盖根级和嵌套级扩展字段移除、
// 属性名为 "x-" 前缀时不被误删、$schema 等元字段保留、以及路径转义处理等场景。
func TestRemoveExtensionFields(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "removes x- fields at root",
			input: `{
				"type": "object",
				"x-custom-meta": "value",
				"properties": {
					"foo": { "type": "string" }
				}
			}`,
			expected: `{
				"type": "object",
				"properties": {
					"foo": { "type": "string" }
				}
			}`,
		},
		{
			name: "removes x- fields in nested properties",
			input: `{
				"type": "object",
				"properties": {
					"foo": {
						"type": "string",
						"x-internal-id": 123
					}
				}
			}`,
			expected: `{
				"type": "object",
				"properties": {
					"foo": {
						"type": "string"
					}
				}
			}`,
		},
		{
			name: "does NOT remove properties named x-",
			input: `{
				"type": "object",
				"properties": {
					"x-data": { "type": "string" },
					"normal": { "type": "number", "x-meta": "remove" }
				},
				"required": ["x-data"]
			}`,
			expected: `{
				"type": "object",
				"properties": {
					"x-data": { "type": "string" },
					"normal": { "type": "number" }
				},
				"required": ["x-data"]
			}`,
		},
		{
			name: "does NOT remove $schema and other meta fields (as requested)",
			input: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"$id": "test",
				"type": "object",
				"properties": {
					"foo": { "type": "string" }
				}
			}`,
			expected: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"$id": "test",
				"type": "object",
				"properties": {
					"foo": { "type": "string" }
				}
			}`,
		},
		{
			name: "handles properties named $schema",
			input: `{
				"type": "object",
				"properties": {
					"$schema": { "type": "string" }
				}
			}`,
			expected: `{
				"type": "object",
				"properties": {
					"$schema": { "type": "string" }
				}
			}`,
		},
		{
			name: "handles escaping in paths",
			input: `{
				"type": "object",
				"properties": {
					"foo.bar": {
						"type": "string",
						"x-meta": "remove"
					}
				},
				"x-root.meta": "remove"
			}`,
			expected: `{
				"type": "object",
				"properties": {
					"foo.bar": {
						"type": "string"
					}
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := removeExtensionFields(tt.input)
			compareJSON(t, tt.expected, actual)
		})
	}
}

// uniqueItems should be stripped and moved to description hint (#2123).
// TestCleanJSONSchemaForAntigravity_UniqueItemsStripped 测试 uniqueItems 约束的移除逻辑，
// 验证 uniqueItems: true 被从 schema 中移除并作为 "uniqueItems: true" 提示追加到 description。
func TestCleanJSONSchemaForAntigravity_UniqueItemsStripped(t *testing.T) {
	input := `{
		"type": "object",
		"properties": {
			"ids": {
				"type": "array",
				"description": "Unique identifiers",
				"items": {"type": "string"},
				"uniqueItems": true
			}
		}
	}`

	result := CleanJSONSchemaForAntigravity(input)

	if strings.Contains(result, `"uniqueItems"`) {
		t.Errorf("uniqueItems should be removed from schema")
	}
	if !strings.Contains(result, "uniqueItems: true") {
		t.Errorf("uniqueItems hint missing in description")
	}
}
