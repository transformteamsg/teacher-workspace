package middleware

import (
	"bufio"
	"errors"
	"maps"
	"net"
	"net/http"
	"runtime/debug"

	"github.com/transformteamsg/teacher-workspace/server/internal/httputil"
)

// Recover is a middleware that responds with a 500 when the handler panics, so
// the client gets a response instead of a dropped connection. The 500 discards
// the handler's changes to the response headers.
//
// The panic is logged at error level with its value and stack trace, through
// the logger from [LoggerFromContext]. Recover must be chained after [RequestID]
// for the entry to carry the request ID, and after [RequestLog] for the request
// to be logged with the 500.
//
// If the handler has committed the final status, by calling WriteHeader,
// writing, or flushing, or has hijacked the connection, no 500 can be sent. The
// panic is logged, then continues up the stack as [http.ErrAbortHandler]. After
// a committed status, that aborts the response, so that a client cannot mistake
// a partial response for a complete one. A hijacked connection stays open until
// the handler closes it.
//
// A panic with [http.ErrAbortHandler] is the handler's own request to abort the
// response, so it is not logged and continues up the stack.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &recoverResponseWriter{ResponseWriter: w}
		initialHeader := w.Header().Clone()

		defer func() {
			v := recover()
			if v == nil {
				return
			}

			if v == http.ErrAbortHandler {
				panic(v)
			}

			logger := LoggerFromContext(r.Context())
			logger.Error("panic serving request", "err", v, "stack", string(debug.Stack()))

			if rw.committed {
				panic(http.ErrAbortHandler)
			}

			// Reset to the headers from when the request reached Recover. Any added
			// since were meant for a response that will not be sent, and some, such
			// as Content-Length, would corrupt the 500.
			h := w.Header()
			clear(h)
			maps.Copy(h, initialHeader)

			httputil.RenderPlain(w, logger, http.StatusInternalServerError)
		}()

		next.ServeHTTP(rw, r)
	})
}

// recoverResponseWriter records whether the response can still be replaced,
// leaving the response itself unchanged.
type recoverResponseWriter struct {
	http.ResponseWriter

	// committed reports whether the response can no longer be replaced, because the
	// final status has been committed or the connection hijacked.
	committed bool
}

// WriteHeader sends status, recording whether it is the response's final status.
func (rw *recoverResponseWriter) WriteHeader(status int) {
	rw.ResponseWriter.WriteHeader(status)

	// A 1xx other than 101 leaves the final status still to come.
	if status >= 200 || status == http.StatusSwitchingProtocols {
		rw.committed = true
	}
}

// Write writes b, which commits 200 if no status has been committed.
func (rw *recoverResponseWriter) Write(b []byte) (int, error) {
	rw.committed = true

	return rw.ResponseWriter.Write(b)
}

// Flush is [recoverResponseWriter.FlushError] without the error, for callers
// that use [http.Flusher].
func (rw *recoverResponseWriter) Flush() {
	_ = rw.FlushError()
}

// FlushError flushes the response, which commits 200 if no status has been
// committed. If the response cannot be flushed, it returns an error wrapping
// [http.ErrNotSupported] and commits nothing.
func (rw *recoverResponseWriter) FlushError() error {
	err := http.NewResponseController(rw.ResponseWriter).Flush()
	if !errors.Is(err, http.ErrNotSupported) {
		rw.committed = true
	}

	return err
}

// Hijack hands over the connection, after which the response can no longer be
// replaced.
func (rw *recoverResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, brw, err := http.NewResponseController(rw.ResponseWriter).Hijack()
	if err == nil {
		rw.committed = true
	}

	return conn, brw, err
}

// Unwrap returns the underlying ResponseWriter, so [http.ResponseController]
// can reach the methods recoverResponseWriter does not override.
func (rw *recoverResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
