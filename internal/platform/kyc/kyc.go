package kyc

import (
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	mw "github.com/thinkparq/edconsultancy-be/internal/middleware"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
)

type Handler struct {
	pool   *pgxpool.Pool
	q      *public.Queries
	logger *slog.Logger
	authMW *mw.Auth
}

func New(pool *pgxpool.Pool, logger *slog.Logger, authMW *mw.Auth) *Handler {
	return &Handler{
		pool:   pool,
		q:      public.New(pool),
		logger: logger,
		authMW: authMW,
	}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/kyc", func(r chi.Router) {
		r.With(h.authMW.RequireTenant).Get("/status", h.getStatus)
		r.With(h.authMW.RequireTenant).Post("/upload", h.upload)
		r.Get("/document/{tenantID}/{docType}", h.getDocument)
	})
}

func (h *Handler) getStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := reqctx.TenantID(ctx)

	tenant, err := h.q.GetTenant(ctx, tenantID)
	if err != nil {
		apiresp.ServerError(w, "Database error")
		return
	}

	rejectionReason := ""
	if tenant.KycRejectionReason != nil {
		rejectionReason = *tenant.KycRejectionReason
	}

	apiresp.OK(w, map[string]string{
		"status":          tenant.KycStatus,
		"rejectionReason": rejectionReason,
	}, "")
}

func saveFile(file multipart.File, header *multipart.FileHeader, tenantID int64) (string, error) {
	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("%s%s", uuid.NewString(), ext)
	uploadDir := filepath.Join("uploads", "kyc", fmt.Sprintf("%d", tenantID))
	
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		return "", err
	}

	dstPath := filepath.Join(uploadDir, filename)
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		return "", err
	}

	return dstPath, nil
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := reqctx.TenantID(ctx)
	userID := reqctx.MustUserID(ctx)

	err := r.ParseMultipartForm(10 << 20) // 10 MB max memory
	if err != nil {
		apiresp.BadRequest(w, "Could not parse form")
		return
	}

	certFile, certHeader, err := r.FormFile("certificate")
	if err != nil {
		apiresp.BadRequest(w, "Certificate is required")
		return
	}
	defer certFile.Close()

	panFile, panHeader, err := r.FormFile("pan")
	if err != nil {
		apiresp.BadRequest(w, "PAN is required")
		return
	}
	defer panFile.Close()

	// Save Certificate
	certPath, err := saveFile(certFile, certHeader, tenantID)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to save certificate", "err", err)
		apiresp.ServerError(w, "Failed to save files")
		return
	}

	// Save PAN
	panPath, err := saveFile(panFile, panHeader, tenantID)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to save pan", "err", err)
		apiresp.ServerError(w, "Failed to save files")
		return
	}

	// Delete old documents first
	_ = h.q.DeleteTenantDocumentsByType(ctx, public.DeleteTenantDocumentsByTypeParams{
		TenantID: tenantID, DocumentType: "certificate",
	})
	_ = h.q.DeleteTenantDocumentsByType(ctx, public.DeleteTenantDocumentsByTypeParams{
		TenantID: tenantID, DocumentType: "pan",
	})

	// Insert new documents
	_, err = h.q.InsertTenantDocument(ctx, public.InsertTenantDocumentParams{
		TenantID: tenantID, DocumentType: "certificate", OriginalName: certHeader.Filename,
		StoredName: filepath.Base(certPath), MimeType: certHeader.Header.Get("Content-Type"),
		StoragePath: certPath, UploadedBy: userID,
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to insert cert record", "err", err)
		apiresp.ServerError(w, "Database error")
		return
	}

	_, err = h.q.InsertTenantDocument(ctx, public.InsertTenantDocumentParams{
		TenantID: tenantID, DocumentType: "pan", OriginalName: panHeader.Filename,
		StoredName: filepath.Base(panPath), MimeType: panHeader.Header.Get("Content-Type"),
		StoragePath: panPath, UploadedBy: userID,
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to insert pan record", "err", err)
		apiresp.ServerError(w, "Database error")
		return
	}
	
	err = h.q.UpdateTenantKYCStatus(ctx, public.UpdateTenantKYCStatusParams{
		Status:   "PENDING_VERIFICATION",
		TenantID: tenantID,
	})
	if err != nil {
		apiresp.ServerError(w, "Database error")
		return
	}

	apiresp.OK(w, nil, "Documents uploaded successfully")
}

func (h *Handler) getDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userTenantID := reqctx.TenantID(ctx)
	userID := reqctx.MustUserID(ctx)

	targetTenantIDStr := chi.URLParam(r, "tenantID")
	docType := chi.URLParam(r, "docType")
	var targetTenantID int64
	fmt.Sscanf(targetTenantIDStr, "%d", &targetTenantID)

	// Authorization
	u, err := h.q.GetUserByID(ctx, userID)
	if err != nil {
		apiresp.ServerError(w, "Auth error")
		return
	}
	
	if !u.IsSuperadmin && userTenantID != targetTenantID {
		apiresp.Forbidden(w, "You are not authorized to view this document")
		return
	}

	docs, err := h.q.GetTenantDocuments(ctx, targetTenantID)
	if err != nil {
		apiresp.ServerError(w, "Database error")
		return
	}

	var found *public.TenantDocument
	for _, d := range docs {
		if d.DocumentType == docType {
			found = &d
			break
		}
	}

	if found == nil {
		apiresp.NotFound(w, "Document not found")
		return
	}

	w.Header().Set("Content-Type", found.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", found.OriginalName))
	http.ServeFile(w, r, found.StoragePath)
}
