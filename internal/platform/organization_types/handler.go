package organization_types

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

func (h *Handler) Mount(r chi.Router) {
	r.Get("/master/organization-types", h.listOrganizationTypes)
	r.Get("/organization-types", h.listOrganizationTypes)

	r.Post("/organization-types", h.createOrganizationType)
	r.Put("/organization-types/{id}", h.updateOrganizationType)
	r.Delete("/organization-types/{id}", h.deleteOrganizationType)
}

func (h *Handler) listOrganizationTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.svc.ListOrganizationTypes(r.Context())
	if err != nil {
		h.logger.Error("failed to list organization types", "err", err)
		apiresp.ServerError(w, "failed to fetch organization types")
		return
	}

	apiresp.OK(w, types, "Organization types fetched successfully")
}

func (h *Handler) createOrganizationType(w http.ResponseWriter, r *http.Request) {
	var dto OrganizationTypeDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		apiresp.BadRequest(w, "invalid payload")
		return
	}

	orgType, err := h.svc.CreateOrganizationType(r.Context(), dto)
	if err != nil {
		h.logger.Error("failed to create organization type", "err", err)
		apiresp.ServerError(w, "failed to create organization type: "+err.Error())
		return
	}

	apiresp.Created(w, orgType, "Organization Type created")
}

func (h *Handler) updateOrganizationType(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid id")
		return
	}

	var dto OrganizationTypeDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		apiresp.BadRequest(w, "invalid payload")
		return
	}

	orgType, err := h.svc.UpdateOrganizationType(r.Context(), id, dto)
	if err != nil {
		h.logger.Error("failed to update organization type", "err", err)
		apiresp.ServerError(w, "failed to update organization type: "+err.Error())
		return
	}

	apiresp.OK(w, orgType, "Organization Type updated")
}

func (h *Handler) deleteOrganizationType(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid id")
		return
	}

	if err := h.svc.DeleteOrganizationType(r.Context(), id); err != nil {
		h.logger.Error("failed to delete organization type", "err", err)
		apiresp.ServerError(w, "failed to delete organization type")
		return
	}

	apiresp.OK(w, nil, "Organization Type deleted")
}
