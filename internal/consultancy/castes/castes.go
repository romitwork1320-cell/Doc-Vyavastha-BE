// Package castes implements /api/StudentCastes (tenant-scoped CRUD).
//
// This is the REFERENCE pattern for every tenant-schema CRUD module: bind/
// validate -> run inside tenancy.InTenantTx (which sets search_path) -> map the
// sqlc row to the FE DTO -> render with the ApiResponse envelope. List
// endpoints return data + totalCount/totalRecords via apiresp.List.
package castes

import (
	"github.com/thinkparq/edconsultancy-be/internal/middleware"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/conv"
	"github.com/thinkparq/edconsultancy-be/internal/db/tenant"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
	"github.com/thinkparq/edconsultancy-be/internal/web"
)

// Handler serves the StudentCastes endpoints.
type Handler struct {
	tm     *tenancy.Manager
	logger *slog.Logger
	mw     *middleware.Auth
}

// New builds the handler.
func New(tm *tenancy.Manager, logger *slog.Logger, mw *middleware.Auth) *Handler {
	return &Handler{tm: tm, logger: logger, mw: mw}
}

// Mount registers routes under the tenant-protected router.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/StudentCastes", func(r chi.Router) {
		r.Get("/", h.list)
		r.With(h.mw.RequirePermission("/castes", "CanAdd")).Post("/", h.create)
		r.Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/castes", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/castes", "CanDelete")).Delete("/{id}", h.delete)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type request struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func toDTO(c tenant.StudentCaste) dto {
	return dto{
		ID:          c.ID.String(),
		Name:        c.Name,
		Description: conv.Str(c.Description),
		Status:      c.Status,
		CreatedAt:   conv.FmtDateTime(c.CreatedAt),
		UpdatedAt:   conv.FmtDateTime(c.UpdatedAt),
	}
}

func statusOrDefault(s string) string {
	if s == "" {
		return "Active"
	}
	return s
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := web.PageFromQuery(r)
	filter := page.FilterValue()
	var items []dto
	var total int64
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.ListCastes(ctx, tenant.ListCastesParams{
			Filter: filter,
			Off:    int32(page.Offset()),
			Lim:    int32(page.Limit()),
		})
		if err != nil {
			return err
		}
		total, err = q.CountCastes(ctx, filter)
		if err != nil {
			return err
		}
		items = make([]dto, len(rows))
		for i, c := range rows {
			items[i] = toDTO(c)
		}
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.List(w, items, total, "")
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	var out dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		c, err := q.GetCaste(ctx, id)
		if err != nil {
			return err
		}
		out = toDTO(c)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "caste not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "")
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req request
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	var out dto
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		c, err := q.CreateCaste(ctx, tenant.CreateCasteParams{
			Name:        req.Name,
			Description: conv.PtrStr(req.Description),
			Status:      statusOrDefault(req.Status),
		})
		if err != nil {
			return err
		}
		out = toDTO(c)
		return nil
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_caste_name" {
			apiresp.Conflict(w, fmt.Sprintf("Caste '%s' already exists.", req.Name))
			return
		}
		h.fail(w, err)
		return
	}
	apiresp.Created(w, out, "caste created")
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	var req request
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	var out dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		c, err := q.UpdateCaste(ctx, tenant.UpdateCasteParams{
			Name:        req.Name,
			Description: conv.PtrStr(req.Description),
			Status:      statusOrDefault(req.Status),
			ID:          id,
		})
		if err != nil {
			return err
		}
		out = toDTO(c)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "caste not found")
		return
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_caste_name" {
			apiresp.Conflict(w, fmt.Sprintf("Caste '%s' already exists.", req.Name))
			return
		}
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "caste updated")
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	var rows int64
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		var derr error
		rows, derr = q.DeleteCaste(ctx, id)
		return derr
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			apiresp.Conflict(w, "Cannot delete caste because it is already used by students or other records.")
			return
		}
		h.fail(w, err)
		return
	}
	if rows == 0 {
		apiresp.NotFound(w, "caste not found")
		return
	}
	apiresp.OK(w, true, "caste deleted")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("StudentCastes handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

// ensure context import is used even if a future refactor drops it
var _ = context.Background
