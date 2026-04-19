package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tnotstar/go-ya/internal/domain"
)

const geminiBaseURL = "https://generativelanguage.googleapis.com"

// GeminiProvider adapts the Google Gemini generateContent API.
//
// Role mapping:
//   - OpenAI "user"      → Gemini "user"
//   - OpenAI "assistant" → Gemini "model"
//   - OpenAI "system"    → Gemini system_instruction (top-level field)
//   - OpenAI "tool"      → Gemini functionResponse part
//
// Auth: API key via query param ?key=<key> or x-goog-api-key header.
// We use the header to avoid key leakage in server access logs.
type GeminiProvider struct {
	baseURL    string
	apiKey     string
	client     *http.Client
}

// NewGemini returns a Gemini provider adapter.
func NewGemini(apiKey string, client *http.Client) *GeminiProvider {
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	return &GeminiProvider{
		baseURL: geminiBaseURL,
		apiKey:  apiKey,
		client:  client,
	}
}

// ProviderType implements Provider.
func (p *GeminiProvider) ProviderType() domain.ProviderType { return domain.ProviderTypeGemini }

// ListModels returns a static list of well-known Gemini models.
// The Gemini REST API does have a /v1beta/models endpoint, but it returns
// research-preview models that may not be suitable for chat; static list is safer.
func (p *GeminiProvider) ListModels(_ context.Context) ([]domain.Model, error) {
	now := time.Now().Unix()
	return []domain.Model{
		{ID: "gemini:gemini-2.5-pro", Object: "model", Created: now, OwnedBy: "google"},
		{ID: "gemini:gemini-2.0-flash", Object: "model", Created: now, OwnedBy: "google"},
		{ID: "gemini:gemini-1.5-pro", Object: "model", Created: now, OwnedBy: "google"},
		{ID: "gemini:gemini-1.5-flash", Object: "model", Created: now, OwnedBy: "google"},
	}, nil
}

// ChatCompletion performs a non-streaming Gemini request.
func (p *GeminiProvider) ChatCompletion(
	ctx context.Context,
	req *domain.ChatCompletionRequest,
) (*domain.ChatCompletionResponse, error) {
	modelID := stripPrefix(req.Model, "gemini:")
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", p.baseURL, modelID)

	gReq, err := canonicalToGemini(req)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(gReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Use header auth to avoid key in URL access logs.
	httpReq.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: do request: %w", err)
	}
	defer resp.Body.Close()

	if err := mapGeminiHTTPError(resp); err != nil {
		return nil, err
	}

	var gResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gResp); err != nil {
		return nil, fmt.Errorf("gemini: decode response: %w", err)
	}
	return geminiToCanonical(&gResp, req.Model), nil
}

// ChatCompletionStream performs a streaming Gemini request using streamGenerateContent.
// Gemini streams newline-delimited JSON objects (each a full partial GenerateContentResponse).
// We translate each chunk to OpenAI SSE format.
func (p *GeminiProvider) ChatCompletionStream(
	ctx context.Context,
	req *domain.ChatCompletionRequest,
) (<-chan StreamEvent, error) {
	modelID := stripPrefix(req.Model, "gemini:")
	// alt=sse forces the response to be true SSE format.
	url := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", p.baseURL, modelID)

	gReq, err := canonicalToGemini(req)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(gReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: build stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", p.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	streamClient := &http.Client{}
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: do stream request: %w", err)
	}
	if err := mapGeminiHTTPError(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}

	ch := make(chan StreamEvent, 32)
	respID := "chatcmpl-" + uuid.NewString()
	created := time.Now().Unix()

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

			var gResp geminiResponse
			if err := json.Unmarshal([]byte(data), &gResp); err != nil {
				ch <- StreamEvent{Err: fmt.Errorf("gemini: decode chunk: %w", err)}
				return
			}

			chunk := geminiResponseToStreamChunk(&gResp, req.Model, respID, created)
			select {
			case ch <- StreamEvent{Chunk: chunk}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
			ch <- StreamEvent{Err: fmt.Errorf("gemini: scanner: %w", err)}
		}
	}()

	return ch, nil
}

// ─── Gemini request/response types ───────────────────────────────────────────

type geminiRequest struct {
	Contents          []geminiContent      `json:"contents"`
	SystemInstruction *geminiContent       `json:"systemInstruction,omitempty"`
	Tools             []geminiTool         `json:"tools,omitempty"`
	GenerationConfig  *geminiGenConfig     `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string              `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *geminiFuncResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type geminiFuncResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFuncDecl `json:"functionDeclarations"`
}

type geminiFuncDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type geminiGenConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate    `json:"candidates"`
	UsageMetadata *geminiUsage         `json:"usageMetadata,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
	Index        int           `json:"index"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// ─── Translation functions ────────────────────────────────────────────────────

