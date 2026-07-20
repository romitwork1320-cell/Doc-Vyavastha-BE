package formtypes

import (
	"github.com/thinkparq/edconsultancy-be/internal/middleware"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

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
	r.Route("/FormTypes", func(r chi.Router) {
		r.Get("/", h.list)
		r.With(h.mw.RequirePermission("/formtypes", "CanAdd")).Post("/", h.create)
		r.Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/formtypes", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/formtypes", "CanDelete")).Delete("/{id}", h.delete)
	})
}

type dto struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	DisplayOrder int32  `json:"displayOrder"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type request struct {
	Name         string `json:"name" validate:"required"`
	Description  string `json:"description"`
	Status       string `json:"status" validate:"required"`
	DisplayOrder int32  `json:"displayOrder"`
}

func toDTO(a tenant.FormType) dto {
	return dto{
		ID:           a.ID.String(),
		Name:         a.Name,
		Description:  conv.Str(a.Description),
		Status:       a.Status,
		DisplayOrder: a.DisplayOrder,
		CreatedAt:    a.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    a.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := web.PageFromQuery(r)
	filter := page.FilterValue()
	limit := page.Limit()
	offset := page.Offset()

	var items []tenant.FormType
	var count int64
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		var err error
		items, err = q.ListFormTypes(ctx, tenant.ListFormTypesParams{
			Filter: filter,
			Lim:    limit,
			Off:    offset,
		})
		if err != nil {
			return err
		}
		count, err = q.CountFormTypes(ctx, filter)
		return err
	})
	if err != nil {
		h.fail(w, err)
		return
	}

	out := make([]dto, len(items))
	for i, item := range items {
		out[i] = toDTO(item)
	}

	apiresp.List(w, out, count, "Form types fetched")
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
		item, derr := q.CreateFormType(ctx, tenant.CreateFormTypeParams{
			Name:         req.Name,
			Description:  conv.PtrStr(req.Description),
			Status:       req.Status,
			DisplayOrder: req.DisplayOrder,
		})
		if derr != nil {
			return derr
		}
		out = toDTO(item)
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.Created(w, out, "Form type created")
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
		item, derr := q.GetFormType(ctx, id)
		if derr != nil {
			return derr
		}
		out = toDTO(item)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Form type not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "Form type fetched")
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
		item, derr := q.UpdateFormType(ctx, tenant.UpdateFormTypeParams{
			ID:           id,
			Name:         req.Name,
			Description:  conv.PtrStr(req.Description),
			Status:       req.Status,
			DisplayOrder: req.DisplayOrder,
		})
		if derr != nil {
			return derr
		}
		out = toDTO(item)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Form type not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "Form type updated")
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}

	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		return q.DeleteFormType(ctx, id)
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, true, "Form type deleted")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("FormTypes handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}
