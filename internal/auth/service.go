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

	"github.com/thinkparq/edconsultancy-be/internal/config"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
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

func (s *Service) userTenants(ctx context.Context, userID int64) ([]WorkspaceSelection, error) {
	rows, err := s.q.ListUserTenantsForSelection(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]WorkspaceSelection, 0, len(rows))
	for _, r := range rows {
		active := r.IsActive != nil && *r.IsActive
		out = append(out, WorkspaceSelection{
			TenantID:      &r.TenantID,
			WorkspaceType: "ORGANIZATION",
			TenantName:    r.TenantName,
			Role:          r.Role,
			IsActive:      active,
		})
	}
	return out, nil
}

func (s *Service) issueFinalSession(ctx context.Context, userID int64, tenantID *int64, workspaceType, ua, ip string) (access, refreshRaw string, err error) {
	if workspaceType == "PERSONAL" {
		access, err = s.issuer.IssueClientAccess(userID)
		if err != nil {
			return "", "", err
		}
		refreshRaw, err = randomToken()
		if err != nil {
			return "", "", err
		}
		tid := int64(0)
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
	} else if workspaceType == "SYSTEM" {
		access, err = s.issuer.IssueSuperAdminAccess(userID)
		if err != nil {
			return "", "", err
		}
		refreshRaw, err = randomToken()
		if err != nil {
			return "", "", err
		}
		if _, err := s.q.InsertRefreshToken(ctx, public.InsertRefreshTokenParams{
			UserID:    userID,
			TenantID:  nil,
			TokenHash: hashToken(refreshRaw),
			UserAgent: strPtr(ua),
			IpAddress: strPtr(ip),
			ExpiresAt: time.Now().Add(s.cfg.JWTRefreshTTL),
		}); err != nil {
			return "", "", err
		}
		return access, refreshRaw, nil
	} else if workspaceType == "ORGANIZATION" && tenantID != nil {
		return s.buildSession(ctx, userID, *tenantID, ua, ip)
	}
	return "", "", biz(http.StatusBadRequest, "Invalid workspace type")
}

func (s *Service) evaluateWorkspaces(ctx context.Context, userID int64, ua, ip string) (AuthResponse, string, error) {
	u, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return AuthResponse{}, "", err
	}
	if u.IsSuperadmin {
		access, refreshRaw, err := s.issueFinalSession(ctx, userID, nil, "SYSTEM", ua, ip)
		if err != nil {
			return AuthResponse{}, "", err
		}
		return AuthResponse{
			NextStep:    "DONE",
			AccessToken: access,
			Workspaces:  nil,
		}, refreshRaw, nil
	}

	tenants, err := s.userTenants(ctx, userID)
	if err != nil {
		return AuthResponse{}, "", err
	}
	var active []WorkspaceSelection
	for _, t := range tenants {
		if t.IsActive {
			active = append(active, t)
		}
	}
	_, err = s.q.GetClientProfileByUserId(ctx, userID)
	if err == nil {
		active = append(active, WorkspaceSelection{
			TenantID:      nil,
			WorkspaceType: "PERSONAL",
			TenantName:    "Personal Workspace",
			Role:          "Client",
			IsActive:      true,
		})
	}

	switch len(active) {
	case 0:
		temp, err := s.issuer.IssueTemp(userID)
		if err != nil {
			return AuthResponse{}, "", err
		}
		return AuthResponse{NextStep: "CREATE_WORKSPACE", TempToken: temp, Workspaces: nil}, "", nil
	default:
		temp, err := s.issuer.IssueTemp(userID)
		if err != nil {
			return AuthResponse{}, "", err
		}
		return AuthResponse{NextStep: "SELECT_WORKSPACE", TempToken: temp, Workspaces: active}, "", nil
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

func (s *Service) Login(ctx context.Context, req loginRequest, ua, ip string) (AuthResponse, string, error) {
	u, err := s.q.GetUserByEmail(ctx, req.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthResponse{}, "", biz(http.StatusNotFound, "IDENTITY_NOT_FOUND")
	}
	if err != nil {
		return AuthResponse{}, "", err
	}
	if u.PasswordHash == nil {
		return AuthResponse{}, "", biz(http.StatusUnauthorized, "Account uses external login (e.g. Google)")
	}
	if !CheckPasswordHash(req.Password, *u.PasswordHash) {
		return AuthResponse{}, "", biz(http.StatusUnauthorized, "Invalid email or password")
	}

	return s.evaluateWorkspaces(ctx, u.ID, ua, ip)
}

func (s *Service) RegisterIdentity(ctx context.Context, req registerIdentityRequest, ua, ip string) (AuthResponse, string, error) {
	_, err := s.q.GetUserByEmail(ctx, req.Email)
	if err == nil {
		return AuthResponse{}, "", biz(http.StatusConflict, "IDENTITY_ALREADY_EXISTS")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return AuthResponse{}, "", err
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		return AuthResponse{}, "", err
	}

	u, err := s.q.CreateUser(ctx, public.CreateUserParams{
		Email:        req.Email,
		PasswordHash: &hash,
		IsActive:     true,
		IsVerified:   true,
	})
	if err != nil {
		return AuthResponse{}, "", err
	}

	return s.evaluateWorkspaces(ctx, u.ID, ua, ip)
}



// SelectTenant issues a final session for the chosen tenant.
func (s *Service) SelectTenant(ctx context.Context, userID int64, req selectTenantRequest, ua, ip string) (AuthResponse, string, error) {
	access, refreshRaw, err := s.issueFinalSession(ctx, userID, req.TenantID, req.WorkspaceType, ua, ip)
	if err != nil {
		return AuthResponse{}, "", err
	}
	return AuthResponse{NextStep: "ENTER_WORKSPACE", AccessToken: access}, refreshRaw, nil
}

// Refresh rotates the refresh token and mints a new access token.
func (s *Service) Refresh(ctx context.Context, raw, ua, ip string) (AuthResponse, string, error) {
	if raw == "" {
		return AuthResponse{}, "", biz(http.StatusUnauthorized, "Missing refresh token")
	}
	row, err := s.q.GetRefreshTokenByHash(ctx, hashToken(raw))
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthResponse{}, "", biz(http.StatusUnauthorized, "Invalid refresh token")
	}
	if err != nil {
		return AuthResponse{}, "", err
	}
	_ = s.q.RevokeRefreshToken(ctx, hashToken(raw))
	
	if row.TenantID == nil {
		access, newRaw, err := s.issueFinalSession(ctx, row.UserID, nil, "SYSTEM", ua, ip)
		if err != nil {
			return AuthResponse{}, "", err
		}
		return AuthResponse{AccessToken: access}, newRaw, nil
	} else if *row.TenantID == 0 {
		access, newRaw, err := s.issueFinalSession(ctx, row.UserID, nil, "PERSONAL", ua, ip)
		if err != nil {
			return AuthResponse{}, "", err
		}
		return AuthResponse{AccessToken: access}, newRaw, nil
	}

	access, newRaw, err := s.buildSession(ctx, row.UserID, *row.TenantID, ua, ip)
	if err != nil {
		return AuthResponse{}, "", err
	}
	return AuthResponse{AccessToken: access}, newRaw, nil
}

