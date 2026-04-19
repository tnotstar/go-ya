package providers

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/tnotstar/go-ya/internal/domain"
)

const llamaCPPDefaultBaseURL = "http://localhost:8080/v1"

// LlamaCPPProvider adapts the llama.cpp local inference server.
//
// llama.cpp exposes an OpenAI-compatible API at /v1/chat/completions and /v1/models.
//
// Unix socket support: if base_url starts with "unix://", the adapter strips the
// scheme and uses the path as a Unix domain socket, translating to http+unix
// transport. Example: "unix:///tmp/llama.sock" → dial("unix", "/tmp/llama.sock").
//
// Usage stats: llama.cpp may not return usage counts; this is handled gracefully
// by the caller (usage may be nil or zero).
//
// API key: local providers typically have no key requirement. Pass an empty
// string for apiKey; no auth header will be set for empty keys.
type LlamaCPPProvider struct {
	*openAILikeProvider
}

// NewLlamaCPP returns a llama.cpp provider adapter.
// baseURL supports "unix://<socketpath>" for Unix socket connections.
func NewLlamaCPP(baseURL, apiKey string) *LlamaCPPProvider {
	if baseURL == "" {
		baseURL = llamaCPPDefaultBaseURL
	}

	var httpClient *http.Client
	var resolvedBaseURL string

	if strings.HasPrefix(baseURL, "unix://") {
		// Extract the socket path from "unix:///path/to/socket".
		socketPath := strings.TrimPrefix(baseURL, "unix://")
		transport := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{
					Timeout: 5 * time.Second,
				}).DialContext(ctx, "unix", socketPath)
			},
		}
		httpClient = &http.Client{Transport: transport}
		// The HTTP host is irrelevant for Unix sockets but must be syntactically valid.
		resolvedBaseURL = "http://localhost/v1"
	} else {
		resolvedBaseURL = baseURL
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}

	return &LlamaCPPProvider{
		openAILikeProvider: newOpenAILikeProvider(
			resolvedBaseURL,
			"Authorization", "Bearer %s",
			nil,
			httpClient,
			domain.ProviderTypeLlamaCPP,
			apiKey, // empty key → no auth header set (see setAuthHeaders)
		),
	}
}
