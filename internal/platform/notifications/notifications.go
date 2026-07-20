// Package notifications implements /api/Notifications (platform-scoped).
//
// Notifications live in the public schema and are addressed by their numeric
// (int64) id and the owning user id. This module talks directly to
// public.Queries: it lists a user's unread notifications, marks a single
// notification read, and marks all of a user's notifications read.
package notifications

import (
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

// Handler serves the Notifications endpoints.
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
	r.Route("/Notifications", func(r chi.Router) {
		r.Get("/unread/{userId}", h.listUnread)
		r.Put("/{id}/read", h.markRead)
		r.Put("/read-all/{userId}", h.markAllRead)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"userId"`
	TenantID  int64  `json:"tenantId"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Type      string `json:"type"`
	Link      string `json:"link"`
	IsRead    bool   `json:"isRead"`
	CreatedAt string `json:"createdAt"`
}

func toDTO(n public.Notification) dto {
	return dto{
		ID:        n.ID,
		UserID:    n.UserID,
		TenantID:  conv.Int64(n.TenantID),
		Title:     conv.Str(n.Title),
		Body:      conv.Str(n.Body),
		Type:      conv.Str(n.Type),
		Link:      conv.Str(n.Link),
		IsRead:    n.IsRead,
		CreatedAt: conv.FmtDateTime(n.CreatedAt),
	}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func (h *Handler) listUnread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := web.ParamInt64(chi.URLParam(r, "userId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid userId")
		return
	}
	rows, err := h.q.ListUnreadNotifications(ctx, userID)
	if err != nil {
		h.fail(w, err)
		return
	}
	items := make([]dto, len(rows))
	for i, n := range rows {
		items[i] = toDTO(n)
	}
	apiresp.OK(w, items, "")
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamInt64(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	err = h.q.MarkNotificationRead(ctx, public.MarkNotificationReadParams{
		ID:     id,
		UserID: reqctx.MustUserID(ctx),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Notification not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, true, "Notification marked read")
}

func (h *Handler) markAllRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := web.ParamInt64(chi.URLParam(r, "userId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid userId")
		return
	}
	if err := h.q.MarkAllNotificationsRead(ctx, userID); err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, true, "All notifications marked read")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("Notifications handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}
