// Package common - copy.go
// 该文件提供了通用的深拷贝工具函数
//
// 使用 github.com/jinzhu/copier 库实现结构体的深拷贝
// 深拷贝会创建对象的完整副本，修改副本不会影响原始对象
package common

import (
	"fmt"

	"github.com/jinzhu/copier"
)

// DeepCopy 对结构体进行深拷贝
//
// 使用泛型支持任意结构体类型
// 深拷贝会递归复制所有字段，包括嵌套的指针、切片、映射等
//
// 参数：
//   - src: 源对象指针
//
// 返回值：
//   - *T: 深拷贝后的对象指针
//   - error: 拷贝错误（如源对象为 nil）
func DeepCopy[T any](src *T) (*T, error) {
	if src == nil {
		return nil, fmt.Errorf("copy source cannot be nil")
	}
	var dst T
	// DeepCopy: 递归深拷贝所有字段
	// IgnoreEmpty: 忽略空值字段（零值不复制）
	err := copier.CopyWithOption(&dst, src, copier.Option{DeepCopy: true, IgnoreEmpty: true})
	if err != nil {
		return nil, err
	}
	return &dst, nil
}
