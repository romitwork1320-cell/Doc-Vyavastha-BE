// Package codesequence implements /api/StudentCodeSequences (tenant-scoped CRUD).
//
// Follows the canonical tenant-schema pattern: bind/validate -> run inside
// tenancy.InTenantTx (which sets search_path) -> map the sqlc row to the FE DTO
// -> render with the ApiResponse envelope. In addition to standard CRUD, it
// exposes a lookup by year-config id.
package codesequence

import (
	"github.com/thinkparq/edconsultancy-be/internal/middleware"
	"context"
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

// Handler serves the StudentCodeSequences endpoints.
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
	r.Route("/StudentCodeSequences", func(r chi.Router) {
		r.Get("/", h.list)
		r.With(h.mw.RequirePermission("/codesequence", "CanAdd")).Post("/", h.create)
		r.Get("/year-config/{yearConfigId}", h.getByYearConfig)
		r.Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/codesequence", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/codesequence", "CanDelete")).Delete("/{id}", h.delete)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID                string `json:"id"`
	YearConfigID      string `json:"yearConfigId"`
	CurrentNumber     int64  `json:"currentNumber"`
	LastGeneratedCode string `json:"lastGeneratedCode"`
	UpdatedAt         string `json:"updatedAt"`
}

type request struct {
	YearConfigID      string `json:"yearConfigId" validate:"required,uuid"`
	CurrentNumber     int64  `json:"currentNumber"`
	LastGeneratedCode string `json:"lastGeneratedCode"`
}

func toDTO(c tenant.StudentCodeSequence) dto {
	return dto{
		ID:                c.ID.String(),
		YearConfigID:      c.YearConfigID.String(),
		CurrentNumber:     c.CurrentNumber,
		LastGeneratedCode: conv.Str(c.LastGeneratedCode),
		UpdatedAt:         conv.FmtDateTime(c.UpdatedAt),
	}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := web.PageFromQuery(r)
	var items []dto
	var total int64
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.ListStudentCodeSequences(ctx, tenant.ListStudentCodeSequencesParams{
			Lim: page.Limit(), Off: page.Offset(),
		})
		if err != nil {
			return err
		}
		total, err = q.CountStudentCodeSequences(ctx)
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
		c, err := q.GetStudentCodeSequence(ctx, id)
		if err != nil {
			return err
		}
		out = toDTO(c)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Code sequence not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "")
}

func (h *Handler) getByYearConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	yearConfigID, err := web.ParamUUID(chi.URLParam(r, "yearConfigId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid yearConfigId")
		return
	}
	var out dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		c, err := q.GetSequenceByYearConfig(ctx, yearConfigID)
		if err != nil {
			return err
		}
		out = toDTO(c)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Code sequence not found")
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
	yearConfigID, err := web.ParamUUID(req.YearConfigID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid yearConfigId")
		return
	}
	var out dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		c, err := q.CreateStudentCodeSequence(ctx, tenant.CreateStudentCodeSequenceParams{
			YearConfigID:      yearConfigID,
			CurrentNumber:     req.CurrentNumber,
			LastGeneratedCode: conv.PtrStr(req.LastGeneratedCode),
		})
		if err != nil {
			return err
		}
		out = toDTO(c)
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.Created(w, out, "Code sequence created")
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
		c, err := q.UpdateStudentCodeSequence(ctx, tenant.UpdateStudentCodeSequenceParams{
			CurrentNumber:     req.CurrentNumber,
			LastGeneratedCode: conv.PtrStr(req.LastGeneratedCode),
			ID:                id,
		})
		if err != nil {
			return err
		}
		out = toDTO(c)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Code sequence not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "Code sequence updated")
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
		rows, derr = q.DeleteStudentCodeSequence(ctx, id)
		return derr
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	if rows == 0 {
		apiresp.NotFound(w, "Code sequence not found")
		return
	}
	apiresp.OK(w, true, "Code sequence deleted")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("StudentCodeSequences handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

// ensure context import is used even if a future refactor drops it
var _ = context.Background