// canonicalToGemini converts a canonical ChatCompletionRequest to a Gemini request.
func canonicalToGemini(req *domain.ChatCompletionRequest) (*geminiRequest, error) {
	gReq := &geminiRequest{}

	// Build generationConfig.
	if req.Temperature != nil || req.TopP != nil || req.MaxTokens != nil {
		gReq.GenerationConfig = &geminiGenConfig{
			Temperature:     req.Temperature,
			TopP:            req.TopP,
			MaxOutputTokens: req.MaxTokens,
		}
		// parse stop sequences if present
		if len(req.Stop) > 0 {
			var stops []string
			// Try as a JSON array first, then as a plain string.
			if err := json.Unmarshal(req.Stop, &stops); err != nil {
				var s string
				if err2 := json.Unmarshal(req.Stop, &s); err2 == nil {
					stops = []string{s}
				}
			}
			if gReq.GenerationConfig == nil {
				gReq.GenerationConfig = &geminiGenConfig{}
			}
			gReq.GenerationConfig.StopSequences = stops
		}
	}

	// Extract system message and build contents.
	for _, msg := range req.Messages {
		switch msg.Role {
		case domain.RoleSystem:
			text := extractTextContent(msg.Content)
			gReq.SystemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: text}},
			}
		case domain.RoleUser:
			gReq.Contents = append(gReq.Contents, geminiContent{
				Role:  "user",
				Parts: []geminiPart{{Text: extractTextContent(msg.Content)}},
			})
		case domain.RoleAssistant:
			parts := []geminiPart{}
			if len(msg.Content) > 0 {
				parts = append(parts, geminiPart{Text: extractTextContent(msg.Content)})
			}
			// Tool calls from assistant.
			for _, tc := range msg.ToolCalls {
				parts = append(parts, geminiPart{
					FunctionCall: &geminiFunctionCall{
						Name: tc.Function.Name,
						Args: json.RawMessage(tc.Function.Arguments),
					},
				})
			}
			gReq.Contents = append(gReq.Contents, geminiContent{Role: "model", Parts: parts})
		case domain.RoleTool:
			// Tool response — maps to functionResponse part.
			gReq.Contents = append(gReq.Contents, geminiContent{
				Role: "user",
				Parts: []geminiPart{{
					FunctionResponse: &geminiFuncResponse{
						Name:     msg.Name,
						Response: msg.Content,
					},
				}},
			})
		}
	}

	// Translate tools.
	if len(req.Tools) > 0 {
		decls := make([]geminiFuncDecl, 0, len(req.Tools))
		for _, t := range req.Tools {
			decls = append(decls, geminiFuncDecl{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			})
		}
		gReq.Tools = []geminiTool{{FunctionDeclarations: decls}}
	}

	return gReq, nil
}

// geminiToCanonical converts a Gemini response to a canonical ChatCompletionResponse.
func geminiToCanonical(resp *geminiResponse, model string) *domain.ChatCompletionResponse {
	choices := make([]domain.Choice, 0, len(resp.Candidates))
	for _, c := range resp.Candidates {
		msg := domain.ChatMessage{Role: domain.RoleAssistant}
		var textParts []string
		for _, part := range c.Content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
			if fc := part.FunctionCall; fc != nil {
				args, _ := json.Marshal(fc.Args)
				msg.ToolCalls = append(msg.ToolCalls, domain.ToolCall{
					ID:   "call-" + uuid.NewString(),
					Type: "function",
					Function: domain.FunctionCall{
						Name:      fc.Name,
						Arguments: string(args),
					},
				})
			}
		}
		combined := strings.Join(textParts, "")
		contentJSON, _ := json.Marshal(combined)
		msg.Content = contentJSON

		choices = append(choices, domain.Choice{
			Index:        c.Index,
			Message:      msg,
			FinishReason: mapGeminiFinishReason(c.FinishReason),
		})
	}

	result := &domain.ChatCompletionResponse{
		ID:      "chatcmpl-" + uuid.NewString(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: choices,
	}
	if u := resp.UsageMetadata; u != nil {
		result.Usage = &domain.Usage{
			PromptTokens:     u.PromptTokenCount,
			CompletionTokens: u.CandidatesTokenCount,
			TotalTokens:      u.TotalTokenCount,
		}
	}
	return result
}

// geminiResponseToStreamChunk converts one streaming Gemini chunk to an OpenAI StreamChunk.
func geminiResponseToStreamChunk(resp *geminiResponse, model, id string, created int64) *domain.StreamChunk {
	choices := make([]domain.StreamChoice, 0, len(resp.Candidates))
	for _, c := range resp.Candidates {
		delta := domain.Delta{}
		for _, part := range c.Content.Parts {
			if part.Text != "" {
				delta.Content += part.Text
			}
		}
		choices = append(choices, domain.StreamChoice{
			Index:        c.Index,
			Delta:        delta,
			FinishReason: mapGeminiFinishReason(c.FinishReason),
		})
	}
	return &domain.StreamChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: choices,
	}
}

// mapGeminiFinishReason converts Gemini finish reasons to canonical FinishReason values.
func mapGeminiFinishReason(reason string) domain.FinishReason {
	switch reason {
	case "STOP":
		return domain.FinishReasonStop
	case "MAX_TOKENS":
		return domain.FinishReasonLength
	case "SAFETY":
		return domain.FinishReasonContentFilter
	case "TOOL_CALLS":
		return domain.FinishReasonToolCalls
	default:
		return domain.FinishReasonNull
	}
}

// mapGeminiHTTPError maps Gemini API HTTP errors to domain errors.
func mapGeminiHTTPError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: Gemini HTTP %d", domain.ErrInvalidKey, resp.StatusCode)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: Gemini HTTP 429", domain.ErrRateLimited)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("%w: Gemini HTTP 503", domain.ErrProviderOverloaded)
	default:
		return fmt.Errorf("%w: Gemini HTTP %d", domain.ErrUpstreamError, resp.StatusCode)
	}
}

// extractTextContent extracts plain text from a json.RawMessage that may be
// a JSON string or an array of content parts.
func extractTextContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try plain string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Try array of content parts.
	var parts []domain.ContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		var sb strings.Builder
		for _, p := range parts {
			if p.Type == "text" {
				sb.WriteString(p.Text)
			}
		}
		return sb.String()
	}
	return ""
}

// stripPrefix removes "prefix:" from model names, e.g. "gemini:gemini-2.0-flash" → "gemini-2.0-flash".
func stripPrefix(model, prefix string) string {
	return strings.TrimPrefix(model, prefix)
}
