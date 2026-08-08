package templates

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
)

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func New(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	return &Handler{
		svc:    NewService(pool),
		logger: logger,
	}
}

func (h *Handler) MountAdmin(r chi.Router) {

	r.Post("/admin/templates/applications", h.createApplicationType)
	r.Get("/admin/templates/applications", h.listApplicationTypes)
	r.Get("/admin/templates/applications/{id}", h.getApplicationType)
	r.Put("/admin/templates/applications/{id}", h.updateApplicationType)
	r.Delete("/admin/templates/applications/{id}", h.deleteApplicationType)
}

func (h *Handler) MountPublic(r chi.Router) {
	r.Get("/templates/applications", h.listApplicationTypes)
	r.Get("/templates/applications/{id}", h.getApplicationType)
}

// -- Application Types

func (h *Handler) createApplicationType(w http.ResponseWriter, r *http.Request) {
	var dto TemplateDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		apiresp.BadRequest(w, "invalid payload")
		return
	}

	appType, err := h.svc.CreateApplicationType(r.Context(), dto)
	if err != nil {
		h.logger.Error("failed to create application type", "err", err)
		apiresp.ServerError(w, "failed to create application type: "+err.Error())
		return
	}

	apiresp.Created(w, appType, "Application Type created")
}

func (h *Handler) updateApplicationType(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid id")
		return
	}

	var dto TemplateDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		apiresp.BadRequest(w, "invalid payload")
		return
	}

	appType, err := h.svc.UpdateApplicationType(r.Context(), id, dto)
	if err != nil {
		h.logger.Error("failed to update application type", "err", err)
		apiresp.ServerError(w, "failed to update application type: "+err.Error())
		return
	}

	apiresp.OK(w, appType, "Application Type updated")
}

func (h *Handler) deleteApplicationType(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid id")
		return
	}

	if err := h.svc.DeleteApplicationType(r.Context(), id); err != nil {
		h.logger.Error("failed to delete application type", "err", err)
		apiresp.ServerError(w, "failed to delete application type")
		return
	}

	apiresp.OK(w, nil, "Application Type deleted")
}

func (h *Handler) listApplicationTypes(w http.ResponseWriter, r *http.Request) {
	appTypes, err := h.svc.ListApplicationTypes(r.Context())
	if err != nil {
		h.logger.Error("failed to list application types", "err", err)
		apiresp.ServerError(w, "failed to list application types: "+err.Error())
		return
	}

	apiresp.OK(w, appTypes, "Application Types fetched")
}

func (h *Handler) getApplicationType(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid id")
		return
	}

	appType, err := h.svc.GetApplicationTypeWithDocuments(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to fetch application type", "err", err)
		apiresp.ServerError(w, "failed to fetch application type")
		return
	}

	apiresp.OK(w, appType, "Application Type details fetched")
}
