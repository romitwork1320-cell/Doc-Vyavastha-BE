package auth

// ── Requests (exact field names the FE sends) ──────────────────────────────

type loginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type registerIdentityRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type googleLoginRequest struct {
	IDToken    string `json:"idToken" validate:"required"`
	RememberMe bool   `json:"rememberMe"`
}

type sendOtpRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type verifyOtpRequest struct {
	Email   string `json:"email" validate:"required,email"`
	OtpCode string `json:"otpCode" validate:"required"`
}

type createWorkspaceRequest struct {
	WorkspaceType string  `json:"workspaceType" validate:"required,oneof=PERSONAL ORGANIZATION"`
	Name          string  `json:"name"`
	FirstName     string  `json:"firstName" validate:"required"`
	LastName      string  `json:"lastName" validate:"required"`
	Email         *string `json:"email"`
	Phone         *string `json:"phone"`
	OrgType       *string `json:"orgType"` // legacy
	OrgTypeId     *int64  `json:"orgTypeId"`
} // FullName for PERSONAL, CompanyName for ORG

type selectTenantRequest struct { // used in selectTenant
	TenantID      *int64 `json:"tenantId"`
	WorkspaceType string `json:"workspaceType" validate:"required"`
	RememberMe    bool   `json:"rememberMe"`
}

type resendVerificationRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// ── Responses (exact field names the FE reads) ─────────────────────────────

type WorkspaceSelection struct {
	TenantID      *int64 `json:"tenantId"`
	WorkspaceType string `json:"workspaceType"`
	TenantName    string `json:"tenantName"`
	Role          string `json:"role"`
	IsActive      bool   `json:"isActive"`
}

type AuthResponse struct {
	NextStep         string               `json:"nextStep"`
	AccessToken      string               `json:"accessToken,omitempty"`
	TempToken        string               `json:"tempToken,omitempty"`
	RefreshToken     string               `json:"refreshToken,omitempty"`
	Workspaces       []WorkspaceSelection `json:"workspaces,omitempty"`
	CurrentWorkspace *WorkspaceSelection  `json:"currentWorkspace,omitempty"`
}

func boolPtr(b bool) *bool { return &b }

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
