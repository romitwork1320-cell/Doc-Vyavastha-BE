// Package conv holds small value converters used when mapping between FE DTOs
// (camelCase JSON, string dates, omitted-as-empty) and the sqlc DB types
// (pointers for nullable columns, time.Time for dates).
package conv

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// PtrStr returns nil for empty strings, otherwise a pointer to s.
func PtrStr(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

// Str dereferences a *string, returning "" when nil.
func Str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// PtrInt32 returns nil when v is 0, else a pointer (for optional int columns).
func PtrInt32(v int32) *int32 {
	if v == 0 {
		return nil
	}
	return &v
}

// Int32 dereferences a *int32 (0 when nil).
func Int32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

// Int64 dereferences a *int64 (0 when nil).
func Int64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// dateLayouts are tried in order when parsing FE-supplied dates.
var dateLayouts = []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"}

// ParseDate parses a (possibly nil/empty) FE date string into *time.Time.
func ParseDate(p *string) *time.Time {
	if p == nil {
		return nil
	}
	s := strings.TrimSpace(*p)
	if s == "" {
		return nil
	}
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return &t
		}
	}
	return nil
}

// FmtDate formats a *time.Time as YYYY-MM-DD (nil-safe).
func FmtDate(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

// FmtDateTime formats a time.Time as RFC3339 (what Angular's Date pipe expects).
func FmtDateTime(t time.Time) string { return t.Format(time.RFC3339) }

// Float64ToNumeric converts a float64 to pgtype.Numeric.
func Float64ToNumeric(f float64) pgtype.Numeric {
	var num pgtype.Numeric
	_ = num.Scan(fmt.Sprintf("%f", f))
	return num
}

// UUIDStr dereferences a *uuid.UUID into a string, returning "" when nil.
func UUIDStr(u *uuid.UUID) string {
	if u == nil {
		return ""
	}
	return u.String()
}

// UUIDToStrPtr converts a *uuid.UUID to a *string.
func UUIDToStrPtr(u *uuid.UUID) *string {
	if u == nil {
		return nil
	}
	s := u.String()
	return &s
}

// StrToUUID converts a string to a uuid.UUID, ignoring errors.
func StrToUUID(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}
	return uuid.Parse(s)
}

// UUIDPtr returns a pointer to the uuid if it's not nil-valued.
func UUIDPtr(u uuid.UUID) *uuid.UUID {
	if u == uuid.Nil {
		return nil
	}
	return &u
}
