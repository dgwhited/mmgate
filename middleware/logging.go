package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/dgwhited/mmgate/auth"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the underlying ResponseWriter to http.ResponseController,
// which is how httputil.ReverseProxy reaches Flush and Hijack. Without it,
// wrapping the writer here would silently break streaming responses and
// websocket upgrades proxied to Mattermost (e.g. /api/v4/websocket).
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		}

		if client := auth.ClientFromContext(r.Context()); client != nil {
			attrs = append(attrs, "client", client.ID)
		}

		switch {
		case rw.statusCode >= 500:
			slog.Error("request", attrs...)
		case rw.statusCode >= 400:
			slog.Warn("request", attrs...)
		default:
			slog.Info("request", attrs...)
		}
	})
}
