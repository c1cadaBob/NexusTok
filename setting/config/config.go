// config.go — 通用配置管理系统
// 职责：提供基于反射的结构体配置注册、加载、保存和导出功能。
// 通过 ConfigManager 统一管理多个配置模块，支持从数据库加载（LoadFromDB）
// 和保存到数据库（SaveToDB），以及配置对象与 map[string]string 之间的
// 双向转换。字段映射基于 struct json tag，支持 string、bool、int、uint、
// float、ptr、map、slice、struct 等类型。

package config

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/c1cada/NexusTok/common"
)

// ConfigManager 统一管理所有配置模块，线程安全。
// 各配置模块通过 Register 注册，键名为模块名称，值为指向配置结构体的指针。
type ConfigManager struct {
	configs map[string]interface{}
	mutex   sync.RWMutex
}

// GlobalConfig 是全局配置管理器单例，供各模块注册和访问配置
var GlobalConfig = NewConfigManager()

// NewConfigManager 创建并返回一个新的 ConfigManager 实例
func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		configs: make(map[string]interface{}),
	}
}

// Register 注册一个配置模块到 ConfigManager。
// 参数：
//   - name: 配置模块名称，作为数据库键名前缀（如 "billing_setting"）
//   - config: 指向配置结构体的指针
func (cm *ConfigManager) Register(name string, config interface{}) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.configs[name] = config
}

// Get 获取指定名称的配置模块。
// 参数：
//   - name: 配置模块名称
//
// 返回值：配置模块实例（指向结构体的指针），未注册时返回 nil
func (cm *ConfigManager) Get(name string) interface{} {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	return cm.configs[name]
}

// LoadFromDB 从数据库选项表加载配置。
// 遍历所有已注册的配置模块，按 "{模块名}." 前缀筛选对应的选项键值对，
// 然后通过反射将值回填到配置结构体中。
// 参数：
//   - options: 数据库中的键值对映射（键格式为 "模块名.字段名"）
//
// 返回值：加载过程中遇到不可恢复的错误时返回 error
func (cm *ConfigManager) LoadFromDB(options map[string]string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for name, config := range cm.configs {
		prefix := name + "."
		configMap := make(map[string]string)

		// 收集属于此配置的所有选项
		for key, value := range options {
			if strings.HasPrefix(key, prefix) {
				configKey := strings.TrimPrefix(key, prefix)
				configMap[configKey] = value
			}
		}

		// 如果找到配置项，则更新配置
		if len(configMap) > 0 {
			if err := updateConfigFromMap(config, configMap); err != nil {
				common.SysError("failed to update config " + name + ": " + err.Error())
				continue
			}
		}
	}

	return nil
}

// SaveToDB 将所有已注册的配置保存到数据库。
// 通过反射将每个配置结构体序列化为扁平的键值对，然后调用 updateFunc 持久化。
// 参数：
//   - updateFunc: 保存单个键值对的回调函数（签名：func(key, value string) error）
//
// 返回值：保存失败时返回 error
func (cm *ConfigManager) SaveToDB(updateFunc func(key, value string) error) error {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	for name, config := range cm.configs {
		configMap, err := configToMap(config)
		if err != nil {
			return err
		}

		for key, value := range configMap {
			dbKey := name + "." + key
			if err := updateFunc(dbKey, value); err != nil {
				return err
			}
		}
	}

	return nil
}

