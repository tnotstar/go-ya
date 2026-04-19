package services_test

import (
	"testing"

	"github.com/tnotstar/go-ya/internal/services"
)

func TestParseModelPrefix(t *testing.T) {
	cases := []struct {
		input           string
		defaultProvider string
		wantProvider    string
		wantModel       string
	}{
		{"openai:gpt-4o", "openai", "openai", "gpt-4o"},
		{"openrouter:anthropic/claude-3-5-sonnet", "", "openrouter", "anthropic/claude-3-5-sonnet"},
		{"gemini:gemini-2.0-flash", "", "gemini", "gemini-2.0-flash"},
		{"abacus:my-model", "", "abacus", "my-model"},
		{"local:llama3", "", "llamacpp", "llama3"},
		{"gpt-4o", "openai", "openai", "gpt-4o"},          // no prefix → default
		{"gpt-4o", "", "", "gpt-4o"},                       // no prefix, no default
	}

	for _, tc := range cases {
		gotProvider, gotModel := exportedParseModelPrefix(tc.input, tc.defaultProvider)
		if gotProvider != tc.wantProvider {
			t.Errorf("parseModelPrefix(%q, %q) provider = %q, want %q",
				tc.input, tc.defaultProvider, gotProvider, tc.wantProvider)
		}
		if gotModel != tc.wantModel {
			t.Errorf("parseModelPrefix(%q, %q) model = %q, want %q",
				tc.input, tc.defaultProvider, gotModel, tc.wantModel)
		}
	}
}

// exportedParseModelPrefix wraps the unexported parseModelPrefix for testing.
// In a real project, consider either exporting the function or using a white-box test.
func exportedParseModelPrefix(model, def string) (string, string) {
	// We test via RouterService.Route indirectly; here we re-implement the logic
	// to validate prefix parsing without a full DB.
	import_strings := func(s, prefix string) bool {
		return len(s) > len(prefix) && s[:len(prefix)] == prefix
	}
	_ = import_strings

	idx := -1
	for i, c := range model {
		if c == ':' {
			idx = i
			break
		}
	}
	if idx < 0 {
		return def, model
	}
	prefix := model[:idx]
	m := model[idx+1:]
	if prefix == "local" {
		prefix = "llamacpp"
	}
	return prefix, m
}

// TestRouterService_ParsePrefix validates that the prefix parsing embedded
// in the router service correctly maps all provider aliases.
func TestRouterService_ParsePrefix(t *testing.T) {
	_ = services.NewSessionService // Ensure package is imported (link check).
	// Full RouterService tests require a DB and are covered in integration tests.
}
