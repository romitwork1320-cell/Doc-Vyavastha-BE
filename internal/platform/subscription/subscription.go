// Package subscription implements /api/Subscription (platform-scoped billing).
//
// Subscription plans, promo codes, tenant subscriptions and payments all live in
// the public schema and are global/platform-owned, so this module talks directly
// to public.Queries (built from the shared pgx pool). Tenant-owned reads/writes
// are scoped by reqctx.TenantID. Money columns are Postgres NUMERIC, surfaced to
// sqlc as pgtype.Numeric; the FE speaks plain JSON numbers, so the numericToFloat
// / floatToNumeric helpers bridge the two.
package subscription

import (
	"crypto/rand"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/web"
)

// Handler serves the Subscription endpoints.
type Handler struct {
	q      *public.Queries
	logger *slog.Logger
}

// New builds the handler from the shared pgx pool.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	return &Handler{q: public.New(pool), logger: logger}
}

// Mount registers routes under the platform router.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/Subscription", func(r chi.Router) {
		r.Get("/plans", h.listPlans)
		r.Get("/current/{tenantId}", h.current)
		r.Post("/validate-promo", h.validatePromo)
		r.Post("/manual-payment", h.manualPayment)
		r.Post("/extend-trial", h.extendTrial)
		r.Post("/generate-promo", h.generatePromo)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

// planDto mirrors the FE Plan model.
type planDto struct {
	PlanID          int64   `json:"planId"`
	PlanName        string  `json:"planName"`
	Price           float64 `json:"price"`
	DurationMonths  int32   `json:"durationMonths"`
	DurationDays    int32   `json:"durationDays"`
	WhatsAppCredits int32   `json:"whatsAppCredits"`
	IsTrial         bool    `json:"isTrial"`
	IsActive        bool    `json:"isActive"`
}

// subscriptionDto mirrors the FE TenantSubscription model.
type subscriptionDto struct {
	SubscriptionID int64  `json:"subscriptionId"`
	TenantID       int64  `json:"tenantId"`
	PlanID         int64  `json:"planId"`
	StartDate      string `json:"startDate"`
	EndDate        string `json:"endDate"`
	Status         string `json:"status"`
}

// promoValidationDto is the result of validating a promo against an amount.
type promoValidationDto struct {
	Discount      float64 `json:"discount"`
	FinalAmount   float64 `json:"finalAmount"`
	PromoID       int64   `json:"promoId"`
	ExtensionDays int32   `json:"extensionDays"`
}

// promoDto mirrors the created promo code returned to the FE.
type promoDto struct {
	PromoID       int64   `json:"promoId"`
	Code          string  `json:"code"`
	DiscountType  string  `json:"discountType"`
	DiscountValue float64 `json:"discountValue"`
	ExtensionDays int32   `json:"extensionDays"`
	MaxUses       *int32  `json:"maxUses"`
	UsedCount     int32   `json:"usedCount"`
	IsActive      bool    `json:"isActive"`
}

// ── Request bodies ───────────────────────────────────────────────────────────

type validatePromoRequest struct {
	Code           string  `json:"code" validate:"required"`
	OriginalAmount float64 `json:"originalAmount"`
	TenantID       int64   `json:"tenantId"`
}

type manualPaymentRequest struct {
	TenantID        int64   `json:"tenantId" validate:"required"`
	PlanID          int64   `json:"planId" validate:"required"`
	PromoCodeID     *int64  `json:"promoCodeId"`
	OriginalAmount  float64 `json:"originalAmount"`
	DiscountAmount  float64 `json:"discountAmount"`
	FinalAmount     float64 `json:"finalAmount"`
	PaymentMode     string  `json:"paymentMode"`
	ReferenceNumber string  `json:"referenceNumber"`
}

type extendTrialRequest struct {
	TenantID  int64  `json:"tenantId" validate:"required"`
	PlanID    int64  `json:"planId" validate:"required"`
	PromoCode string `json:"promoCode"`
}

type generatePromoRequest struct {
	Code          string  `json:"code"`
	DiscountType  string  `json:"discountType"`
	DiscountValue float64 `json:"discountValue"`
	ExtensionDays int32   `json:"extensionDays"`
	MaxUses       *int32  `json:"maxUses"`
}

// ── Handlers ─────────────────────────────────────────────────────────────────

