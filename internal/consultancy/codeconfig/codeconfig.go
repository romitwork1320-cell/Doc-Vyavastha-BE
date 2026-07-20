// Package codeconfig implements /api/StudentCodeConfigurations (tenant-scoped CRUD).
//
// It follows the canonical tenant-schema pattern (see the categories package):
// bind/validate -> run inside tenancy.InTenantTx (which sets search_path) ->
// map the sqlc row to the FE DTO -> render with the ApiResponse envelope.
//
// In addition to standard CRUD, it exposes GET /category/{categoryId} which
// returns the single currently-active code config for a category.
package codeconfig

import (
	"github.com/thinkparq/edconsultancy-be/internal/middleware"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

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

// Handler serves the StudentCodeConfigurations endpoints.
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
	r.Route("/StudentCodeConfigurations", func(r chi.Router) {
		r.Get("/", h.list)
		r.With(h.mw.RequirePermission("/codeconfig", "CanAdd")).Post("/", h.create)
		r.Get("/category/{categoryId}", h.getActiveByCategory)
		r.Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/codeconfig", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/codeconfig", "CanDelete")).Delete("/{id}", h.delete)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID            string  `json:"id"`
	CategoryID    string  `json:"categoryId"`
	BusinessYear  float64 `json:"businessYear"`
	Prefix        string  `json:"prefix"`
	Separator     string  `json:"separator"`
	PaddingLength float64 `json:"paddingLength"`
	ResetSequence bool    `json:"resetSequence"`
	IsActive      bool    `json:"isActive"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

type request struct {
	CategoryID    string `json:"categoryId" validate:"required,uuid"`
	BusinessYear  int32  `json:"businessYear" validate:"required,min=2000,max=2100"`
	Prefix        string `json:"prefix" validate:"required,min=1,max=10,alphanum"`
	Separator     string `json:"separator" validate:"max=5"`
	PaddingLength int32  `json:"paddingLength" validate:"min=1,max=10"`
	ResetSequence bool   `json:"resetSequence"`
	IsActive      bool   `json:"isActive"`
}

func toDTO(c tenant.StudentCodeYearConfig) dto {
	return dto{
		ID:            c.ID.String(),
		CategoryID:    c.CategoryID.String(),
		BusinessYear:  float64(c.BusinessYear),
		Prefix:        c.Prefix,
		Separator:     c.Separator,
		PaddingLength: float64(c.PaddingLength),
		ResetSequence: c.ResetSequence,
		IsActive:      c.IsActive,
		CreatedAt:     conv.FmtDateTime(c.CreatedAt),
		UpdatedAt:     conv.FmtDateTime(c.UpdatedAt),
	}
}

// separatorOrDefault mirrors categories.statusOrDefault: applies the FE default.
func separatorOrDefault(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// paddingOrDefault defaults the zero-value padding length to 4.
func paddingOrDefault(v int32) int32 {
	if v == 0 {
		return 4
	}
	return v
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := web.PageFromQuery(r)
	var items []dto
	var total int64
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.ListStudentCodeConfigs(ctx, tenant.ListStudentCodeConfigsParams{
			Filter: page.FilterValue(), Lim: page.Limit(), Off: page.Offset(),
		})
		if err != nil {
			return err
		}
		total, err = q.CountStudentCodeConfigs(ctx, page.FilterValue())
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
		c, err := q.GetStudentCodeConfig(ctx, id)
		if err != nil {
			return err
		}
		out = toDTO(c)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Code configuration not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "")
}

// getActiveByCategory returns the single active config for a category.
func (h *Handler) getActiveByCategory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	categoryID, err := web.ParamUUID(chi.URLParam(r, "categoryId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid categoryId")
		return
	}
	var out dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		c, err := q.GetActiveConfigByCategory(ctx, categoryID)
		if err != nil {
			return err
		}
		out = toDTO(c)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "No active code configuration for category")
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
	categoryID, err := web.ParamUUID(req.CategoryID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid categoryId")
		return
	}
	req.Prefix = strings.ToUpper(strings.TrimSpace(req.Prefix))

	if req.BusinessYear < int32(time.Now().Year()) {
		apiresp.BadRequest(w, "Business year must be between 2000 and 2100 and cannot be in the past.")
		return
	}

	var out dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		c, err := q.CreateStudentCodeConfig(ctx, tenant.CreateStudentCodeConfigParams{
			CategoryID:    categoryID,
			BusinessYear:  req.BusinessYear,
			Prefix:        req.Prefix,
			Separator:     separatorOrDefault(req.Separator),
			PaddingLength: paddingOrDefault(req.PaddingLength),
			ResetSequence: req.ResetSequence,
			IsActive:      req.IsActive,
		})
		if err != nil {
			return err
		}
		out = toDTO(c)
		return nil
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == "uq_prefix" {
				apiresp.Conflict(w, fmt.Sprintf("Prefix '%s' is already in use and cannot be reused.", req.Prefix))
				return
			}
			if pgErr.ConstraintName == "uq_year_config_cat_active" {
				apiresp.Conflict(w, "An active configuration already exists for this category. Please deactivate the existing configuration first.")
				return
			}
			apiresp.Conflict(w, "A unique constraint violation occurred.")
			return
		}
		h.fail(w, err)
		return
	}
	apiresp.Created(w, out, "Code configuration created")
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
	categoryID, err := web.ParamUUID(req.CategoryID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid categoryId")
		return
	}
	req.Prefix = strings.ToUpper(strings.TrimSpace(req.Prefix))

	if req.BusinessYear < int32(time.Now().Year()) {
		apiresp.BadRequest(w, "Business year must be between 2000 and 2100 and cannot be in the past.")
		return
	}

	var out dto
	var warningMsg string
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		existing, err := q.GetStudentCodeConfig(ctx, id)
		if err != nil {
			return err
		}

		inUse, err := q.CheckYearConfigInUse(ctx, id)
		if err != nil {
			return err
		}

		// Rule 5: If in use, structural fields cannot be modified
		if inUse {
			if existing.Prefix != req.Prefix || existing.CategoryID != categoryID || existing.BusinessYear != req.BusinessYear || existing.Separator != separatorOrDefault(req.Separator) || existing.PaddingLength != paddingOrDefault(req.PaddingLength) {
				return errors.New("in_use_violation")
			}
		}

		// Rule 8 warning message check
		if existing.IsActive && !req.IsActive {
			warningMsg = "Code configuration updated. Warning: No active configuration will remain for this category. New students cannot be created until another configuration is activated."
		}

		c, err := q.UpdateStudentCodeConfig(ctx, tenant.UpdateStudentCodeConfigParams{
			CategoryID:    categoryID,
			BusinessYear:  req.BusinessYear,
			Prefix:        req.Prefix,
			Separator:     separatorOrDefault(req.Separator),
			PaddingLength: paddingOrDefault(req.PaddingLength),
			ResetSequence: req.ResetSequence,
			IsActive:      req.IsActive,
			ID:            id,
		})
		if err != nil {
			return err
		}
		out = toDTO(c)
		return nil
	})
	if err != nil {
		if err.Error() == "in_use_violation" {
			apiresp.Conflict(w, "This configuration is already used by students and cannot be modified.")
			return
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == "uq_prefix" {
				apiresp.Conflict(w, fmt.Sprintf("Prefix '%s' is already in use and cannot be reused.", req.Prefix))
				return
			}
			if pgErr.ConstraintName == "uq_year_config_cat_active" {
				apiresp.Conflict(w, "An active configuration already exists for this category. Please deactivate the existing configuration first.")
				return
			}
			apiresp.Conflict(w, "A unique constraint violation occurred.")
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			apiresp.NotFound(w, "Code configuration not found")
			return
		}
		h.fail(w, err)
		return
	}
	
	msg := "Code configuration updated"
	if warningMsg != "" {
		msg = warningMsg
	}
	apiresp.OK(w, out, msg)
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
		rows, derr = q.DeleteStudentCodeConfig(ctx, id)
		return derr
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	if rows == 0 {
		apiresp.NotFound(w, "Code configuration not found")
		return
	}
	apiresp.OK(w, true, "Code configuration deleted")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("StudentCodeConfigurations handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

// ensure context import is used even if a future refactor drops it
var _ = context.Background
