// Package ctxkeys defines Echo context keys shared between the server and
// handler packages to avoid import cycles.
package ctxkeys

// Key is the type for all Echo context value keys in this gateway.
type Key string

const (
	// UserID is the context key for the authenticated user identifier.
	UserID Key = "user_id"
	// ClientType is the context key for the detected downstream client type.
	ClientType Key = "client_type"
)
