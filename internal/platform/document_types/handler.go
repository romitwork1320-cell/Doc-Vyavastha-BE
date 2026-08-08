package document_types

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

func (h *Handler) MountPublic(r chi.Router) {
	// Not used
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/master/document-types", h.listDocumentTypes)
	r.Get("/document-types", h.listDocumentTypes) // Alias
	
	// CRUD
	r.Post("/document-types", h.createDocumentType)
	r.Put("/document-types/{id}", h.updateDocumentType)
	r.Delete("/document-types/{id}", h.deleteDocumentType)
}

func (h *Handler) listDocumentTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.svc.ListDocumentTypes(r.Context())
	if err != nil {
		h.logger.Error("failed to list document types", "err", err)
		apiresp.ServerError(w, "failed to fetch document types")
		return
	}

	apiresp.OK(w, types, "Document types fetched successfully")
}

func (h *Handler) createDocumentType(w http.ResponseWriter, r *http.Request) {
	var dto DocumentTypeDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		apiresp.BadRequest(w, "invalid payload")
		return
	}

	docType, err := h.svc.CreateDocumentType(r.Context(), dto)
	if err != nil {
		h.logger.Error("failed to create document type", "err", err)
		apiresp.ServerError(w, "failed to create document type: "+err.Error())
		return
	}

	apiresp.Created(w, docType, "Document Type created")
}

func (h *Handler) updateDocumentType(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid id")
		return
	}

	var dto DocumentTypeDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		apiresp.BadRequest(w, "invalid payload")
		return
	}

	docType, err := h.svc.UpdateDocumentType(r.Context(), id, dto)
	if err != nil {
		h.logger.Error("failed to update document type", "err", err)
		apiresp.ServerError(w, "failed to update document type: "+err.Error())
		return
	}

	apiresp.OK(w, docType, "Document Type updated")
}

func (h *Handler) deleteDocumentType(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid id")
		return
	}

	if err := h.svc.DeleteDocumentType(r.Context(), id); err != nil {
		h.logger.Error("failed to delete document type", "err", err)
		apiresp.ServerError(w, "failed to delete document type")
		return
	}

	apiresp.OK(w, nil, "Document Type deleted")
}
