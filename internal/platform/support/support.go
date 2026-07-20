// Package support implements /api/Support (platform-scoped help desk).
//
// Tickets and their message threads live in the public schema (they are
// tenant-owned but globally addressed by numeric id), so this module talks
// directly to public.Queries. Tenant-owned reads are scoped by
// reqctx.TenantID when the caller does not supply an explicit tenantId.
package support

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

// Handler serves the Support endpoints.
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
	r.Route("/Support", func(r chi.Router) {
		r.Post("/create", h.create)
		r.Post("/reply", h.reply)
		r.Get("/list", h.list)
		r.Get("/{id}", h.get)
		r.Put("/{ticketId}/status", h.updateStatus)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

// ticketListDto is the summary view of a ticket (also used as the detail header).
type ticketListDto struct {
	TicketID  int64  `json:"ticketId"`
	TenantID  int64  `json:"tenantId"`
	UserID    int64  `json:"userId"`
	Subject   string `json:"subject"`
	Category  string `json:"category"`
	Priority  string `json:"priority"`
	Status    string `json:"status"`
	CreatedOn string `json:"createdOn"`
	UpdatedOn string `json:"updatedOn"`
}

// messageDto is a single entry in a ticket's reply thread.
type messageDto struct {
	ID            int64   `json:"id"`
	Message       string  `json:"message"`
	IsAdminReply  bool    `json:"isAdminReply"`
	CreatedOn     string  `json:"createdOn"`
	AttachmentURL *string `json:"attachmentUrl"`
}

// ticketDetailDto bundles a ticket header with its full message history.
type ticketDetailDto struct {
	Header  ticketListDto `json:"header"`
	History []messageDto  `json:"history"`
}

// createTicketDto is the body for POST /create.
type createTicketDto struct {
	Subject       string `json:"subject" validate:"required"`
	Category      string `json:"category"`
	Priority      string `json:"priority"`
	Description   string `json:"description"`
	AttachmentURL string `json:"attachmentUrl"`
	TenantID      int64  `json:"tenantId"`
	UserID        int64  `json:"userId"`
}

// replyTicketDto is the body for POST /reply.
type replyTicketDto struct {
	TicketID      int64  `json:"ticketId" validate:"required"`
	SenderUserID  int64  `json:"senderUserId"`
	IsAdminReply  bool   `json:"isAdminReply"`
	Message       string `json:"message" validate:"required"`
	AttachmentURL string `json:"attachmentUrl"`
}

// statusDto is the body for PUT /{ticketId}/status.
type statusDto struct {
	Status string `json:"status"`
}

func toTicketDTO(t public.SupportTicket) ticketListDto {
	return ticketListDto{
		TicketID:  t.TicketID,
		TenantID:  t.TenantID,
		UserID:    t.UserID,
		Subject:   t.Subject,
		Category:  conv.Str(t.Category),
		Priority:  conv.Str(t.Priority),
		Status:    t.Status,
		CreatedOn: conv.FmtDateTime(t.CreatedAt),
		UpdatedOn: conv.FmtDateTime(t.UpdatedAt),
	}
}

func toMessageDTO(m public.TicketMessage) messageDto {
	return messageDto{
		ID:            m.ID,
		Message:       m.Message,
		IsAdminReply:  m.IsAdminReply,
		CreatedOn:     conv.FmtDateTime(m.CreatedOn),
		AttachmentURL: m.AttachmentUrl,
	}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

// create opens a new support ticket and returns its numeric id.
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createTicketDto
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	ticket, err := h.q.CreateTicket(ctx, public.CreateTicketParams{
		TenantID:      req.TenantID,
		UserID:        req.UserID,
		Subject:       req.Subject,
		Category:      conv.PtrStr(req.Category),
		Priority:      conv.PtrStr(req.Priority),
		Description:   conv.PtrStr(req.Description),
		AttachmentUrl: conv.PtrStr(req.AttachmentURL),
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.Created(w, ticket.TicketID, "Ticket created")
}

// reply appends a message to a ticket thread and bumps the ticket's updated_at.
func (h *Handler) reply(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req replyTicketDto
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	if _, err := h.q.AddTicketReply(ctx, public.AddTicketReplyParams{
		TicketID:      req.TicketID,
		SenderUserID:  req.SenderUserID,
		IsAdminReply:  req.IsAdminReply,
		Message:       req.Message,
		AttachmentUrl: conv.PtrStr(req.AttachmentURL),
	}); err != nil {
		h.fail(w, err)
		return
	}
	if err := h.q.TouchTicket(ctx, req.TicketID); err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, true, "Reply added")
}

// list returns tickets filtered by tenant and (optionally) status. tenantId=0
// returns tickets across all tenants (per the SQL); when the query param is
// absent it defaults to the caller's tenant.
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	tenantID := reqctx.TenantID(ctx)
	if v := q.Get("tenantId"); v != "" {
		parsed, err := web.ParamInt64(v)
		if err != nil {
			apiresp.BadRequest(w, "Invalid tenantId")
			return
		}
		tenantID = parsed
	}

	rows, err := h.q.ListTickets(ctx, public.ListTicketsParams{
		TenantID: tenantID,
		Status:   q.Get("status"),
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	items := make([]ticketListDto, len(rows))
	for i, t := range rows {
		items[i] = toTicketDTO(t)
	}
	apiresp.List(w, items, int64(len(items)), "")
}

// get returns a ticket header plus its full message history.
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamInt64(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	ticket, err := h.q.GetTicket(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "Ticket not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	msgs, err := h.q.ListTicketMessages(ctx, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	history := make([]messageDto, len(msgs))
	for i, m := range msgs {
		history[i] = toMessageDTO(m)
	}
	apiresp.OK(w, ticketDetailDto{Header: toTicketDTO(ticket), History: history}, "")
}

// updateStatus changes a ticket's status.
func (h *Handler) updateStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamInt64(chi.URLParam(r, "ticketId"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid ticketId")
		return
	}
	var req statusDto
	if err := web.Bind(r, &req); err != nil {
		apiresp.BadRequest(w, err.Error())
		return
	}
	if err := h.q.UpdateTicketStatus(ctx, public.UpdateTicketStatusParams{
		Status:   statusOrDefault(req.Status),
		TicketID: id,
	}); err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, true, "Status updated")
}

func statusOrDefault(s string) string {
	if s == "" {
		return "Active"
	}
	return s
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("Support handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}
