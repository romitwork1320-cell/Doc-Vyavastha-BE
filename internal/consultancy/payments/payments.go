// Package payments implements /api/StudentPayments (tenant-scoped). Payment and
// receipt numbers are generated server-side, atomically inside the tenant
// transaction, from dedicated DB sequences (never derived from COUNT()).
//
// It mirrors the canonical tenant-CRUD pattern: bind/validate -> run inside
// tenancy.InTenantTx (which sets search_path) -> map the sqlc row to the FE DTO
// -> render with the ApiResponse envelope.
package payments

import (
	"github.com/thinkparq/edconsultancy-be/internal/middleware"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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

// Handler serves the StudentPayments endpoints.
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
	r.Route("/StudentPayments", func(r chi.Router) {
		r.With(h.mw.RequirePermission("/payments", "CanView")).Get("/", h.list)
		r.With(h.mw.RequirePermission("/payments", "CanAdd")).Post("/", h.create)
		r.With(h.mw.RequirePermission("/payments", "CanView")).Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/payments", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/payments", "CanDelete")).Delete("/{id}", h.delete)
		r.Get("/student/{studentId}", h.byStudent)
		r.Get("/fee-plan/{feePlanId}", h.byFeePlan)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID               string  `json:"id"`
	PaymentNumber    string  `json:"paymentNumber"`
	ReceiptNumber    string  `json:"receiptNumber"`
	StudentID        string  `json:"studentId"`
	StudentFeePlanID string  `json:"studentFeePlanId"`
	PaymentDate      *string `json:"paymentDate"`
	Amount           float64 `json:"amount"`
	PaymentMethod    string  `json:"paymentMethod"`
	ReferenceNumber  string  `json:"referenceNumber"`
	Remarks          string  `json:"remarks"`
	CreatedAt        string  `json:"createdAt"`
	UpdatedAt        string  `json:"updatedAt"`
	FeeTypeName      string  `json:"feeTypeName"`
	CreatedByName    string  `json:"createdByName"`
}

type request struct {
	StudentID        string  `json:"studentId" validate:"required"`
	StudentFeePlanID string  `json:"studentFeePlanId" validate:"required"`
	PaymentDate      *string `json:"paymentDate"`
	Amount           float64 `json:"amount"`
	PaymentMethod    string  `json:"paymentMethod"`
	ReferenceNumber  string  `json:"referenceNumber"`
	Remarks          string  `json:"remarks"`
}

// The list/get/by-student/by-plan queries all return the same shape (a JOIN Row
// carrying fee_type_name); a single mapper keyed on those fields serves them all.
func toDTO(
	id uuid.UUID,
	paymentNumber string,
	receiptNumber *string,
	studentID, studentFeePlanID uuid.UUID,
	paymentDate time.Time,
	amount pgtype.Numeric,
	paymentMethod, referenceNumber, remarks *string,
	createdAt, updatedAt time.Time,
	feeTypeName *string,
	createdByName string,
) dto {
	return dto{
		ID:               id.String(),
		PaymentNumber:    paymentNumber,
		ReceiptNumber:    conv.Str(receiptNumber),
		StudentID:        studentID.String(),
		StudentFeePlanID: studentFeePlanID.String(),
		PaymentDate:      conv.FmtDate(&paymentDate),
		Amount:           numericToFloat(amount),
		PaymentMethod:    conv.Str(paymentMethod),
		ReferenceNumber:  conv.Str(referenceNumber),
		Remarks:          conv.Str(remarks),
		CreatedAt:        conv.FmtDateTime(createdAt),
		UpdatedAt:        conv.FmtDateTime(updatedAt),
		FeeTypeName:      conv.Str(feeTypeName),
		CreatedByName:    createdByName,
	}
}

func listRowToDTO(p tenant.ListStudentPaymentsRow) dto {
	return toDTO(p.ID, p.PaymentNumber, p.ReceiptNumber, p.StudentID, p.StudentFeePlanID,
		p.PaymentDate, p.Amount, p.PaymentMethod, p.ReferenceNumber, p.Remarks,
		p.CreatedAt, p.UpdatedAt, p.FeeTypeName, p.CreatedByName)
}

func getRowToDTO(p tenant.GetStudentPaymentRow) dto {
	return toDTO(p.ID, p.PaymentNumber, p.ReceiptNumber, p.StudentID, p.StudentFeePlanID,
		p.PaymentDate, p.Amount, p.PaymentMethod, p.ReferenceNumber, p.Remarks,
		p.CreatedAt, p.UpdatedAt, p.FeeTypeName, p.CreatedByName)
}

func byStudentRowToDTO(p tenant.GetPaymentsByStudentRow) dto {
	return toDTO(p.ID, p.PaymentNumber, p.ReceiptNumber, p.StudentID, p.StudentFeePlanID,
		p.PaymentDate, p.Amount, p.PaymentMethod, p.ReferenceNumber, p.Remarks,
		p.CreatedAt, p.UpdatedAt, p.FeeTypeName, p.CreatedByName)
}

func byFeePlanRowToDTO(p tenant.GetPaymentsByFeePlanRow) dto {
	return toDTO(p.ID, p.PaymentNumber, p.ReceiptNumber, p.StudentID, p.StudentFeePlanID,
		p.PaymentDate, p.Amount, p.PaymentMethod, p.ReferenceNumber, p.Remarks,
		p.CreatedAt, p.UpdatedAt, p.FeeTypeName, p.CreatedByName)
}



