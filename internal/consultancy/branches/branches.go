// Package branches implements /api/Branches (tenant-scoped CRUD).
package branches

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
)

// Handler serves the Branches endpoints.
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
	r.Route("/Branches", func(r chi.Router) {
		r.With(h.mw.RequirePermission("/branches", "CanView")).Get("/", h.list)
		r.With(h.mw.RequirePermission("/branches", "CanAdd")).Post("/", h.create)
		r.With(h.mw.RequirePermission("/branches", "CanView")).Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/branches", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/branches", "CanDelete")).Delete("/{id}", h.delete)
		// User assignment
		r.Post("/{id}/users/{userId}", h.assignUser)
		r.Delete("/{id}/users/{userId}", h.removeUser)
	})
	r.Route("/UserBranches", func(r chi.Router) {
		r.Get("/user/{userId}", h.userBranches)
		r.Get("/me", h.myBranches)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	Address   string `json:"address"`
	Contact   string `json:"contact"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type request struct {
	Name    string `json:"name" validate:"required"`
	Code    string `json:"code"`
	Address string `json:"address"`
	Contact string `json:"contact"`
	Status  string `json:"status"`
}

func toDTO(b tenant.Branch) dto {
	return dto{
		ID:        b.ID.String(),
		Name:      b.Name,
		Code:      conv.Str(b.Code),
		Address:   conv.Str(b.Address),
		Contact:   conv.Str(b.Contact),
		Status:    b.Status,
		CreatedAt: conv.FmtDateTime(b.CreatedAt),
		UpdatedAt: conv.FmtDateTime(b.UpdatedAt),
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
	var items []dto
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.ListBranches(ctx)
		if err != nil {
			return err
		}
		items = make([]dto, len(rows))
		for i, b := range rows {
			items[i] = toDTO(b)
		}
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.List(w, items, int64(len(items)), "")
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
		b, err := q.GetBranch(ctx, id)
		if err != nil {
			return err
		}
		out = toDTO(b)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Branch not found")
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
		b, err := q.CreateBranch(ctx, tenant.CreateBranchParams{
			Name:    req.Name,
			Code:    conv.PtrStr(req.Code),
			Address: conv.PtrStr(req.Address),
			Contact: conv.PtrStr(req.Contact),
			Status:  statusOrDefault(req.Status),
		})
		if err != nil {
			return err
		}
		out = toDTO(b)
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.Created(w, out, "Branch created")
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
		b, err := q.UpdateBranch(ctx, tenant.UpdateBranchParams{
			ID:      id,
			Name:    req.Name,
			Code:    conv.PtrStr(req.Code),
			Address: conv.PtrStr(req.Address),
			Contact: conv.PtrStr(req.Contact),
			Status:  statusOrDefault(req.Status),
		})
		if err != nil {
			return err
		}
		out = toDTO(b)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Branch not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "Branch updated")
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		return q.DeleteBranch(ctx, id)
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			apiresp.Conflict(w, "Cannot delete branch because it has assigned students or users.")
			return
		}
		h.fail(w, err)
		return
	}
	apiresp.OK(w, true, "Branch deleted")
}

func (h *Handler) assignUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	branchID, err := web.ParamUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid branch id")
		return
	}
	userID, err := web.ParamInt64(chi.URLParam(r, "userId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid user id")
		return
	}
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		return q.AssignUserToBranch(ctx, tenant.AssignUserToBranchParams{
			BranchID: branchID,
			UserID:   userID,
		})
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, true, "User assigned to branch")
}

func (h *Handler) removeUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	branchID, err := web.ParamUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid branch id")
		return
	}
	userID, err := web.ParamInt64(chi.URLParam(r, "userId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid user id")
		return
	}
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		return q.RemoveUserFromBranch(ctx, tenant.RemoveUserFromBranchParams{
			BranchID: branchID,
			UserID:   userID,
		})
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, true, "User removed from branch")
}

func (h *Handler) userBranches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := web.ParamInt64(chi.URLParam(r, "userId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid user id")
		return
	}
	var items []dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.GetUserBranches(ctx, userID)
		if err != nil {
			return err
		}
		items = make([]dto, len(rows))
		for i, b := range rows {
			items[i] = toDTO(b)
		}
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.List(w, items, int64(len(items)), "")
}

func (h *Handler) myBranches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := reqctx.MustUserID(ctx)
	
	var items []dto
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.GetUserBranches(ctx, userID)
		if err != nil {
			return err
		}
		items = make([]dto, len(rows))
		for i, b := range rows {
			items[i] = toDTO(b)
		}
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.List(w, items, int64(len(items)), "")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("Branches handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

// ensure context import is used
var _ = context.Background
