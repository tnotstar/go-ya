// Package domain defines the core enumerations used across the gateway.
package domain

// ProviderType identifies the upstream LLM provider.
type ProviderType string

const (
	ProviderTypeOpenAI    ProviderType = "openai"
	ProviderTypeOpenRouter ProviderType = "openrouter"
	ProviderTypeGemini    ProviderType = "gemini"
	ProviderTypeAbacus    ProviderType = "abacus"
	ProviderTypeLlamaCPP  ProviderType = "llamacpp"
	ProviderTypeMock      ProviderType = "mock"
)

// ClientType identifies the downstream client that sent the request.
type ClientType string

const (
	ClientTypeLibreChat  ClientType = "librechat"
	ClientTypeClaudeCode ClientType = "claudecode"
	ClientTypeOpencode   ClientType = "opencode"
	ClientTypeAPI        ClientType = "api"
	ClientTypeUnknown    ClientType = "unknown"
)

// Role defines message roles for chat interactions.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// FinishReason indicates why the model stopped generating tokens.
type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonLength        FinishReason = "length"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonNull          FinishReason = ""
)
