// Package web holds request-side HTTP helpers: JSON binding with validation,
// pagination parsing, and typed URL/query parameter extraction. It mirrors the
// FE's PaginationRequestDto and the conventions its services use.
package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// Bind decodes the JSON request body into dst and runs struct validation.
// Unknown JSON fields are ignored (the FE sends full model objects that carry
// more keys than a given request struct declares — e.g. id/createdAt on create).
// Returns a human-readable error suitable for a 400 response.
func Bind(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("empty request body")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 8<<20)) // 8 MiB guard
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("empty request body")
		}
		return errors.New("invalid request body: " + err.Error())
	}
	return validateStruct(dst)
}

func validateStruct(dst any) error {
	if err := validate.Struct(dst); err != nil {
		var ve validator.ValidationErrors
		if errors.As(err, &ve) && len(ve) > 0 {
			f := ve[0]
			return errors.New("validation failed on field '" + f.Field() + "' (" + f.Tag() + ")")
		}
		return err
	}
	return nil
}

// Page is the parsed pagination/filter request (matches FE PaginationRequestDto).
type Page struct {
	PageIndex     int      `json:"pageIndex"`
	PageSize      int      `json:"pageSize"`
	Filter        *string  `json:"filter"`
	SortColumn    *string  `json:"sortColumn"`
	SortDirection *string  `json:"sortDirection"`
	FromDate      *string  `json:"fromDate"`
	ToDate        *string  `json:"toDate"`
	MinPrice      *float64 `json:"minPrice"`
	MaxPrice      *float64 `json:"maxPrice"`
}

// Limit returns the SQL LIMIT (page size, clamped to a sane range).
func (p Page) Limit() int32 {
	size := p.PageSize
	if size <= 0 {
		size = 20
	}
	if size > 500 {
		size = 500
	}
	return int32(size)
}

// Offset returns the SQL OFFSET derived from the 0-based page index.
func (p Page) Offset() int32 {
	idx := p.PageIndex
	if idx < 0 {
		idx = 0
	}
	return int32(idx) * p.Limit()
}

// FilterValue returns the trimmed text filter, or "" if absent.
func (p Page) FilterValue() string {
	if p.Filter == nil {
		return ""
	}
	return strings.TrimSpace(*p.Filter)
}

// Sort returns a validated (column, direction) pair. `column` is resolved
// against the allowed map (FE-facing name -> SQL column); if the requested
// column is not allowed, fallbackCol/ASC is returned.
func (p Page) Sort(allowed map[string]string, fallbackCol string) (col, dir string) {
	col = fallbackCol
	if p.SortColumn != nil {
		if mapped, ok := allowed[strings.ToLower(strings.TrimSpace(*p.SortColumn))]; ok {
			col = mapped
		}
	}
	dir = "ASC"
	if p.SortDirection != nil && strings.EqualFold(strings.TrimSpace(*p.SortDirection), "desc") {
		dir = "DESC"
	}
	return col, dir
}

// PageFromQuery builds a Page from query-string params (the GET-list convention).
func PageFromQuery(r *http.Request) Page {
	q := r.URL.Query()
	p := Page{
		PageIndex: atoiDefault(q.Get("pageIndex"), 0),
		PageSize:  atoiDefault(q.Get("pageSize"), 20),
	}
	if v := q.Get("filter"); v != "" {
		p.Filter = &v
	}
	if v := q.Get("sortColumn"); v != "" {
		p.SortColumn = &v
	}
	if v := q.Get("sortDirection"); v != "" {
		p.SortDirection = &v
	}
	if v := q.Get("fromDate"); v != "" {
		p.FromDate = &v
	}
	if v := q.Get("toDate"); v != "" {
		p.ToDate = &v
	}
	return p
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// ─── URL/query param helpers ────────────────────────────────────────────────

// ParamInt64 parses a chi URL parameter as int64.
func ParamInt64(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

// ParamUUID parses a chi URL parameter as a UUID.
func ParamUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(s))
}
