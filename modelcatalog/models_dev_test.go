package modelcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchModelsDevCatalogWithWebSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/catalog.json", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"models": {
				"openai/gpt-web": {
					"id": "openai/gpt-web",
					"name": "GPT Web",
					"reasoning": true
				}
			},
			"providers": {
				"openai": {
					"id": "openai",
					"name": "OpenAI",
					"models": {
						"gpt-web": {
							"id": "gpt-web",
							"cost": {"input": 1, "output": 4}
						}
					}
				}
			}
		}`))
	}))
	t.Cleanup(server.Close)

	result, err := FetchModelsDevCatalogWithFallback(context.Background(), ModelsDevFetchOptions{
		CatalogURL: server.URL + "/catalog.json",
	})

	require.NoError(t, err)
	require.False(t, result.FallbackUsed)
	require.Equal(t, CatalogOriginModelsDevWeb, result.CatalogOrigin)
	require.Contains(t, result.Catalog.Models, "openai/gpt-web")
	require.Contains(t, result.Catalog.Providers["openai"].Models, "gpt-web")
	require.Equal(t, 1, result.Catalog.Manifest.ModelCount)
}

func TestFetchModelsDevCatalogWithGitHubFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog.json":
			http.Error(w, "web down", http.StatusBadGateway)
		case "/tree":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"tree": [
					{"path": "README.md", "type": "blob"},
					{"path": "models/openai/gpt-github.toml", "type": "blob"},
					{"path": "providers/openai/provider.toml", "type": "blob"},
					{"path": "providers/openai/models/gpt-github.toml", "type": "blob"}
				]
			}`))
		case "/raw/models/openai/gpt-github.toml":
			_, _ = w.Write([]byte(`
id = "gpt-github"
name = "GPT GitHub"
reasoning = true
`))
		case "/raw/providers/openai/provider.toml":
			_, _ = w.Write([]byte(`
id = "openai"
name = "OpenAI"
`))
		case "/raw/providers/openai/models/gpt-github.toml":
			_, _ = w.Write([]byte(`
id = "gpt-github"

[cost]
input = 2
output = 8
`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	result, err := FetchModelsDevCatalogWithFallback(context.Background(), ModelsDevFetchOptions{
		CatalogURL: server.URL + "/catalog.json",
		TreeURL:    server.URL + "/tree",
		RawBaseURL: server.URL + "/raw",
	})

	require.NoError(t, err)
	require.True(t, result.FallbackUsed)
	require.Equal(t, CatalogOriginModelsDevGitHub, result.CatalogOrigin)
	require.Equal(t, FallbackStageGitHub, result.FallbackStage)
	require.Equal(t, ModelsDevGitHubRepo, result.GitHubRepo)
	require.Contains(t, result.FallbackReason, "502")
	require.Contains(t, result.Catalog.Models, "openai/gpt-github")
	require.Contains(t, result.Catalog.Providers["openai"].Models, "gpt-github")
}

func TestIsModelsDevCatalogTOMLPath(t *testing.T) {
	require.True(t, IsModelsDevCatalogTOMLPath("models/openai/gpt.toml"))
	require.True(t, IsModelsDevCatalogTOMLPath("providers/openai/provider.toml"))
	require.True(t, IsModelsDevCatalogTOMLPath("providers/openai/models/gpt.toml"))
	require.False(t, IsModelsDevCatalogTOMLPath("README.md"))
	require.False(t, IsModelsDevCatalogTOMLPath("providers/openai/secrets/key.toml"))
	require.False(t, IsModelsDevCatalogTOMLPath("models/openai/nested/gpt.toml"))
}
