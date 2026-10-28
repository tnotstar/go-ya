# AGENTS.md — go-ya AI Gateway

## Project Overview

go-ya is a protocol-agnostic AI Gateway written in Go. It exposes OpenAI-compatible REST endpoints (`/v1/chat/completions`, `/v1/models`) and an Anthropic-compatible endpoint (`/v1/messages`), routing requests to multiple upstream LLM providers with session persistence and encrypted key management.

**Module**: `github.com/tnotstar/go-ya` · **Go**: 1.26+ · **Entry point**: `cmd/gateway/main.go`

## Commands

```bash
# Run the gateway server
go run cmd/gateway/main.go serve

# CLI: manage providers
go run cmd/gateway/main.go providers add --name openai --type openai
go run cmd/gateway/main.go providers list
go run cmd/gateway/main.go providers enable <id>
go run cmd/gateway/main.go providers disable <id>

# CLI: manage API keys (prompts stdin if --key is omitted)
go run cmd/gateway/main.go keys add --provider openai --label my-key
go run cmd/gateway/main.go keys list --provider openai
go run cmd/gateway/main.go keys rotate --id <key-id> --label new-key

# CLI: inspect sessions
go run cmd/gateway/main.go sessions list --user <user-id>
go run cmd/gateway/main.go sessions show <session-id>

# Docker (production)
docker-compose up -d

# Tests
go test ./...
```

## Architecture

```
cmd/gateway/main.go          — Single binary entry point (urfave/cli)
internal/
├── config/                  — Env-var config (GATEWAY_ prefix, envconfig)
├── domain/                  — Core types: models, enums, errors, canonical request/response
├── ctxkeys/                 — Echo context key constants (avoids import cycles)
├── crypto/                  — AES-256-GCM encryption, SHA-256 key hashing
├── persistence/             — GORM repos (Provider, Key, Session, Message) + DB init
├── providers/               — Provider interface, registry, and adapters
│   ├── provider.go          — Provider interface definition
│   ├── registry.go          — Factory: builds Provider from domain.Provider + API key
│   ├── openai_like.go       — Shared adapter for OpenAI-compatible APIs
│   ├── openai.go            — OpenAI (embeds openai_like)
│   ├── openrouter.go        — OpenRouter (embeds openai_like + model cache)
│   ├── abacus.go            — Abacus.AI (embeds openai_like, custom auth header)
│   ├── llamacpp.go          — llama.cpp (embeds openai_like, Unix socket support)
│   ├── gemini.go            — Google Gemini (standalone, full translation layer)
│   └── mock.go              — Test double
├── translators/             — Protocol translation: Anthropic ↔ canonical OpenAI format
├── services/                — Business logic layer
│   ├── router.go            — Prefix-based routing + key fallback/retry
│   ├── keystore.go          — Key selection (round-robin), encrypt, decrypt, rotate
│   ├── session.go           — Session lifecycle + message persistence
│   └── syncmap.go           — sync.Map wrapper for atomic key counters
├── handlers/                — Echo HTTP handlers (chat, messages, models, health)
├── server/                  — Echo server setup, middleware stack, route registration
└── cli/                     — CLI command implementations (serve, providers, keys, sessions)
```

**Data flow**: HTTP request → middleware (auth, client detection, logging) → handler → `RouterService.RouteWithFallback` → `KeyStore` picks key → `Registry.Build` creates provider → provider calls upstream → response persisted async → returned to client.

## Key Patterns & Conventions

### Canonical Internal Format
All cross-provider communication uses OpenAI Chat Completions schema as the canonical format (`domain.ChatCompletionRequest`/`Response`). Provider adapters either pass through (OpenAI-compatible) or translate (Gemini, Anthropic via `translators/`).

### Model Prefix Routing
Models are specified as `"provider:model"` (e.g., `openai:gpt-4o`, `openrouter:anthropic/claude-3-5-sonnet`). No prefix uses `GATEWAY_DEFAULT_PROVIDER`. `"local:"` is aliased to `"llamacpp"`.

