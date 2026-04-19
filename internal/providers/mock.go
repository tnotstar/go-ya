package providers

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/tnotstar/go-ya/internal/domain"
)

// MockProvider is a configurable test double for the Provider interface.
// All methods are goroutine-safe.
type MockProvider struct {
	mu     sync.Mutex
	models []domain.Model
	resp   *domain.ChatCompletionResponse
	chunks []domain.StreamChunk
	err    error
}

// NewMock returns a MockProvider with sensible defaults.
func NewMock() *MockProvider {
	content, _ := json.Marshal("Mock response from test provider.")
	return &MockProvider{
		models: []domain.Model{
			{ID: "mock:mock-model", Object: "model", Created: time.Now().Unix(), OwnedBy: "mock"},
		},
		resp: &domain.ChatCompletionResponse{
			ID:      newResponseID(),
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "mock:mock-model",
			Choices: []domain.Choice{
				{
					Index: 0,
					Message: domain.ChatMessage{
						Role:    domain.RoleAssistant,
						Content: json.RawMessage(content),
					},
					FinishReason: domain.FinishReasonStop,
				},
			},
			Usage: &domain.Usage{PromptTokens: 10, CompletionTokens: 8, TotalTokens: 18},
		},
	}
}

// SetError configures the mock to return the given error on all calls.
func (m *MockProvider) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// SetResponse configures the non-streaming response returned by the mock.
func (m *MockProvider) SetResponse(resp *domain.ChatCompletionResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resp = resp
}

// ProviderType implements Provider.
func (m *MockProvider) ProviderType() domain.ProviderType { return domain.ProviderTypeMock }

// ListModels returns the configured mock models.
func (m *MockProvider) ListModels(_ context.Context) ([]domain.Model, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return m.models, nil
}

// ChatCompletion returns the configured mock response.
func (m *MockProvider) ChatCompletion(_ context.Context, _ *domain.ChatCompletionRequest) (*domain.ChatCompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

// ChatCompletionStream sends the configured chunks as a stream.
func (m *MockProvider) ChatCompletionStream(ctx context.Context, _ *domain.ChatCompletionRequest) (<-chan StreamEvent, error) {
	m.mu.Lock()
	err := m.err
	chunks := m.chunks
	resp := m.resp
	m.mu.Unlock()

	if err != nil {
		return nil, err
	}

	ch := make(chan StreamEvent, maxInt(len(chunks), 1))

	// If no explicit chunks are configured, synthesize a single chunk from resp.
	if len(chunks) == 0 && resp != nil {
		var content string
		if len(resp.Choices) > 0 {
			_ = json.Unmarshal(resp.Choices[0].Message.Content, &content)
		}
		chunks = []domain.StreamChunk{
			{
				ID:      resp.ID,
				Object:  "chat.completion.chunk",
				Created: resp.Created,
				Model:   resp.Model,
				Choices: []domain.StreamChoice{
					{
						Index:        0,
						Delta:        domain.Delta{Content: content},
						FinishReason: domain.FinishReasonStop,
					},
				},
			},
		}
	}

	go func() {
		defer close(ch)
		for i := range chunks {
			select {
			case ch <- StreamEvent{Chunk: &chunks[i]}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
