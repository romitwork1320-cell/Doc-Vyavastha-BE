package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/api/idtoken"

	"github.com/google/uuid"
	"github.com/thinkparq/edconsultancy-be/internal/config"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/db/tenant"
	"github.com/thinkparq/edconsultancy-be/internal/email"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
	"github.com/thinkparq/edconsultancy-be/internal/token"
)

// bizError is a handled, FE-facing failure (rendered as 200 + success:false,
// except where the handler maps it to a real status).
type bizError struct {
	status int
	msg    string
}

func (e *bizError) Error() string      { return e.msg }
func biz(status int, msg string) error { return &bizError{status: status, msg: msg} }

// errNoWorkspace signals an authenticated user with no usable tenant. Callers
// translate it differently (login => error; google/otp => profile-incomplete).
var errNoWorkspace = &bizError{status: http.StatusForbidden, msg: "No active workspace assigned to this account"}

// Service implements the authentication flows.
type Service struct {
	pool   *pgxpool.Pool
	q      *public.Queries
	issuer *token.Issuer
	mailer email.Sender
	cfg    *config.Config
	tm     *tenancy.Manager
	logger *slog.Logger
}

// NewService builds the auth Service.
func NewService(pool *pgxpool.Pool, issuer *token.Issuer, mailer email.Sender, cfg *config.Config, tm *tenancy.Manager, logger *slog.Logger) *Service {
	return &Service{pool: pool, q: public.New(pool), issuer: issuer, mailer: mailer, cfg: cfg, tm: tm, logger: logger}
}

// ── Session helpers ────────────────────────────────────────────────────────

func (s *Service) buildSession(ctx context.Context, userID, tenantID int64, ua, ip string) (access, refreshRaw string, err error) {
	mem, err := s.q.GetMembership(ctx, public.GetMembershipParams{UserID: userID, TenantID: tenantID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", biz(http.StatusForbidden, "You do not have access to this workspace")
	}
	if err != nil {
		return "", "", err
	}
	urls, err := s.q.GetUserPagePermissions(ctx, public.GetUserPagePermissionsParams{UserID: userID, TenantID: tenantID})
	if err != nil {
		return "", "", err
	}
	perms := make([]token.PagePermission, 0, len(urls))
	for _, u := range urls {
		perms = append(perms, token.PagePermission{
			PageUrl:   u.RouteUrl,
			CanView:   u.CanView,
			CanAdd:    u.CanAdd,
			CanEdit:   u.CanEdit,
			CanDelete: u.CanDelete,
		})
	}

	access, err = s.issuer.IssueAccess(userID, tenantID, mem.RoleName, perms)
	if err != nil {
		return "", "", err
	}
	refreshRaw, err = randomToken()
	if err != nil {
		return "", "", err
	}
	tid := tenantID
	if _, err := s.q.InsertRefreshToken(ctx, public.InsertRefreshTokenParams{
		UserID:    userID,
		TenantID:  &tid,
		TokenHash: hashToken(refreshRaw),
		UserAgent: strPtr(ua),
		IpAddress: strPtr(ip),
		ExpiresAt: time.Now().Add(s.cfg.JWTRefreshTTL),
	}); err != nil {
		return "", "", err
	}
	return access, refreshRaw, nil
}

func (s *Service) userTenants(ctx context.Context, userID int64) ([]tenantSelection, error) {
	rows, err := s.q.ListUserTenantsForSelection(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]tenantSelection, 0, len(rows))
	for _, r := range rows {
		active := r.IsActive != nil && *r.IsActive
		out = append(out, tenantSelection{TenantID: r.TenantID, TenantName: r.TenantName, Role: r.Role, IsActive: active})
	}
	return out, nil
}

