package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tnotstar/go-ya/internal/domain"
)

// openAILikeProvider is a generic adapter for any OpenAI-compatible upstream.
// It is embedded by OpenAI, OpenRouter, Abacus, and llama.cpp adapters.
type openAILikeProvider struct {
	baseURL        string
	authHeaderName string // e.g. "Authorization" with "Bearer " prefix, or "apiKey"
	authHeaderFmt  string // format string for the header value, e.g. "Bearer %s"
	extraHeaders   map[string]string
	client         *http.Client
	ptype          domain.ProviderType
	apiKey         string
}

func newOpenAILikeProvider(
	baseURL string,
	authHeaderName, authHeaderFmt string,
	extraHeaders map[string]string,
	client *http.Client,
	ptype domain.ProviderType,
	apiKey string,
) *openAILikeProvider {
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	return &openAILikeProvider{
		baseURL:        strings.TrimRight(baseURL, "/"),
		authHeaderName: authHeaderName,
		authHeaderFmt:  authHeaderFmt,
		extraHeaders:   extraHeaders,
		client:         client,
		ptype:          ptype,
		apiKey:         apiKey,
	}
}

// ProviderType implements Provider.
func (p *openAILikeProvider) ProviderType() domain.ProviderType { return p.ptype }

// ListModels calls GET {baseURL}/models and returns the model list.
func (p *openAILikeProvider) ListModels(ctx context.Context) ([]domain.Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("openai_like: list models: %w", err)
	}
	p.setAuthHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai_like: list models: %w", err)
	}
	defer resp.Body.Close()

	if err := mapHTTPError(resp); err != nil {
		return nil, err
	}

	var result struct {
		Data []domain.Model `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai_like: list models decode: %w", err)
	}
	return result.Data, nil
}

// ChatCompletion implements Provider (non-streaming).
func (p *openAILikeProvider) ChatCompletion(
	ctx context.Context,
	req *domain.ChatCompletionRequest,
) (*domain.ChatCompletionResponse, error) {
	// Force stream off for the non-streaming path.
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("openai_like: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai_like: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	p.setAuthHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai_like: do request: %w", err)
	}
	defer resp.Body.Close()

	if err := mapHTTPError(resp); err != nil {
		return nil, err
	}

	var result domain.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai_like: decode response: %w", err)
	}
	return &result, nil
}

// ChatCompletionStream implements Provider (SSE streaming).
func (p *openAILikeProvider) ChatCompletionStream(
	ctx context.Context,
	req *domain.ChatCompletionRequest,
) (<-chan StreamEvent, error) {
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("openai_like: marshal stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai_like: build stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	p.setAuthHeaders(httpReq)

	// Use a client without read timeout for streaming responses.
	streamClient := &http.Client{} // no timeout; context handles cancellation
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai_like: do stream request: %w", err)
	}

	if err := mapHTTPError(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}

	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}
			var chunk domain.StreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				ch <- StreamEvent{Err: fmt.Errorf("openai_like: decode chunk: %w", err)}
				return
			}
			select {
			case ch <- StreamEvent{Chunk: &chunk}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
			ch <- StreamEvent{Err: fmt.Errorf("openai_like: scanner: %w", err)}
		}
	}()

	return ch, nil
}

// setAuthHeaders applies auth and extra headers to the outgoing request.
// The API key is never copied into error messages or logs.
func (p *openAILikeProvider) setAuthHeaders(req *http.Request) {
	if p.authHeaderName != "" && p.apiKey != "" {
		val := p.apiKey
		if p.authHeaderFmt != "" {
			val = fmt.Sprintf(p.authHeaderFmt, p.apiKey)
		}
		req.Header.Set(p.authHeaderName, val)
	}
	for k, v := range p.extraHeaders {
		req.Header.Set(k, v)
	}
}

// mapHTTPError converts upstream HTTP error codes to domain sentinel errors.
// The upstream error body (bounded) is included so failures carry their real
// cause instead of a bare status code.
func mapHTTPError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	detail := readErrorSnippet(resp)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: HTTP 401: %s", domain.ErrInvalidKey, detail)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: HTTP 429: %s", domain.ErrRateLimited, detail)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("%w: HTTP 503: %s", domain.ErrProviderOverloaded, detail)
	default:
		return fmt.Errorf("%w: HTTP %d: %s", domain.ErrUpstreamError, resp.StatusCode, detail)
	}
}

// readErrorSnippet drains a bounded prefix of the upstream error body.
func readErrorSnippet(resp *http.Response) string {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "(empty body)"
	}
	return s
}

// newResponseID generates a unique response ID in OpenAI format.
func newResponseID() string {
	return "chatcmpl-" + uuid.NewString()
}
