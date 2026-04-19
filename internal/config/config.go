// Package config loads gateway configuration from environment variables.
// It uses kelseyhightower/envconfig as the parsing backend and provides
// sensible defaults suitable for local development with SQLite.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all runtime configuration for the gateway.
// Values are sourced from environment variables with the prefix GATEWAY_.
type Config struct {
	// Server settings.
	Port         int           `envconfig:"PORT"         default:"8080"`
	ReadTimeout  time.Duration `envconfig:"READ_TIMEOUT"  default:"30s"`
	WriteTimeout time.Duration `envconfig:"WRITE_TIMEOUT" default:"120s"`

	// Database settings.
	DBDriver string `envconfig:"DB_DRIVER" default:"sqlite"`
	DBDSN    string `envconfig:"DB_DSN"    default:"gateway.db"`

	// Security.
	// MasterKey must be a 64-character hex string (32 bytes decoded) supplied
	// via the GATEWAY_MASTER_KEY environment variable. It is never persisted.
	MasterKey string `envconfig:"MASTER_KEY" required:"true"`

	// AuthTokens is a comma-separated list of "token:user_id" pairs used
	// for simple bearer-token authentication. Leave empty to disable auth.
	AuthTokens string `envconfig:"AUTH_TOKENS" default:""`

	// Logging.
	// LogFormat is either "json" (production) or "text" (development).
	LogFormat string `envconfig:"LOG_FORMAT" default:"text"`
	LogLevel  string `envconfig:"LOG_LEVEL"  default:"info"`

	// Routing defaults.
	DefaultProvider string `envconfig:"DEFAULT_PROVIDER" default:""`
	DefaultModel    string `envconfig:"DEFAULT_MODEL"    default:""`

	// CORS.
	CORSOrigins string `envconfig:"CORS_ORIGINS" default:"*"`

	// OpenRouter attribution headers (optional).
	OpenRouterReferer string `envconfig:"OPENROUTER_REFERER" default:"http://localhost"`
	OpenRouterTitle   string `envconfig:"OPENROUTER_TITLE"   default:"go-ya AI Gateway"`

	// Provider model cache TTL.
	ModelCacheTTL time.Duration `envconfig:"MODEL_CACHE_TTL" default:"5m"`

	// Max retries on rate-limit before failing over.
	MaxRetries int `envconfig:"MAX_RETRIES" default:"3"`
}

// Load reads configuration from environment variables prefixed with GATEWAY_.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("GATEWAY", &cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &cfg, nil
}

// AuthTokenMap parses AuthTokens into a map[token]userID.
// Each entry must be in "token:userID" format; malformed entries are skipped.
func (c *Config) AuthTokenMap() map[string]string {
	m := make(map[string]string)
	if c.AuthTokens == "" {
		return m
	}
	for _, pair := range strings.Split(c.AuthTokens, ",") {
		pair = strings.TrimSpace(pair)
		idx := strings.Index(pair, ":")
		if idx < 1 {
			continue
		}
		token := pair[:idx]
		userID := pair[idx+1:]
		if token != "" && userID != "" {
			m[token] = userID
		}
	}
	return m
}

// CORSOriginList returns CORSOrigins as a slice of strings.
func (c *Config) CORSOriginList() []string {
	return strings.Split(c.CORSOrigins, ",")
}

// MasterKeyBytes decodes MasterKey from hex into 32 raw bytes.
func (c *Config) MasterKeyBytes() ([]byte, error) {
	key, err := decodeHex(c.MasterKey)
	if err != nil {
		return nil, fmt.Errorf("config: GATEWAY_MASTER_KEY is not valid hex: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("config: GATEWAY_MASTER_KEY must be exactly 32 bytes (64 hex chars), got %d", len(key))
	}
	return key, nil
}

// decodeHex decodes a hexadecimal string into bytes.
func decodeHex(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd-length hex string")
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi, ok1 := fromHexChar(s[i])
		lo, ok2 := fromHexChar(s[i+1])
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("invalid hex character at position %d", i)
		}
		b[i/2] = (hi << 4) | lo
	}
	return b, nil
}

func fromHexChar(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