// finishAuth decides between final session (single active tenant), workspace
// selection (multiple), or errNoWorkspace (none).
func (s *Service) finishAuth(ctx context.Context, userID int64, ua, ip string) (googleLoginResponse, string, error) {
	tenants, err := s.userTenants(ctx, userID)
	if err != nil {
		return googleLoginResponse{}, "", err
	}
	var active []tenantSelection
	for _, t := range tenants {
		if t.IsActive {
			active = append(active, t)
		}
	}
	switch len(active) {
	case 0:
		return googleLoginResponse{}, "", errNoWorkspace
	case 1:
		access, refreshRaw, err := s.buildSession(ctx, userID, active[0].TenantID, ua, ip)
		if err != nil {
			return googleLoginResponse{}, "", err
		}
		return googleLoginResponse{RequiresSelection: false, Token: access, IsProfileComplete: boolPtr(true)}, refreshRaw, nil
	default:
		temp, err := s.issuer.IssueTemp(userID)
		if err != nil {
			return googleLoginResponse{}, "", err
		}
		return googleLoginResponse{RequiresSelection: true, Token: temp, Tenants: active, IsProfileComplete: boolPtr(true)}, "", nil
	}
}

// UserIDFromToken extracts the user id from a valid access/temp token (used by
// /Auth/select-tenant, which is called with the temp token as a bearer).
func (s *Service) UserIDFromToken(raw string) (int64, error) {
	if raw == "" {
		return 0, biz(http.StatusUnauthorized, "Missing token")
	}
	c, err := s.issuer.Parse(raw)
	if err != nil {
		return 0, biz(http.StatusUnauthorized, "Invalid or expired token")
	}
	return c.UserIDInt(), nil
}

// ── Flows ──────────────────────────────────────────────────────────────────

// SelectTenant issues a final session for the chosen tenant.
func (s *Service) SelectTenant(ctx context.Context, userID, tenantID int64, ua, ip string) (loginResponseData, string, error) {
	access, refreshRaw, err := s.buildSession(ctx, userID, tenantID, ua, ip)
	if err != nil {
		return loginResponseData{}, "", err
	}
	return loginResponseData{Token: access}, refreshRaw, nil
}

// Refresh rotates the refresh token and mints a new access token.
func (s *Service) Refresh(ctx context.Context, raw, ua, ip string) (tokenResponseData, string, error) {
	if raw == "" {
		return tokenResponseData{}, "", biz(http.StatusUnauthorized, "Missing refresh token")
	}
	row, err := s.q.GetRefreshTokenByHash(ctx, hashToken(raw))
	if errors.Is(err, pgx.ErrNoRows) {
		return tokenResponseData{}, "", biz(http.StatusUnauthorized, "Invalid refresh token")
	}
	if err != nil {
		return tokenResponseData{}, "", err
	}
	if row.TenantID == nil {
		return tokenResponseData{}, "", biz(http.StatusUnauthorized, "Invalid session")
	}
	_ = s.q.RevokeRefreshToken(ctx, hashToken(raw))
	access, newRaw, err := s.buildSession(ctx, row.UserID, *row.TenantID, ua, ip)
	if err != nil {
		return tokenResponseData{}, "", err
	}
	return tokenResponseData{AccessToken: access}, newRaw, nil
}

// Logout revokes the presented refresh token.
func (s *Service) Logout(ctx context.Context, raw string) {
	if raw != "" {
		_ = s.q.RevokeRefreshToken(ctx, hashToken(raw))
	}
}

