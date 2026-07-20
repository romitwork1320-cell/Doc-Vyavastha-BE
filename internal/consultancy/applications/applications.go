// Package applications implements /api/StudentApplications (tenant-scoped CRUD).
//
// Follows the canonical tenant module pattern (see categories.go): bind/validate
// -> run inside tenancy.InTenantTx (which sets search_path) -> map the sqlc row
// to the FE DTO -> render with the ApiResponse envelope.
//
// FE field mapping note: the DB columns portal_username / portal_password are
// the application-portal login credentials and are exposed to the FE under the
// json keys "userId" and "password" respectively.
package applications

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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/conv"
	"github.com/thinkparq/edconsultancy-be/internal/db/tenant"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
	"github.com/thinkparq/edconsultancy-be/internal/web"
	"github.com/thinkparq/edconsultancy-be/pkg/csvutil"
)

const StatusNameSubmitted = "Submitted"
var errStop = errors.New("stop")

// Handler serves the StudentApplications endpoints.
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
	r.Route("/StudentApplications", func(r chi.Router) {
		r.With(h.mw.RequirePermission("/student-applications", "CanView")).Get("/", h.list)
		r.With(h.mw.RequirePermission("/student-applications", "CanView")).Get("/export", h.export)
		r.With(h.mw.RequirePermission("/student-applications", "CanAdd")).Post("/", h.create)
		r.With(h.mw.RequirePermission("/student-applications", "CanView")).Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/student-applications", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/student-applications", "CanDelete")).Delete("/{id}", h.delete)
		r.Get("/student/{studentId}", h.byStudent)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID                  string  `json:"id"`
	StudentID           string  `json:"studentId"`
	ApplicationTypeID   string  `json:"applicationTypeId"`
	ApplicationStatusID string  `json:"applicationStatusId"`
	ApplicationNumber   string  `json:"applicationNumber"`
	ApplicationName     string  `json:"applicationName"`
	LastDate            *string `json:"lastDate"`
	AppliedDate         *string `json:"appliedDate"`
	SubmittedDate       *string `json:"submittedDate"`
	FormTypeID          string       `json:"formTypeId"`
	Colleges            []CollegeDto `json:"colleges"`
	UserID              string  `json:"userId"`   // DB: portal_username
	Password            string  `json:"password"` // DB: portal_password
	Remarks             string  `json:"remarks"`
	CreatedAt           string  `json:"createdAt"`
	UpdatedAt           string  `json:"updatedAt"`
}

type CollegeDto struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type request struct {
	StudentID           string `json:"studentId" validate:"required"`
	ApplicationTypeID   string `json:"applicationTypeId" validate:"required"`
	ApplicationStatusID string `json:"applicationStatusId" validate:"required"`
	ApplicationNumber   string `json:"applicationNumber"`
	ApplicationName     string `json:"applicationName"`
	LastDate            string `json:"lastDate"`
	AppliedDate         string `json:"appliedDate"`
	SubmittedDate       string `json:"submittedDate"`
	FormTypeID          string   `json:"formTypeId"`
	CollegeIDs          []string `json:"collegeIds"`
	UserID              string `json:"userId"`
	Password            string `json:"password"`
	Remarks             string `json:"remarks"`
}