### Provider Adapters
- **OpenAI-compatible providers** (OpenAI, OpenRouter, Abacus, llama.cpp) embed `openAILikeProvider` which handles SSE streaming, auth headers, and error mapping.
- **Gemini** is standalone — it has its own request/response types and full translation layer in `gemini.go`.
- **Anthropic** is handled at the handler level via `translators/anthropic.go`; no dedicated provider adapter since requests are routed to an OpenAI-compatible backend.

### Key Management
- API keys encrypted at rest with AES-256-GCM (`crypto/`), each uniquely salted.
- `KeyStore` uses atomic round-robin (`sync.Map` + `atomic.Int64`) for key distribution per provider.
- On rate-limit (429) or invalid key (401), the router disables the key and retries with the next one (`RouteWithFallback`).
- Key rotation is atomic via `KeyRepo.DisableAndCreate` (transactional).

### Error Handling
Domain sentinel errors (`domain/errors.go`) are used throughout: `ErrProviderNotFound`, `ErrKeyNotFound`, `ErrRateLimited`, `ErrInvalidKey`, `ErrProviderOverloaded`, `ErrUpstreamError`, `ErrSessionNotFound`. Handlers map these to HTTP status codes via `mapProviderError` in `handlers/helpers.go`. Provider HTTP errors are mapped in `openai_like.go:mapHTTPError` and `gemini.go:mapGeminiHTTPError`.

### Config
All config via env vars with `GATEWAY_` prefix, parsed by `kelseyhightower/envconfig`. See `.env.example` for all options. `GATEWAY_MASTER_KEY` is required (64-char hex = 32 bytes).

### Database
- GORM with AutoMigrate on startup. SQLite for dev (`gateway.db` with WAL mode), Postgres for production.
- SQLite DSN auto-appends `?_journal_mode=WAL&_foreign_keys=on`.
- All GORM models use UUID primary keys assigned in `BeforeCreate` hooks.

### Context Keys
`ctxkeys/keys.go` defines typed keys (`UserID`, `ClientType`) as `string` constants to avoid collisions. Set by middleware, read by handlers.

### Client Detection
`server/middleware.go:detectClientType` infers downstream client (LibreChat, Claude Code, OpenCode) from User-Agent and request headers.

## Testing

- Tests use standard `go test` with `*_test.go` files in external test packages (e.g., `package handlers_test`).
- Integration tests (`handlers/integration_test.go`) spin up an in-memory SQLite DB, mock provider, and full Echo server.
- `MockProvider` (`providers/mock.go`) is thread-safe and configurable for responses, chunks, and errors.
- The crypto package tests (`crypto/aes_test.go`) validate encrypt/decrypt round-trips, nonce uniqueness, and wrong-key rejection.
- Translator tests (`translators/anthropic_test.go`) validate bidirectional conversion between Anthropic and canonical formats.

## Gotchas

- **Streaming uses separate HTTP client**: `openai_like.go:ChatCompletionStream` creates a new `http.Client{}` without timeout for streaming — context cancellation handles timeouts, not the client's Timeout field.
- **Async persistence**: Message persistence after chat/messages completion is fire-and-forget (`go h.persistMessages(...)`) using `context.Background()`, not the request context.
- **Gemini models are hardcoded**: `GeminiProvider.ListModels` returns a static list of known models rather than calling the Gemini API, to avoid returning research-preview models.
- **Abacus auth header is configurable**: Abacus uses `"apiKey"` header by default (not `Authorization: Bearer`), but this is configurable via `auth_header_name` in the DB and the `--auth-header` CLI flag.
- **llama.cpp supports Unix sockets**: Base URLs starting with `unix://` trigger a custom HTTP transport that dials a Unix domain socket.
- **OpenRouter caches model list**: `OpenRouterProvider` caches models with a configurable TTL (`GATEWAY_MODEL_CACHE_TTL`, default 5m) using a mutex.
- **Empty API key = no auth header**: `openAILikeProvider.setAuthHeaders` skips setting the auth header when the API key is empty, which is the expected behavior for local providers like llama.cpp.
- **router_test.go re-implements parseModelPrefix**: The test file re-implements the parsing logic locally rather than testing the unexported function directly. This is a known limitation.
- **No linter/formatter config found**: No `.golangci.yml`, Makefile, or lint targets detected in the repository.