// GoogleAuth authenticates (or provisions) a user via a Google ID token.
func (s *Service) GoogleAuth(ctx context.Context, idToken, ua, ip string) (googleLoginResponse, string, error) {
	payload, err := idtoken.Validate(ctx, idToken, s.cfg.GoogleClientID)
	if err != nil {
		return googleLoginResponse{}, "", biz(http.StatusUnauthorized, "Invalid Google token")
	}
	gmail, _ := payload.Claims["email"].(string)
	if gmail == "" {
		return googleLoginResponse{}, "", biz(http.StatusBadRequest, "Google account has no email")
	}
	sub := payload.Subject

	u, err := s.q.GetUserByEmail(ctx, gmail)
	if errors.Is(err, pgx.ErrNoRows) {
		nu, cerr := s.q.CreateUser(ctx, public.CreateUserParams{Email: gmail, GoogleID: &sub, IsActive: true, IsVerified: true})
		if cerr != nil {
			return googleLoginResponse{}, "", cerr
		}
		_ = nu
		// Brand new account → must complete profile (create a workspace).
		return googleLoginResponse{RequiresSelection: false, IsNewUser: boolPtr(true), IsProfileComplete: boolPtr(false)}, "", nil
	}
	if err != nil {
		return googleLoginResponse{}, "", err
	}
	if u.GoogleID == nil {
		_ = s.q.SetUserGoogleID(ctx, public.SetUserGoogleIDParams{GoogleID: &sub, ID: u.ID})
	}
	resp, refreshRaw, err := s.finishAuth(ctx, u.ID, ua, ip)
	if errors.Is(err, errNoWorkspace) {
		return googleLoginResponse{RequiresSelection: false, IsNewUser: boolPtr(false), IsProfileComplete: boolPtr(false)}, "", nil
	}
	return resp, refreshRaw, err
}

// SendOTP generates and emails a one-time login code.
func (s *Service) SendOTP(ctx context.Context, email string) error {
	code, err := numericOTP(s.cfg.OTPLength)
	if err != nil {
		return err
	}
	if _, err := s.q.InsertOTP(ctx, public.InsertOTPParams{
		Email:     email,
		CodeHash:  hashToken(code),
		Purpose:   "login",
		ExpiresAt: time.Now().Add(s.cfg.OTPTTL),
	}); err != nil {
		return err
	}
	body := fmt.Sprintf(`<p>Your EDConsultancy login code is <b>%s</b>. It expires in %d minutes.</p>`, code, int(s.cfg.OTPTTL.Minutes()))
	return s.mailer.Send(ctx, email, "Your login code", body)
}

// VerifyOTP validates a login code and establishes a session (or signals that
// the profile must be completed first).
func (s *Service) VerifyOTP(ctx context.Context, email, code, ua, ip string) (otpLoginResponse, string, error) {
	otp, err := s.q.GetLatestOTP(ctx, public.GetLatestOTPParams{Email: email, Purpose: "login"})
	if errors.Is(err, pgx.ErrNoRows) {
		return otpLoginResponse{}, "", biz(http.StatusBadRequest, "No code was requested for this email")
	}
	if err != nil {
		return otpLoginResponse{}, "", err
	}
	if time.Now().After(otp.ExpiresAt) {
		return otpLoginResponse{}, "", biz(http.StatusBadRequest, "Code has expired")
	}
	if otp.Attempts >= 5 {
		return otpLoginResponse{}, "", biz(http.StatusTooManyRequests, "Too many attempts; request a new code")
	}
	if hashToken(code) != otp.CodeHash {
		_ = s.q.IncrementOTPAttempts(ctx, otp.ID)
		return otpLoginResponse{}, "", biz(http.StatusBadRequest, "Invalid code")
	}
	_ = s.q.ConsumeOTP(ctx, otp.ID)

	isNew := false
	u, err := s.q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		u, err = s.q.CreateUser(ctx, public.CreateUserParams{Email: email, IsActive: true, IsVerified: true})
		isNew = true
	}
	if err != nil {
		return otpLoginResponse{}, "", err
	}

	resp, refreshRaw, err := s.finishAuth(ctx, u.ID, ua, ip)
	if errors.Is(err, errNoWorkspace) {
		return otpLoginResponse{IsProfileComplete: false, IsNewUser: true}, "", nil
	}
	if err != nil {
		return otpLoginResponse{}, "", err
	}
	return otpLoginResponse{IsProfileComplete: true, IsNewUser: isNew, TenantData: &resp}, refreshRaw, nil
}

