package applications

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/platform/activity"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
)

// Handler handles HTTP requests for applications.
type Handler struct {
	pool   *pgxpool.Pool
	svc    *Service
	logger *slog.Logger
}

// New constructs the applications Handler.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	return &Handler{
		pool:   pool,
		svc:    NewService(tenancy.NewManager(pool)),
		logger: logger,
	}
}

// Service returns the underlying applications Service.
func (h *Handler) Service() *Service {
	return h.svc
}

func getTenantID(r *http.Request) int64 {
	tenantID := reqctx.TenantID(r.Context())
	if tenantID == 0 {
		tStr := r.URL.Query().Get("tenantId")
		tenantID, _ = strconv.ParseInt(tStr, 10, 64)
	}
	return tenantID
}

// Mount registers routes for the applications module.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/Applications", func(r chi.Router) {
		r.Post("/", h.createApplication)
		r.Get("/", h.listApplications)
		r.Get("/{id}", h.getApplication)
		r.Put("/{id}", h.updateApplication)
		r.Patch("/{id}/status", h.updateStatus)
		r.Patch("/{id}/archive", h.archive)
		r.Patch("/{id}/restore", h.restore)
		r.Post("/{id}/requirements", h.addRequirement)
		r.Get("/{id}/timeline", h.getTimeline)
		r.Get("/{id}/requirements/{reqID}/versions/{versionID}/files/{fileID}/download", h.downloadVersionFile)
		r.Post("/{id}/requirements/{reqID}/review", h.reviewRequirement)
		r.Post("/{id}/final-deliverables", h.uploadFinalDeliverable)
		r.Get("/{id}/final-deliverables/{deliverableId}/download", h.downloadFinalDeliverable)
	})
}

func (h *Handler) createApplication(w http.ResponseWriter, r *http.Request) {
	var dto ApplicationDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		apiresp.BadRequest(w, "invalid request body")
		return
	}

	tenantID := reqctx.TenantID(r.Context())
	app, err := h.svc.CreateApplication(r.Context(), tenantID, dto)
	if err != nil {
		h.logger.Error("failed to create application", "err", err)
		apiresp.ServerError(w, "failed to create application")
		return
	}

	activity.LogActivity(h.pool, reqctx.MustUserID(r.Context()), tenantID, "Created Application: "+app.Title)
	apiresp.Created(w, app, "Application created successfully")
}

func (h *Handler) listApplications(w http.ResponseWriter, r *http.Request) {
	clientIDStr := r.URL.Query().Get("clientId")
	var clientID int64
	if clientIDStr != "" {
		clientID, _ = strconv.ParseInt(clientIDStr, 10, 64)
	}

	userID := reqctx.MustUserID(r.Context())
	profile, err := h.svc.tm.Pub().GetClientProfileByUserId(r.Context(), userID)
	if err == nil {
		// Enforce clientID for client users
		clientID = profile.ID
	}

	tenantID := getTenantID(r)
	apps, err := h.svc.ListApplications(r.Context(), tenantID, clientID)
	if err != nil {
		h.logger.Error("failed to list applications", "err", err)
		apiresp.ServerError(w, "failed to list applications: "+err.Error())
		return
	}

	apiresp.OK(w, apps, "Applications fetched")
}

func (h *Handler) getApplication(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid id")
		return
	}

	tenantID := getTenantID(r)
	app, err := h.svc.GetApplicationWithDetails(r.Context(), tenantID, id)
	if err != nil {
		apiresp.ServerError(w, "failed to fetch application")
		return
	}

	apiresp.OK(w, app, "Application fetched")
}

func (h *Handler) updateApplication(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid id")
		return
	}

	var payload struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiresp.BadRequest(w, "invalid payload")
		return
	}

	tenantID := getTenantID(r)
	app, err := h.svc.UpdateApplication(r.Context(), tenantID, id, payload.Title)
	if err != nil {
		apiresp.ServerError(w, "failed to update application: "+err.Error())
		return
	}

	apiresp.OK(w, app, "Application updated")
}

func (h *Handler) updateStatus(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid id")
		return
	}

	var payload struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiresp.BadRequest(w, "invalid payload")
		return
	}

	tenantID := getTenantID(r)
	app, err := h.svc.UpdateApplicationStatus(r.Context(), tenantID, id, payload.Status)
	if err != nil {
		apiresp.ServerError(w, "failed to update status")
		return
	}

	activity.LogActivity(h.pool, reqctx.MustUserID(r.Context()), tenantID, "Updated Application Status to "+payload.Status)
	apiresp.OK(w, app, "Status updated")
}

