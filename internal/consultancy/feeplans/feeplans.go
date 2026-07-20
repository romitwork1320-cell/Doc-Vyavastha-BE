// Package feeplans implements /api/StudentFeePlans (tenant-scoped CRUD).
//
// A student fee plan is the agreed fee for a student (optionally tagged with a
// fee type). The list/get/by-student queries LEFT JOIN fee_types to surface the
// fee type's name; create/update return only the base row (no join), so we map
// those without a fee type name. Collected/pending/status figures are computed
// by the FE and are intentionally not derived here.
package feeplans

import (
	"github.com/thinkparq/edconsultancy-be/internal/middleware"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/conv"
	"github.com/thinkparq/edconsultancy-be/internal/db/tenant"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
	"github.com/thinkparq/edconsultancy-be/internal/web"
)

// Handler serves the StudentFeePlans endpoints.
type Handler struct {
	tm     *tenancy.Manager
	logger *slog.Logger
	mw     *middleware.Auth
}

// New builds the handler.
func New(tm *tenancy.Manager, logger *slog.Logger, mw *middleware.Auth) *Handler {
	return &Handler{tm: tm, logger: logger, mw: mw}
}

// Mount registers routes.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/StudentFeePlans", func(r chi.Router) {
		r.With(h.mw.RequirePermission("/fee-plans", "CanView")).Get("/", h.list)
		r.With(h.mw.RequirePermission("/fee-plans", "CanAdd")).Post("/", h.create)
		r.Get("/student/{studentId}", h.byStudent)
		r.With(h.mw.RequirePermission("/fee-plans", "CanView")).Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/fee-plans", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/fee-plans", "CanDelete")).Delete("/{id}", h.delete)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID             string  `json:"id"`
	StudentID      string  `json:"studentId"`
	FeeTypeID      string  `json:"feeTypeId"`
	FeeName        string  `json:"feeName"`
	TotalAmount    float64 `json:"totalAmount"`
	DiscountAmount float64 `json:"discountAmount"`
	DiscountReason string  `json:"discountReason"`
	Remarks        string  `json:"remarks"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
	FeeTypeName    string  `json:"feeTypeName"`
	CreatedByName  string  `json:"createdByName"`
}

type request struct {
	StudentID      string  `json:"studentId" validate:"required"`
	FeeTypeID      string  `json:"feeTypeId"` // optional; empty -> NULL
	FeeName        string  `json:"feeName"`
	TotalAmount    float64 `json:"totalAmount"`
	DiscountAmount float64 `json:"discountAmount"`
	DiscountReason string  `json:"discountReason"`
	Remarks        string  `json:"remarks"`
}

// fields holds the subset of columns shared by every fee-plan row so the JOIN
// Row structs and the base StudentFeePlan model can flow through one mapper.
type fields struct {
	ID             uuid.UUID
	StudentID      uuid.UUID
	FeeTypeID      *uuid.UUID
	FeeName        *string
	TotalAmount    pgtype.Numeric
	DiscountAmount pgtype.Numeric
	DiscountReason *string
	Remarks        *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	FeeTypeName    *string // nil for create/update (no join)
	CreatedByName  string
}

func toDTO(f fields) dto {
	return dto{
		ID:             f.ID.String(),
		StudentID:      f.StudentID.String(),
		FeeTypeID:      uuidStr(f.FeeTypeID),
		FeeName:        conv.Str(f.FeeName),
		TotalAmount:    numFloat(f.TotalAmount),
		DiscountAmount: numFloat(f.DiscountAmount),
		DiscountReason: conv.Str(f.DiscountReason),
		Remarks:        conv.Str(f.Remarks),
		CreatedAt:      conv.FmtDateTime(f.CreatedAt),
		UpdatedAt:      conv.FmtDateTime(f.UpdatedAt),
		FeeTypeName:    conv.Str(f.FeeTypeName),
		CreatedByName:  f.CreatedByName,
	}
}

// rowDTO maps a JOIN row (carries fee_type_name).
func rowDTO(id, studentID uuid.UUID, feeTypeID *uuid.UUID, feeName *string, total, discount pgtype.Numeric, reason, remarks *string, created, updated time.Time, feeTypeName *string, createdByName string) dto {
	return toDTO(fields{
		ID: id, StudentID: studentID, FeeTypeID: feeTypeID, FeeName: feeName,
		TotalAmount: total, DiscountAmount: discount, DiscountReason: reason,
		Remarks: remarks, CreatedAt: created, UpdatedAt: updated, FeeTypeName: feeTypeName,
		CreatedByName: createdByName,
	})
}


// ── Handlers ────────────────────────────────_________________________________

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := web.PageFromQuery(r)
	var items []dto
	var total int64
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)

	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.ListStudentFeePlans(ctx, tenant.ListStudentFeePlansParams{BranchID: branchUUID, Filter: page.FilterValue(), Lim: page.Limit(), Off: page.Offset()})
		if err != nil {
			return err
		}
		total, err = q.CountStudentFeePlans(ctx, tenant.CountStudentFeePlansParams{BranchID: branchUUID, Filter: page.FilterValue()})
		if err != nil {
			return err
		}
		items = make([]dto, len(rows))
		for i, p := range rows {
			items[i] = rowDTO(p.ID, p.StudentID, p.FeeTypeID, p.FeeName, p.TotalAmount, p.DiscountAmount, p.DiscountReason, p.Remarks, p.CreatedAt, p.UpdatedAt, p.FeeTypeName, p.CreatedByName)
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
		p, err := q.GetStudentFeePlan(ctx, id)
		if err != nil {
			return err
		}
		out = rowDTO(p.ID, p.StudentID, p.FeeTypeID, p.FeeName, p.TotalAmount, p.DiscountAmount, p.DiscountReason, p.Remarks, p.CreatedAt, p.UpdatedAt, p.FeeTypeName, p.CreatedByName)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Fee plan not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "")
}

func (h *Handler) byStudent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	studentID, err := web.ParamUUID(chi.URLParam(r, "studentId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid studentId")
		return
	}
	var items []dto
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)

	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.GetFeePlansByStudent(ctx, tenant.GetFeePlansByStudentParams{StudentID: studentID, BranchID: branchUUID})
		if err != nil {
			return err
		}
		items = make([]dto, len(rows))
		for i, p := range rows {
			items[i] = rowDTO(p.ID, p.StudentID, p.FeeTypeID, p.FeeName, p.TotalAmount, p.DiscountAmount, p.DiscountReason, p.Remarks, p.CreatedAt, p.UpdatedAt, p.FeeTypeName, p.CreatedByName)
		}
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, items, "")
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := reqctx.MustUserID(ctx)
	var req request
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	studentID, err := uuid.Parse(req.StudentID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid studentId")
		return
	}
	feeTypeID, err := optUUID(req.FeeTypeID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid feeTypeId")
		return
	}
	var out dto
	
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)
	
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		p, err := q.CreateStudentFeePlan(ctx, tenant.CreateStudentFeePlanParams{
			StudentID:      studentID,
			BranchID:       branchUUID,
			FeeTypeID:      feeTypeID,
			FeeName:        conv.PtrStr(req.FeeName),
			TotalAmount:    numFromFloat(req.TotalAmount),
			DiscountAmount: numFromFloat(req.DiscountAmount),
			DiscountReason: conv.PtrStr(req.DiscountReason),
			Remarks:        conv.PtrStr(req.Remarks),
			CreatedBy:      &userID,
		})
		if err != nil {
			return err
		}
		row, err := q.GetStudentFeePlan(ctx, p.ID)
		if err != nil {
			return err
		}
		out = rowDTO(row.ID, row.StudentID, row.FeeTypeID, row.FeeName, row.TotalAmount, row.DiscountAmount, row.DiscountReason, row.Remarks, row.CreatedAt, row.UpdatedAt, row.FeeTypeName, row.CreatedByName)
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.Created(w, out, "Fee plan created")
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
	feeTypeID, err := optUUID(req.FeeTypeID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid feeTypeId")
		return
	}
	var out dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		p, err := q.UpdateStudentFeePlan(ctx, tenant.UpdateStudentFeePlanParams{
			FeeTypeID:      feeTypeID,
			FeeName:        conv.PtrStr(req.FeeName),
			TotalAmount:    numFromFloat(req.TotalAmount),
			DiscountAmount: numFromFloat(req.DiscountAmount),
			DiscountReason: conv.PtrStr(req.DiscountReason),
			Remarks:        conv.PtrStr(req.Remarks),
			ID:             id,
		})
		if err != nil {
			return err
		}
		row, err := q.GetStudentFeePlan(ctx, p.ID)
		if err != nil {
			return err
		}
		out = rowDTO(row.ID, row.StudentID, row.FeeTypeID, row.FeeName, row.TotalAmount, row.DiscountAmount, row.DiscountReason, row.Remarks, row.CreatedAt, row.UpdatedAt, row.FeeTypeName, row.CreatedByName)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Fee plan not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "Fee plan updated")
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
		rows, derr = q.DeleteStudentFeePlan(ctx, id)
		return derr
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	if rows == 0 {
		apiresp.NotFound(w, "Fee plan not found")
		return
	}
	apiresp.OK(w, true, "Fee plan deleted")
}

// ── helpers ──────────────────────────────────────────────────────────────────

// uuidStr renders a nullable uuid as its string, or "" when nil.
func uuidStr(p *uuid.UUID) string {
	if p == nil {
		return ""
	}
	return p.String()
}

// optUUID parses an optional uuid string: empty -> (nil, nil).
func optUUID(s string) (*uuid.UUID, error) {
	if s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// numFloat converts a pgtype.Numeric to a plain float64 (0 when invalid/NULL).
func numFloat(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}

// numFromFloat builds a pgtype.Numeric from a float64 for INSERT/UPDATE params.
func numFromFloat(v float64) pgtype.Numeric {
	var n pgtype.Numeric
	// Scan from the decimal string representation so the scale is preserved.
	_ = n.Scan(strconv.FormatFloat(v, 'f', -1, 64))
	return n
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("StudentFeePlans handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

var _ = context.Background
