// Package main 提供模型仓库维护命令。
//
// 该命令只处理模型仓库文件，不读取生产数据库。它用于把已审查的 catalog JSON
// 导入为 TOML 仓库，或从 TOML 仓库重新生成构建时嵌入的 JSON/manifest。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/modelcatalog"
)

func main() {
	if len(os.Args) < 2 {
		usageAndExit()
	}
	switch os.Args[1] {
	case "import-json":
		runImportJSON(os.Args[2:])
	case "generate":
		runGenerate(os.Args[2:])
	default:
		usageAndExit()
	}
}

func runImportJSON(args []string) {
	fs := flag.NewFlagSet("import-json", flag.ExitOnError)
	source := fs.String("source", "", "source catalog JSON file")
	repoDir := fs.String("repo", modelcatalog.RepositoryDefaultDir, "target model catalog repository dir")
	_ = fs.Parse(args)
	if *source == "" {
		fatalf("missing -source")
	}
	buf, err := os.ReadFile(*source)
	if err != nil {
		fatalf("read source: %v", err)
	}
	var catalog modelcatalog.Catalog
	if err := common.Unmarshal(buf, &catalog); err != nil {
		fatalf("parse source catalog: %v", err)
	}
	if err := modelcatalog.WriteCatalogToRepository(*repoDir, &catalog); err != nil {
		fatalf("write repository: %v", err)
	}
	fmt.Printf("imported %s into %s\n", *source, *repoDir)
}

func runGenerate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	repoDir := fs.String("repo", modelcatalog.RepositoryDefaultDir, "model catalog repository dir")
	_ = fs.Parse(args)
	catalog, err := modelcatalog.LoadRepository(*repoDir)
	if err != nil {
		fatalf("load repository: %v", err)
	}
	if err := modelcatalog.WriteGeneratedFiles(*repoDir, catalog); err != nil {
		fatalf("write generated files: %v", err)
	}
	fmt.Printf("generated catalog files in %s\n", *repoDir)
}

func usageAndExit() {
	_, _ = fmt.Fprintln(os.Stderr, "usage: catalogtool import-json -source <catalog.json> [-repo modelcatalog/repository]")
	_, _ = fmt.Fprintln(os.Stderr, "       catalogtool generate [-repo modelcatalog/repository]")
	os.Exit(2)
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
