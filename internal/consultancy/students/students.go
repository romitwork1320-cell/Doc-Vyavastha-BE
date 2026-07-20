// Package students implements /api/Students (tenant-scoped) including the
// year-wise student-code generation described in the architecture doc. The code
// is generated atomically inside the tenant transaction by locking and
// incrementing the per-year-config sequence row (never derived from COUNT()).
package students

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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/conv"
	"github.com/thinkparq/edconsultancy-be/internal/db/tenant"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
	"github.com/thinkparq/edconsultancy-be/internal/web"
	"github.com/thinkparq/edconsultancy-be/pkg/csvutil"
)

// Handler serves the Students endpoints.
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
	r.Route("/Students", func(r chi.Router) {
		r.With(h.mw.RequirePermission("/students", "CanView")).Get("/", h.list)
		r.With(h.mw.RequirePermission("/students", "CanView")).Get("/export", h.export)
		r.With(h.mw.RequirePermission("/students", "CanAdd")).Post("/", h.create)
		r.With(h.mw.RequirePermission("/students", "CanView")).Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/students", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/students", "CanDelete")).Delete("/{id}", h.delete)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID                  string  `json:"id"`
	StudentCode         string  `json:"studentCode"`
	CategoryID          string  `json:"categoryId"`
	CasteID             *string `json:"casteId,omitempty"`
	YearConfigID        *string `json:"yearConfigId,omitempty"`
	FullName            string  `json:"fullName"`
	FatherName          string  `json:"fatherName"`
	MotherName          string  `json:"motherName"`
	Gender              string  `json:"gender"`
	Email               string  `json:"email"`
	PrimaryMobile       string  `json:"primaryMobile"`
	SecondaryMobile     string  `json:"secondaryMobile"`
	WhatsAppMobile      string  `json:"whatsAppMobile"`
	HomeAddress         string  `json:"homeAddress"`
	City                string  `json:"city"`
	State               string  `json:"state"`
	Pincode             string  `json:"pincode"`
	SchoolName          string  `json:"schoolName"`
	PassingBoard        string  `json:"passingBoard"`
	TenthPassingYear    int32   `json:"tenthPassingYear"`
	TwelfthPassingYear  int32   `json:"twelfthPassingYear"`
	ScholarshipUID      string  `json:"scholarshipUid"`
	ScholarshipPassword string  `json:"scholarshipPassword"`
	ProfilePhotoURL     string  `json:"profilePhotoUrl"`
	Status              string  `json:"status"`
	Remarks             string  `json:"remarks"`
	CreatedAt           string   `json:"createdAt"`
	UpdatedAt           string   `json:"updatedAt"`
	AssignedCodes       []string `json:"assignedCodes,omitempty"`
	AppPortalID         *string  `json:"appPortalId,omitempty"`
	AppPortalPassword   *string  `json:"appPortalPassword,omitempty"`
}

type request struct {
	CategoryID          string `json:"categoryId" validate:"required"`
	CasteID             string `json:"casteId"`
	BusinessYear        int32  `json:"businessYear"` // optional; pins which year-config to use
	FullName            string `json:"fullName" validate:"required"`
	FatherName          string `json:"fatherName"`
	MotherName          string `json:"motherName"`
	Gender              string `json:"gender"`
	Email               string `json:"email"`
	PrimaryMobile       string `json:"primaryMobile" validate:"required"`
	SecondaryMobile     string `json:"secondaryMobile"`
	WhatsAppMobile      string `json:"whatsAppMobile"`
	HomeAddress         string `json:"homeAddress"`
	City                string `json:"city"`
	State               string `json:"state"`
	Pincode             string `json:"pincode"`
	SchoolName          string `json:"schoolName"`
	PassingBoard        string `json:"passingBoard"`
	TenthPassingYear    int32  `json:"tenthPassingYear"`
	TwelfthPassingYear  int32  `json:"twelfthPassingYear"`
	ScholarshipUID      string `json:"scholarshipUid"`
	ScholarshipPassword string `json:"scholarshipPassword"`
	ProfilePhotoURL     string `json:"profilePhotoUrl"`
	Status                  string  `json:"status"`
	Remarks                 string  `json:"remarks"`
	InitialPaymentAmount    float64 `json:"initialPaymentAmount"`
	InitialPaymentMethod    string  `json:"initialPaymentMethod"`
	InitialPaymentDate      string  `json:"initialPaymentDate"`
	InitialPaymentReference string  `json:"initialPaymentReference"`
}