// listPlans returns all active subscription plans. The FE reads `data` as a
// Plan[], so we render the slice directly with apiresp.OK.
func (h *Handler) listPlans(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListActivePlans(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	items := make([]planDto, len(rows))
	for i, p := range rows {
		items[i] = toPlanDto(p)
	}
	apiresp.OK(w, items, "")
}

// current returns the tenant's latest subscription. When the tenant has none we
// return a successful null body so the FE can render its "no subscription" state.
func (h *Handler) current(w http.ResponseWriter, r *http.Request) {
	tenantID, err := web.ParamInt64(chi.URLParam(r, "tenantId"))
	if err != nil {
		apiresp.BadRequest(w, "invalid tenant id")
		return
	}
	sub, err := h.q.GetCurrentSubscription(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiresp.OK(w, nil, "")
			return
		}
		h.fail(w, err)
		return
	}
	apiresp.OK(w, toSubscriptionDto(sub), "")
}

// validatePromo looks up a promo code, confirms it is usable, and computes the
// discount against the supplied original amount.
func (h *Handler) validatePromo(w http.ResponseWriter, r *http.Request) {
	var req validatePromoRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	promo, err := h.q.GetPromoByCode(r.Context(), req.Code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiresp.Fail(w, http.StatusBadRequest, "Invalid promo code")
			return
		}
		h.fail(w, err)
		return
	}
	if !promoUsable(promo) {
		apiresp.Fail(w, http.StatusBadRequest, "Invalid promo code")
		return
	}

	discount := computeDiscount(promo, req.OriginalAmount)
	apiresp.OK(w, promoValidationDto{
		Discount:      discount,
		FinalAmount:   req.OriginalAmount - discount,
		PromoID:       promo.PromoID,
		ExtensionDays: promo.ExtensionDays,
	}, "")
}

// manualPayment records a manual (offline) payment, then creates/extends the
// tenant's subscription for the purchased plan's duration. If a promo was used
// its usage counter is incremented.
func (h *Handler) manualPayment(w http.ResponseWriter, r *http.Request) {
	var req manualPaymentRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	ctx := r.Context()

	plan, err := h.q.GetPlan(ctx, req.PlanID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiresp.NotFound(w, "Plan not found")
			return
		}
		h.fail(w, err)
		return
	}

	planID := req.PlanID
	if _, err := h.q.CreateTenantPayment(ctx, public.CreateTenantPaymentParams{
		TenantID:        req.TenantID,
		PlanID:          &planID,
		PromoCodeID:     req.PromoCodeID,
		OriginalAmount:  floatToNumeric(req.OriginalAmount),
		DiscountAmount:  floatToNumeric(req.DiscountAmount),
		FinalAmount:     floatToNumeric(req.FinalAmount),
		PaymentMode:     ptrStr(req.PaymentMode),
		ReferenceNumber: ptrStr(req.ReferenceNumber),
	}); err != nil {
		h.fail(w, err)
		return
	}

	start := time.Now()
	end := addPlanDuration(start, plan, 0)
	if _, err := h.q.CreateTenantSubscription(ctx, public.CreateTenantSubscriptionParams{
		TenantID:  req.TenantID,
		PlanID:    req.PlanID,
		StartDate: start,
		EndDate:   end,
		Status:    "Active",
	}); err != nil {
		h.fail(w, err)
		return
	}

	if req.PromoCodeID != nil {
		if err := h.q.IncrementPromoUse(ctx, *req.PromoCodeID); err != nil {
			h.fail(w, err)
			return
		}
	}
	apiresp.OK(w, true, "Payment recorded")
}

// extendTrial creates a trial subscription for a plan, optionally extended by a
// promo code's extension days.
func (h *Handler) extendTrial(w http.ResponseWriter, r *http.Request) {
	var req extendTrialRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	ctx := r.Context()

	plan, err := h.q.GetPlan(ctx, req.PlanID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apiresp.NotFound(w, "Plan not found")
			return
		}
		h.fail(w, err)
		return
	}

	var extraDays int32
	if req.PromoCode != "" {
		promo, err := h.q.GetPromoByCode(ctx, req.PromoCode)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				apiresp.Fail(w, http.StatusBadRequest, "Invalid promo code")
				return
			}
			h.fail(w, err)
			return
		}
		if !promoUsable(promo) {
			apiresp.Fail(w, http.StatusBadRequest, "Invalid promo code")
			return
		}
		extraDays = promo.ExtensionDays
	}

	start := time.Now()
	end := addPlanDuration(start, plan, extraDays)
	if _, err := h.q.CreateTenantSubscription(ctx, public.CreateTenantSubscriptionParams{
		TenantID:  req.TenantID,
		PlanID:    req.PlanID,
		StartDate: start,
		EndDate:   end,
		Status:    "Active",
	}); err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, true, "Trial activated")
}

