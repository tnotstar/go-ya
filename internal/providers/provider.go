// Package providers defines the Provider interface and related types
// used by all upstream LLM provider adapters.
package providers

import (
	"context"

	"github.com/tnotstar/go-ya/internal/domain"
)

// Provider is the contract that every upstream LLM adapter must satisfy.
// It is defined here (in the consumer package) per Go interface design guidelines:
// interfaces belong to the consumer, not the implementor.
type Provider interface {
	// ListModels returns the models available from this provider.
	ListModels(ctx context.Context) ([]domain.Model, error)
	// ChatCompletion performs a non-streaming chat completion request.
	ChatCompletion(ctx context.Context, req *domain.ChatCompletionRequest) (*domain.ChatCompletionResponse, error)
	// ChatCompletionStream starts a streaming chat completion.
	// The returned channel is closed when the stream ends or an error occurs.
	ChatCompletionStream(ctx context.Context, req *domain.ChatCompletionRequest) (<-chan StreamEvent, error)
	// ProviderType returns the type enum of this adapter.
	ProviderType() domain.ProviderType
}

// StreamEvent wraps a single streamed payload with an optional error.
// Either Chunk or Err is non-zero; never both.
type StreamEvent struct {
	Chunk *domain.StreamChunk
	Err   error
}
