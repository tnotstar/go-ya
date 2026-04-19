# go-ya AI Gateway

`go-ya` is a production-ready, protocol-agnostic AI Gateway written in Go. It provides OpenAI-compatible REST endpoints, intelligently routes requests to multiple LLM upstream providers (OpenAI, OpenRouter, Google Gemini, Abacus.AI, and local llama.cpp), persists user sessions in a database, and securely handles API keys with AES-256-GCM encryption.

## Quickstart

1. Copy `.env.example` to `.env` and generate a master key:
   ```bash
   cp .env.example .env
   # Generate a 64-character hex master key
   openssl rand -hex 32
   ```
2. Set `GATEWAY_MASTER_KEY` in `.env` with the generated value.

3. Run the database and gateway via Docker Compose:
   ```bash
   docker-compose up -d
   ```
   Or run natively using `go run cmd/gateway/main.go serve`.

## Features

- **OpenAI Compatibility**: Complete drop-in replacement for OpenAI endpoints (`/v1/chat/completions`, `/v1/models`).
- **Anthropic Compatibility**: Full native support for Anthropic's Messages API (`/v1/messages`) including SSE streaming conversions to serve downstream clients like Claude Code.
- **Provider Support**: Out-of-the-box adapters for OpenAI, OpenRouter (w/ caching & attribution), Gemini, Abacus.AI, and llama.cpp.
- **Session Persistence**: Stores full conversation history tracking last provider and model via GORM. SQLite for dev and Postgres for production.
- **Key Store**: At-rest encrypted API keys with atomic rotation and round-robin load distribution.
- **CLI Commands**: Manage providers, rotate API keys securely, and inspect sessions right from the binary.

## CLI Usage

Add a provider:
```bash
go run cmd/gateway/main.go providers add --name openai --type openai
```

Add an API key securely (will prompt on stdin):
```bash
go run cmd/gateway/main.go keys add --provider openai --label my-secret-key
```

Start the server:
```bash
go run cmd/gateway/main.go serve
```

## Security Design

All downstream requests are stateless and utilize simple Bearer token HTTP authentication map configured inside `GATEWAY_AUTH_TOKENS`. The gateway isolates the downstream clients from actual provider API keys so billing limits can be effectively segmented from application usage. API keys are encrypted at rest with `AES-256-GCM`, uniquely salted.