func (h *Handler) reviewRequirement(w http.ResponseWriter, r *http.Request) {
	reqIDStr := chi.URLParam(r, "reqID")
	reqID, err := strconv.ParseInt(reqIDStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid reqID")
		return
	}

	var payload struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiresp.BadRequest(w, "invalid payload")
		return
	}

	tenantID := getTenantID(r)
	req, err := h.svc.ReviewRequirement(r.Context(), tenantID, reqID, payload.Status, payload.Reason)
	if err != nil {
		apiresp.ServerError(w, "failed to review requirement")
		return
	}

	apiresp.OK(w, req, "Requirement updated")
}

func (h *Handler) uploadFinalDeliverable(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid id")
		return
	}

	err = r.ParseMultipartForm(10 << 20) // 10 MB limit
	if err != nil {
		apiresp.BadRequest(w, "invalid form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		apiresp.BadRequest(w, "file missing")
		return
	}
	defer file.Close()

	tenantID := getTenantID(r)
	app, err := h.svc.UploadFinalDeliverable(r.Context(), tenantID, id, header)
	if err != nil {
		apiresp.ServerError(w, "failed to upload deliverable")
		return
	}

	activity.LogActivity(h.pool, reqctx.MustUserID(r.Context()), tenantID, "Completed Application via Final Deliverable")
	apiresp.OK(w, app, "Final deliverable uploaded and application completed")
}

func (h *Handler) archive(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil { apiresp.BadRequest(w, "invalid id"); return }

	tenantID := getTenantID(r)
	err = h.svc.ArchiveApplication(r.Context(), tenantID, id)
	if err != nil {
		apiresp.ServerError(w, "failed to archive")
		return
	}
	apiresp.OK(w, nil, "Archived successfully")
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil { apiresp.BadRequest(w, "invalid id"); return }

	tenantID := getTenantID(r)
	err = h.svc.RestoreApplication(r.Context(), tenantID, id)
	if err != nil {
		apiresp.ServerError(w, "failed to restore")
		return
	}
	apiresp.OK(w, nil, "Restored successfully")
}

func (h *Handler) addRequirement(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil { apiresp.BadRequest(w, "invalid id"); return }

	var req RequirementDto
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiresp.BadRequest(w, "invalid payload")
		return
	}

	tenantID := getTenantID(r)
	created, err := h.svc.AddRequirement(r.Context(), tenantID, id, req)
	if err != nil {
		apiresp.ServerError(w, "failed to add requirement: " + err.Error())
		return
	}
	apiresp.Created(w, created, "Requirement added")
}

func (h *Handler) getTimeline(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil { apiresp.BadRequest(w, "invalid id"); return }

	tenantID := getTenantID(r)
	timeline, err := h.svc.GetTimeline(r.Context(), tenantID, id)
	if err != nil {
		apiresp.ServerError(w, "failed to fetch timeline")
		return
	}
	apiresp.OK(w, timeline, "Timeline fetched")
}

func (h *Handler) downloadVersionFile(w http.ResponseWriter, r *http.Request) {
	fileIDStr := chi.URLParam(r, "fileID")
	fileID, err := strconv.ParseInt(fileIDStr, 10, 64)
	if err != nil { apiresp.BadRequest(w, "invalid fileID"); return }

	tenantID := getTenantID(r)
	filePath, filename, err := h.svc.GetDocumentVersionFile(r.Context(), tenantID, fileID)
	if err != nil {
		if strings.Contains(err.Error(), "revoked") {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		apiresp.ServerError(w, "failed to get file: "+err.Error())
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	http.ServeFile(w, r, filePath)
}

func (h *Handler) downloadFinalDeliverable(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil { apiresp.BadRequest(w, "invalid id"); return }

	delIdStr := chi.URLParam(r, "deliverableId")
	delId, err := strconv.ParseInt(delIdStr, 10, 64)
	if err != nil { apiresp.BadRequest(w, "invalid deliverable id"); return }

	tenantID := getTenantID(r)
	filePath, filename, err := h.svc.GetFinalDeliverableFile(r.Context(), tenantID, id, delId)
	if err != nil {
		apiresp.ServerError(w, "failed to get file: "+err.Error())
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	http.ServeFile(w, r, filePath)
}

