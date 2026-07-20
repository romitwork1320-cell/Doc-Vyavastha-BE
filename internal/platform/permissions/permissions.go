// Package permissions implements /api/Permissions (platform-scoped RBAC matrix).
//
// It exposes the role/page permission matrix the FE renders as a grid: a GET
// that assembles pages + roles + the current permission rows into a single
// PermissionMatrixDto, and a PUT that bulk-upserts an array of permission
// changes. RBAC catalog data (pages/roles/role_page_permissions) lives in the
// public schema and is global, so this module talks directly to public.Queries.
package permissions

import (
	"github.com/thinkparq/edconsultancy-be/internal/middleware"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
)

// Handler serves the Permissions endpoints.
type Handler struct {
	q      *public.Queries
	logger *slog.Logger
	mw     *middleware.Auth
}

// New builds the handler from the shared pgx pool.
func New(pool *pgxpool.Pool, logger *slog.Logger, mw *middleware.Auth) *Handler {
	return &Handler{q: public.New(pool), logger: logger, mw: mw}
}

// Mount registers routes under the platform router.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/Permissions", func(r chi.Router) {
		r.With(h.mw.RequirePermission("/permissions", "CanView")).Get("/", h.get)
		r.Put("/", h.update)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type pageDto struct {
	PageID   int64  `json:"pageId"`
	PageName string `json:"pageName"`
	RouteURL string `json:"routeUrl"`
}

type roleDto struct {
	RoleID   int64  `json:"roleId"`
	RoleName string `json:"roleName"`
}

type rolePagePermissionDto struct {
	RoleID    int64 `json:"roleId"`
	PageID    int64 `json:"pageId"`
	CanView   bool  `json:"canView"`
	CanAdd    bool  `json:"canAdd"`
	CanEdit   bool  `json:"canEdit"`
	CanDelete bool  `json:"canDelete"`
}

type permissionMatrixDto struct {
	Pages        []pageDto               `json:"pages"`
	Roles        []roleDto               `json:"roles"`
	Permissions  []rolePagePermissionDto `json:"permissions"`
	TotalRecords int64                   `json:"totalRecords"`
}

// permissionUpdateDto is one row of the PUT body array.
type permissionUpdateDto struct {
	RoleID    int64 `json:"roleId"`
	PageID    int64 `json:"pageId"`
	CanView   bool  `json:"canView"`
	CanAdd    bool  `json:"canAdd"`
	CanEdit   bool  `json:"canEdit"`
	CanDelete bool  `json:"canDelete"`
}

func toPageDTO(p public.Page) pageDto {
	return pageDto{
		PageID:   p.PageID,
		PageName: p.PageName,
		RouteURL: p.RouteUrl,
	}
}

func toRoleDTO(r public.Role) roleDto {
	return roleDto{
		RoleID:   r.RoleID,
		RoleName: r.RoleName,
	}
}

func toPermissionDTO(p public.RolePagePermission) rolePagePermissionDto {
	return rolePagePermissionDto{
		RoleID:    p.RoleID,
		PageID:    p.PageID,
		CanView:   p.CanView,
		CanAdd:    p.CanAdd,
		CanEdit:   p.CanEdit,
		CanDelete: p.CanDelete,
	}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

// get assembles the full permission matrix (pages + roles + current grants).
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pages, err := h.q.ListPages(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	roles, err := h.q.ListRoles(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	perms, err := h.q.ListRolePagePermissions(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}

	// Build non-nil slices so empties serialize as [] rather than null.
	pageItems := make([]pageDto, len(pages))
	for i, p := range pages {
		pageItems[i] = toPageDTO(p)
	}
	roleItems := make([]roleDto, len(roles))
	for i, ro := range roles {
		roleItems[i] = toRoleDTO(ro)
	}
	permItems := make([]rolePagePermissionDto, len(perms))
	for i, p := range perms {
		permItems[i] = toPermissionDTO(p)
	}

	matrix := permissionMatrixDto{
		Pages:        pageItems,
		Roles:        roleItems,
		Permissions:  permItems,
		TotalRecords: int64(len(permItems)),
	}
	apiresp.OK(w, matrix, "")
}

// update bulk-upserts an array of role/page permission rows.
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// The PUT body is a JSON array, so decode directly into a slice
	// (web.Bind targets a single struct).
	var updates []permissionUpdateDto
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}

	for _, u := range updates {
		if err := h.q.UpsertRolePagePermission(ctx, public.UpsertRolePagePermissionParams{
			RoleID:    u.RoleID,
			PageID:    u.PageID,
			CanView:   u.CanView,
			CanAdd:    u.CanAdd,
			CanEdit:   u.CanEdit,
			CanDelete: u.CanDelete,
		}); err != nil {
			h.fail(w, err)
			return
		}
	}
	apiresp.OK(w, true, "Permissions updated")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("Permissions handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}