// CompleteProfile finishes onboarding: records the profile, provisions the
// user's first workspace (tenant), and returns a final session.
func (s *Service) CompleteProfile(ctx context.Context, req completeProfileRequest, ua, ip string) (googleLoginResponse, string, error) {
	u, err := s.q.GetUserByEmail(ctx, req.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		u, err = s.q.CreateUser(ctx, public.CreateUserParams{Email: req.Email, IsActive: true, IsVerified: true})
	}
	if err != nil {
		return googleLoginResponse{}, "", err
	}

	if _, err := s.q.UpsertUserProfile(ctx, public.UpsertUserProfileParams{
		UserID:        u.ID,
		FirstName:     strPtr(req.FirstName),
		LastName:      strPtr(req.LastName),
		ContactNumber: strPtr(req.MobileNumber),
	}); err != nil {
		return googleLoginResponse{}, "", err
	}

	t, err := s.tm.ProvisionNewTenant(ctx, tenancy.NewTenant{
		TenantName:   req.CompanyName,
		CompanyName:  req.CompanyName,
		ContactPhone: req.MobileNumber,
		ContactEmail: req.Email,
		CreatedBy:    &u.ID,
	})
	if err != nil {
		return googleLoginResponse{}, "", err
	}
	role, err := s.q.EnsureRole(ctx, "Admin")
	if err != nil {
		return googleLoginResponse{}, "", err
	}
	if _, err := s.q.AddUserToTenant(ctx, public.AddUserToTenantParams{
		UserID: u.ID, TenantID: t.TenantID, RoleID: &role.RoleID, Status: "Active", IsOwner: true,
	}); err != nil {
		return googleLoginResponse{}, "", err
	}

	// Create and assign default branch
	err = s.tm.InTenantTxByID(ctx, t.TenantID, func(tq *tenant.Queries) error {
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
			UserID:   u.ID,
			BranchID: branchID,
		})
	})
	if err != nil {
		return googleLoginResponse{}, "", err
	}

	resp, refreshRaw, err := s.finishAuth(ctx, u.ID, ua, ip)
	if err != nil {
		return googleLoginResponse{}, "", err
	}
	resp.IsProfileComplete = boolPtr(true)
	return resp, refreshRaw, nil
}

// VerifyEmail marks the account verified using a one-time token.
func (s *Service) VerifyEmail(ctx context.Context, userID int64, rawToken string) error {
	at, err := s.q.GetAuthTokenByHash(ctx, public.GetAuthTokenByHashParams{TokenHash: hashToken(rawToken), Purpose: "verify_email"})
	if errors.Is(err, pgx.ErrNoRows) {
		return biz(http.StatusBadRequest, "Invalid or expired verification link")
	}
	if err != nil {
		return err
	}
	if at.UserID != userID {
		return biz(http.StatusBadRequest, "Token does not match user")
	}
	if err := s.q.MarkUserVerified(ctx, userID); err != nil {
		return err
	}
	return s.q.ConsumeAuthToken(ctx, at.ID)
}

// ResendVerification re-sends the verification email (no-ops silently if the
// email is unknown or already verified, to avoid account enumeration).
func (s *Service) ResendVerification(ctx context.Context, email string) error {
	u, err := s.q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if u.IsVerified {
		return nil
	}
	return s.sendVerification(ctx, u)
}

func (s *Service) sendVerification(ctx context.Context, u public.User) error {
	raw, err := randomToken()
	if err != nil {
		return err
	}
	if _, err := s.q.InsertAuthToken(ctx, public.InsertAuthTokenParams{
		UserID:    u.ID,
		TokenHash: hashToken(raw),
		Purpose:   "verify_email",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		return err
	}
	link := fmt.Sprintf("%s/verify-email?userId=%d&token=%s", s.cfg.ClientURL, u.ID, raw)
	body := fmt.Sprintf(`<p>Welcome to EDConsultancy! Please verify your email:</p><p><a href="%s">Verify my email</a></p>`, link)
	return s.mailer.Send(ctx, u.Email, "Verify your email", body)
}
