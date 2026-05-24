// str.go - 字符串搜索与 Aho-Corasick 多模式匹配
// 本文件提供字符串搜索和多模式匹配功能。
// 包括 Sunday 单模式搜索算法、字符串去重、
// 以及基于 Aho-Corasick 自动机的多模式匹配（支持缓存复用）。
// Aho-Corasick 机主要用于敏感词检测等场景。
package service

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"

	goahocorasick "github.com/anknown/ahocorasick"
)

// SundaySearch 使用 Sunday 算法在文本中搜索模式串。
// Sunday 算法是一种高效的单模式串匹配算法，通过偏移表实现跳跃式匹配。
// 参数:
//   - text: 待搜索的文本串
//   - pattern: 模式串（搜索目标）
// 返回值:
//   - bool: 是否找到匹配
func SundaySearch(text string, pattern string) bool {
	// 计算偏移表
	offset := make(map[rune]int)
	for i, c := range pattern {
		offset[c] = len(pattern) - i
	}

	// 文本串长度和模式串长度
	n, m := len(text), len(pattern)

	// 主循环，i表示当前对齐的文本串位置
	for i := 0; i <= n-m; {
		// 检查子串
		j := 0
		for j < m && text[i+j] == pattern[j] {
			j++
		}
		// 如果完全匹配，返回匹配位置
		if j == m {
			return true
		}

		// 如果还有剩余字符，则检查下一位字符在偏移表中的值
		if i+m < n {
			next := rune(text[i+m])
			if val, ok := offset[next]; ok {
				i += val // 存在于偏移表中，进行跳跃
			} else {
				i += len(pattern) + 1 // 不存在于偏移表中，跳过整个模式串长度
			}
		} else {
			break
		}
	}
	return false // 如果没有找到匹配，返回-1
}

// RemoveDuplicate 对字符串切片去重，保持原始顺序。
// 使用 map 实现 O(n) 时间复杂度的去重。
// 参数:
//   - s: 待去重的字符串切片
// 返回值:
//   - []string: 去重后的字符串切片
func RemoveDuplicate(s []string) []string {
	result := make([]string, 0, len(s))
	temp := map[string]struct{}{}
	for _, item := range s {
		if _, ok := temp[item]; !ok {
			temp[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

// InitAc 初始化 Aho-Corasick 自动机。
// 将字典中的字符串转换为 rune 切片并构建自动机。
// 参数:
//   - dict: 字典字符串列表（敏感词列表）
// 返回值:
//   - *goahocorasick.Machine: 构建好的 Aho-Corasick 自动机，构建失败返回 nil
func InitAc(dict []string) *goahocorasick.Machine {
	m := new(goahocorasick.Machine)
	runes := readRunes(dict)
	if err := m.Build(runes); err != nil {
		fmt.Println(err)
		return nil
	}
	return m
}

// acCache Aho-Corasick 自动机的全局缓存，按字典哈希值索引
var acCache sync.Map

// acKey 计算字典的缓存键。
// 对字典进行归一化（小写、去空格、排序）后使用 FNV-64a 哈希。
// 参数:
//   - dict: 字典字符串列表
// 返回值:
//   - string: 缓存键（十六进制哈希值），空字典返回空字符串
func acKey(dict []string) string {
	if len(dict) == 0 {
		return ""
	}
	normalized := make([]string, 0, len(dict))
	for _, w := range dict {
		w = strings.ToLower(strings.TrimSpace(w))
		if w != "" {
			normalized = append(normalized, w)
		}
	}
	if len(normalized) == 0 {
		return ""
	}
	sort.Strings(normalized)
	hasher := fnv.New64a()
	for _, w := range normalized {
		hasher.Write([]byte{0})
		hasher.Write([]byte(w))
	}
	return fmt.Sprintf("%x", hasher.Sum64())
}

// getOrBuildAC 获取或构建 Aho-Corasick 自动机。
// 优先从缓存中获取，缓存未命中时构建新的自动机并存入缓存。
// 使用 LoadOrStore 保证并发安全，避免重复构建。
// 参数:
//   - dict: 字典字符串列表
// 返回值:
//   - *goahocorasick.Machine: Aho-Corasick 自动机实例，字典为空或构建失败返回 nil
func getOrBuildAC(dict []string) *goahocorasick.Machine {
	key := acKey(dict)
	if key == "" {
		return nil
	}
	if v, ok := acCache.Load(key); ok {
		if m, ok2 := v.(*goahocorasick.Machine); ok2 {
			return m
		}
	}
	m := InitAc(dict)
	if m == nil {
		return nil
	}
	if actual, loaded := acCache.LoadOrStore(key, m); loaded {
		if cached, ok := actual.(*goahocorasick.Machine); ok {
			return cached
		}
	}
	return m
}

// readRunes 将字典字符串列表转换为 rune 切片列表。
// 每个字符串会被转为小写并去除首尾空白。
// 参数:
//   - dict: 字典字符串列表
// 返回值:
//   - [][]rune: 转换后的 rune 切片列表
func readRunes(dict []string) [][]rune {
	var runes [][]rune

	for _, word := range dict {
		word = strings.ToLower(word)
		l := bytes.TrimSpace([]byte(word))
		runes = append(runes, bytes.Runes(l))
	}

	return runes
}

// AcSearch 使用 Aho-Corasick 自动机进行多模式匹配搜索。
// 在文本中搜索字典中的所有模式串，返回是否匹配及匹配到的词列表。
// 参数:
//   - findText: 待搜索的文本
//   - dict: 字典（模式串列表）
//   - stopImmediately: 是否在找到第一个匹配后立即停止
// 返回值:
//   - bool: 是否找到匹配
//   - []string: 匹配到的模式串列表
func AcSearch(findText string, dict []string, stopImmediately bool) (bool, []string) {
	if len(dict) == 0 {
		return false, nil
	}
	if len(findText) == 0 {
		return false, nil
	}
	m := getOrBuildAC(dict)
	if m == nil {
		return false, nil
	}
	hits := m.MultiPatternSearch([]rune(findText), stopImmediately)
	if len(hits) > 0 {
		words := make([]string, 0)
		for _, hit := range hits {
			words = append(words, string(hit.Word))
		}
		return true, words
	}
	return false, nil
}
