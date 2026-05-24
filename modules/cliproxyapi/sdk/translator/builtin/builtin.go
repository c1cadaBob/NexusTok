// Package builtin 向 SDK 用户暴露内置翻译器注册。
// 通过 side-effect 导入 internal/translator 包来注册所有内置翻译器。
package builtin

import (
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"

	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator"
)

// Registry 返回已填充所有内置翻译器的默认注册表。
func Registry() *sdktranslator.Registry {
	return sdktranslator.Default()
}

// Pipeline 返回已包含内置翻译器的管道。
func Pipeline() *sdktranslator.Pipeline {
	return sdktranslator.NewPipeline(sdktranslator.Default())
}
