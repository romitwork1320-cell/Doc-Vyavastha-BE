package auth

// ── Requests (exact field names the FE sends) ──────────────────────────────

type selectTenantRequest struct {
	TenantID   int64 `json:"tenantId" validate:"required"`
	RememberMe bool  `json:"rememberMe"`
}

type googleLoginRequest struct {
	IDToken    string `json:"idToken" validate:"required"`
	RememberMe bool   `json:"rememberMe"`
}

type googleSignupRequest struct {
	IDToken string `json:"idToken" validate:"required"`
}

type sendOtpRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type verifyOtpRequest struct {
	Email   string `json:"email" validate:"required,email"`
	OtpCode string `json:"otpCode" validate:"required"`
}

type completeProfileRequest struct {
	Email        string `json:"email" validate:"required,email"`
	FirstName    string `json:"firstName" validate:"required"`
	LastName     string `json:"lastName"`
	MobileNumber string `json:"mobileNumber"`
	CompanyName  string `json:"companyName" validate:"required"`
}

type resendVerificationRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ── Responses (exact field names the FE reads) ─────────────────────────────

type tenantSelection struct {
	TenantID   int64  `json:"tenantId"`
	TenantName string `json:"tenantName"`
	Role       string `json:"role"`
	IsActive   bool   `json:"isActive"`
}

type googleLoginResponse struct {
	RequiresSelection bool              `json:"requiresSelection"`
	Token             string            `json:"token,omitempty"`
	RefreshToken      string            `json:"refreshToken,omitempty"`
	Tenants           []tenantSelection `json:"tenants,omitempty"`
	IsProfileComplete *bool             `json:"isProfileComplete,omitempty"`
	IsNewUser         *bool             `json:"isNewUser,omitempty"`
}

type loginResponseData struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken,omitempty"`
}

type tokenResponseData struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
}

type otpLoginResponse struct {
	IsProfileComplete bool                 `json:"isProfileComplete"`
	IsNewUser         bool                 `json:"isNewUser"`
	AuthData          *loginResponseData   `json:"authData,omitempty"`
	TenantData        *googleLoginResponse `json:"tenantData,omitempty"`
}

func boolPtr(b bool) *bool { return &b }

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
