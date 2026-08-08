package magiclink

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/platform/applications"
)

// Handler handles HTTP requests for magic links.
type Handler struct {
	svc    *Service
	appSvc *applications.Service
	pubQ   *public.Queries
	logger *slog.Logger
}

// NewHandler constructs the magiclink Handler.
func NewHandler(pool *pgxpool.Pool, logger *slog.Logger, appSvc *applications.Service) *Handler {
	return &Handler{
		svc:    NewService(pool),
		appSvc: appSvc,
		pubQ:   public.New(pool),
		logger: logger,
	}
}

// MountPublic registers routes for public unauthenticated access to magic links.
func (h *Handler) MountPublic(r chi.Router) {
	r.Route("/MagicLink", func(r chi.Router) {
		r.Get("/{token}", h.getMagicLinkDetails)
		r.Post("/{token}/upload/{reqID}", h.uploadDocument)
		r.Post("/{token}/upload/{reqID}/vault", h.uploadFromVault)
		r.Get("/{token}/vault/folders", h.getVaultFolders)
		r.Get("/{token}/vault/folders/{typeId}/files", h.getVaultFiles)
	})
}

// Mount registers routes for the organization to generate links.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/Applications/{appID}/MagicLink", func(r chi.Router) {
		r.Post("/generate", h.generateLink)
	})
}

func (h *Handler) getMagicLinkDetails(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	link, err := h.svc.ValidateLink(r.Context(), token)
	if err != nil {
		apiresp.Unauthorized(w, "invalid or expired link")
		return
	}

	// Fetch application details using the linked ApplicationID
	details, err := h.appSvc.GetApplicationWithDetails(r.Context(), link.TenantID, link.ApplicationID)
	if err != nil {
		apiresp.ServerError(w, "failed to load application details")
		return
	}

	apiresp.OK(w, details, "Magic link validated")
}

func (h *Handler) uploadDocument(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	link, err := h.svc.ValidateLink(r.Context(), token)
	if err != nil {
		apiresp.Unauthorized(w, "invalid or expired link")
		return
	}

	reqIDStr := chi.URLParam(r, "reqID")
	reqID, err := strconv.ParseInt(reqIDStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid reqID")
		return
	}

	err = r.ParseMultipartForm(10 << 20) // 10 MB
	if err != nil {
		apiresp.BadRequest(w, "invalid form")
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		apiresp.BadRequest(w, "files missing")
		return
	}

	// Call applications service to upload
	version, err := h.appSvc.UploadRequirementVersion(r.Context(), link.TenantID, reqID, files, true)
	if err != nil {
		apiresp.ServerError(w, "upload failed")
		return
	}

	apiresp.OK(w, version, "Document uploaded successfully")
}

func (h *Handler) getVaultFolders(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	link, err := h.svc.ValidateLink(r.Context(), token)
	if err != nil {
		apiresp.Unauthorized(w, "invalid or expired link")
		return
	}

	folders, err := h.pubQ.ListClientVaultFolders(r.Context(), link.ClientID)
	if err != nil {
		apiresp.OK(w, []interface{}{}, "No folders found")
		return
	}
	
	var vaultFolders []map[string]interface{}
	for _, f := range folders {
		vaultFolders = append(vaultFolders, map[string]interface{}{
			"document_type_id":   f.DocumentTypeID,
			"document_type_name": f.DocumentTypeName,
			"file_count":         f.FileCount,
		})
	}
	
	apiresp.OK(w, vaultFolders, "Vault folders retrieved")
}

func (h *Handler) getVaultFiles(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	link, err := h.svc.ValidateLink(r.Context(), token)
	if err != nil {
		apiresp.Unauthorized(w, "invalid or expired link")
		return
	}

	typeIDStr := chi.URLParam(r, "typeId")
	typeID, err := strconv.ParseInt(typeIDStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid typeId")
		return
	}

	files, err := h.pubQ.ListClientVaultFiles(r.Context(), public.ListClientVaultFilesParams{
		ClientID:       link.ClientID,
		DocumentTypeID: typeID,
	})
	if err != nil {
		apiresp.OK(w, []interface{}{}, "No files found")
		return
	}
	
	var vaultFiles []map[string]interface{}
	for _, f := range files {
		grants, _ := h.pubQ.GetDocumentAccessGrants(r.Context(), f.ID)
		vaultFiles = append(vaultFiles, map[string]interface{}{
			"id":                 f.ID,
			"document_type_id":   f.DocumentTypeID,
			"document_type_name": f.DocumentTypeName,
			"file_name":          f.FileName,
			"original_file_name": f.OriginalFileName,
			"file_size":          f.FileSize,
			"mime_type":          f.MimeType,
			"storage_path":       f.StoragePath,
			"uploaded_by_name":   f.UploadedByName,
			"source":             f.Source,
			"created_at":         f.CreatedAt,
			"grants":             grants,
		})
	}
	
	apiresp.OK(w, vaultFiles, "Vault files retrieved")
}

func (h *Handler) uploadFromVault(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	link, err := h.svc.ValidateLink(r.Context(), token)
	if err != nil {
		apiresp.Unauthorized(w, "invalid or expired link")
		return
	}

	reqIDStr := chi.URLParam(r, "reqID")
	reqID, err := strconv.ParseInt(reqIDStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid reqID")
		return
	}

	var payload struct {
		ClientDocumentIDs []int64 `json:"clientDocumentIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiresp.BadRequest(w, "invalid payload")
		return
	}

	version, err := h.appSvc.UploadRequirementVersionFromVault(r.Context(), link.TenantID, reqID, payload.ClientDocumentIDs, true)
	if err != nil {
		apiresp.ServerError(w, err.Error())
		return
	}

	apiresp.OK(w, version, "Document attached from vault successfully")
}

func (h *Handler) generateLink(w http.ResponseWriter, r *http.Request) {
	appIDStr := chi.URLParam(r, "appID")
	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "invalid appID")
		return
	}

	// Assuming tenant ID and client ID are sent in payload or grabbed from app
	var payload struct {
		TenantID int64 `json:"tenantId"`
		ClientID int64 `json:"clientId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		apiresp.BadRequest(w, "invalid payload")
		return
	}

	token, err := h.svc.GenerateLink(r.Context(), payload.TenantID, appID, payload.ClientID)
	if err != nil {
		apiresp.ServerError(w, "failed to generate link")
		return
	}
	
	_ = h.appSvc.LogTimelineEvent(r.Context(), payload.TenantID, appID, "LINK_GENERATED", "ORGANIZATION", "Magic link generated")

	// Also save this token string back to the Application record so it knows its current token
	// Assuming magic_link_token is mapped to string or similar UUID type
	// If it's a UUID, we parse it first:
	// But let's skip the setter here and do it inside GenerateLink for better atomicity.
	// Actually, wait, SetMagicLinkToken isn't exposed in applications.Service safely right now.
	// Let's just return it. The GenerateLink created it in the magic_links table!

	apiresp.OK(w, map[string]string{"token": token}, "Magic link generated")
}
