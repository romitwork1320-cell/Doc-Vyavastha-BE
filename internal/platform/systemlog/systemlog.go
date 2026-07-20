// Package systemlog implements /SystemLog (platform-schema system log viewer).
//
// This is a PLATFORM module: it talks to the shared `public` schema via a
// public.Queries built on the pool. It mirrors the categories reference
// pattern: bind/validate -> run the query -> map the sqlc row to the FE DTO ->
// render with the ApiResponse envelope. The list endpoint carries data + total
// via apiresp.List. System logs are a global/platform resource, so there is no
// per-tenant scoping here.
package systemlog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
	"github.com/thinkparq/edconsultancy-be/internal/conv"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/web"
)

// Handler serves the SystemLog endpoints.
type Handler struct {
	q      *public.Queries
	logger *slog.Logger
}

// New builds the handler from the shared pool.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	return &Handler{q: public.New(pool), logger: logger}
}

// Mount registers routes under the platform router.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/SystemLog", func(r chi.Router) {
		r.Post("/get-all", h.getAll)
		r.Get("/{id}", h.get)
		r.Delete("/{id}", h.delete)
		r.Delete("/cleanup/{days}", h.cleanup)
	})
}

// ── DTOs ────────────────────────────────────────────────────────────────────

type dto struct {
	LogID      int64  `json:"logId"`
	LogDate    string `json:"logDate"`
	LogLevel   string `json:"logLevel"`
	Source     string `json:"source"`
	Message    string `json:"message"`
	StackTrace string `json:"stackTrace"`
	TenantName string `json:"tenantName"`
	UserID     string `json:"userId"`
	RequestURL string `json:"requestUrl"`
	IPAddress  string `json:"ipAddress"`
}

func toDTO(l public.SystemLog) dto {
	return dto{
		LogID:      l.LogID,
		LogDate:    conv.FmtDateTime(l.LogDate),
		LogLevel:   l.LogLevel,
		Source:     conv.Str(l.Source),
		Message:    conv.Str(l.Message),
		StackTrace: conv.Str(l.StackTrace),
		TenantName: conv.Str(l.TenantName),
		UserID:     conv.Str(l.UserID),
		RequestURL: conv.Str(l.RequestUrl),
		IPAddress:  conv.Str(l.IpAddress),
	}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

// getAll mirrors the FE PaginationRequestDto: it decodes the JSON body directly
// into a web.Page so the same field semantics (pageIndex/pageSize/filter/
// fromDate/toDate) apply as the GET-list convention used elsewhere.
func (h *Handler) getAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var page web.Page
	if r.Body != nil {
		// Tolerate an empty body (FE may post no filters at all).
		if err := json.NewDecoder(r.Body).Decode(&page); err != nil && !errors.Is(err, io.EOF) {
			apiresp.BadRequest(w, "invalid request body: "+err.Error())
			return
		}
	}

	from := conv.ParseDate(page.FromDate)
	to := conv.ParseDate(page.ToDate)

	rows, err := h.q.ListSystemLogs(ctx, public.ListSystemLogsParams{
		Filter:   page.FilterValue(),
		FromDate: from,
		ToDate:   to,
		Off:      page.Offset(),
		Lim:      page.Limit(),
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	total, err := h.q.CountSystemLogs(ctx, public.CountSystemLogsParams{
		Filter:   page.FilterValue(),
		FromDate: from,
		ToDate:   to,
	})
	if err != nil {
		h.fail(w, err)
		return
	}

	items := make([]dto, len(rows))
	for i, l := range rows {
		items[i] = toDTO(l)
	}
	apiresp.List(w, items, total, "")
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamInt64(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	l, err := h.q.GetSystemLog(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		apiresp.NotFound(w, "System log not found")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, toDTO(l), "")
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := web.ParamInt64(chi.URLParam(r, "id"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid id")
		return
	}
	rows, err := h.q.DeleteSystemLog(ctx, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	if rows == 0 {
		apiresp.NotFound(w, "System log not found")
		return
	}
	apiresp.OK(w, true, "System log deleted")
}

// cleanup deletes all system logs older than the given number of days.
func (h *Handler) cleanup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	days, err := web.ParamInt64(chi.URLParam(r, "days"))
	if err != nil {
		apiresp.BadRequest(w, "Invalid days")
		return
	}
	if _, err := h.q.CleanupSystemLogs(ctx, int32(days)); err != nil {
		h.fail(w, err)
		return
	}
	apiresp.OK(w, true, "System logs cleaned up")
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	h.logger.Error("SystemLog handler error", "err", err)
	apiresp.ServerError(w, "Something went wrong")
}

// ensure context import is used even if a future refactor drops it
var _ = context.Background
