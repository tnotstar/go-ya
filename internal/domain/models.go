package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ─── GORM Models ─────────────────────────────────────────────────────────────

// Provider represents an upstream LLM provider registration in the database.
type Provider struct {
	ID             string       `gorm:"type:text;primaryKey"          json:"id"`
	Name           string       `gorm:"type:text;uniqueIndex;not null" json:"name"`
	Type           ProviderType `gorm:"type:text;index;not null"       json:"type"`
	BaseURL        string       `gorm:"type:text;not null"             json:"base_url"`
	Enabled        bool         `gorm:"index;default:true"             json:"enabled"`
	IsLocal        bool         `gorm:"default:false"                  json:"is_local"`
	AuthHeaderName string       `gorm:"type:text"                      json:"auth_header_name"`
	Metadata       JSONMap      `gorm:"type:text"                      json:"metadata,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`

	// Has-many association (not loaded by default).
	Keys []ProviderKey `gorm:"foreignKey:ProviderID" json:"-"`
}

// BeforeCreate assigns a UUID primary key if none is set.
func (p *Provider) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	return nil
}

// ProviderKey stores an encrypted API key for a provider.
// The master key used for encryption is never persisted here.
type ProviderKey struct {
	ID           string    `gorm:"type:text;primaryKey"              json:"id"`
	ProviderID   string    `gorm:"type:text;index;not null"          json:"provider_id"`
	Label        string    `gorm:"type:text;not null"                json:"label"`
	EncryptedKey []byte    `gorm:"type:blob;not null"                json:"-"` // never expose
	KeyHash      string    `gorm:"type:text;uniqueIndex;not null"    json:"key_hash"`
	Enabled      bool      `gorm:"index;default:true"                json:"enabled"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	Provider Provider `gorm:"foreignKey:ProviderID" json:"-"`
}

// BeforeCreate assigns a UUID primary key if none is set.
func (k *ProviderKey) BeforeCreate(tx *gorm.DB) error {
	if k.ID == "" {
		k.ID = uuid.NewString()
	}
	return nil
}

// Session groups a sequence of messages from a single conversation context.
type Session struct {
	ID           string     `gorm:"type:text;primaryKey"       json:"id"`
	UserID       string     `gorm:"type:text;index;not null"   json:"user_id"`
	ClientType   ClientType `gorm:"type:text;index;not null"   json:"client_type"`
	LastProvider string     `gorm:"type:text"                  json:"last_provider"`
	LastModel    string     `gorm:"type:text"                  json:"last_model"`
	Metadata     JSONMap    `gorm:"type:text"                  json:"metadata,omitempty"`
	CreatedAt    time.Time  `gorm:"index"                      json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	Messages []Message `gorm:"foreignKey:SessionID" json:"-"`
}

// BeforeCreate assigns a UUID primary key if none is set.
func (s *Session) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	return nil
}

// Message stores a single turn within a session.
type Message struct {
	ID         string   `gorm:"type:text;primaryKey"              json:"id"`
	SessionID  string   `gorm:"type:text;index:idx_session_time"  json:"session_id"`
	Role       Role     `gorm:"type:text;not null"                json:"role"`
	Content    string   `gorm:"type:text"                         json:"content"`
	Name       string   `gorm:"type:text"                         json:"name,omitempty"`
	ToolCalls  JSONRaw  `gorm:"type:text"                         json:"tool_calls,omitempty"`
	TokenCount int      `gorm:"default:0"                         json:"token_count"`
	CreatedAt  time.Time `gorm:"index:idx_session_time"            json:"created_at"`

	Session Session `gorm:"foreignKey:SessionID" json:"-"`
}

// BeforeCreate assigns a UUID primary key if none is set.
func (m *Message) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	return nil
}

// ─── Custom GORM Types ────────────────────────────────────────────────────────

// JSONMap is a map[string]any that transparently serializes to/from TEXT in the DB.
type JSONMap map[string]any

// Scan implements the sql.Scanner interface for reading from the database.
func (j *JSONMap) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into JSONMap", value)
	}
	return json.Unmarshal(bytes, j)
}

// Value implements the driver.Valuer interface for writing to the database.
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	b, err := json.Marshal(j)
	return string(b), err
}

// JSONRaw stores raw JSON bytes as TEXT in the database.
type JSONRaw []byte

// Scan implements sql.Scanner for JSONRaw.
func (j *JSONRaw) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*j = append((*j)[:0], v...)
	case string:
		*j = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into JSONRaw", value)
	}
	return nil
}

// Value implements driver.Valuer for JSONRaw.
func (j JSONRaw) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return string(j), nil
}
