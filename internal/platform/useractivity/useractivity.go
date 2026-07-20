// Package useractivity implements /api/UserActivity (platform-scoped audit log).
//
// It exposes a read-only feed of recent user-activity audit records. The
// user_activities catalog lives in the public schema, so this module talks
// directly to public.Queries. The single endpoint returns a paginated,
// most-recent-first list via the apiresp.List envelope.
package useractivity

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/conv"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/web"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/google/uuid"
)

// Handler serves the UserActivity endpoints.
type Handler struct {
	pool   *pgxpool.Pool
	q      *public.Queries
	logger *slog.Logger
}

// New builds the handler from the shared pgx pool.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	return &Handler{pool: pool, q: public.New(pool), logger: logger}
}

// Mount registers routes under the platform router.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/UserActivity", func(r chi.Router) {
		r.Get("/recent", h.recent)
		r.Get("/entity/{id}", h.entity)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID          int64  `json:"id"`
	UserID      string `json:"userId"`
	UserName    string `json:"userName"`
	URL         string `json:"url"`
	Method      string `json:"method"`
	IPAddress   string `json:"ipAddress"`
	CreatedOn   string `json:"createdOn"`
	Description string `json:"description"`
	Changes     string `json:"changes"`
}

func toDTO(a public.UserActivity) dto {
	return dto{
		ID:          a.ID,
		UserID:      a.UserID,
		UserName:    conv.Str(a.UserName),
		URL:         a.Url,
		Method:      a.Method,
		IPAddress:   conv.Str(a.IpAddress),
		CreatedOn:   conv.FmtDateTime(a.CreatedOn),
		Description: conv.Str(a.Description),
		Changes:     conv.Str(a.Changes),
	}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (h *Handler) recent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := web.PageFromQuery(r)

	tID := reqctx.TenantID(ctx)
	var tenantID *int64
	if tID > 0 {
		tenantID = &tID
	}

	bID := r.Header.Get("X-Branch-ID")
	if bID == "" {
		bID = reqctx.BranchID(ctx)
	}

	var branchID *uuid.UUID
	if bID != "" {
		if parsed, err := uuid.Parse(bID); err == nil {
			branchID = &parsed
		}
	}

	rows, err := h.q.ListRecentUserActivities(ctx, public.ListRecentUserActivitiesParams{
		TenantID: tenantID,
		BranchID: branchID,
		Off: page.Offset(),
		Lim: page.Limit(),
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	total, err := h.q.CountUserActivities(ctx, public.CountUserActivitiesParams{
		TenantID: tenantID,
		BranchID: branchID,
	})
	if err != nil {
		h.fail(w, err)
		return
	}

	items := make([]dto, len(rows))
	for i, a := range rows {
		items[i] = toDTO(a)
	}
	apiresp.List(w, items, total, "")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("UserActivity handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

func (h *Handler) entity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if id == "" {
		apiresp.BadRequest(w, "missing entity id")
		return
	}
	page := web.PageFromQuery(r)

	pattern := "%" + id + "%"
	
	query := `
		SELECT id, user_id, user_name, tenant_id, url, method, ip_address, description, changes, created_on 
		FROM user_activities
		WHERE (url LIKE $1 OR changes LIKE $1) AND tenant_id = $4 AND branch_id = $5
		ORDER BY created_on DESC
		LIMIT $2 OFFSET $3
	`
	
	tID := reqctx.TenantID(ctx)
	var tenantID *int64
	if tID > 0 {
		tenantID = &tID
	}

	bID := r.Header.Get("X-Branch-ID")
	if bID == "" {
		bID = reqctx.BranchID(ctx)
	}

	var branchID *uuid.UUID
	if bID != "" {
		if parsed, err := uuid.Parse(bID); err == nil {
			branchID = &parsed
		}
	}

	rows, err := h.pool.Query(ctx, query, pattern, page.Limit(), page.Offset(), tenantID, branchID)
	if err != nil {
		h.fail(w, err)
		return
	}
	defer rows.Close()

	var items []dto
	for rows.Next() {
		var a public.UserActivity
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.UserName, &a.TenantID, &a.Url, &a.Method, 
			&a.IpAddress, &a.Description, &a.Changes, &a.CreatedOn,
		); err != nil {
			h.fail(w, err)
			return
		}
		items = append(items, toDTO(a))
	}
	if rows.Err() != nil {
		h.fail(w, rows.Err())
		return
	}

	var total int64
	countQuery := `
		SELECT count(*)
		FROM user_activities
		WHERE url LIKE $1 OR changes LIKE $1
	`
	if err := h.pool.QueryRow(ctx, countQuery, pattern).Scan(&total); err != nil {
		h.fail(w, err)
		return
	}

	if items == nil {
		items = []dto{}
	}
	apiresp.List(w, items, total, "")
}