// Logout revokes the presented refresh token.
func (s *Service) Logout(ctx context.Context, raw string) {
	if raw != "" {
		_ = s.q.RevokeRefreshToken(ctx, hashToken(raw))
	}
}

// GoogleAuth authenticates (or provisions) a user via a Google ID token.
func (s *Service) GoogleAuth(ctx context.Context, idToken, ua, ip string) (AuthResponse, string, error) {
	payload, err := idtoken.Validate(ctx, idToken, s.cfg.GoogleClientID)
	if err != nil {
		return AuthResponse{}, "", biz(http.StatusUnauthorized, "Invalid Google token")
	}
	gmail, _ := payload.Claims["email"].(string)
	if gmail == "" {
		return AuthResponse{}, "", biz(http.StatusBadRequest, "Google account has no email")
	}
	sub := payload.Subject

	u, err := s.q.GetUserByEmail(ctx, gmail)
	if errors.Is(err, pgx.ErrNoRows) {
		nu, cerr := s.q.CreateUser(ctx, public.CreateUserParams{Email: gmail, GoogleID: &sub, IsActive: true, IsVerified: true})
		if cerr != nil {
			return AuthResponse{}, "", cerr
		}
		u = nu
	} else if err != nil {
		return AuthResponse{}, "", err
	} else if u.GoogleID == nil {
		_ = s.q.SetUserGoogleID(ctx, public.SetUserGoogleIDParams{GoogleID: &sub, ID: u.ID})
	}
	
	return s.evaluateWorkspaces(ctx, u.ID, ua, ip)
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
func (s *Service) VerifyOTP(ctx context.Context, email, code, ua, ip string) (AuthResponse, string, error) {
	otp, err := s.q.GetLatestOTP(ctx, public.GetLatestOTPParams{Email: email, Purpose: "login"})
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthResponse{}, "", biz(http.StatusBadRequest, "No code was requested for this email")
	}
	if err != nil {
		return AuthResponse{}, "", err
	}
	if time.Now().After(otp.ExpiresAt) {
		return AuthResponse{}, "", biz(http.StatusBadRequest, "Code has expired")
	}
	if otp.Attempts >= 5 {
		return AuthResponse{}, "", biz(http.StatusTooManyRequests, "Too many attempts; request a new code")
	}
	if hashToken(code) != otp.CodeHash {
		_ = s.q.IncrementOTPAttempts(ctx, otp.ID)
		return AuthResponse{}, "", biz(http.StatusBadRequest, "Invalid code")
	}
	_ = s.q.ConsumeOTP(ctx, otp.ID)

	u, err := s.q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		u, err = s.q.CreateUser(ctx, public.CreateUserParams{Email: email, IsActive: true, IsVerified: true})
	}
	if err != nil {
		return AuthResponse{}, "", err
	}

	return s.evaluateWorkspaces(ctx, u.ID, ua, ip)
}

