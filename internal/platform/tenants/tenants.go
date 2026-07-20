// Package tenants implements /api/Tenants (platform-scoped tenant management).
//
// This is a PLATFORM module: it talks to the public schema via public.Queries
// (built from the shared pool) and keeps a tenancy.Manager so it can provision
// a brand-new tenant (which materializes a dedicated schema) on create. Reads
// map sqlc rows to the FE DTO and render through the ApiResponse envelope.
package tenants

import (
	"github.com/thinkparq/edconsultancy-be/internal/middleware"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/conv"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/db/tenant"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
	"github.com/thinkparq/edconsultancy-be/internal/web"
)

// Handler serves the Tenants endpoints.
type Handler struct {
	q      *public.Queries
	tm     *tenancy.Manager
	mw     *middleware.Auth
	logger *slog.Logger
}

// New builds the handler. The pool feeds the public queries; tm is retained so
// create can provision a new tenant + schema.
func New(pool *pgxpool.Pool, tm *tenancy.Manager, logger *slog.Logger, mw *middleware.Auth) *Handler {
	return &Handler{q: public.New(pool), tm: tm, logger: logger, mw: mw}
}

// Mount registers routes. chi matches static segments before path params, so
// "/stats" is reached even though "/{tenantId}/..." routes also exist.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/Tenants", func(r chi.Router) {
		r.With(h.mw.RequirePermission("/tenants", "CanView")).Get("/", h.list)
		r.Get("/stats", h.stats)
		r.With(h.mw.RequirePermission("/tenants", "CanAdd")).Post("/", h.create)
		r.Put("/{tenantId}/whatsapp-balance", h.addBalance)
		r.Put("/{tenantId}/status", h.setStatus)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type tenantDTO struct {
	TenantID            int64   `json:"tenantId"`
	CompanyName         string  `json:"companyName"`
	TenantName          string  `json:"tenantName"`
	ContactPhone        string  `json:"contactPhone"`
	WhatsAppSentCount   int64   `json:"whatsAppSentCount"`
	WhatsAppBalance     *int32  `json:"whatsAppBalance"`
	IsActive            bool    `json:"isActive"`
	CreatedDate         string  `json:"createdDate"`
	UpdatedDate         *string `json:"updatedDate"`
	CreatedBy           *int64  `json:"createdBy"`
	UpdatedBy           *int64  `json:"updatedBy"`
	SubscriptionEndDate *string `json:"subscriptionEndDate"`
	SubscriptionStatus  string  `json:"subscriptionStatus"`
}

type dashboardStatsDTO struct {
	TotalTenants  int64 `json:"totalTenants"`
	ActiveTenants int64 `json:"activeTenants"`
	TotalBalance  int64 `json:"totalBalance"`
}

type createTenantDTO struct {
	CompanyName  string `json:"companyName" validate:"required"`
	ContactPhone string `json:"contactPhone"`
}

type addBalanceRequest struct {
	AmountToAdd int32 `json:"amountToAdd"`
}

type setStatusRequest struct {
	IsActive bool `json:"isActive"`
}

// rowToDTO maps a ListTenants join row (carries the latest subscription) to the
// FE DTO.
func rowToDTO(t public.ListTenantsRow) tenantDTO {
	return tenantDTO{
		TenantID:            t.TenantID,
		CompanyName:         conv.Str(t.CompanyName),
		TenantName:          t.TenantName,
		ContactPhone:        conv.Str(t.ContactPhone),
		WhatsAppSentCount:   t.WhatsappSentCount,
		WhatsAppBalance:     t.WhatsappBalance,
		IsActive:            t.IsActive,
		CreatedDate:         conv.FmtDateTime(t.CreatedAt),
		UpdatedDate:         conv.FmtDate(t.UpdatedAt),
		CreatedBy:           t.CreatedBy,
		UpdatedBy:           t.UpdatedBy,
		SubscriptionEndDate: conv.PtrStr(t.SubscriptionEndDate), // 'YYYY-MM-DD' or "" -> nil
		SubscriptionStatus:  t.SubscriptionStatus,
	}
}