func toDTO(a tenant.StudentApplication) dto {
	return dto{
		ID:                  a.ID.String(),
		StudentID:           a.StudentID.String(),
		ApplicationTypeID:   a.ApplicationTypeID.String(),
		ApplicationStatusID: a.ApplicationStatusID.String(),
		ApplicationNumber:   conv.Str(a.ApplicationNumber),
		ApplicationName:     conv.Str(a.ApplicationName),
		LastDate:            conv.FmtDate(a.LastDate),
		AppliedDate:         conv.FmtDate(a.AppliedDate),
		SubmittedDate:       conv.FmtDate(a.SubmittedDate),
		FormTypeID:          conv.UUIDStr(a.FormTypeID),
		Colleges:            []CollegeDto{},
		UserID:              conv.Str(a.PortalUsername),
		Password:            conv.Str(a.PortalPassword),
		Remarks:             conv.Str(a.Remarks),
		CreatedAt:           conv.FmtDateTime(a.CreatedAt),
		UpdatedAt:           conv.FmtDateTime(a.UpdatedAt),
	}
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
		rows, err := q.ListStudentApplications(ctx, tenant.ListStudentApplicationsParams{BranchID: branchUUID, Filter: page.FilterValue(), Lim: page.Limit(), Off: page.Offset()})
		if err != nil {
			return err
		}
		total, err = q.CountStudentApplications(ctx, tenant.CountStudentApplicationsParams{BranchID: branchUUID, Filter: page.FilterValue()})
		if err != nil {
			return err
		}
		items = make([]dto, len(rows))
		var appIDs []uuid.UUID
		for i, a := range rows {
			items[i] = toDTO(a)
			appIDs = append(appIDs, a.ID)
		}

		// Fetch colleges
		if len(appIDs) > 0 {
			colleges, err := q.GetCollegesForApplications(ctx, appIDs)
			if err != nil {
				return err
			}
			collegeMap := make(map[uuid.UUID][]CollegeDto)
			for _, c := range colleges {
				collegeMap[c.ApplicationID] = append(collegeMap[c.ApplicationID], CollegeDto{
					ID:   c.ID.String(),
					Name: c.Name,
				})
			}
			for i, item := range items {
				if cols, ok := collegeMap[uuid.MustParse(item.ID)]; ok {
					items[i].Colleges = cols
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

func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := web.PageFromQuery(r)

	var rows [][]string
	headers := []string{
		"Application Number", "Student Code", "Student Name",
		"Application Type", "Application Status", "College Name",
		"Form Type", "Portal User ID", "Portal Password",
		"Applied Date", "Submitted Date", "Last Date",
		"Remarks", "Created Date", "Updated Date",
	}

	// Fetch all colleges in bulk
	err := h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		// Use limit 1,000,000 to fetch all filtered records without pagination
		dbRows, err := q.ExportStudentApplications(ctx, page.FilterValue())
		if err != nil {
			return err
		}

		var appIDs []uuid.UUID
		for _, a := range dbRows {
			appIDs = append(appIDs, a.ID)
		}
		
		collegeMap := make(map[uuid.UUID][]string)
		if len(appIDs) > 0 {
			colleges, err := q.GetCollegesForApplications(ctx, appIDs)
			if err != nil {
				return err
			}
			for _, c := range colleges {
				collegeMap[c.ApplicationID] = append(collegeMap[c.ApplicationID], c.Name)
			}
		}

		for _, a := range dbRows {
			collegeNames := strings.Join(collegeMap[a.ID], ", ")
			row := []string{
				conv.Str(a.ApplicationNumber),
				a.StudentCode,
				a.StudentName,
				conv.Str(a.ApplicationTypeName),
				conv.Str(a.ApplicationStatusName),
				collegeNames,
				conv.Str(a.FormTypeName),
				conv.Str(a.PortalUsername),
				conv.Str(a.PortalPassword),
				csvutil.FormatDatePointer(a.AppliedDate),
				csvutil.FormatDatePointer(a.SubmittedDate),
				csvutil.FormatDatePointer(a.LastDate),
				conv.Str(a.Remarks),
				csvutil.FormatDate(a.CreatedAt),
				csvutil.FormatDate(a.UpdatedAt),
			}
			rows = append(rows, row)
		}
		return nil
	})

	if err != nil {
		h.fail(w, err)
		return
	}

	filename := fmt.Sprintf("StudentApplications_%s.csv", time.Now().Format("2006-01-02"))
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
		a, err := q.GetStudentApplication(ctx, id)
		if err != nil {
			return err
		}
		out = toDTO(a)
		
		cols, err := q.GetCollegesForApplication(ctx, id)
		if err != nil {
			return err
		}
		for _, c := range cols {
			out.Colleges = append(out.Colleges, CollegeDto{
				ID:   c.ID.String(),
				Name: c.Name,
			})
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Application not found")
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
		rows, err := q.GetApplicationsByStudent(ctx, tenant.GetApplicationsByStudentParams{StudentID: studentID, BranchID: branchUUID})
		if err != nil {
			return err
		}
		items = make([]dto, len(rows))
		for i, a := range rows {
			items[i] = toDTO(a)
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
	branchIDStr := reqctx.BranchID(ctx)
	branchUUID, _ := uuid.Parse(branchIDStr)

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
	typeID, err := uuid.Parse(req.ApplicationTypeID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid applicationTypeId")
		return
	}
	statusID, err := uuid.Parse(req.ApplicationStatusID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid applicationStatusId")
		return
	}

	appliedDate := conv.ParseDate(&req.AppliedDate)
	if appliedDate != nil && appliedDate.After(time.Now().Truncate(24*time.Hour).Add(24*time.Hour).Add(-time.Nanosecond)) {
		apiresp.BadRequest(w, "Applied Date cannot be in the future")
		return
	}

	userID := reqctx.MustUserID(ctx)

	var res dto
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		seq, err := q.NextAppSequence(ctx)
		if err != nil {
			return err
		}
		appNumber := fmt.Sprintf("APP%05d", seq)

		var formTypeID *uuid.UUID
		if req.FormTypeID != "" {
			if parsed, err := uuid.Parse(req.FormTypeID); err == nil {
				formTypeID = &parsed
			}
		}

		row, err := q.CreateStudentApplication(ctx, tenant.CreateStudentApplicationParams{
			StudentID:           studentID,
			BranchID:            branchUUID,
			ApplicationTypeID:   typeID,
			ApplicationStatusID: statusID,
			ApplicationNumber:   &appNumber,
			ApplicationName:     conv.PtrStr(req.ApplicationName),
			LastDate:            conv.ParseDate(&req.LastDate),
			AppliedDate:         appliedDate,
			SubmittedDate:       conv.ParseDate(&req.SubmittedDate),
			FormTypeID:          formTypeID,
			PortalUsername:      conv.PtrStr(req.UserID),
			PortalPassword:      conv.PtrStr(req.Password),
			Remarks:             conv.PtrStr(req.Remarks),
			CreatedBy:           &userID,
			UpdatedBy:           &userID,
		})
		if err != nil {
			return err
		}
		
		for _, cidStr := range req.CollegeIDs {
			cid, err := uuid.Parse(cidStr)
			if err != nil {
				return err
			}
			err = q.AddStudentApplicationCollege(ctx, tenant.AddStudentApplicationCollegeParams{
				ApplicationID: row.ID,
				CollegeID:     cid,
			})
			if err != nil {
				return err
			}
		}

		res = toDTO(row)
		
		cols, err := q.GetCollegesForApplication(ctx, row.ID)
		if err != nil {
			return err
		}
		for _, c := range cols {
			res.Colleges = append(res.Colleges, CollegeDto{
				ID:   c.ID.String(),
				Name: c.Name,
			})
		}
		
		return nil
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.Created(w, res, "Application created")
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
	studentID, err := uuid.Parse(req.StudentID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid studentId")
		return
	}
	typeID, err := uuid.Parse(req.ApplicationTypeID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid applicationTypeId")
		return
	}
	statusID, err := uuid.Parse(req.ApplicationStatusID)
	if err != nil {
		apiresp.BadRequest(w, "Invalid applicationStatusId")
		return
	}

	appliedDate := conv.ParseDate(&req.AppliedDate)
	if appliedDate != nil && appliedDate.After(time.Now().Truncate(24*time.Hour).Add(24*time.Hour).Add(-time.Nanosecond)) {
		apiresp.BadRequest(w, "Applied Date cannot be in the future")
		return
	}

	userID := reqctx.MustUserID(ctx)

	var res dto
	var bizMsg string
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		existing, err := q.GetStudentApplication(ctx, id)
		if err != nil {
			return err
		}
		if existing.StudentID != studentID {
			bizMsg = "Student cannot be changed after application creation."
			return errStop
		}
		if existing.ApplicationTypeID != typeID {
			bizMsg = "Application Type cannot be modified after application creation."
			return errStop
		}

		submittedDate := conv.ParseDate(&req.SubmittedDate)
		status, err := q.GetApplicationStatus(ctx, statusID)
		if err == nil {
			if status.Name == StatusNameSubmitted && existing.SubmittedDate == nil {
				now := time.Now()
				submittedDate = &now
			}
		}

		var formTypeID *uuid.UUID
		if req.FormTypeID != "" {
			if parsed, err := uuid.Parse(req.FormTypeID); err == nil {
				formTypeID = &parsed
			}
		}

		row, err := q.UpdateStudentApplication(ctx, tenant.UpdateStudentApplicationParams{
			ApplicationTypeID:   typeID,
			ApplicationStatusID: statusID,
			ApplicationNumber:   existing.ApplicationNumber,
			ApplicationName:     conv.PtrStr(req.ApplicationName),
			LastDate:            conv.ParseDate(&req.LastDate),
			AppliedDate:         appliedDate,
			SubmittedDate:       submittedDate,
			FormTypeID:          formTypeID,
			PortalUsername:      conv.PtrStr(req.UserID),
			PortalPassword:      conv.PtrStr(req.Password),
			Remarks:             conv.PtrStr(req.Remarks),
			UpdatedBy:           &userID,
			ID:                  id,
		})
		if err != nil {
			return err
		}

		err = q.ClearStudentApplicationColleges(ctx, id)
		if err != nil {
			return err
		}

		for _, cidStr := range req.CollegeIDs {
			cid, err := uuid.Parse(cidStr)
			if err != nil {
				return err
			}
			err = q.AddStudentApplicationCollege(ctx, tenant.AddStudentApplicationCollegeParams{
				ApplicationID: id,
				CollegeID:     cid,
			})
			if err != nil {
				return err
			}
		}

		res = toDTO(row)
		
		cols, err := q.GetCollegesForApplication(ctx, row.ID)
		if err != nil {
			return err
		}
		for _, c := range cols {
			res.Colleges = append(res.Colleges, CollegeDto{
				ID:   c.ID.String(),
				Name: c.Name,
			})
		}

		return nil
	})
	if errors.Is(err, errStop) {
		apiresp.Conflict(w, bizMsg)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Application not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, res, "Application updated")
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamUUID(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	uid := reqctx.MustUserID(ctx)
	var rows int64
	err = h.tm.InTenantTx(ctx, reqctx.Schema(ctx), func(q *tenant.Queries) error {
		var derr error
		rows, derr = q.SoftDeleteStudentApplication(ctx, tenant.SoftDeleteStudentApplicationParams{
			DeletedBy: &uid,
			ID:        id,
		})
		return derr
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	if rows == 0 {
		apiresp.NotFound(w, "Application not found")
		return
	}
	apiresp.OK(w, true, "Application deleted")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("StudentApplications handler error", "err", err)
	apiresp.ServerError(w, err.Error())
}

var _ = context.Background
