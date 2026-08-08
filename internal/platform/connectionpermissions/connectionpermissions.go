package connectionpermissions

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/platform/activity"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
)

type Handler struct {
	q      *public.Queries
	logger *slog.Logger
	pool   *pgxpool.Pool
}

func New(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	return &Handler{
		q:      public.New(pool),
		logger: logger,
		pool:   pool,
	}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/connection-permissions", func(r chi.Router) {
		r.Patch("/{id}", h.updatePermissions)
	})
}

type updatePermissionsRequest struct {
	ViewProfile         bool `json:"viewProfile"`
	ViewDocuments       bool `json:"viewDocuments"`
	UploadDocuments     bool `json:"uploadDocuments"`
	CreateApplications  bool `json:"createApplications"`
	ViewApplications    bool `json:"viewApplications"`
	ApproveApplications bool `json:"approveApplications"`
	ManageConnection    bool `json:"manageConnection"`
}

func (h *Handler) updatePermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := reqctx.MustUserID(ctx)

	// Ensure caller is a Client by getting their client profile
	clientProfile, err := h.q.GetClientProfileByUserId(ctx, userID)
	if err != nil {
		apiresp.Forbidden(w, "Only clients can manage connection permissions")
		return
	}

	idStr := chi.URLParam(r, "id")
	connID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "Invalid connection ID")
		return
	}

	var req updatePermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiresp.BadRequest(w, "Invalid request body")
		return
	}

	// Verify connection exists, is active, and belongs to this client
	conn, err := h.q.GetClientConnectionById(ctx, connID)
	if err != nil {
		if err == pgx.ErrNoRows {
			apiresp.NotFound(w, "Connection not found")
		} else {
			apiresp.ServerError(w, "Database error")
		}
		return
	}

	if conn.ClientID != clientProfile.ID {
		apiresp.Forbidden(w, "Access denied")
		return
	}

	if conn.Status != "ACCEPTED" {
		apiresp.BadRequest(w, "Cannot update permissions without an active connection")
		return
	}

	// Update permissions
	perms, err := h.q.UpsertConnectionPermissions(ctx, public.UpsertConnectionPermissionsParams{
		ConnectionID:        connID,
		ViewProfile:         req.ViewProfile,
		ViewDocuments:       req.ViewDocuments,
		UploadDocuments:     req.UploadDocuments,
		CreateApplications:  req.CreateApplications,
		ViewApplications:    req.ViewApplications,
		ApproveApplications: req.ApproveApplications,
		ManageConnection:    req.ManageConnection,
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to update connection permissions", "err", err)
		apiresp.ServerError(w, "Failed to update connection permissions")
		return
	}

	apiresp.OK(w, perms, "Permissions updated successfully")
	activity.LogActivity(h.pool, userID, conn.TenantID, "Connection Permissions Updated")
}
