// regex.go 提供带缓存的正则表达式匹配工具函数。
// 使用 sync.Map 缓存已编译的正则表达式，避免重复编译的性能开销。
package openaicompat

import (
	"regexp"
	"sync"
)

// compiledRegexCache 编译后的正则表达式缓存，键为正则模式字符串，值为 *regexp.Regexp
var compiledRegexCache sync.Map // map[string]*regexp.Regexp

// matchAnyRegex 检查字符串 s 是否匹配 patterns 中的任意一个正则表达式。
// 编译后的正则会被缓存，无效的正则模式会被跳过（不视为匹配）。
//
// 参数：
//   - patterns: 正则表达式模式列表
//   - s: 待匹配的字符串
//
// 返回：
//   - bool: true 表示至少匹配一个模式
func matchAnyRegex(patterns []string, s string) bool {
	if len(patterns) == 0 || s == "" {
		return false
	}
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		// 尝试从缓存中获取已编译的正则
		re, ok := compiledRegexCache.Load(pattern)
		if !ok {
			// 缓存未命中，编译正则并存入缓存
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				// 无效的正则模式跳过，不影响运行时流量
				continue
			}
			re = compiled
			compiledRegexCache.Store(pattern, re)
		}
		if re.(*regexp.Regexp).MatchString(s) {
			return true
		}
	}
	return false
}
