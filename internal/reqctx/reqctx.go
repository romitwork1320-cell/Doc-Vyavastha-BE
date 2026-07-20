// Package reqctx carries the authenticated identity through the request
// context. It is dependency-free so any module can read the identity without
// importing the middleware (avoids import cycles).
package reqctx

import "context"

type ctxKey int

const identityKey ctxKey = iota

// Identity is the resolved caller for an authenticated request.
type Identity struct {
	UserID   int64
	TenantID int64 // 0 when no tenant is selected yet (temp token)
	BranchID string // "" when no branch is selected yet
	Role          string
	Schema        string // resolved tenant schema (empty when TenantID == 0)
	Scope         string // "" for a full access token, "select_tenant" for a temp token
	RequestURL    string
	RequestMethod string
	IPAddress     string
}

// With returns a child context carrying the identity.
func With(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// Get returns the identity and whether one was present.
func Get(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey).(Identity)
	return id, ok
}

// MustUserID returns the caller's user id (0 if unauthenticated).
func MustUserID(ctx context.Context) int64 {
	id, _ := Get(ctx)
	return id.UserID
}

// TenantID returns the caller's selected tenant id (0 if none).
func TenantID(ctx context.Context) int64 {
	id, _ := Get(ctx)
	return id.TenantID
}

// Schema returns the caller's resolved tenant schema ("" if none).
func Schema(ctx context.Context) string {
	id, _ := Get(ctx)
	return id.Schema
}

// BranchID returns the caller's active branch ID ("" if none).
func BranchID(ctx context.Context) string {
	id, _ := Get(ctx)
	return id.BranchID
}

// RequestURL returns the request URL path.
func RequestURL(ctx context.Context) string {
	id, _ := Get(ctx)
	return id.RequestURL
}

// RequestMethod returns the HTTP method.
func RequestMethod(ctx context.Context) string {
	id, _ := Get(ctx)
	return id.RequestMethod
}

// IPAddress returns the caller's IP address.
func IPAddress(ctx context.Context) string {
	id, _ := Get(ctx)
	return id.IPAddress
}
