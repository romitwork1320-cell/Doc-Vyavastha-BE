// Package feetypes implements /api/FeeTypes (tenant-scoped CRUD).
//
// Mirrors the categories reference module: bind/validate -> run inside
// tenancy.InTenantTx (which sets search_path) -> map the sqlc row to the FE
// DTO -> render with the ApiResponse envelope. List endpoints return data +
// totalCount/totalRecords via apiresp.List.
package feetypes

import (
	"github.com/thinkparq/edconsultancy-be/internal/middleware"
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/google/uuid"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/conv"
	"github.com/thinkparq/edconsultancy-be/internal/db/tenant"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
	"github.com/thinkparq/edconsultancy-be/internal/web"
)

// Handler serves the FeeTypes endpoints.
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
	r.Route("/FeeTypes", func(r chi.Router) {
		r.Get("/", h.list)
		r.With(h.mw.RequirePermission("/feetypes", "CanAdd")).Post("/", h.create)
		r.Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/feetypes", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/feetypes", "CanDelete")).Delete("/{id}", h.delete)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Amount      float64  `json:"amount"`
	Status      string   `json:"status"`
	CategoryIDs []string `json:"categoryIds"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

type request struct {
	Name        string   `json:"name" validate:"required"`
	Description string   `json:"description"`
	Amount      float64  `json:"amount" validate:"required,min=0"`
	Status      string   `json:"status"`
	CategoryIDs []string `json:"categoryIds"`
}

func toDTO(c tenant.FeeType) dto {
	amt, _ := c.Amount.Float64Value()
	return dto{
		ID:          c.ID.String(),
		Name:        c.Name,
		Description: conv.Str(c.Description),
		Amount:      amt.Float64,
		Status:      c.Status,
		CategoryIDs: []string{}, // initialized to empty by default
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
	var items []dto
	var total int64
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.ListFeeTypes(ctx, tenant.ListFeeTypesParams{
			Filter: page.FilterValue(), Lim: page.Limit(), Off: page.Offset(),
		})
		if err != nil {
			return err
		}
		total, err = q.CountFeeTypes(ctx, page.FilterValue())
		if err != nil {
			return err
		}
		items = make([]dto, len(rows))
		for i, c := range rows {
			items[i] = toDTO(c)
		}
		
		allCats, err := q.GetAllFeeTypeCategories(ctx)
		if err == nil {
			catMap := make(map[string][]string)
			for _, ac := range allCats {
				fid := ac.FeeTypeID.String()
				catMap[fid] = append(catMap[fid], ac.CategoryID.String())
			}
			for i := range items {
				if cats, ok := catMap[items[i].ID]; ok {
					items[i].CategoryIDs = cats
				}
			}
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
		c, err := q.GetFeeType(ctx, id)
		if err != nil {
			return err
		}
		out = toDTO(c)
		cats, err := q.GetCategoriesForFeeType(ctx, id)
		if err == nil {
			for _, catId := range cats {
				out.CategoryIDs = append(out.CategoryIDs, catId.String())
			}
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Fee type not found")
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
		amt := conv.Float64ToNumeric(req.Amount)
		c, err := q.CreateFeeType(ctx, tenant.CreateFeeTypeParams{
			Name:        req.Name,
			Description: conv.PtrStr(req.Description),
			Amount:      amt,
			Status:      statusOrDefault(req.Status),
		})
		if err != nil {
			return err
		}
		out = toDTO(c)
		
		for _, catIdStr := range req.CategoryIDs {
			catId, err := uuid.Parse(catIdStr)
			if err == nil {
				err = q.AssignCategoryToFeeType(ctx, tenant.AssignCategoryToFeeTypeParams{
					FeeTypeID:  c.ID,
					CategoryID: catId,
				})
				if err != nil {
					var pgErr *pgconn.PgError
					if errors.As(err, &pgErr) && pgErr.Code == "23505" {
						return errors.New("One or more selected categories are already assigned to another fee type.")
					}
					return err
				}
				out.CategoryIDs = append(out.CategoryIDs, catId.String())
			}
		}
		
		return nil
	})
	if err != nil {
		if err.Error() == "One or more selected categories are already assigned to another fee type." {
			apiresp.BadRequest(w, err.Error())
			return
		}
		h.fail(w, err)
		return
	}
	apiresp.Created(w, out, "Fee type created")
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
		amt := conv.Float64ToNumeric(req.Amount)
		c, err := q.UpdateFeeType(ctx, tenant.UpdateFeeTypeParams{
			Name:        req.Name,
			Description: conv.PtrStr(req.Description),
			Amount:      amt,
			Status:      statusOrDefault(req.Status),
			ID:          id,
		})
		if err != nil {
			return err
		}
		out = toDTO(c)

		// Update categories mappings
		_ = q.ClearCategoriesForFeeType(ctx, id)
		for _, catIdStr := range req.CategoryIDs {
			catId, err := uuid.Parse(catIdStr)
			if err == nil {
				err = q.AssignCategoryToFeeType(ctx, tenant.AssignCategoryToFeeTypeParams{
					FeeTypeID:  id,
					CategoryID: catId,
				})
				if err != nil {
					var pgErr *pgconn.PgError
					if errors.As(err, &pgErr) && pgErr.Code == "23505" {
						return errors.New("One or more selected categories are already assigned to another fee type.")
					}
					return err
				}
				out.CategoryIDs = append(out.CategoryIDs, catId.String())
			}
		}

		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Fee type not found")
		return
	}
	if err != nil {
		if err.Error() == "One or more selected categories are already assigned to another fee type." {
			apiresp.BadRequest(w, err.Error())
			return
		}
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "Fee type updated")
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
		rows, derr = q.DeleteFeeType(ctx, id)
		return derr
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	if rows == 0 {
		apiresp.NotFound(w, "Fee type not found")
		return
	}
	apiresp.OK(w, true, "Fee type deleted")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("FeeTypes handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

// ensure context import is used even if a future refactor drops it
var _ = context.Background
