package domain

import "errors"

// Sentinel errors for domain-level failure conditions.
// These allow callers to use errors.Is for type-safe error handling
// without importing concrete error types from provider packages.
var (
	// ErrProviderNotFound is returned when no provider matches the routing request.
	ErrProviderNotFound = errors.New("provider not found")

	// ErrKeyNotFound is returned when no active API key exists for a provider.
	ErrKeyNotFound = errors.New("no active key found for provider")

	// ErrRateLimited is returned when an upstream provider signals HTTP 429.
	ErrRateLimited = errors.New("provider rate limited")

	// ErrInvalidKey is returned when the upstream provider rejects the API key (HTTP 401).
	ErrInvalidKey = errors.New("invalid provider API key")

	// ErrProviderOverloaded is returned when the upstream provider is temporarily unavailable (HTTP 503).
	ErrProviderOverloaded = errors.New("provider overloaded")

	// ErrSessionNotFound is returned when a session ID does not exist in the database.
	ErrSessionNotFound = errors.New("session not found")

	// ErrUpstreamError is returned for generic non-classified upstream HTTP errors.
	ErrUpstreamError = errors.New("upstream provider error")
)
