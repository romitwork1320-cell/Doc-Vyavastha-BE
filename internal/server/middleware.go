package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/thinkparq/edconsultancy-be/internal/apiresp"
)

// requestLogger emits one structured log line per request.
func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.logger.LogAttrs(r.Context(), slog.LevelInfo, "http",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", ww.Status()),
			slog.Int("bytes", ww.BytesWritten()),
			slog.Duration("dur", time.Since(start)),
			slog.String("reqid", chimw.GetReqID(r.Context())),
		)
	})
}

// recoverer converts panics into a 500 envelope instead of crashing the server.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.ErrorContext(r.Context(), "panic recovered",
					"err", rec,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				apiresp.ServerError(w, "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