func (s *Service) CreateWorkspace(ctx context.Context, userID int64, req createWorkspaceRequest, ua, ip string) (AuthResponse, string, error) {
	var tenantID *int64
	tenantName := req.Name
	roleName := "Owner"

	if req.WorkspaceType == "PERSONAL" {
		clientProfile, err := s.q.GetClientProfileByUserId(ctx, userID)
		if err == nil && clientProfile.ConnectionCode != "" {
			return AuthResponse{}, "", biz(http.StatusConflict, "Personal account already exists")
		}

		var connCode string
		for i := 0; i < 5; i++ {
			code, cerr := generateConnectionCode(8)
			if cerr != nil {
				return AuthResponse{}, "", cerr
			}
			_, cerr = s.q.GetClientProfileByCode(ctx, code)
			if errors.Is(cerr, pgx.ErrNoRows) {
				connCode = code
				break
			}
		}
		if connCode == "" {
			return AuthResponse{}, "", biz(http.StatusInternalServerError, "Failed to generate unique connection code")
		}

		clientProfile, cerr := s.q.CreateClientProfile(ctx, public.CreateClientProfileParams{
			UserID:         userID,
			ConnectionCode: connCode,
			FullName:       strPtr(req.FirstName + " " + req.LastName),
		})
		if cerr != nil {
			return AuthResponse{}, "", cerr
		}
		
		tenantName = "Personal Workspace"
		roleName = "Client"

	} else if req.WorkspaceType == "ORGANIZATION" {
		var contactEmail, contactPhone string
		if req.Email != nil {
			contactEmail = *req.Email
		}
		if req.Phone != nil {
			contactPhone = *req.Phone
		}
		t, err := s.tm.ProvisionNewTenant(ctx, tenancy.NewTenant{
			TenantName:         req.Name,
			CompanyName:        req.Name,
			ContactPhone:       contactPhone,
			ContactEmail:       contactEmail,
			OrgType:            req.OrgType,
			OrganizationTypeID: req.OrgTypeId,
			CreatedBy:          &userID,
		})
		if err != nil {
			return AuthResponse{}, "", err
		}

		role, err := s.q.GetRoleByName(ctx, "Owner")
		if err != nil {
			role, err = s.q.GetRoleByName(ctx, "Admin")
			if err != nil {
				return AuthResponse{}, "", fmt.Errorf("getting admin/owner role: %w", err)
			}
		}

		_, err = s.q.AddUserToTenant(ctx, public.AddUserToTenantParams{
			UserID:   userID,
			TenantID: t.TenantID,
			RoleID:   &role.RoleID,
			Status:   "Active",
			IsOwner:  true,
		})
		if err != nil {
			return AuthResponse{}, "", err
		}
		
		_, _ = s.q.UpsertCompanyProfile(ctx, public.UpsertCompanyProfileParams{
			TenantID:     t.TenantID,
			CompanyName:  strPtr(req.Name),
			ContactEmail: strPtr(""),
			ContactPhone: strPtr(""),
		})

		tenantID = &t.TenantID
	} else {
		return AuthResponse{}, "", biz(http.StatusBadRequest, "Invalid workspace type")
	}

	_, _ = s.q.UpsertUserProfile(ctx, public.UpsertUserProfileParams{
		UserID:    userID,
		FirstName: strPtr(req.FirstName),
		LastName:  strPtr(req.LastName),
		JobTitle:  strPtr(""),
		ContactNumber: strPtr(""),
	})

	access, refreshRaw, err := s.issueFinalSession(ctx, userID, tenantID, req.WorkspaceType, ua, ip)
	if err != nil {
		return AuthResponse{}, "", err
	}

	currentWorkspace := WorkspaceSelection{
		TenantID:      tenantID,
		WorkspaceType: req.WorkspaceType,
		TenantName:    tenantName,
		Role:          roleName,
		IsActive:      true,
	}

	return AuthResponse{NextStep: "ENTER_WORKSPACE", AccessToken: access, CurrentWorkspace: &currentWorkspace}, refreshRaw, nil
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


