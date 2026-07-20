// Package apptypes implements /api/ApplicationTypes (tenant-scoped CRUD).
//
// Mirrors the canonical categories module: bind/validate -> run inside
// tenancy.InTenantTx (which sets search_path) -> map the sqlc row to the FE
// DTO -> render with the ApiResponse envelope. List endpoints return data +
// totalCount/totalRecords via apiresp.List.
package apptypes

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

// Handler serves the ApplicationTypes endpoints.
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
	r.Route("/ApplicationTypes", func(r chi.Router) {
		r.Get("/", h.list)
		r.With(h.mw.RequirePermission("/application-types", "CanAdd")).Post("/", h.create)
		r.Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/application-types", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/application-types", "CanDelete")).Delete("/{id}", h.delete)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	InUse       bool   `json:"inUse"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type request struct {
	Name        string `json:"name" validate:"required,min=2,max=100"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func toDTO(a tenant.ApplicationType, inUse bool) dto {
	return dto{
		ID:          a.ID.String(),
		Name:        a.Name,
		Description: conv.Str(a.Description),
		Status:      a.Status,
		InUse:       inUse,
		CreatedAt:   conv.FmtDateTime(a.CreatedAt),
		UpdatedAt:   conv.FmtDateTime(a.UpdatedAt),
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
		rows, err := q.ListApplicationTypes(ctx, tenant.ListApplicationTypesParams{
			Filter: page.FilterValue(), Lim: page.Limit(), Off: page.Offset(),
		})
		if err != nil {
			return err
		}
		total, err = q.CountApplicationTypes(ctx, page.FilterValue())
		if err != nil {
			return err
		}
		items = make([]dto, 0, len(rows))
		for _, a := range rows {
			inUse, _ := q.CheckApplicationTypeInUse(ctx, a.ID)
			items = append(items, toDTO(a, inUse))
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
		c, err := q.GetApplicationType(ctx, id)
		if err != nil {
			return err
		}
		inUse, _ := q.CheckApplicationTypeInUse(ctx, id)
		out = toDTO(c, inUse)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Application type not found")
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
		apiresp.BadRequest(w, "Application Type Name is required.")
		return
	}

	var out dto
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		c, err := q.CreateApplicationType(ctx, tenant.CreateApplicationTypeParams{
			Name:        req.Name,
			Description: conv.PtrStr(req.Description),
			Status:      statusOrDefault(req.Status),
		})
		if err != nil {
			return err
		}
		out = toDTO(c, false)
		return nil
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_app_type_name" {
			apiresp.Conflict(w, fmt.Sprintf("Application Type '%s' already exists.", req.Name))
			return
		}
		h.fail(w, err)
		return
	}
	apiresp.Created(w, out, "Application type created")
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
		apiresp.BadRequest(w, "Application Type Name is required.")
		return
	}

	var out dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		existing, err := q.GetApplicationType(ctx, id)
		if err != nil {
			return err
		}
		
		inUse, err := q.CheckApplicationTypeInUse(ctx, id)
		if err != nil {
			return err
		}
		
		if inUse && existing.Name != req.Name {
			return errors.New("in_use_violation")
		}

		c, err := q.UpdateApplicationType(ctx, tenant.UpdateApplicationTypeParams{
			Name:        req.Name,
			Description: conv.PtrStr(req.Description),
			Status:      statusOrDefault(req.Status),
			ID:          id,
		})
		if err != nil {
			return err
		}
		out = toDTO(c, inUse)
		return nil
	})
	if err != nil {
		if err.Error() == "in_use_violation" {
			apiresp.Conflict(w, "This application type is already in use and its name cannot be modified.")
			return
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_app_type_name" {
			apiresp.Conflict(w, fmt.Sprintf("Application Type '%s' already exists.", req.Name))
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			apiresp.NotFound(w, "Application type not found")
			return
		}
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "Application type updated")
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
		rows, derr = q.DeleteApplicationType(ctx, id)
		return derr
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	if rows == 0 {
		apiresp.NotFound(w, "Application type not found")
		return
	}
	apiresp.OK(w, true, "Application type deleted")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("ApplicationTypes handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

// ensure context import is used even if a future refactor drops it
var _ = context.Background
