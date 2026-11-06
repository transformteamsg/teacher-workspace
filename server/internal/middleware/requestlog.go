package middleware

import (
	"bufio"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// RequestLog is a middleware that logs each request at info level once the next
// handler in the chain returns or panics, with its method, URL path, status,
// and duration in milliseconds. A panic continues up the stack once it is
// logged.
//
// The entry is written through the logger from [LoggerFromContext], so
// RequestLog must be chained after [RequestID] for it to carry the request ID.
//
// The status is the final one the handler committed. Writing, flushing, or
// returning without calling WriteHeader first commits 200. The status is left
// out if the handler panics or hijacks the connection before committing one.
//
// The entry for a hijacked connection also has hijacked set to true. The entry
// for a handler that panics also has aborted set to true, as its status may not
// have reached the client: a panic can discard a buffered response.
func RequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rw := &requestLogResponseWriter{ResponseWriter: w}
		returned := false

		defer func() {
			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			}
			if !returned {
				attrs = append(attrs, slog.Bool("aborted", true))
			}
			if rw.status != 0 {
				attrs = append(attrs, slog.Int("status", rw.status))
			}
			if rw.hijacked {
				attrs = append(attrs, slog.Bool("hijacked", true))
			}

			logger := LoggerFromContext(r.Context())
			logger.LogAttrs(r.Context(), slog.LevelInfo, "request", attrs...)
		}()

		next.ServeHTTP(rw, r)

		returned = true

		// Records the 200 net/http sends for a response that was never written.
		rw.setStatus(http.StatusOK)
	})
}

// requestLogResponseWriter records the final status of the response written
// through it, leaving the response itself unchanged.
type requestLogResponseWriter struct {
	http.ResponseWriter

	// status is the final status the handler committed, or 0 if it has not committed
	// one.
	status int

	// hijacked reports whether the handler took over the connection.
	hijacked bool
}

// WriteHeader sends status, recording it if it is the response's final status.
func (rw *requestLogResponseWriter) WriteHeader(status int) {
	// A 1xx other than 101 leaves the final status still to come.
	if status >= 200 || status == http.StatusSwitchingProtocols {
		rw.setStatus(status)
	}

	rw.ResponseWriter.WriteHeader(status)
}

// Write writes b, which commits 200 if no status has been committed.
func (rw *requestLogResponseWriter) Write(b []byte) (int, error) {
	rw.setStatus(http.StatusOK)

	return rw.ResponseWriter.Write(b)
}

// Flush is [requestLogResponseWriter.FlushError] without the error, for callers
// that use [http.Flusher].
func (rw *requestLogResponseWriter) Flush() {
	_ = rw.FlushError()
}

// FlushError flushes the response, which commits 200 if no status has been
// committed. If the response cannot be flushed, it returns an error wrapping
// [http.ErrNotSupported] and commits nothing.
func (rw *requestLogResponseWriter) FlushError() error {
	err := http.NewResponseController(rw.ResponseWriter).Flush()
	if !errors.Is(err, http.ErrNotSupported) {
		rw.setStatus(http.StatusOK)
	}

	return err
}

// Hijack hands over the connection. Once it succeeds, the final status no longer
// changes, because a response sent over the connection cannot be observed and
// one written through rw is not sent.
func (rw *requestLogResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, brw, err := http.NewResponseController(rw.ResponseWriter).Hijack()
	if err == nil {
		rw.hijacked = true
	}

	return conn, brw, err
}

// Unwrap returns the underlying ResponseWriter, so [http.ResponseController]
// can reach the methods requestLogResponseWriter does not override.
func (rw *requestLogResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// setStatus records status as the committed status, unless one is already
// committed or the connection has been hijacked.
func (rw *requestLogResponseWriter) setStatus(status int) {
	if rw.status == 0 && !rw.hijacked {
		rw.status = status
	}
}
