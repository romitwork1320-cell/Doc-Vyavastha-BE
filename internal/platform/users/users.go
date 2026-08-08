// Package users implements /api/Users (platform-schema team/user management).
//
// This is a PLATFORM module: it talks to the shared `public` schema via a
// public.Queries built on the pool. Tenant-owned rows (memberships, per-user
// page permissions) are scoped by the caller's selected tenant id, read from
// reqctx.TenantID(ctx). It mirrors the categories reference pattern: bind/
// validate -> run the query -> map the sqlc row to the FE DTO -> render with
// the ApiResponse envelope. List endpoints carry data + total via apiresp.List.
package users

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/thinkparq/edconsultancy-be/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/web"
)

// Handler serves the Users endpoints.
type Handler struct {
	pool   *pgxpool.Pool
	q      *public.Queries
	logger *slog.Logger
	mw     *middleware.Auth
}

// New builds the handler from the shared pool.
func New(pool *pgxpool.Pool, logger *slog.Logger, mw *middleware.Auth) *Handler {
	return &Handler{pool: pool, q: public.New(pool), logger: logger, mw: mw}
}

// Mount registers routes under the platform router.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/Users", func(r chi.Router) {
		r.With(h.mw.RequirePermission("/users", "CanView")).Get("/", h.list)
		r.With(h.mw.RequirePermission("/users", "CanAdd")).Post("/", h.create)
		r.Get("/lookup", h.lookup)
		r.Get("/pages", h.pages)
		r.Post("/grant-permission", h.grantPermission)
		// "/{id}/permissions" is unambiguous against "/{id}" in chi, so the
		// order relative to the bare "/{id}" handlers does not matter.
		r.Get("/{userId}/permissions", h.userPermissions)
		r.With(h.mw.RequirePermission("/users", "CanView")).Get("/{id}", h.get)
		r.With(h.mw.RequirePermission("/users", "CanEdit")).Put("/{id}", h.update)
		r.With(h.mw.RequirePermission("/users", "CanDelete")).Delete("/{id}", h.delete)
		r.Delete("/{id}/tenant/{tenantId}", h.removeFromTenant)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID            int64   `json:"id"`
	Username      string  `json:"username"`
	Role          string  `json:"role"`
	IsActive      bool    `json:"isActive"`
	TenantID      int64   `json:"tenantId"`
	FirstName     *string `json:"firstName"`
	LastName      *string `json:"lastName"`
	ContactNumber *string `json:"contactNumber"`
}

type pageDto struct {
	PageID   int64  `json:"pageId"`
	PageName string `json:"pageName"`
	RouteUrl string `json:"routeUrl"`
}

type userPagePermissionDto struct {
	PageUrl   string `json:"PageUrl"`
	CanView   bool   `json:"CanView"`
	CanAdd    bool   `json:"CanAdd"`
	CanEdit   bool   `json:"CanEdit"`
	CanDelete bool   `json:"CanDelete"`
}

type userTenantLookupDto struct {
	UserID      int64  `json:"userId"`
	DisplayName string `json:"displayName"`
	RoleName    string `json:"roleName"`
}

// teamCreateDto is the POST / body (username is the user's email).
type teamCreateDto struct {
	Username      string  `json:"username" validate:"required"`
	RoleName      string  `json:"roleName" validate:"required"`
	TenantID      int64   `json:"tenantId" validate:"required"`
	FirstName     *string `json:"firstName"`
	LastName      *string `json:"lastName"`
	ContactNumber *string `json:"contactNumber"`
}

// teamUpdateDto is the PUT /{id} body.
type teamUpdateDto struct {
	RoleName      string  `json:"roleName" validate:"required"`
	IsActive      bool    `json:"isActive"`
	TenantID      int64   `json:"tenantId" validate:"required"`
	FirstName     *string `json:"firstName"`
	LastName      *string `json:"lastName"`
	ContactNumber *string `json:"contactNumber"`
}

// grantPermissionDto is the POST /grant-permission body.
type grantPermissionDto struct {
	UserID  int64 `json:"userId" validate:"required"`
	PageID  int64 `json:"pageId" validate:"required"`
	CanView bool  `json:"canView"`
}

