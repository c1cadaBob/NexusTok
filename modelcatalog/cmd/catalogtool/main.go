// Package main 提供模型仓库维护命令。
//
// 该命令只处理模型仓库文件，不读取生产数据库。它用于把已审查的 catalog JSON
// 导入为 TOML 仓库，或从 TOML 仓库重新生成构建时嵌入的 JSON/manifest。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

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
	case "sync-models-dev":
		runSyncModelsDev(os.Args[2:])
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

func runSyncModelsDev(args []string) {
	fs := flag.NewFlagSet("sync-models-dev", flag.ExitOnError)
	repoDir := fs.String("repo", modelcatalog.RepositoryDefaultDir, "target model catalog repository dir")
	catalogURL := fs.String("catalog-url", modelcatalog.ModelsDevDefaultCatalogURL, "models.dev catalog URL")
	treeURL := fs.String("github-tree-url", modelcatalog.ModelsDevGitHubDefaultTreeURL, "models.dev GitHub tree API URL")
	rawBase := fs.String("github-raw-base", modelcatalog.ModelsDevGitHubDefaultRawBase, "models.dev GitHub raw base URL")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP timeout")
	_ = fs.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result, err := modelcatalog.FetchModelsDevCatalogWithFallback(ctx, modelcatalog.ModelsDevFetchOptions{
		CatalogURL: *catalogURL,
		TreeURL:    *treeURL,
		RawBaseURL: *rawBase,
		Timeout:    *timeout,
	})
	if err != nil {
		fatalf("sync models.dev catalog: %v", err)
	}
	if result.Catalog == nil {
		fatalf("sync models.dev catalog: empty catalog")
	}
	if err := modelcatalog.WriteCatalogToRepository(*repoDir, result.Catalog); err != nil {
		fatalf("write repository: %v", err)
	}
	manifest := result.Catalog.Manifest
	fmt.Printf("synced models.dev catalog into %s\n", *repoDir)
	fmt.Printf("origin=%s fallback=%t stage=%s models=%d providers=%d version=%s\n",
		result.CatalogOrigin,
		result.FallbackUsed,
		result.FallbackStage,
		manifest.ModelCount,
		manifest.ProviderCount,
		manifest.Version,
	)
	if result.FallbackReason != "" {
		fmt.Printf("fallback_reason=%s\n", result.FallbackReason)
	}
}

func usageAndExit() {
	_, _ = fmt.Fprintln(os.Stderr, "usage: catalogtool import-json -source <catalog.json> [-repo modelcatalog/repository]")
	_, _ = fmt.Fprintln(os.Stderr, "       catalogtool generate [-repo modelcatalog/repository]")
	_, _ = fmt.Fprintln(os.Stderr, "       catalogtool sync-models-dev [-repo modelcatalog/repository]")
	os.Exit(2)
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
