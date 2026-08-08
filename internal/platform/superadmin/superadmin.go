package superadmin

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/web"
)

type Handler struct {
	pool   *pgxpool.Pool
	q      *public.Queries
	logger *slog.Logger
}

func New(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	return &Handler{
		pool:   pool,
		q:      public.New(pool),
		logger: logger,
	}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/super-admin/organizations", func(r chi.Router) {
		r.Get("/", h.listOrganizations)
		r.Get("/{id}", h.getOrganization)
		r.Post("/{id}/approve", h.approveOrganization)
		r.Post("/{id}/reject", h.rejectOrganization)
	})
}


func (h *Handler) listOrganizations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenants, err := h.q.ListTenants(ctx, public.ListTenantsParams{
		Filter: "",
		Lim:    1000,
		Off:    0,
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to list tenants", "err", err)
		apiresp.ServerError(w, "Database error")
		return
	}
	apiresp.OK(w, tenants, "")
}

func (h *Handler) getOrganization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	tenantID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "Invalid organization ID")
		return
	}

	tenant, err := h.q.GetTenant(ctx, tenantID)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to get tenant", "err", err)
		apiresp.ServerError(w, "Database error")
		return
	}
	apiresp.OK(w, tenant, "")
}

type rejectRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) approveOrganization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := reqctx.MustUserID(ctx)
	idStr := chi.URLParam(r, "id")
	tenantID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "Invalid organization ID")
		return
	}

	err = h.q.VerifyTenantKYC(ctx, public.VerifyTenantKYCParams{
		VerifiedBy: &userID,
		TenantID:   tenantID,
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to approve kyc", "err", err)
		apiresp.ServerError(w, "Database error")
		return
	}

	apiresp.OK(w, nil, "Organization verified successfully")
}

func (h *Handler) rejectOrganization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := reqctx.MustUserID(ctx)
	idStr := chi.URLParam(r, "id")
	tenantID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		apiresp.BadRequest(w, "Invalid organization ID")
		return
	}

	var req rejectRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}

	err = h.q.RejectTenantKYC(ctx, public.RejectTenantKYCParams{
		RejectedBy: &userID,
		Reason:     &req.Reason,
		TenantID:   tenantID,
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to reject kyc", "err", err)
		apiresp.ServerError(w, "Database error")
		return
	}

	apiresp.OK(w, nil, "Organization rejected successfully")
}