// modelToDTO maps the base Tenant model (no subscription columns) to the FE DTO.
// Used by the create/update responses that return the bare tenants row.
func modelToDTO(t public.Tenant) tenantDTO {
	return tenantDTO{
		TenantID:          t.TenantID,
		CompanyName:       conv.Str(t.CompanyName),
		TenantName:        t.TenantName,
		ContactPhone:      conv.Str(t.ContactPhone),
		WhatsAppSentCount: t.WhatsappSentCount,
		WhatsAppBalance:   t.WhatsappBalance,
		IsActive:          t.IsActive,
		CreatedDate:       conv.FmtDateTime(t.CreatedAt),
		UpdatedDate:       conv.FmtDate(t.UpdatedAt),
		CreatedBy:         t.CreatedBy,
		UpdatedBy:         t.UpdatedBy,
	}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := web.PageFromQuery(r)

	rows, err := h.q.ListTenants(ctx, public.ListTenantsParams{
		Filter: page.FilterValue(), Lim: page.Limit(), Off: page.Offset(),
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	total, err := h.q.CountTenants(ctx, page.FilterValue())
	if err != nil {
		h.fail(w, err)
		return
	}
	items := make([]tenantDTO, len(rows))
	for i, t := range rows {
		items[i] = rowToDTO(t)
	}
	apiresp.List(w, items, total, "")
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := h.q.GetTenantStats(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, dashboardStatsDTO{
		TotalTenants:  s.TotalTenants,
		ActiveTenants: s.ActiveTenants,
		TotalBalance:  s.TotalBalance,
	}, "")
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createTenantDTO
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	userID := reqctx.MustUserID(ctx)
	t, err := h.tm.ProvisionNewTenant(ctx, tenancy.NewTenant{
		TenantName:   req.CompanyName,
		CompanyName:  req.CompanyName,
		ContactPhone: req.ContactPhone,
		CreatedBy:    &userID,
	})
	if err != nil {
		h.fail(w, err)
		return
	}

	// 1. Give the creating user the Owner role (or Admin if Owner is not seeded, assuming Owner)
	role, err := h.q.GetRoleByName(ctx, "Owner")
	if err != nil {
		role, err = h.q.GetRoleByName(ctx, "Admin")
		if err != nil {
			h.fail(w, fmt.Errorf("getting admin/owner role: %w", err))
			return
		}
	}

	_, err = h.q.AddUserToTenant(ctx, public.AddUserToTenantParams{
		UserID:   userID,
		TenantID: t.TenantID,
		RoleID:   &role.RoleID,
		Status:   "Active",
		IsOwner:  true,
	})
	if err != nil {
		h.fail(w, fmt.Errorf("adding user to tenant: %w", err))
		return
	}

	// 2. Assign the user to the default branch created by migrations
	err = h.tm.InTenantTxByID(ctx, t.TenantID, func(tq *tenant.Queries) error {
		branches, err := tq.ListBranches(ctx)
		if err != nil {
			return fmt.Errorf("listing branches: %w", err)
		}
		
		var branchID uuid.UUID
		if len(branches) > 0 {
			branchID = branches[0].ID
		} else {
			branchCode := "MAIN"
			branchAddr := "Head Office"
			b, err := tq.CreateBranch(ctx, tenant.CreateBranchParams{
				Name:    "Main Branch",
				Code:    &branchCode,
				Address: &branchAddr,
				Status:  "Active",
			})
			if err != nil {
				return fmt.Errorf("creating default branch: %w", err)
			}
			branchID = b.ID
		}
		
		return tq.AssignUserToBranch(ctx, tenant.AssignUserToBranchParams{
			UserID:   userID,
			BranchID: branchID,
		})
	})
	if err != nil {
		h.fail(w, err)
		return
	}

	// FE expects the new tenant id (a number) as the created payload.
	apiresp.Created(w, t.TenantID, "Tenant created")
}

func (h *Handler) addBalance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID, err := web.ParamInt64(chi.URLParam(r, "tenantId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid tenantId")
		return
	}
	var req addBalanceRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	amount := req.AmountToAdd
	t, err := h.q.AddTenantWhatsappBalance(ctx, public.AddTenantWhatsappBalanceParams{
		AmountToAdd: &amount,
		TenantID:    tenantID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Tenant not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, modelToDTO(t), "Balance updated")
}

func (h *Handler) setStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID, err := web.ParamInt64(chi.URLParam(r, "tenantId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid tenantId")
		return
	}
	var req setStatusRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	if err := h.q.UpdateTenantStatus(ctx, public.UpdateTenantStatusParams{
		IsActive: req.IsActive,
		TenantID: tenantID,
	}); err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, true, "Status updated")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("Tenants handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

// ensure context import is used even if a future refactor drops it
var _ = context.Background
