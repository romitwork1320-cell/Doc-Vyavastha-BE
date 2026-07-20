package feecollections

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"log/slog"
	
	"github.com/go-chi/chi/v5"
	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/conv"
	"github.com/thinkparq/edconsultancy-be/internal/db/tenant"
	"github.com/thinkparq/edconsultancy-be/internal/middleware"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
	"github.com/thinkparq/edconsultancy-be/internal/web"
)

type Handler struct {
	tm *tenancy.Manager
	logger *slog.Logger
	mw *middleware.Auth
}

func New(tm *tenancy.Manager, logger *slog.Logger, mw *middleware.Auth) *Handler {
	return &Handler{tm: tm, logger: logger, mw: mw}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/FormFeeCollections", func(r chi.Router) {
		r.With(h.mw.RequirePermission("/daily-collections", "CanView")).Get("/", h.list)
		r.With(h.mw.RequirePermission("/daily-collections", "CanAdd")).Post("/", h.create)
		r.With(h.mw.RequirePermission("/daily-collections", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/daily-collections", "CanEdit")).Patch("/{id}/status", h.updateStatus)
	})
}

type collectionDto struct {
	ID                         string  `json:"id"`
	ApplicationID              string  `json:"applicationId"`
	ApplicationName            string  `json:"applicationName"`
	StudentID                  string  `json:"studentId"`
	StudentName                string  `json:"studentName"`
	StaffID                    string  `json:"staffId"`
	StaffName                  string  `json:"staffName"`
	CollegeFeeAmount           float64 `json:"collegeFeeAmount"`
	PaymentToCollegeMethod     string  `json:"paymentToCollegeMethod"`
	StudentReimbursementMethod string  `json:"studentReimbursementMethod"`
	StudentTransactionRef      string  `json:"studentTransactionRef"`
	StaffRemarks               string  `json:"staffRemarks"`
	AdminVerificationStatus    string  `json:"adminVerificationStatus"`
	AdminRemarks               string  `json:"adminRemarks"`
	VerifiedBy                 *string `json:"verifiedBy"`
	VerifiedAt                 *string `json:"verifiedAt"`
	CreatedAt                  string  `json:"createdAt"`
}