// configToMap 将配置结构体转换为 map[string]string。
// 使用反射遍历结构体字段，按 json tag 确定键名，
// 将各类型字段值序列化为字符串。支持 string、bool、int、uint、float、
// ptr、map、slice、struct 等类型。
// 参数：
//   - config: 指向配置结构体的指针或结构体值
//
// 返回值：字段名到字符串值的映射，以及序列化错误
func configToMap(config interface{}) (map[string]string, error) {
	result := make(map[string]string)

	val := reflect.ValueOf(config)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil, nil
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// 跳过未导出字段
		if !fieldType.IsExported() {
			continue
		}

		// 获取json标签作为键名
		key := fieldType.Tag.Get("json")
		if key == "" || key == "-" {
			key = fieldType.Name
		}

		// 处理不同类型的字段
		var strValue string
		switch field.Kind() {
		case reflect.String:
			strValue = field.String()
		case reflect.Bool:
			strValue = strconv.FormatBool(field.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			strValue = strconv.FormatInt(field.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			strValue = strconv.FormatUint(field.Uint(), 10)
		case reflect.Float32, reflect.Float64:
			strValue = strconv.FormatFloat(field.Float(), 'f', -1, 64)
		case reflect.Ptr:
			// 处理指针类型：如果非 nil，序列化指向的值
			if !field.IsNil() {
				bytes, err := json.Marshal(field.Interface())
				if err != nil {
					return nil, err
				}
				strValue = string(bytes)
			} else {
				// nil 指针序列化为 "null"
				strValue = "null"
			}
		case reflect.Map, reflect.Slice, reflect.Struct:
			// 复杂类型使用JSON序列化
			bytes, err := json.Marshal(field.Interface())
			if err != nil {
				return nil, err
			}
			strValue = string(bytes)
		default:
			// 跳过不支持的类型
			continue
		}

		result[key] = strValue
	}

	return result, nil
}

// updateConfigFromMap 从 map[string]string 更新配置结构体的字段值。
// 使用反射遍历结构体字段，查找 configMap 中对应的键并设置值。
// 对 Map 类型字段采用整体替换语义（非合并），确保被删除的键不会残留。
// 参数：
//   - config: 指向配置结构体的指针
//   - configMap: 字段名到字符串值的映射
//
// 返回值：更新过程中遇到不可恢复的错误时返回 error
func updateConfigFromMap(config interface{}, configMap map[string]string) error {
	val := reflect.ValueOf(config)
	if val.Kind() != reflect.Ptr {
		return nil
	}
	val = val.Elem()

	if val.Kind() != reflect.Struct {
		return nil
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// 跳过未导出字段
		if !fieldType.IsExported() {
			continue
		}

		// 获取json标签作为键名
		key := fieldType.Tag.Get("json")
		if key == "" || key == "-" {
			key = fieldType.Name
		}

		// 检查map中是否有对应的值
		strValue, ok := configMap[key]
		if !ok {
			continue
		}

		// 根据字段类型设置值
		if !field.CanSet() {
			continue
		}

		switch field.Kind() {
		case reflect.String:
			field.SetString(strValue)
		case reflect.Bool:
			boolValue, err := strconv.ParseBool(strValue)
			if err != nil {
				continue
			}
			field.SetBool(boolValue)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			intValue, err := strconv.ParseInt(strValue, 10, 64)
			if err != nil {
				// 兼容 float 格式的字符串（如 "2.000000"）
				floatValue, fErr := strconv.ParseFloat(strValue, 64)
				if fErr != nil {
					continue
				}
				intValue = int64(floatValue)
			}
			field.SetInt(intValue)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			uintValue, err := strconv.ParseUint(strValue, 10, 64)
			if err != nil {
				// 兼容 float 格式的字符串
				floatValue, fErr := strconv.ParseFloat(strValue, 64)
				if fErr != nil || floatValue < 0 {
					continue
				}
				uintValue = uint64(floatValue)
			}
			field.SetUint(uintValue)
		case reflect.Float32, reflect.Float64:
			floatValue, err := strconv.ParseFloat(strValue, 64)
			if err != nil {
				continue
			}
			field.SetFloat(floatValue)
		case reflect.Ptr:
			// 处理指针类型
			if strValue == "null" {
				field.Set(reflect.Zero(field.Type()))
			} else {
				// 如果指针是 nil，需要先初始化
				if field.IsNil() {
					field.Set(reflect.New(field.Type().Elem()))
				}
				// 反序列化到指针指向的值
				err := json.Unmarshal([]byte(strValue), field.Interface())
				if err != nil {
					continue
				}
			}
		case reflect.Map:
			// json.Unmarshal merges into existing maps (keeps old keys that are
			// absent from the new JSON). Allocate a fresh map so removed keys
			// are properly cleared.
			fresh := reflect.New(field.Type())
			if err := json.Unmarshal([]byte(strValue), fresh.Interface()); err != nil {
				continue
			}
			field.Set(fresh.Elem())
		case reflect.Slice, reflect.Struct:
			err := json.Unmarshal([]byte(strValue), field.Addr().Interface())
			if err != nil {
				continue
			}
		}
	}

	return nil
}

// ConfigToMap 将配置对象转换为 map[string]string（导出版本）。
// 参数：
//   - config: 指向配置结构体的指针
//
// 返回值：字段名到字符串值的映射，以及序列化错误
func ConfigToMap(config interface{}) (map[string]string, error) {
	return configToMap(config)
}

// UpdateConfigFromMap 从 map[string]string 更新配置对象（导出版本）。
// 参数：
//   - config: 指向配置结构体的指针
//   - configMap: 字段名到字符串值的映射
//
// 返回值：更新失败时返回 error
func UpdateConfigFromMap(config interface{}, configMap map[string]string) error {
	return updateConfigFromMap(config, configMap)
}

// ExportAllConfigs 导出所有已注册的配置为扁平的 key-value 结构。
// 键名格式为 "{模块名}.{字段名}"，值为字符串形式。
// 返回值：所有配置项的扁平映射
func (cm *ConfigManager) ExportAllConfigs() map[string]string {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	result := make(map[string]string)

	for name, cfg := range cm.configs {
		configMap, err := ConfigToMap(cfg)
		if err != nil {
			continue
		}

		// 使用 "模块名.配置项" 的格式添加到结果中
		for key, value := range configMap {
			result[name+"."+key] = value
		}
	}

	return result
}
