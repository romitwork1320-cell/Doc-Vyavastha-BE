// Package profile implements /api/Profile (platform-scoped).
//
// It serves the combined company + user profile the FE renders on the settings
// screen: a protected GET that loads both the per-tenant company profile and the
// per-user profile into a single FullProfileDto, a protected PUT that upserts
// both halves, and an UNAUTHENTICATED GET that exposes just a tenant's logo +
// company name (used by the login screen, hence MountPublic). Profile data lives
// in the public schema (keyed by tenant_id / user_id), so this module talks
// directly to public.Queries.
package profile

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/conv"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/web"
)

// Handler serves the Profile endpoints.
type Handler struct {
	q      *public.Queries
	logger *slog.Logger
}

// New builds the handler from the shared pgx pool.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	return &Handler{q: public.New(pool), logger: logger}
}

// Mount registers the tenant-protected routes under the platform router.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/Profile", func(r chi.Router) {
		r.Get("/", h.get)
		r.Put("/", h.update)
	})
}

// MountPublic registers the unauthenticated route (used by the login screen).
func (h *Handler) MountPublic(r chi.Router) {
	r.Get("/Profile/public-logo/{tenantId}", h.publicLogo)
}

// ── DTOs ────────────────────────────────────────────────────────────────────

// companyProfileDto carries the per-tenant company details. The odd-cased JSON
// keys (companyLogoURL, ciN_NO, msmE_NO) intentionally mirror the FE contract.
type companyProfileDto struct {
	CompanyName    string `json:"companyName"`
	CompanyLogoURL string `json:"companyLogoURL"`
	ContactEmail   string `json:"contactEmail"`
	ContactPhone   string `json:"contactPhone"`
	SupportPhone   string `json:"supportPhone"`
	Website        string `json:"website"`
	AddressLine1   string `json:"addressLine1"`
	AddressLine2   string `json:"addressLine2"`
	City           string `json:"city"`
	State          string `json:"state"`
	Country        string `json:"country"`
	PostalCode     string `json:"postalCode"`
	Gstin          string `json:"gstin"`
	Pan            string `json:"pan"`
	CinNo          string `json:"ciN_NO"`
	MsmeNo         string `json:"msmE_NO"`
}

// userProfileDto carries the per-user profile details.
type userProfileDto struct {
	FirstName     string `json:"firstName"`
	LastName      string `json:"lastName"`
	JobTitle      string `json:"jobTitle"`
	ContactNumber string `json:"contactNumber"`
}

// fullProfileDto is the combined payload for both GET and PUT.
type fullProfileDto struct {
	CompanyProfile companyProfileDto `json:"companyProfile"`
	UserProfile    userProfileDto    `json:"userProfile"`
}

func toCompanyDTO(c public.CompanyProfile) companyProfileDto {
	return companyProfileDto{
		CompanyName:    conv.Str(c.CompanyName),
		CompanyLogoURL: conv.Str(c.CompanyLogoUrl),
		ContactEmail:   conv.Str(c.ContactEmail),
		ContactPhone:   conv.Str(c.ContactPhone),
		SupportPhone:   conv.Str(c.SupportPhone),
		Website:        conv.Str(c.Website),
		AddressLine1:   conv.Str(c.AddressLine1),
		AddressLine2:   conv.Str(c.AddressLine2),
		City:           conv.Str(c.City),
		State:          conv.Str(c.State),
		Country:        conv.Str(c.Country),
		PostalCode:     conv.Str(c.PostalCode),
		Gstin:          conv.Str(c.Gstin),
		Pan:            conv.Str(c.Pan),
		CinNo:          conv.Str(c.CinNo),
		MsmeNo:         conv.Str(c.MsmeNo),
	}
}

func toUserDTO(u public.UserProfile) userProfileDto {
	return userProfileDto{
		FirstName:     conv.Str(u.FirstName),
		LastName:      conv.Str(u.LastName),
		JobTitle:      conv.Str(u.JobTitle),
		ContactNumber: conv.Str(u.ContactNumber),
	}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := reqctx.MustUserID(ctx)
	tenantID := reqctx.TenantID(ctx)

	// A missing row is a valid "not yet filled in" state, so treat ErrNoRows as
	// an empty zero struct rather than an error.
	user, err := h.q.GetUserProfile(ctx, userID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, err)
		return
	}
	company, err := h.q.GetCompanyProfile(ctx, tenantID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, err)
		return
	}

	out := fullProfileDto{
		CompanyProfile: toCompanyDTO(company),
		UserProfile:    toUserDTO(user),
	}
	apiresp.OK(w, out, "")
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := reqctx.MustUserID(ctx)
	tenantID := reqctx.TenantID(ctx)

	var req fullProfileDto
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}

	c := req.CompanyProfile
	u := req.UserProfile

	if _, err := h.q.UpsertUserProfile(ctx, public.UpsertUserProfileParams{
		UserID:        userID,
		FirstName:     conv.PtrStr(u.FirstName),
		LastName:      conv.PtrStr(u.LastName),
		JobTitle:      conv.PtrStr(u.JobTitle),
		ContactNumber: conv.PtrStr(u.ContactNumber),
	}); err != nil {
		h.fail(w, err)
		return
	}

	if _, err := h.q.UpsertCompanyProfile(ctx, public.UpsertCompanyProfileParams{
		TenantID:       tenantID,
		CompanyName:    conv.PtrStr(c.CompanyName),
		CompanyLogoUrl: conv.PtrStr(c.CompanyLogoURL),
		ContactEmail:   conv.PtrStr(c.ContactEmail),
		ContactPhone:   conv.PtrStr(c.ContactPhone),
		SupportPhone:   conv.PtrStr(c.SupportPhone),
		Website:        conv.PtrStr(c.Website),
		AddressLine1:   conv.PtrStr(c.AddressLine1),
		AddressLine2:   conv.PtrStr(c.AddressLine2),
		City:           conv.PtrStr(c.City),
		State:          conv.PtrStr(c.State),
		Country:        conv.PtrStr(c.Country),
		PostalCode:     conv.PtrStr(c.PostalCode),
		Gstin:          conv.PtrStr(c.Gstin),
		Pan:            conv.PtrStr(c.Pan),
		CinNo:          conv.PtrStr(c.CinNo),
		MsmeNo:         conv.PtrStr(c.MsmeNo),
	}); err != nil {
		h.fail(w, err)
		return
	}

	apiresp.OK(w, true, "Profile updated")
}

// publicLogoDto is the minimal, unauthenticated branding payload.
type publicLogoDto struct {
	LogoURL     string `json:"logoUrl"`
	CompanyName string `json:"companyName"`
}

func (h *Handler) publicLogo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID, err := web.ParamInt64(chi.URLParam(r, "tenantId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid tenantId")
		return
	}

	row, err := h.q.GetPublicTenantLogo(ctx, tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Unknown tenant: return an empty, nil-safe payload rather than an error.
		apiresp.OK(w, publicLogoDto{}, "")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}

	apiresp.OK(w, publicLogoDto{
		LogoURL:     conv.Str(row.LogoUrl),
		CompanyName: row.CompanyName,
	}, "")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("Profile handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

// ensure context import is used even if a future refactor drops it
var _ = context.Background
