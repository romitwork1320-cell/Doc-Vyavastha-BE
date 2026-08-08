// Package token issues and validates the JWTs the Angular FE consumes.
//
// Claim contract (jwt-decode reads these exact keys/casing):
//   - "UserId"   : string  (PascalCase) — FE parseInt()s it
//   - "TenantId" : string  (PascalCase) — selected tenant; omitted on temp token
//   - "role"     : string  (lowercase)
//   - "permissions": PagePermission[] (lowercase) — fallback only; the FE's
//     source of truth is GET /Users/{id}/permissions
//   - "exp"      : number
package token

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ScopeSelectTenant marks a short-lived token issued before tenant selection.
const ScopeSelectTenant = "select_tenant"

// PagePermission mirrors the FE's PagePermission shape (PascalCase keys).
type PagePermission struct {
	PageUrl   string `json:"PageUrl"`
	CanView   bool   `json:"CanView"`
	CanAdd    bool   `json:"CanAdd"`
	CanEdit   bool   `json:"CanEdit"`
	CanDelete bool   `json:"CanDelete"`
}

// Claims is the JWT payload.
type Claims struct {
	UserID      string           `json:"UserId"`
	TenantID    string           `json:"TenantId,omitempty"`
	Role        string           `json:"role,omitempty"`
	Permissions []PagePermission `json:"permissions,omitempty"`
	Scope       string           `json:"scope,omitempty"`
	jwt.RegisteredClaims
}

// Issuer signs and verifies access/temp tokens.
type Issuer struct {
	secret    []byte
	issuer    string
	accessTTL time.Duration
	tempTTL   time.Duration
}

// NewIssuer builds an Issuer.
func NewIssuer(secret, issuer string, accessTTL, tempTTL time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), issuer: issuer, accessTTL: accessTTL, tempTTL: tempTTL}
}

// IssueAccess mints a full access token for a user within a selected tenant.
func (i *Issuer) IssueAccess(userID, tenantID int64, role string, perms []PagePermission) (string, error) {
	now := time.Now()
	c := Claims{
		UserID:      strconv.FormatInt(userID, 10),
		TenantID:    strconv.FormatInt(tenantID, 10),
		Role:        role,
		Permissions: perms,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.accessTTL)),
		},
	}
	return i.sign(c)
}

// IssueClientAccess mints a full access token for a client user (no tenant).
func (i *Issuer) IssueClientAccess(userID int64) (string, error) {
	now := time.Now()
	c := Claims{
		UserID:   strconv.FormatInt(userID, 10),
		TenantID: "0",
		Role:     "Client",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.accessTTL)),
		},
	}
	return i.sign(c)
}

// IssueSuperAdminAccess mints an access token for Super Admins.
func (i *Issuer) IssueSuperAdminAccess(userID int64) (string, error) {
	now := time.Now()
	c := Claims{
		UserID:   strconv.FormatInt(userID, 10),
		TenantID: "0",
		Role:     "SuperAdmin",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.accessTTL)),
		},
	}
	return i.sign(c)
}

// IssueTemp mints a short-lived token used only to call /Auth/select-tenant.
func (i *Issuer) IssueTemp(userID int64) (string, error) {
	now := time.Now()
	c := Claims{
		UserID: strconv.FormatInt(userID, 10),
		Scope:  ScopeSelectTenant,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.tempTTL)),
		},
	}
	return i.sign(c)
}

func (i *Issuer) sign(c Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(i.secret)
}

// Parse validates the signature + expiry and returns the claims.
func (i *Issuer) Parse(tokenStr string) (*Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(tokenStr, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return i.secret, nil
	}, jwt.WithIssuer(i.issuer))
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UserIDInt returns the integer user id from the claims.
func (c *Claims) UserIDInt() int64 {
	n, _ := strconv.ParseInt(c.UserID, 10, 64)
	return n
}

// TenantIDInt returns the integer tenant id (0 if absent).
func (c *Claims) TenantIDInt() int64 {
	if c.TenantID == "" {
		return 0
	}
	n, _ := strconv.ParseInt(c.TenantID, 10, 64)
	return n
}