// ── Handlers ─────────────────────────────────────────────────────────────────

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := web.PageFromQuery(r)
	var items []dto
	var total int64
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)

	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.ListStudentPayments(ctx, tenant.ListStudentPaymentsParams{BranchID: branchUUID, Filter: page.FilterValue(), Lim: page.Limit(), Off: page.Offset()})
		if err != nil {
			return err
		}
		total, err = q.CountStudentPayments(ctx, tenant.CountStudentPaymentsParams{BranchID: branchUUID, Filter: page.FilterValue()})
		if err != nil {
			return err
		}
		items = make([]dto, len(rows))
		for i, p := range rows {
			items[i] = listRowToDTO(p)
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
		p, err := q.GetStudentPayment(ctx, id)
		if err != nil {
			return err
		}
		out = getRowToDTO(p)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Payment not found")
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
	feePlanID, err := uuid.Parse(req.StudentFeePlanID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid studentFeePlanId")
		return
	}

	// PaymentDate is a DATE column; default to today when the client omits it.
	paymentDate := time.Now()
	if pd := conv.ParseDate(req.PaymentDate); pd != nil {
		paymentDate = *pd
	}

	var out dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		// Generate the human-facing identifiers atomically from DB sequences.
		n, err := q.NextPaymentSeq(ctx)
		if err != nil {
			return err
		}
		paymentNumber := fmt.Sprintf("PAY%04d", n)
		rn, err := q.NextReceiptSeq(ctx)
		if err != nil {
			return err
		}
		receiptNumber := fmt.Sprintf("RCPT%05d", rn)

		amount, err := floatToNumeric(req.Amount)
		if err != nil {
			return err
		}
		
		branchIDStr := reqctx.BranchID(ctx)
		branchUUID, _ := uuid.Parse(branchIDStr)

		p, err := q.CreateStudentPayment(ctx, tenant.CreateStudentPaymentParams{
			PaymentNumber:    paymentNumber,
			ReceiptNumber:    &receiptNumber,
			StudentID:        studentID,
			BranchID:         branchUUID,
			StudentFeePlanID: feePlanID,
			PaymentDate:      paymentDate,
			Amount:           amount,
			PaymentMethod:    conv.PtrStr(req.PaymentMethod),
			ReferenceNumber:  conv.PtrStr(req.ReferenceNumber),
			Remarks:          conv.PtrStr(req.Remarks),
			CreatedBy:        &userID,
		})
		if err != nil {
			return err
		}
		row, err := q.GetStudentPayment(ctx, p.ID)
		if err != nil {
			return err
		}
		out = getRowToDTO(row)
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.Created(w, out, "Payment created")
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

	paymentDate := time.Now()
	if pd := conv.ParseDate(req.PaymentDate); pd != nil {
		paymentDate = *pd
	}

	var out dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		amount, err := floatToNumeric(req.Amount)
		if err != nil {
			return err
		}
		p, err := q.UpdateStudentPayment(ctx, tenant.UpdateStudentPaymentParams{
			PaymentDate:     paymentDate,
			Amount:          amount,
			PaymentMethod:   conv.PtrStr(req.PaymentMethod),
			ReferenceNumber: conv.PtrStr(req.ReferenceNumber),
			Remarks:         conv.PtrStr(req.Remarks),
			ID:              id,
		})
		if err != nil {
			return err
		}
		row, err := q.GetStudentPayment(ctx, p.ID)
		if err != nil {
			return err
		}
		out = getRowToDTO(row)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Payment not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "Payment updated")
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
		rows, derr = q.DeleteStudentPayment(ctx, id)
		return derr
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	if rows == 0 {
		apiresp.NotFound(w, "Payment not found")
		return
	}
	apiresp.OK(w, true, "Payment deleted")
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
		rows, err := q.GetPaymentsByStudent(ctx, tenant.GetPaymentsByStudentParams{StudentID: studentID, BranchID: branchUUID})
		if err != nil {
			return err
		}
		items = make([]dto, len(rows))
		for i, p := range rows {
			items[i] = byStudentRowToDTO(p)
		}
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.List(w, items, int64(len(items)), "")
}

func (h *Handler) byFeePlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	feePlanID, err := web.ParamUUID(chi.URLParam(r, "feePlanId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid feePlanId")
		return
	}
	var items []dto
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)

	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.GetPaymentsByFeePlan(ctx, tenant.GetPaymentsByFeePlanParams{StudentFeePlanID: feePlanID, BranchID: branchUUID})
		if err != nil {
			return err
		}
		items = make([]dto, len(rows))
		for i, p := range rows {
			items[i] = byFeePlanRowToDTO(p)
		}
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.List(w, items, int64(len(items)), "")
}

// ── Numeric helpers (DB NUMERIC <-> plain JSON number) ───────────────────────

// numericToFloat renders a pgtype.Numeric as a float64 (0 when NULL/invalid).
func numericToFloat(n pgtype.Numeric) float64 {
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}

// floatToNumeric converts a JSON float64 into a pgtype.Numeric for INSERT/UPDATE.
func floatToNumeric(v float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(fmt.Sprintf("%v", v)); err != nil {
		return pgtype.Numeric{}, err
	}
	return n, nil
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("StudentPayments handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

var _ = context.Background
