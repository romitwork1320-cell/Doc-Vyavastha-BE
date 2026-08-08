// Package middleware holds cross-cutting HTTP middleware (auth, audit).
package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
	"github.com/thinkparq/edconsultancy-be/internal/token"
)

// Auth validates the Bearer JWT and populates the request identity.
type Auth struct {
	issuer *token.Issuer
	tm     *tenancy.Manager
	logger *slog.Logger
	pool   *pgxpool.Pool
}

// NewAuth builds the auth middleware.
func NewAuth(issuer *token.Issuer, tm *tenancy.Manager, logger *slog.Logger, pool *pgxpool.Pool) *Auth {
	return &Auth{issuer: issuer, tm: tm, logger: logger, pool: pool}
}

// Require enforces a valid access token. A real HTTP 401 is returned on failure
// so the FE interceptor triggers its single-flight refresh.
func (a *Auth) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		raw := bearer(r)
		if raw == "" {
			apiresp.Unauthorized(w, "Missing bearer token")
			return
		}
		claims, err := a.issuer.Parse(raw)
		if err != nil {
			apiresp.Unauthorized(w, "Invalid or expired token")
			return
		}

		ip := r.Header.Get("X-Forwarded-For")
		if ip == "" {
			ip = r.RemoteAddr
		}

		id := reqctx.Identity{
			UserID:        claims.UserIDInt(),
			TenantID:      claims.TenantIDInt(),
			Role:          claims.Role,
			Scope:         claims.Scope,
			RequestURL:    r.URL.Path,
			RequestMethod: r.Method,
			IPAddress:     ip,
		}
		if id.TenantID > 0 {
			schema, err := a.tm.Schema(r.Context(), id.TenantID)
			if err != nil {
				a.logger.ErrorContext(r.Context(), "resolve tenant schema", "tenant", id.TenantID, "err", err)
				apiresp.ServerError(w, "Tenant resolution failed")
				return
			}
			id.Schema = schema
		}

		next.ServeHTTP(w, r.WithContext(reqctx.With(r.Context(), id)))
	})
}

// RequireTenant is like Require but also rejects callers without a selected
// tenant (used by tenant-scoped business endpoints).
func (a *Auth) RequireTenant(next http.Handler) http.Handler {
	return a.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		
		id, ok := reqctx.Get(r.Context())
		if ok && strings.EqualFold(id.Role, "SuperAdmin") {
			next.ServeHTTP(w, r)
			return
		}
		
		if reqctx.Schema(r.Context()) == "" {
			a.logger.ErrorContext(r.Context(), "RequireTenant failed", "id", id, "ok", ok, "url", r.URL.String())
			apiresp.Forbidden(w, "No tenant selected")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// RequireBranch is like RequireTenant but also strictly requires X-Branch-ID.
func (a *Auth) RequireBranch(next http.Handler) http.Handler {
	return a.RequireTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		branchID := r.Header.Get("X-Branch-ID")
		if branchID == "" {
			apiresp.Forbidden(w, "X-Branch-ID header is required")
			return
		}
		
		id, _ := reqctx.Get(r.Context())
		
		// Verify branch access
		var exists bool
		q := `SELECT EXISTS(
			SELECT 1 FROM ` + id.Schema + `.user_branches ub 
			JOIN ` + id.Schema + `.branches b ON b.id = ub.branch_id
			WHERE ub.user_id = $1 AND ub.branch_id = $2 AND b.status = 'Active' AND b.deleted_at IS NULL
		)`
		err := a.pool.QueryRow(r.Context(), q, id.UserID, branchID).Scan(&exists)
		if err != nil || !exists {
			apiresp.Forbidden(w, "Access to this branch is denied")
			return
		}
		
		// Inject BranchID into context
		id.BranchID = branchID
		next.ServeHTTP(w, r.WithContext(reqctx.With(r.Context(), id)))
	}))
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h != "" {
		const p = "Bearer "
		if len(h) > len(p) && strings.EqualFold(h[:len(p)], p) {
			return strings.TrimSpace(h[len(p):])
		}
	}
	
	// Fallback to query parameter for streaming endpoints (e.g. iframes)
	q := r.URL.Query().Get("access_token")
	if q != "" {
		return q
	}
	
	return ""
}

// RequirePermission enforces a specific permission on a resource.
func (a *Auth) RequirePermission(resource string, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			id, ok := reqctx.Get(r.Context())
			if !ok {
				apiresp.Unauthorized(w, "Authentication required")
				return
			}
			
			if strings.EqualFold(id.Role, "SuperAdmin") || strings.EqualFold(id.Role, "Admin") {
				next.ServeHTTP(w, r)
				return
			}
			
			allowed := false
			var canView, canAdd, canEdit, canDelete bool
			q := `
				SELECT rpp.can_view, rpp.can_add, rpp.can_edit, rpp.can_delete 
				FROM public.role_page_permissions rpp
				JOIN public.roles r ON r.role_id = rpp.role_id
				JOIN public.pages p ON p.page_id = rpp.page_id
				WHERE r.role_name = $1 AND p.route_url = $2
			`
			err := a.pool.QueryRow(r.Context(), q, id.Role, resource).Scan(&canView, &canAdd, &canEdit, &canDelete)
			if err == nil {
				switch action {
				case "CanView":
					allowed = canView
				case "CanAdd":
					allowed = canAdd
				case "CanEdit":
					allowed = canEdit
				case "CanDelete":
					allowed = canDelete
				}
			}
			
			if !allowed {
				// Log unauthorized access
				logQ := `
					INSERT INTO public.user_activities (user_id, user_name, tenant_id, url, method, ip_address, description, created_on, branch_id)
					VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8)
				`
				var branchUUID *string
				if id.BranchID != "" {
					branchUUID = &id.BranchID
				}
				ip := r.Header.Get("X-Forwarded-For")
				if ip == "" {
					ip = r.RemoteAddr
				}
				
				a.pool.Exec(r.Context(), logQ, id.UserID, "User ID " + string(id.UserID), id.TenantID, r.URL.Path, r.Method, ip, "Unauthorized Access Attempt to "+resource+" ("+action+")", branchUUID)
				
				apiresp.Forbidden(w, "You do not have permission to perform this action.")
				return
			}
			
			next.ServeHTTP(w, r)
		})
	}
}