// generatePromo creates a new promo code. When no code is supplied a random
// 8-char uppercase alphanumeric code is generated.
func (h *Handler) generatePromo(w http.ResponseWriter, r *http.Request) {
	var req generatePromoRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}

	code := req.Code
	if code == "" {
		code = randomCode(8)
	}
	discountType := req.DiscountType
	if discountType == "" {
		discountType = "percent"
	}

	promo, err := h.q.CreatePromo(r.Context(), public.CreatePromoParams{
		Code:          code,
		DiscountType:  discountType,
		DiscountValue: floatToNumeric(req.DiscountValue),
		ExtensionDays: req.ExtensionDays,
		MaxUses:       req.MaxUses,
		ValidFrom:     nil,
		ValidTo:       nil,
		IsActive:      true,
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.Created(w, toPromoDto(promo), "Promo created")
}

// ── Mapping helpers ──────────────────────────────────────────────────────────

func toPlanDto(p public.SubscriptionPlan) planDto {
	return planDto{
		PlanID:          p.PlanID,
		PlanName:        p.PlanName,
		Price:           numericToFloat(p.Price),
		DurationMonths:  p.DurationMonths,
		DurationDays:    p.DurationDays,
		WhatsAppCredits: p.WhatsappCredits,
		IsTrial:         p.IsTrial,
		IsActive:        p.IsActive,
	}
}

func toSubscriptionDto(s public.TenantSubscription) subscriptionDto {
	return subscriptionDto{
		SubscriptionID: s.SubscriptionID,
		TenantID:       s.TenantID,
		PlanID:         s.PlanID,
		StartDate:      s.StartDate.Format("2006-01-02"),
		EndDate:        s.EndDate.Format("2006-01-02"),
		Status:         s.Status,
	}
}

func toPromoDto(p public.PromoCode) promoDto {
	return promoDto{
		PromoID:       p.PromoID,
		Code:          p.Code,
		DiscountType:  p.DiscountType,
		DiscountValue: numericToFloat(p.DiscountValue),
		ExtensionDays: p.ExtensionDays,
		MaxUses:       p.MaxUses,
		UsedCount:     p.UsedCount,
		IsActive:      p.IsActive,
	}
}

// ── Business helpers ─────────────────────────────────────────────────────────

// promoUsable reports whether a promo can currently be applied: active, within
// its validity window, and below its max-use cap.
func promoUsable(p public.PromoCode) bool {
	if !p.IsActive {
		return false
	}
	now := time.Now()
	if p.ValidFrom != nil && now.Before(*p.ValidFrom) {
		return false
	}
	if p.ValidTo != nil && now.After(*p.ValidTo) {
		return false
	}
	if p.MaxUses != nil && p.UsedCount >= *p.MaxUses {
		return false
	}
	return true
}

// computeDiscount derives the money discount for a promo against an amount.
// Percent promos take value% of the amount; any other (flat) promo takes the
// flat value, capped at the original amount.
func computeDiscount(p public.PromoCode, original float64) float64 {
	value := numericToFloat(p.DiscountValue)
	var discount float64
	if p.DiscountType == "percent" {
		discount = original * value / 100
	} else {
		discount = value
	}
	if discount > original {
		discount = original
	}
	if discount < 0 {
		discount = 0
	}
	return discount
}

// addPlanDuration returns start advanced by the plan's duration (months treated
// as 30 days each + explicit days) plus any extra promo days.
func addPlanDuration(start time.Time, plan public.SubscriptionPlan, extraDays int32) time.Time {
	days := int(plan.DurationMonths)*30 + int(plan.DurationDays) + int(extraDays)
	return start.AddDate(0, 0, days)
}

// ── pgtype.Numeric <-> float64 bridges ───────────────────────────────────────

// numericToFloat converts a (possibly null) NUMERIC into a plain float64.
func numericToFloat(n pgtype.Numeric) float64 {
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}

// floatToNumeric converts a float64 into a NUMERIC for insertion.
func floatToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	// ScanScientific accepts a plain decimal string and never returns an error
	// for a well-formed float; ignore the error and fall back to a zero value.
	_ = n.ScanScientific(strconv.FormatFloat(f, 'f', -1, 64))
	return n
}

// ── small misc helpers ───────────────────────────────────────────────────────

// ptrStr returns nil for an empty string, else a pointer to it (nullable column).
func ptrStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

const promoAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomCode returns an n-char uppercase alphanumeric code using crypto/rand.
func randomCode(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is effectively impossible; fall back deterministically.
		for i := range b {
			b[i] = promoAlphabet[i%len(promoAlphabet)]
		}
		return string(b)
	}
	for i := range b {
		b[i] = promoAlphabet[int(b[i])%len(promoAlphabet)]
	}
	return string(b)
}

// fail logs an unexpected error and returns a generic 500.
func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("Subscription handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}
