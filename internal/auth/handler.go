package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/config"
	"github.com/thinkparq/edconsultancy-be/internal/token"
	"github.com/thinkparq/edconsultancy-be/internal/web"
)

// Handler exposes the /Auth endpoints.
type Handler struct {
	svc    *Service
	cfg    *config.Config
	logger *slog.Logger
}

// NewHandler builds the auth Handler.
func NewHandler(svc *Service, cfg *config.Config, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, cfg: cfg, logger: logger}
}

// Mount registers all /Auth routes (this group is public — no auth middleware).
func (h *Handler) Mount(r chi.Router) {
	r.Route("/Auth", func(r chi.Router) {
		r.Post("/select-tenant", h.selectTenant)
		r.Post("/refresh-token", h.refresh)
		r.Post("/logout", h.logout)
		r.Post("/login", h.login)
		r.Post("/register-identity", h.registerIdentity)
		r.Post("/google-login", h.googleLogin)
		r.Post("/send-otp", h.sendOTP)
		r.Post("/verify-otp", h.verifyOTP)
		r.Post("/create-workspace", h.createWorkspace)
		r.Post("/resend-verification", h.resendVerification)
		r.Get("/verify-email", h.verifyEmail)
	})
}

func (h *Handler) selectTenant(w http.ResponseWriter, r *http.Request) {
	var req selectTenantRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	userID, err := h.svc.UserIDFromToken(bearer(r))
	if err != nil {
		apiresp.Unauthorized(w, "Invalid or expired token")
		return
	}
	resp, refreshRaw, err := h.svc.SelectTenant(r.Context(), userID, req, r.UserAgent(), clientIP(r))
	if err != nil {
		h.fail(w, err)
		return
	}
	setRefreshCookie(w, h.cfg, refreshRaw, h.cfg.JWTRefreshTTL)
	apiresp.OK(w, resp, "Workspace selected")
}

func (h *Handler) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var req createWorkspaceRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	userID, err := h.svc.UserIDFromToken(bearer(r))
	if err != nil {
		apiresp.Unauthorized(w, "Invalid or expired token")
		return
	}
	resp, refreshRaw, err := h.svc.CreateWorkspace(r.Context(), userID, req, r.UserAgent(), clientIP(r))
	if err != nil {
		h.fail(w, err)
		return
	}
	if refreshRaw != "" {
		setRefreshCookie(w, h.cfg, refreshRaw, h.cfg.JWTRefreshTTL)
	}
	apiresp.Created(w, resp, "Workspace created")
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	resp, newRaw, err := h.svc.Refresh(r.Context(), readRefreshCookie(r, h.cfg), r.UserAgent(), clientIP(r))
	if err != nil {
		clearRefreshCookie(w, h.cfg)
		h.fail(w, err)
		return
	}
	setRefreshCookie(w, h.cfg, newRaw, h.cfg.JWTRefreshTTL)
	apiresp.OK(w, resp, "Token refreshed")
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	h.svc.Logout(r.Context(), readRefreshCookie(r, h.cfg))
	clearRefreshCookie(w, h.cfg)
	apiresp.OK(w, nil, "Logged out")
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	resp, refreshRaw, err := h.svc.Login(r.Context(), req, r.UserAgent(), clientIP(r))
	if err != nil {
		h.fail(w, err)
		return
	}
	if refreshRaw != "" {
		setRefreshCookie(w, h.cfg, refreshRaw, h.cfg.JWTRefreshTTL)
	}
	apiresp.OK(w, resp, "Login successful")
}

func (h *Handler) registerIdentity(w http.ResponseWriter, r *http.Request) {
	var req registerIdentityRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	resp, refreshRaw, err := h.svc.RegisterIdentity(r.Context(), req, r.UserAgent(), clientIP(r))
	if err != nil {
		h.fail(w, err)
		return
	}
	if refreshRaw != "" {
		setRefreshCookie(w, h.cfg, refreshRaw, h.cfg.JWTRefreshTTL)
	}
	apiresp.OK(w, resp, "Identity registered")
}

func (h *Handler) googleLogin(w http.ResponseWriter, r *http.Request) {
	var req googleLoginRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	resp, refreshRaw, err := h.svc.GoogleAuth(r.Context(), req.IDToken, r.UserAgent(), clientIP(r))
	if err != nil {
		h.fail(w, err)
		return
	}
	if refreshRaw != "" {
		setRefreshCookie(w, h.cfg, refreshRaw, h.cfg.JWTRefreshTTL)
	}
	apiresp.OK(w, resp, "Google login successful")
}

func (h *Handler) sendOTP(w http.ResponseWriter, r *http.Request) {
	var req sendOtpRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	if err := h.svc.SendOTP(r.Context(), req.Email); err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, "sent", "OTP sent")
}

func (h *Handler) verifyOTP(w http.ResponseWriter, r *http.Request) {
	var req verifyOtpRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	resp, refreshRaw, err := h.svc.VerifyOTP(r.Context(), req.Email, req.OtpCode, r.UserAgent(), clientIP(r))
	if err != nil {
		h.fail(w, err)
		return
	}
	if refreshRaw != "" {
		setRefreshCookie(w, h.cfg, refreshRaw, h.cfg.JWTRefreshTTL)
	}
	apiresp.OK(w, resp, "OTP verified")
}



func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	var req resendVerificationRequest
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	if err := h.svc.ResendVerification(r.Context(), req.Email); err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, nil, "If the email exists and is unverified, a new link was sent.")
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.ParseInt(r.URL.Query().Get("userId"), 10, 64)
	tok := r.URL.Query().Get("token")
	if userID == 0 || tok == "" {
		apiresp.BadRequest(w, "Missing userId or token")
		return
	}
	if err := h.svc.VerifyEmail(r.Context(), userID, tok); err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, nil, "Email verified")
}

// fail renders a business error as a handled envelope, or logs+500s a system error.
func (h *Handler) fail(w http.ResponseWriter, err error) {
	var be *bizError
	if errors.As(err, &be) {
		apiresp.Fail(w, be.status, be.msg)
		return
	}
	h.logger.Error("auth handler error", "err", err)
	apiresp.ServerError(w, err.Error())
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
		return strings.TrimSpace(h[len(p):])
	}
	return ""
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if i := strings.IndexByte(ip, ','); i > 0 {
			return strings.TrimSpace(ip[:i])
		}
		return ip
	}
	return r.RemoteAddr
}

// compile-time: ensure token pkg is referenced (scope constant used in service).
var _ = token.ScopeSelectTenant
