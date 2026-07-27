package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// requestLogResponseWriter captures the response status code so RequestLog can
// include it in the access log.
type requestLogResponseWriter struct {
	http.ResponseWriter

	status      int
	wroteHeader bool
}

// WriteHeader records the status on the first call and forwards every call to
// the underlying ResponseWriter, matching net/http's behavior.
func (rw *requestLogResponseWriter) WriteHeader(status int) {
	if !rw.wroteHeader {
		rw.status = status
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(status)
}

// Write forwards to the underlying ResponseWriter.
func (rw *requestLogResponseWriter) Write(b []byte) (int, error) {
	return rw.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter so http.ResponseController can
// reach optional interfaces (Flush, Hijack, SetWriteDeadline, etc.) on the real writer.
func (rw *requestLogResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// RequestLog is an HTTP middleware that emits one structured access-log entry
// per request with the HTTP method, URL path, response status code, and total
// request duration in milliseconds. It logs through the request-scoped logger
// from the context (see LoggerFromContext), so RequestLog must be chained after
// RequestID to include the request ID in each log line.
func RequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Seeded with 200 OK, which net/http sends for handlers that never call
		// WriteHeader.
		rw := &requestLogResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		logger := LoggerFromContext(r.Context())
		logger.LogAttrs(r.Context(), slog.LevelInfo, "request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rw.status),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	})
}
