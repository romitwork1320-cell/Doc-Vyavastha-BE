// Package appstatuses implements /api/ApplicationStatuses (tenant-scoped CRUD).
//
// Follows the canonical tenant CRUD pattern (see internal/consultancy/
// categories): bind/validate -> run inside tenancy.InTenantTx (which sets
// search_path) -> map the sqlc row to the FE DTO -> render with the
// ApiResponse envelope. List endpoints return data + totalCount/totalRecords
// via apiresp.List.
package appstatuses

import (
	"github.com/thinkparq/edconsultancy-be/internal/middleware"
	"context"
	"errors"
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
	"fmt"
	"strings"
)

// Handler serves the ApplicationStatuses endpoints.
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
	r.Route("/ApplicationStatuses", func(r chi.Router) {
		r.Get("/", h.list)
		r.With(h.mw.RequirePermission("/application-statuses", "CanAdd")).Post("/", h.create)
		r.Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/application-statuses", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/application-statuses", "CanDelete")).Delete("/{id}", h.delete)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	ColorCode    string  `json:"colorCode"`
	Status       string  `json:"status"`
	DisplayOrder float64 `json:"displayOrder"`
	InUse        bool    `json:"inUse"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
}

type request struct {
	Name         string `json:"name" validate:"required,min=2,max=100"`
	Description  string `json:"description"`
	ColorCode    string `json:"colorCode" validate:"required,hexcolor"`
	Status       string `json:"status"`
	DisplayOrder int32  `json:"displayOrder" validate:"required,min=1"`
}

func toDTO(s tenant.ApplicationStatus, inUse bool) dto {
	return dto{
		ID:           s.ID.String(),
		Name:         s.Name,
		Description:  conv.Str(s.Description),
		ColorCode:    conv.Str(s.ColorCode),
		Status:       s.Status,
		DisplayOrder: float64(s.DisplayOrder),
		InUse:        inUse,
		CreatedAt:    conv.FmtDateTime(s.CreatedAt),
		UpdatedAt:    conv.FmtDateTime(s.UpdatedAt),
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
	var items []dto
	var total int64
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.ListApplicationStatuses(ctx, tenant.ListApplicationStatusesParams{
			Filter: page.FilterValue(), Lim: page.Limit(), Off: page.Offset(),
		})
		if err != nil {
			return err
		}
		total, err = q.CountApplicationStatuses(ctx, page.FilterValue())
		if err != nil {
			return err
		}
		items = make([]dto, 0, len(rows))
		for _, c := range rows {
			inUse, _ := q.CheckApplicationStatusInUse(ctx, c.ID)
			items = append(items, toDTO(c, inUse))
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
		c, err := q.GetApplicationStatus(ctx, id)
		if err != nil {
			return err
		}
		inUse, _ := q.CheckApplicationStatusInUse(ctx, id)
		out = toDTO(c, inUse)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Application status not found")
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
	
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		apiresp.BadRequest(w, "Application Status Name is required.")
		return
	}

	var out dto
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		c, err := q.CreateApplicationStatus(ctx, tenant.CreateApplicationStatusParams{
			Name:         req.Name,
			Description:  conv.PtrStr(req.Description),
			ColorCode:    conv.PtrStr(req.ColorCode),
			Status:       statusOrDefault(req.Status),
			DisplayOrder: req.DisplayOrder,
		})
		if err != nil {
			return err
		}
		out = toDTO(c, false)
		return nil
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_app_status_name" {
			apiresp.Conflict(w, fmt.Sprintf("Application Status '%s' already exists.", req.Name))
			return
		}
		h.fail(w, err)
		return
	}
	apiresp.Created(w, out, "Application status created")
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
	
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		apiresp.BadRequest(w, "Application Status Name is required.")
		return
	}

	var out dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		existing, err := q.GetApplicationStatus(ctx, id)
		if err != nil {
			return err
		}
		inUse, err := q.CheckApplicationStatusInUse(ctx, id)
		if err != nil {
			return err
		}
		if inUse && existing.Name != req.Name {
			return errors.New("in_use_violation")
		}

		c, err := q.UpdateApplicationStatus(ctx, tenant.UpdateApplicationStatusParams{
			Name:         req.Name,
			Description:  conv.PtrStr(req.Description),
			ColorCode:    conv.PtrStr(req.ColorCode),
			Status:       statusOrDefault(req.Status),
			DisplayOrder: req.DisplayOrder,
			ID:           id,
		})
		if err != nil {
			return err
		}
		out = toDTO(c, inUse)
		return nil
	})
	if err != nil {
		if err.Error() == "in_use_violation" {
			apiresp.Conflict(w, "This application status is already in use and its name cannot be modified.")
			return
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_app_status_name" {
			apiresp.Conflict(w, fmt.Sprintf("Application Status '%s' already exists.", req.Name))
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			apiresp.NotFound(w, "Application status not found")
			return
		}
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "Application status updated")
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		inUse, err := q.CheckApplicationStatusInUse(ctx, id)
		if err != nil {
			return err
		}
		if inUse {
			return errors.New("in_use_violation")
		}
		
		rows, err := q.DeleteApplicationStatus(ctx, id)
		if err != nil {
			return err
		}
		if rows == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
	if err != nil {
		if err.Error() == "in_use_violation" {
			apiresp.Conflict(w, "Cannot delete application status because it is already used by existing applications.")
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			apiresp.NotFound(w, "Application status not found")
			return
		}
		h.fail(w, err)
		return
	}
	apiresp.OK(w, true, "Application status deleted")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("ApplicationStatuses handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

// ensure context import is used even if a future refactor drops it
var _ = context.Background