func listRowToDTO(r public.ListTenantUsersRow) dto {
	return dto{
		ID:            r.ID,
		Username:      r.Username,
		Role:          r.Role,
		IsActive:      r.IsActive,
		TenantID:      r.TenantID,
		FirstName:     r.FirstName,
		LastName:      r.LastName,
		ContactNumber: r.ContactNumber,
	}
}

func getRowToDTO(r public.GetTenantUserRow) dto {
	return dto{
		ID:            r.ID,
		Username:      r.Username,
		Role:          r.Role,
		IsActive:      r.IsActive,
		TenantID:      r.TenantID,
		FirstName:     r.FirstName,
		LastName:      r.LastName,
		ContactNumber: r.ContactNumber,
	}
}

func statusOrDefault(s string) string {
	if s == "" {
		return "Active"
	}
	return s
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := reqctx.TenantID(ctx)
	page := web.PageFromQuery(r)

	rows, err := h.q.ListTenantUsers(ctx, public.ListTenantUsersParams{
		TenantID: tenantID,
		Filter:   page.FilterValue(),
		Lim:      page.Limit(),
		Off:      page.Offset(),
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	total, err := h.q.CountTenantUsers(ctx, public.CountTenantUsersParams{
		TenantID: tenantID,
		Filter:   page.FilterValue(),
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	items := make([]dto, len(rows))
	for i, row := range rows {
		items[i] = listRowToDTO(row)
	}
	apiresp.List(w, items, total, "")
}

func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.q.ListTenantUserLookup(ctx, reqctx.TenantID(ctx))
	if err != nil {
		h.fail(w, err)
		return
	}
	items := make([]userTenantLookupDto, len(rows))
	for i, row := range rows {
		items[i] = userTenantLookupDto{
			UserID:      row.UserID,
			DisplayName: row.DisplayName,
			RoleName:    row.RoleName,
		}
	}
	apiresp.OK(w, items, "")
}

func (h *Handler) pages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.q.ListPages(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	items := make([]pageDto, len(rows))
	for i, p := range rows {
		items[i] = pageDto{
			PageID:   p.PageID,
			PageName: p.PageName,
			RouteUrl: p.RouteUrl,
		}
	}
	apiresp.OK(w, items, "")
}

func (h *Handler) grantPermission(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req grantPermissionDto
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	if err := h.q.GrantUserPagePermission(ctx, public.GrantUserPagePermissionParams{
		UserID:   req.UserID,
		TenantID: reqctx.TenantID(ctx),
		PageID:   req.PageID,
		CanView:  req.CanView,
	}); err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, true, "Permission updated")
}

func (h *Handler) userPermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := web.ParamInt64(chi.URLParam(r, "userId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid user id")
		return
	}
	
	tenantID := reqctx.TenantID(ctx)
	var items []userPagePermissionDto

	if tenantID == 0 {
		// Personal workspace has no tenant schema, default to "Client" role permissions
		q := `
			SELECT p.route_url, rpp.can_view, rpp.can_add, rpp.can_edit, rpp.can_delete 
			FROM public.role_page_permissions rpp
			JOIN public.roles r ON r.role_id = rpp.role_id
			JOIN public.pages p ON p.page_id = rpp.page_id
			WHERE r.role_name = 'Client'
		`
		rows, err := h.pool.Query(ctx, q)
		if err == nil {
			for rows.Next() {
				var routeUrl string
				var canView, canAdd, canEdit, canDelete bool
				if err := rows.Scan(&routeUrl, &canView, &canAdd, &canEdit, &canDelete); err == nil {
					items = append(items, userPagePermissionDto{
						PageUrl:   routeUrl,
						CanView:   canView,
						CanAdd:    canAdd,
						CanEdit:   canEdit,
						CanDelete: canDelete,
					})
				}
			}
			rows.Close()
		}
	} else {
		rows, err := h.q.GetUserPagePermissions(ctx, public.GetUserPagePermissionsParams{
			UserID:   userID,
			TenantID: tenantID,
		})
		if err != nil {
			h.fail(w, err)
			return
		}
		for _, r := range rows {
			items = append(items, userPagePermissionDto{
				PageUrl:   r.RouteUrl,
				CanView:   r.CanView,
				CanAdd:    r.CanAdd,
				CanEdit:   r.CanEdit,
				CanDelete: r.CanDelete,
			})
		}
	}

	apiresp.OK(w, items, "")
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req teamCreateDto
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}

	// Check if user already exists
	user, err := h.q.GetUserByEmail(ctx, req.Username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Create the user (email == username, no password yet, unverified).
			user, err = h.q.CreateUser(ctx, public.CreateUserParams{
				Email:        req.Username,
				PasswordHash: nil,
				GoogleID:     nil,
				IsActive:     true,
				IsVerified:   false,
			})
			if err != nil {
				h.fail(w, err)
				return
			}
		} else {
			h.fail(w, err)
			return
		}
	}

	// Upsert the user profile with provided names and phone
	if _, perr := h.q.UpsertUserProfile(ctx, public.UpsertUserProfileParams{
		UserID:        user.ID,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		ContactNumber: req.ContactNumber,
	}); perr != nil {
		// Log the error but continue (user was already created)
		h.logger.ErrorContext(ctx, "failed to upsert user profile", "user", user.ID, "err", perr)
	}

	// Resolve (or create) the role, then attach the user to the tenant.
	role, err := h.q.EnsureRole(ctx, req.RoleName)
	if err != nil {
		h.fail(w, err)
		return
	}
	roleID := role.RoleID
	if _, err := h.q.AddUserToTenant(ctx, public.AddUserToTenantParams{
		UserID:   user.ID,
		TenantID: req.TenantID,
		RoleID:   &roleID,
		Status:   statusOrDefault(""),
		IsOwner:  false,
	}); err != nil {
		h.fail(w, err)
		return
	}

	apiresp.Created(w, dto{
		ID:            user.ID,
		Username:      user.Email,
		Role:          req.RoleName,
		IsActive:      user.IsActive,
		TenantID:      req.TenantID,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		ContactNumber: req.ContactNumber,
	}, "User created")
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamInt64(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	row, err := h.q.GetTenantUser(ctx, public.GetTenantUserParams{
		UserID:   id,
		TenantID: reqctx.TenantID(ctx),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "User not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, getRowToDTO(row), "")
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamInt64(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	var req teamUpdateDto
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}

	// Resolve (or create) the role, update the membership, then the user flag.
	role, err := h.q.EnsureRole(ctx, req.RoleName)
	if err != nil {
		h.fail(w, err)
		return
	}
	roleID := role.RoleID
	if err := h.q.UpdateMembership(ctx, public.UpdateMembershipParams{
		RoleID:   &roleID,
		Status:   statusOrDefault(""),
		UserID:   id,
		TenantID: req.TenantID,
	}); err != nil {
		h.fail(w, err)
		return
	}
	if err := h.q.UpdateUserActive(ctx, public.UpdateUserActiveParams{
		IsActive: req.IsActive,
		ID:       id,
	}); err != nil {
		h.fail(w, err)
		return
	}

	// Also update their profile info if provided
	if _, perr := h.q.UpsertUserProfile(ctx, public.UpsertUserProfileParams{
		UserID:        id,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		ContactNumber: req.ContactNumber,
	}); perr != nil {
		h.logger.ErrorContext(ctx, "failed to upsert user profile on update", "user", id, "err", perr)
	}

	apiresp.OK(w, true, "User updated")
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamInt64(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	rows, err := h.q.SoftDeleteUser(ctx, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	if rows == 0 {
		apiresp.NotFound(w, "User not found")
		return
	}
	apiresp.OK(w, true, "User deleted")
}

func (h *Handler) removeFromTenant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := web.ParamInt64(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	tenantID, err := web.ParamInt64(chi.URLParam(r, "tenantId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid tenant id")
		return
	}
	rows, err := h.q.RemoveUserFromTenant(ctx, public.RemoveUserFromTenantParams{
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	if rows == 0 {
		apiresp.NotFound(w, "Membership not found")
		return
	}
	apiresp.OK(w, true, "User removed from tenant")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("Users handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

// ensure context import is used even if a future refactor drops it
var _ = context.Background
