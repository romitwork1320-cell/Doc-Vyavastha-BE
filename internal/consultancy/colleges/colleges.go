// Package colleges implements /api/Colleges (tenant-scoped CRUD).
package colleges

import (
	"github.com/thinkparq/edconsultancy-be/internal/middleware"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/conv"
	"github.com/thinkparq/edconsultancy-be/internal/db/tenant"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
	"github.com/thinkparq/edconsultancy-be/internal/web"
)

type Handler struct {
	tm     *tenancy.Manager
	logger *slog.Logger
	mw     *middleware.Auth
}

func New(tm *tenancy.Manager, logger *slog.Logger, mw *middleware.Auth) *Handler {
	return &Handler{tm: tm, logger: logger, mw: mw}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/Colleges", func(r chi.Router) {
		r.Get("/", h.list)
		r.With(h.mw.RequirePermission("/colleges", "CanAdd")).Post("/", h.create)
		r.Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/colleges", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/colleges", "CanDelete")).Delete("/{id}", h.delete)
	})
}

type dto struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type request struct {
	Name string `json:"name" validate:"required"`
}

func toDTO(c tenant.College) dto {
	return dto{
		ID:        c.ID.String(),
		Name:      c.Name,
		CreatedAt: conv.FmtDateTime(c.CreatedAt.Time),
		UpdatedAt: conv.FmtDateTime(c.UpdatedAt.Time),
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var items []dto
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.ListColleges(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			items = append(items, toDTO(row))
		}
		return nil
	})
	if err != nil {
		h.logger.Error("list colleges failed", "error", err)
		apiresp.ServerError(w, "Failed to list colleges")
		return
	}
	apiresp.List(w, items, int64(len(items)), "")
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid college ID")
		return
	}
	var res dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		row, err := q.GetCollege(ctx, id)
		if err != nil {
			return err
		}
		res = toDTO(row)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "College not found")
		return
	}
	if err != nil {
		h.logger.Error("get college failed", "id", id, "error", err)
		apiresp.ServerError(w, "Failed to fetch college")
		return
	}
	apiresp.OK(w, res, "")
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req request
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	tid := reqctx.TenantID(ctx)
	userID := reqctx.MustUserID(ctx)
	var res dto
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		row, err := q.CreateCollege(ctx, tenant.CreateCollegeParams{
			TenantID:  tid,
			Name:      req.Name,
			CreatedBy: &userID,
			UpdatedBy: &userID,
		})
		if err != nil {
			return err
		}
		res = toDTO(row)
		return nil
	})
	if isUniqueViolation(err, "uq_tenant_college_name") {
		apiresp.Conflict(w, "A college with this name already exists")
		return
	}
	if err != nil {
		h.logger.Error("create college failed", "error", err)
		apiresp.ServerError(w, "Failed to create college")
		return
	}
	apiresp.Created(w, res, "")
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid college ID")
		return
	}
	var req request
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	userID := reqctx.MustUserID(ctx)
	var res dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		row, err := q.UpdateCollege(ctx, tenant.UpdateCollegeParams{
			ID:        id,
			Name:      req.Name,
			UpdatedBy: &userID,
		})
		if err != nil {
			return err
		}
		res = toDTO(row)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "College not found")
		return
	}
	if isUniqueViolation(err, "uq_tenant_college_name") {
		apiresp.Conflict(w, "A college with this name already exists")
		return
	}
	if err != nil {
		h.logger.Error("update college failed", "id", id, "error", err)
		apiresp.ServerError(w, "Failed to update college")
		return
	}
	apiresp.OK(w, res, "")
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid college ID")
		return
	}
	uid := reqctx.MustUserID(ctx)
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		_, derr := q.SoftDeleteCollege(ctx, tenant.SoftDeleteCollegeParams{
			ID:        id,
			UpdatedBy: &uid,
		})
		return derr
	})
	if err != nil {
		h.logger.Error("delete college failed", "id", id, "error", err)
		apiresp.ServerError(w, "Failed to delete college")
		return
	}
	apiresp.OK(w, nil, "")
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if constraint == "" || pgErr.ConstraintName == constraint {
			return true
		}
	}
	return false
}
