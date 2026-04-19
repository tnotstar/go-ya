package providers

import (
	"net/http"

	"github.com/tnotstar/go-ya/internal/domain"
)

// AbacusProvider adapts the Abacus.AI API.
//
// Abacus.AI exposes an OpenAI-compatible endpoint. Authentication uses the
// "apiKey" header (not "Authorization: Bearer") by default, though this is
// configurable via the authHeaderName / authHeaderFmt parameters.
//
// If Abacus.AI changes the auth scheme, update the header name stored in the
// providers.auth_header_name DB column and re-create the adapter via the registry.
type AbacusProvider struct {
	*openAILikeProvider
}

// NewAbacus returns an Abacus.AI adapter.
// authHeaderName defaults to "apiKey" if empty; authHeaderFmt defaults to "%s".
func NewAbacus(baseURL, apiKey, authHeaderName, authHeaderFmt string, client *http.Client) *AbacusProvider {
	if authHeaderName == "" {
		authHeaderName = "apiKey"
	}
	if authHeaderFmt == "" {
		authHeaderFmt = "%s"
	}
	return &AbacusProvider{
		openAILikeProvider: newOpenAILikeProvider(
			baseURL,
			authHeaderName, authHeaderFmt,
			nil,
			client,
			domain.ProviderTypeAbacus,
			apiKey,
		),
	}
}