type createReq struct {
	ApplicationID              string  `json:"applicationId" validate:"required"`
	CollegeFeeAmount           float64 `json:"collegeFeeAmount" validate:"required"`
	PaymentToCollegeMethod     string  `json:"paymentToCollegeMethod" validate:"required"`
	StudentReimbursementMethod string  `json:"studentReimbursementMethod" validate:"required"`
	StudentTransactionRef      string  `json:"studentTransactionRef"`
	StaffRemarks               string  `json:"staffRemarks"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createReq
	if err := web.Bind(r, &req); err != nil {
		h.fail(w, err)
		return
	}

	appID, err := conv.StrToUUID(req.ApplicationID)
	if err != nil {
		h.fail(w, errors.New("invalid application ID"))
		return
	}
	branchIDStr := reqctx.BranchID(ctx)
	branchID, err := conv.StrToUUID(branchIDStr)
	if err != nil {
		h.fail(w, errors.New("invalid branch ID"))
		return
	}
	userID := reqctx.MustUserID(ctx)

	var res tenant.ApplicationFeeCollection
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		var err error
		res, err = q.CreateApplicationFeeCollection(ctx, tenant.CreateApplicationFeeCollectionParams{
			ApplicationID:              appID,
			StaffID:                    userID,
			CollegeFeeAmount:           conv.Float64ToNumeric(req.CollegeFeeAmount),
			PaymentToCollegeMethod:     req.PaymentToCollegeMethod,
			StudentReimbursementMethod: req.StudentReimbursementMethod,
			StudentTransactionRef:      conv.PtrStr(req.StudentTransactionRef),
			StaffRemarks:               conv.PtrStr(req.StaffRemarks),
			AdminVerificationStatus:    "Pending",
			AdminRemarks:               conv.PtrStr(""),
			BranchID:                   branchID,
			CreatedBy:                  &userID,
			UpdatedBy:                  &userID,
		})
		return err
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, toDto(res), "Created successfully")
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamUUID(chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, errors.New("invalid id"))
		return
	}

	var req createReq
	if err := web.Bind(r, &req); err != nil {
		h.fail(w, err)
		return
	}

	userID := reqctx.MustUserID(ctx)
	var res tenant.ApplicationFeeCollection
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		// Check if it's pending before allowing edit
		existing, err := q.GetApplicationFeeCollection(ctx, id)
		if err != nil {
			return err
		}
		if existing.AdminVerificationStatus != "Pending" {
			return errors.New("cannot edit a verified collection")
		}

		res, err = q.UpdateApplicationFeeCollection(ctx, tenant.UpdateApplicationFeeCollectionParams{
			CollegeFeeAmount:           conv.Float64ToNumeric(req.CollegeFeeAmount),
			PaymentToCollegeMethod:     conv.PtrStr(req.PaymentToCollegeMethod),
			StudentReimbursementMethod: conv.PtrStr(req.StudentReimbursementMethod),
			StudentTransactionRef:      conv.PtrStr(req.StudentTransactionRef),
			StaffRemarks:               conv.PtrStr(req.StaffRemarks),
			UpdatedBy:                  &userID,
			ID:                         id,
		})
		return err
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, toDto(res), "Updated successfully")
}

type statusReq struct {
	Status  string `json:"status" validate:"required"`
	Remarks string `json:"remarks"`
}

func (h *Handler) updateStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamUUID(chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, errors.New("invalid id"))
		return
	}

	var req statusReq
	if err := web.Bind(r, &req); err != nil {
		h.fail(w, err)
		return
	}

	userID := reqctx.MustUserID(ctx)
	var res tenant.ApplicationFeeCollection
	now := time.Now()
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		var err error
		var verifiedAt *time.Time
		var verifiedBy *int64
		if req.Status == "Verified" {
			verifiedAt = &now
			verifiedBy = &userID
		}

		res, err = q.UpdateApplicationFeeCollection(ctx, tenant.UpdateApplicationFeeCollectionParams{
			AdminVerificationStatus: conv.PtrStr(req.Status),
			AdminRemarks:            conv.PtrStr(req.Remarks),
			VerifiedBy:              verifiedBy,
			VerifiedAt:              verifiedAt,
			UpdatedBy:               &userID,
			ID:                      id,
		})
		return err
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, toDto(res), "Status updated")
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	branchIDStr := reqctx.BranchID(ctx)
	branchID, _ := conv.StrToUUID(branchIDStr)

	var staffID *int64
	if s := r.URL.Query().Get("staffId"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			staffID = &v
		}
	}

	// Role based filtering: if the user does NOT have the Admin/Manager ability to see all,
	// restrict to their own staff ID.
	identity, _ := reqctx.Get(ctx)
	userRole := identity.Role
	if userRole != "Admin" && userRole != "Super Admin" {
		uid := reqctx.MustUserID(ctx)
		staffID = &uid
	}

	var status *string
	if s := r.URL.Query().Get("status"); s != "" {
		status = &s
	}

	var startDate, endDate *time.Time
	if s := r.URL.Query().Get("startDate"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			startDate = &t
		}
	}
	if s := r.URL.Query().Get("endDate"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			endDate = &t
		}
	}

	var results []tenant.ListApplicationFeeCollectionsRow
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		var err error
		results, err = q.ListApplicationFeeCollections(ctx, tenant.ListApplicationFeeCollectionsParams{
			BranchID:  branchID,
			StaffID:   staffID,
			Status:    status,
			StartDate: startDate,
			EndDate:   endDate,
		})
		return err
	})
	if err != nil {
		h.fail(w, err)
		return
	}

	var dtos []collectionDto
	for _, res := range results {
		amount, _ := res.CollegeFeeAmount.Float64Value()
		
		staffName := ""
		if res.StaffFirstName != nil {
			staffName = *res.StaffFirstName
			if res.StaffLastName != nil {
				staffName += " " + *res.StaffLastName
			}
		} else {
			if res.StaffEmail != nil {
				staffName = *res.StaffEmail
			}
		}

		studentName := ""
		if res.StudentFirstName != nil {
			studentName = *res.StudentFirstName
			if res.StudentLastName != nil {
				studentName += " " + *res.StudentLastName
			}
		} else {
			if res.StudentEmail != nil {
				studentName = *res.StudentEmail
			}
		}

		appname := ""
		if res.ApplicationName != nil {
			appname = *res.ApplicationName
		}

		var verifiedByName *string
		if res.VerifiedByFirstName != nil {
			vName := *res.VerifiedByFirstName
			if res.VerifiedByLastName != nil {
				vName += " " + *res.VerifiedByLastName
			}
			verifiedByName = &vName
		}
		
		var vAt *string
		if res.VerifiedAt != nil {
			vAtStr := res.VerifiedAt.Format(time.RFC3339)
			vAt = &vAtStr
		}

		dtos = append(dtos, collectionDto{
			ID:                         res.ID.String(),
			ApplicationID:              res.ApplicationID.String(),
			ApplicationName:            appname,
			StudentID:                  res.StudentID.String(),
			StudentName:                studentName,
			StaffID:                    strconv.FormatInt(res.StaffID, 10),
			StaffName:                  staffName,
			CollegeFeeAmount:           amount.Float64,
			PaymentToCollegeMethod:     res.PaymentToCollegeMethod,
			StudentReimbursementMethod: res.StudentReimbursementMethod,
			StudentTransactionRef:      conv.Str(res.StudentTransactionRef),
			StaffRemarks:               conv.Str(res.StaffRemarks),
			AdminVerificationStatus:    res.AdminVerificationStatus,
			AdminRemarks:               conv.Str(res.AdminRemarks),
			VerifiedBy:                 verifiedByName,
			VerifiedAt:                 vAt,
			CreatedAt:                  conv.FmtDateTime(res.CreatedAt),
		})
	}
	if dtos == nil {
		dtos = []collectionDto{}
	}
	apiresp.List(w, dtos, int64(len(dtos)), "")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	apiresp.ServerError(w, err.Error())
}

func toDto(c tenant.ApplicationFeeCollection) collectionDto {
    amount, _ := c.CollegeFeeAmount.Float64Value()
	return collectionDto{
		ID:                         c.ID.String(),
		ApplicationID:              c.ApplicationID.String(),
		StaffID:                    strconv.FormatInt(c.StaffID, 10),
		CollegeFeeAmount:           amount.Float64,
		PaymentToCollegeMethod:     c.PaymentToCollegeMethod,
		StudentReimbursementMethod: c.StudentReimbursementMethod,
		StudentTransactionRef:      conv.Str(c.StudentTransactionRef),
		StaffRemarks:               conv.Str(c.StaffRemarks),
		AdminVerificationStatus:    c.AdminVerificationStatus,
		AdminRemarks:               conv.Str(c.AdminRemarks),
		CreatedAt:                  conv.FmtDateTime(c.CreatedAt),
	}
}