func toDTO(s tenant.Student, codes []string) dto {
	return dto{
		ID:                  s.ID.String(),
		StudentCode:         s.StudentCode,
		CategoryID:          s.CategoryID.String(),
		CasteID:             conv.UUIDToStrPtr(s.CasteID),
		YearConfigID:        conv.UUIDToStrPtr(s.YearConfigID),
		FullName:            s.FullName,
		FatherName:          conv.Str(s.FatherName),
		MotherName:          conv.Str(s.MotherName),
		Gender:              conv.Str(s.Gender),
		Email:               conv.Str(s.Email),
		PrimaryMobile:       s.PrimaryMobile,
		SecondaryMobile:     conv.Str(s.SecondaryMobile),
		WhatsAppMobile:      conv.Str(s.WhatsappMobile),
		HomeAddress:         conv.Str(s.HomeAddress),
		City:                conv.Str(s.City),
		State:               conv.Str(s.State),
		Pincode:             conv.Str(s.Pincode),
		SchoolName:          conv.Str(s.SchoolName),
		PassingBoard:        conv.Str(s.PassingBoard),
		TenthPassingYear:    conv.Int32(s.TenthPassingYear),
		TwelfthPassingYear:  conv.Int32(s.TwelfthPassingYear),
		ScholarshipUID:      conv.Str(s.ScholarshipUid),
		ScholarshipPassword: conv.Str(s.ScholarshipPassword),
		ProfilePhotoURL:     conv.Str(s.ProfilePhotoUrl),
		Status:              s.Status,
		Remarks:             conv.Str(s.Remarks),
		CreatedAt:           conv.FmtDateTime(s.CreatedAt),
		UpdatedAt:           conv.FmtDateTime(s.UpdatedAt),
		AssignedCodes:       codes,
	}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)
	page := web.PageFromQuery(r)
	var appTypeID *uuid.UUID
	if atid := r.URL.Query().Get("applicationTypeId"); atid != "" {
		if parsed, err := uuid.Parse(atid); err == nil {
			appTypeID = &parsed
		}
	}
	var items []dto
	var total int64
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		rows, err := q.ListStudents(ctx, tenant.ListStudentsParams{BranchID: branchUUID, Filter: page.FilterValue(), ApplicationTypeID: appTypeID, Lim: page.Limit(), Off: page.Offset()})
		if err != nil {
			return err
		}
		total, err = q.CountStudents(ctx, tenant.CountStudentsParams{BranchID: branchUUID, Filter: page.FilterValue(), ApplicationTypeID: appTypeID})
		if err != nil {
			return err
		}
		items = make([]dto, len(rows))
		
		// Collect IDs
		var studentIDs []uuid.UUID
		for _, s := range rows {
			studentIDs = append(studentIDs, s.ID)
		}
		
		// Fetch assigned codes
		assignedRecords, err := q.GetAssignedCodesByStudentIDs(ctx, studentIDs)
		if err != nil {
			return err
		}
		
		// Group by student ID
		codesMap := make(map[uuid.UUID][]string)
		for _, ac := range assignedRecords {
			codesMap[ac.StudentID] = append(codesMap[ac.StudentID], ac.StudentCode)
		}
		
		var appMap map[uuid.UUID]tenant.StudentApplication
		if appTypeID != nil && len(studentIDs) > 0 {
			apps, err := q.GetApplicationsByStudentsAndType(ctx, tenant.GetApplicationsByStudentsAndTypeParams{
				ApplicationTypeID: *appTypeID,
				StudentIds:        studentIDs,
			})
			if err == nil {
				appMap = make(map[uuid.UUID]tenant.StudentApplication)
				for _, app := range apps {
					appMap[app.StudentID] = app
				}
			}
		}

		for i, s := range rows {
			item := toDTO(s, codesMap[s.ID])
			if appMap != nil {
				if app, ok := appMap[s.ID]; ok {
					item.AppPortalID = app.PortalUsername
					item.AppPortalPassword = app.PortalPassword
				}
			}
			items[i] = item
		}
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.List(w, items, total, "")
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)
	page := web.PageFromQuery(r) // we use the filter but ignore lim/off
	var appTypeID *uuid.UUID
	if atid := r.URL.Query().Get("applicationTypeId"); atid != "" {
		if parsed, err := uuid.Parse(atid); err == nil {
			appTypeID = &parsed
		}
	}

	var rows [][]string
	headers := []string{
		"Student Code", "Full Name", "Caste",
		"Father Name", "Mother Name", "Gender", "Email",
		"Primary Mobile", "Secondary Mobile", "WhatsApp Mobile",
		"Home Address", "City", "State", "Pincode",
		"School Name", "Passing Board", "10th Passing Year", "12th Passing Year",
		"Scholarship UID", "Scholarship Password", "Category",
		"Status", "Remarks", "Created Date", "Updated Date",
	}

	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		// Use limit 1,000,000 to fetch all filtered records
		dbRows, err := q.ExportStudents(ctx, tenant.ExportStudentsParams{
			BranchID: branchUUID,
			Filter:            page.FilterValue(),
			ApplicationTypeID: appTypeID,
		})
		if err != nil {
			return err
		}

		for _, s := range dbRows {
			casteName := ""
			if s.CasteName != nil {
				casteName = *s.CasteName
			}
			row := []string{
				s.StudentCode,
				s.FullName,
				casteName,
				conv.Str(s.FatherName),
				conv.Str(s.MotherName),
				conv.Str(s.Gender),
				conv.Str(s.Email),
				s.PrimaryMobile,
				conv.Str(s.SecondaryMobile),
				conv.Str(s.WhatsappMobile),
				conv.Str(s.HomeAddress),
				conv.Str(s.City),
				conv.Str(s.State),
				conv.Str(s.Pincode),
				conv.Str(s.SchoolName),
				conv.Str(s.PassingBoard),
				fmt.Sprintf("%v", conv.Int32(s.TenthPassingYear)),
				fmt.Sprintf("%v", conv.Int32(s.TwelfthPassingYear)),
				conv.Str(s.ScholarshipUid),
				conv.Str(s.ScholarshipPassword),
				conv.Str(s.CategoryName),
				s.Status,
				conv.Str(s.Remarks),
				csvutil.FormatDate(s.CreatedAt),
				csvutil.FormatDate(s.UpdatedAt),
			}
			// Clean up "0" for passing years if they are zero
			if row[16] == "0" {
				row[16] = ""
			}
			if row[17] == "0" {
				row[17] = ""
			}
			rows = append(rows, row)
		}
		return nil
	})

	if err != nil {
		h.fail(w, err)
		return
	}

	filename := fmt.Sprintf("Students_%s.csv", time.Now().Format("2006-01-02"))
	if err := csvutil.WriteCSV(w, filename, headers, rows); err != nil {
		h.logger.Error("failed to write csv", "err", err)
	}
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
		s, err := q.GetStudent(ctx, id)
		if err != nil {
			return err
		}
		
		assignedRecords, err := q.GetAssignedCodesByStudent(ctx, id)
		if err != nil {
			return err
		}
		
		var assignedCodes []string
		for _, ac := range assignedRecords {
			assignedCodes = append(assignedCodes, ac.StudentCode)
		}
		
		out = toDTO(s, assignedCodes)
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Student not found")
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
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)
	var req request
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	categoryID, err := uuid.Parse(req.CategoryID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid categoryId")
		return
	}
	uid := reqctx.MustUserID(ctx)

	var out dto
	var bizMsg string
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		// 1. Resolve the active year-config for this category (and year, if given).
		var cfg tenant.StudentCodeYearConfig
		var cerr error
		if req.BusinessYear != 0 {
			cfg, cerr = q.GetActiveConfigByCategoryYear(ctx, tenant.GetActiveConfigByCategoryYearParams{CategoryID: categoryID, BusinessYear: req.BusinessYear})
		} else {
			cfg, cerr = q.GetActiveConfigByCategory(ctx, categoryID)
		}
		if errors.Is(cerr, pgx.ErrNoRows) {
			bizMsg = "No active student-code configuration exists for this category"
			return errStop
		}
		if cerr != nil {
			return cerr
		}

		// 2. Atomically take the next sequence number.
		if err := q.EnsureSequenceExists(ctx, cfg.ID); err != nil {
			return err
		}
		seq, err := q.GetSequenceByYearConfigForUpdate(ctx, cfg.ID)
		if err != nil {
			return err
		}
		next := seq.CurrentNumber + 1
		code := formatCode(cfg.Prefix, cfg.Separator, next, cfg.PaddingLength)
		if _, err := q.SetSequenceValue(ctx, tenant.SetSequenceValueParams{
			CurrentNumber: next, LastGeneratedCode: &code, YearConfigID: cfg.ID,
		}); err != nil {
			return err
		}

		casteID, _ := conv.StrToUUID(req.CasteID)

		// 3. Insert Student
		s, err := q.CreateStudent(ctx, tenant.CreateStudentParams{
			BranchID:          branchUUID,
			StudentCode:         code,
			CategoryID:          categoryID,
			CasteID:             conv.UUIDPtr(casteID),
			YearConfigID:        conv.UUIDPtr(cfg.ID),
			FullName:            req.FullName,
			FatherName:          conv.PtrStr(req.FatherName),
			MotherName:          conv.PtrStr(req.MotherName),
			Gender:              conv.PtrStr(req.Gender),
			Email:               conv.PtrStr(req.Email),
			PrimaryMobile:       req.PrimaryMobile,
			SecondaryMobile:     conv.PtrStr(req.SecondaryMobile),
			WhatsappMobile:      conv.PtrStr(req.WhatsAppMobile),
			HomeAddress:         conv.PtrStr(req.HomeAddress),
			City:                conv.PtrStr(req.City),
			State:               conv.PtrStr(req.State),
			Pincode:             conv.PtrStr(req.Pincode),
			SchoolName:          conv.PtrStr(req.SchoolName),
			PassingBoard:        conv.PtrStr(req.PassingBoard),
			TenthPassingYear:    conv.PtrInt32(req.TenthPassingYear),
			TwelfthPassingYear:  conv.PtrInt32(req.TwelfthPassingYear),
			ScholarshipUid:      conv.PtrStr(req.ScholarshipUID),
			ScholarshipPassword: conv.PtrStr(req.ScholarshipPassword),
			ProfilePhotoUrl:     conv.PtrStr(req.ProfilePhotoURL),
			Status:              statusOrDefault(req.Status),
			Remarks:             conv.PtrStr(req.Remarks),
			CreatedBy:           &uid,
			UpdatedBy:           &uid,
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_student_mobile" {
				bizMsg = "A student with this mobile number already exists."
				return errStop
			}
			return err
		}

		_, err = q.CreateAssignedCode(ctx, tenant.CreateAssignedCodeParams{
			StudentID:    s.ID,
			CategoryID:   categoryID,
			YearConfigID: cfg.ID,
			StudentCode:  code,
		})
		if err != nil {
			return err
		}

		// 4. Create Fee Plan based on selected category (if any)
		if ft, err := q.GetFeeTypeByCategory(ctx, categoryID); err == nil {
			var zeroNumeric pgtype.Numeric
			_ = zeroNumeric.Scan("0")
			sfp, cerr := q.CreateStudentFeePlan(ctx, tenant.CreateStudentFeePlanParams{
				BranchID:       branchUUID,
				StudentID:      s.ID,
				FeeTypeID:      &ft.ID,
				FeeName:        &ft.Name,
				TotalAmount:    ft.Amount,
				DiscountAmount: zeroNumeric,
				CreatedBy:      &uid,
			})
			if cerr != nil {
				return cerr
			}

			// 5. If Initial Payment Amount > 0, create Student Payment
			if req.InitialPaymentAmount > 0 {
				paymentDate := time.Now()
				if pd := conv.ParseDate(&req.InitialPaymentDate); pd != nil {
					paymentDate = *pd
				}

				pn, cerr := q.NextPaymentSeq(ctx)
				if cerr != nil {
					return cerr
				}
				rn, cerr := q.NextReceiptSeq(ctx)
				if cerr != nil {
					return cerr
				}
				
				var amt pgtype.Numeric
				if scanErr := amt.Scan(fmt.Sprintf("%v", req.InitialPaymentAmount)); scanErr != nil {
					return scanErr
				}

				paymentMethod := "Cash"
				if req.InitialPaymentMethod != "" {
					paymentMethod = req.InitialPaymentMethod
				}

				receiptNumber := fmt.Sprintf("RCPT%05d", rn)
				_, cerr = q.CreateStudentPayment(ctx, tenant.CreateStudentPaymentParams{
					BranchID:         branchUUID,
					PaymentNumber:    fmt.Sprintf("PAY%04d", pn),
					ReceiptNumber:    &receiptNumber,
					StudentID:        s.ID,
					StudentFeePlanID: sfp.ID,
					PaymentDate:      paymentDate,
					Amount:           amt,
					PaymentMethod:    &paymentMethod,
					ReferenceNumber:  conv.PtrStr(req.InitialPaymentReference),
					Remarks:          conv.PtrStr("Initial payment at registration"),
					CreatedBy:        &uid,
				})
				if cerr != nil {
					return cerr
				}
			}
		}

		out = toDTO(s, []string{code})
		return nil
	})
	if bizMsg != "" {
		apiresp.BadRequest(w, bizMsg)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.Created(w, out, "Student created")
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
	categoryID, err := uuid.Parse(req.CategoryID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid categoryId")
		return
	}
	uid := reqctx.MustUserID(ctx)

	var out dto
	var bizMsg string
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		existing, err := q.GetStudent(ctx, id)
		if err != nil {
			return err
		}

		newStudentCode := existing.StudentCode
		newYearConfigID := existing.YearConfigID

		if existing.CategoryID != categoryID {
			// Check if they already have a code for this category
			assignedCode, err := q.GetAssignedCodeByCategory(ctx, tenant.GetAssignedCodeByCategoryParams{
				StudentID:  id,
				CategoryID: categoryID,
			})

			if errors.Is(err, pgx.ErrNoRows) {
				// Generate new code
				cfg, err := q.GetActiveConfigByCategory(ctx, categoryID)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						bizMsg = "No active student-code configuration exists for this category."
						return errStop
					}
					return err
				}
				newYearConfigID = conv.UUIDPtr(cfg.ID)

				if err := q.EnsureSequenceExists(ctx, cfg.ID); err != nil {
					return err
				}
				seq, err := q.GetSequenceByYearConfigForUpdate(ctx, cfg.ID)
				if err != nil {
					return err
				}
				next := seq.CurrentNumber + 1
				newStudentCode = formatCode(cfg.Prefix, cfg.Separator, next, cfg.PaddingLength)
				if _, err := q.SetSequenceValue(ctx, tenant.SetSequenceValueParams{
					CurrentNumber:     next,
					LastGeneratedCode: &newStudentCode,
					YearConfigID:      cfg.ID,
				}); err != nil {
					return err
				}

				// Insert into assigned codes
				_, err = q.CreateAssignedCode(ctx, tenant.CreateAssignedCodeParams{
					StudentID:    id,
					CategoryID:   categoryID,
					YearConfigID: cfg.ID,
					StudentCode:  newStudentCode,
				})
				if err != nil {
					return err
				}
			} else if err == nil {
				// Reuse existing assigned code
				newStudentCode = assignedCode.StudentCode
				newYearConfigID = conv.UUIDPtr(assignedCode.YearConfigID)
			} else {
				return err
			}
		}

		casteID, _ := conv.StrToUUID(req.CasteID)
		s, err := q.UpdateStudent(ctx, tenant.UpdateStudentParams{
			CategoryID:          categoryID,
			YearConfigID:        newYearConfigID,
			StudentCode:         newStudentCode,
			BranchID:            existing.BranchID,
			CasteID:             conv.UUIDPtr(casteID),
			FullName:            req.FullName,
			FatherName:          conv.PtrStr(req.FatherName),
			MotherName:          conv.PtrStr(req.MotherName),
			Gender:              conv.PtrStr(req.Gender),
			Email:               conv.PtrStr(req.Email),
			PrimaryMobile:       req.PrimaryMobile,
			SecondaryMobile:     conv.PtrStr(req.SecondaryMobile),
			WhatsappMobile:      conv.PtrStr(req.WhatsAppMobile),
			HomeAddress:         conv.PtrStr(req.HomeAddress),
			City:                conv.PtrStr(req.City),
			State:               conv.PtrStr(req.State),
			Pincode:             conv.PtrStr(req.Pincode),
			SchoolName:          conv.PtrStr(req.SchoolName),
			PassingBoard:        conv.PtrStr(req.PassingBoard),
			TenthPassingYear:    conv.PtrInt32(req.TenthPassingYear),
			TwelfthPassingYear:  conv.PtrInt32(req.TwelfthPassingYear),
			ScholarshipUid:      conv.PtrStr(req.ScholarshipUID),
			ScholarshipPassword: conv.PtrStr(req.ScholarshipPassword),
			ProfilePhotoUrl:     conv.PtrStr(req.ProfilePhotoURL),
			Status:              statusOrDefault(req.Status),
			Remarks:             conv.PtrStr(req.Remarks),
			UpdatedBy:           &uid,
			ID:                  id,
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_student_mobile" {
				bizMsg = "A student with this mobile number already exists."
				return errStop
			}
			return err
		}
		
		assignedRecords, err := q.GetAssignedCodesByStudent(ctx, id)
		if err != nil {
			return err
		}
		
		var assignedCodes []string
		for _, ac := range assignedRecords {
			assignedCodes = append(assignedCodes, ac.StudentCode)
		}

		out = toDTO(s, assignedCodes)
		return nil
	})
	if bizMsg != "" {
		apiresp.BadRequest(w, bizMsg)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Student not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, out, "Student updated")
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	var rows int64
	var bizMsg string
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)

	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		apps, aerr := q.GetApplicationsByStudent(ctx, tenant.GetApplicationsByStudentParams{StudentID: id, BranchID: branchUUID})
		if aerr != nil {
			return aerr
		}
		if len(apps) > 0 {
			bizMsg = "Cannot delete student because applications exist."
			return errStop
		}

		var derr error
		rows, derr = q.SoftDeleteStudent(ctx, id)
		return derr
	})
	if bizMsg != "" {
		apiresp.Conflict(w, bizMsg)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	if rows == 0 {
		apiresp.NotFound(w, "Student not found")
		return
	}
	apiresp.OK(w, true, "Student deleted")
}

// errStop unwinds the tx for a handled business condition (rolls back cleanly).
var errStop = errors.New("stop")

func formatCode(prefix, sep string, num int64, pad int32) string {
	// Padding functionality removed as per user request
	return fmt.Sprintf("%s%s%d", prefix, sep, num)
}

func statusOrDefault(s string) string {
	if s == "" {
		return "Active"
	}
	return s
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, errStop) {
		return // business message already written by caller
	}
	h.logger.Error("Students handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

var _ = context.Background
