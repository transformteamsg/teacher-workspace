package middleware

import (
	"bufio"
	"bytes"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestRecover(t *testing.T) {
	t.Run("leaves the handler's response unchanged when the handler does not panic", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Custom", "custom value")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("hello"))
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Recover(next).ServeHTTP(rec, req)

		if want, got := http.StatusCreated, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if want, got := "custom value", rec.Result().Header.Get("X-Custom"); want != got {
			t.Errorf("want X-Custom: %q; got: %q", want, got)
		}
		if want, got := "hello", rec.Body.String(); want != got {
			t.Errorf("want rec.Body: %q; got: %q", want, got)
		}
	})

	t.Run("sends every status the handler passes to WriteHeader", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusEarlyHints)
			w.WriteHeader(http.StatusCreated)
			w.WriteHeader(http.StatusTeapot)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := &statusRecordingWriter{ResponseRecorder: httptest.NewRecorder()}

		Recover(next).ServeHTTP(rec, req)

		want := []int{http.StatusEarlyHints, http.StatusCreated, http.StatusTeapot}
		if got := rec.statuses; !slices.Equal(want, got) {
			t.Errorf("want rec.statuses: %v; got: %v", want, got)
		}
	})

	t.Run("flushes the response when the handler flushes", func(t *testing.T) {
		for _, test := range []struct {
			name string
			next http.HandlerFunc
		}{
			{
				name: "through http.ResponseController",
				next: func(w http.ResponseWriter, r *http.Request) {
					_ = http.NewResponseController(w).Flush()
				},
			},
			{
				name: "through http.Flusher",
				next: func(w http.ResponseWriter, r *http.Request) {
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
				},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

				req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
				rec := httptest.NewRecorder()

				Recover(test.next).ServeHTTP(rec, req)

				if !rec.Flushed {
					t.Error("want rec.Flushed: true; got: false")
				}
			})
		}
	})

	t.Run("reports http.ErrNotSupported when the response cannot be flushed", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		var flushErr error
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			flushErr = http.NewResponseController(w).Flush()
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		w := unflushableWriter{ResponseWriter: httptest.NewRecorder()}

		Recover(next).ServeHTTP(w, req)

		if !errors.Is(flushErr, http.ErrNotSupported) {
			t.Errorf("want err: %v; got: %v", http.ErrNotSupported, flushErr)
		}
	})

	t.Run("hands over the connection when the handler hijacks it", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		var (
			hijacked  net.Conn
			brw       *bufio.ReadWriter
			hijackErr error
		)
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hijacked, brw, hijackErr = http.NewResponseController(w).Hijack()
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := newHijackableWriter(t)

		Recover(next).ServeHTTP(rec, req)

		if hijackErr != nil {
			t.Fatalf("want err: nil; got: %v", hijackErr)
		}
		if want := rec.conn; want != hijacked {
			t.Errorf("want hijacked: %v; got: %v", want, hijacked)
		}
		if brw == nil {
			t.Error("want brw: non-nil; got: nil")
		}
	})

	t.Run("reports http.ErrNotSupported when the connection cannot be hijacked", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		var hijackErr error
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _, hijackErr = http.NewResponseController(w).Hijack()
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Recover(next).ServeHTTP(rec, req)

		if !errors.Is(hijackErr, http.ErrNotSupported) {
			t.Errorf("want err: %v; got: %v", http.ErrNotSupported, hijackErr)
		}
	})

	t.Run("responds with a 500 when the handler panics before committing the final status", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("handler failed")
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		recovered := panicValue(func() {
			Recover(next).ServeHTTP(rec, req)
		})

		if recovered != nil {
			t.Errorf("want ServeHTTP: no panic; got: %v", recovered)
		}
		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if want, got := http.StatusText(http.StatusInternalServerError), rec.Body.String(); want != got {
			t.Errorf("want rec.Body: %q; got: %q", want, got)
		}
	})

	t.Run("responds with a 500 when the handler panics after sending a 1xx status", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusEarlyHints)
			panic("handler failed")
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := &statusRecordingWriter{ResponseRecorder: httptest.NewRecorder()}

		recovered := panicValue(func() {
			Recover(next).ServeHTTP(rec, req)
		})

		if recovered != nil {
			t.Errorf("want ServeHTTP: no panic; got: %v", recovered)
		}
		want := []int{http.StatusEarlyHints, http.StatusInternalServerError}
		if got := rec.statuses; !slices.Equal(want, got) {
			t.Errorf("want rec.statuses: %v; got: %v", want, got)
		}
	})

	t.Run("responds with a 500 when WriteHeader rejects the handler's status", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(1000)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		recovered := panicValue(func() {
			Recover(next).ServeHTTP(rec, req)
		})

		if recovered != nil {
			t.Errorf("want ServeHTTP: no panic; got: %v", recovered)
		}
		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
	})

	t.Run("responds with a 500 when the handler panics after failing to flush the response", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		var flushErr error
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			flushErr = http.NewResponseController(w).Flush()
			panic("handler failed")
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		w := unflushableWriter{ResponseWriter: rec}

		recovered := panicValue(func() {
			Recover(next).ServeHTTP(w, req)
		})

		if !errors.Is(flushErr, http.ErrNotSupported) {
			t.Fatalf("http.ResponseController.Flush: %v", flushErr)
		}
		if recovered != nil {
			t.Errorf("want ServeHTTP: no panic; got: %v", recovered)
		}
		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
	})

	t.Run("responds with a 500 when the handler panics after failing to hijack the connection", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		var hijackErr error
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _, hijackErr = http.NewResponseController(w).Hijack()
			panic("handler failed")
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		recovered := panicValue(func() {
			Recover(next).ServeHTTP(rec, req)
		})

		if !errors.Is(hijackErr, http.ErrNotSupported) {
			t.Fatalf("http.ResponseController.Hijack: %v", hijackErr)
		}
		if recovered != nil {
			t.Errorf("want ServeHTTP: no panic; got: %v", recovered)
		}
		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
	})

	t.Run("discards the handler's changes to the response headers when sending the 500", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Initial", "changed by the handler")
			w.Header().Del("X-Removed")
			w.Header().Set("X-Custom", "custom value")
			w.Header().Set("Content-Length", "999")
			panic("handler failed")
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		rec.Header().Set("X-Initial", "initial value")
		rec.Header().Set("X-Removed", "removed by the handler")

		recovered := panicValue(func() {
			Recover(next).ServeHTTP(rec, req)
		})

		if recovered != nil {
			t.Fatalf("want ServeHTTP: no panic; got: %v", recovered)
		}
		header := rec.Result().Header
		if want, got := "initial value", header.Get("X-Initial"); want != got {
			t.Errorf("want X-Initial: %q; got: %q", want, got)
		}
		if want, got := "removed by the handler", header.Get("X-Removed"); want != got {
			t.Errorf("want X-Removed: %q; got: %q", want, got)
		}
		if got := header.Get("X-Custom"); got != "" {
			t.Errorf("want X-Custom: empty; got: %q", got)
		}
		if got := header.Get("Content-Length"); got != "" {
			t.Errorf("want Content-Length: empty; got: %q", got)
		}
	})

	t.Run("aborts the response when the handler panics", func(t *testing.T) {
		for _, test := range []struct {
			name string
			next http.HandlerFunc
		}{
			{
				name: "after committing the final status",
				next: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusCreated)
					panic("handler failed")
				},
			},
			{
				name: "after committing a 101 status",
				next: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusSwitchingProtocols)
					panic("handler failed")
				},
			},
			{
				name: "after writing",
				next: func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte("hello"))
					panic("handler failed")
				},
			},
			{
				name: "after flushing through http.ResponseController",
				next: func(w http.ResponseWriter, r *http.Request) {
					_ = http.NewResponseController(w).Flush()
					panic("handler failed")
				},
			},
			{
				name: "after flushing through http.Flusher",
				next: func(w http.ResponseWriter, r *http.Request) {
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
					panic("handler failed")
				},
			},
			{
				name: "with http.ErrAbortHandler before committing the final status",
				next: func(w http.ResponseWriter, r *http.Request) {
					panic(http.ErrAbortHandler)
				},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

				req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
				rec := &statusRecordingWriter{ResponseRecorder: httptest.NewRecorder()}

				recovered := panicValue(func() {
					Recover(test.next).ServeHTTP(rec, req)
				})

				if want := http.ErrAbortHandler; want != recovered {
					t.Errorf("want recovered: %v; got: %v", want, recovered)
				}
				if got := rec.statuses; slices.Contains(got, http.StatusInternalServerError) {
					t.Errorf("want rec.statuses: without %d; got: %v", http.StatusInternalServerError, got)
				}
			})
		}
	})

	t.Run("aborts the response when the handler panics after hijacking the connection", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _, _ = http.NewResponseController(w).Hijack()
			panic("handler failed")
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := newHijackableWriter(t)

		recovered := panicValue(func() {
			Recover(next).ServeHTTP(rec, req)
		})

		if !rec.hijacked {
			t.Fatal("want rec.hijacked: true; got: false")
		}
		if want := http.ErrAbortHandler; want != recovered {
			t.Errorf("want recovered: %v; got: %v", want, recovered)
		}
		if got := rec.Body.String(); got != "" {
			t.Errorf("want rec.Body: empty; got: %q", got)
		}
	})

	t.Run("logs nothing when the handler does not panic", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		Recover(next).ServeHTTP(rec, req)

		if got := logs.String(); got != "" {
			t.Errorf("want logs: empty; got: %q", got)
		}
	})

	t.Run("logs an error when the handler panics", func(t *testing.T) {
		for _, test := range []struct {
			name          string
			next          http.HandlerFunc
			wantRecovered any
		}{
			{
				name: "before committing the final status",
				next: func(w http.ResponseWriter, r *http.Request) {
					panic("handler failed")
				},
				wantRecovered: nil,
			},
			{
				name: "after committing the final status",
				next: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusCreated)
					panic("handler failed")
				},
				wantRecovered: http.ErrAbortHandler,
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))
				ctx := WithLogger(t.Context(), logger)

				req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
				rec := httptest.NewRecorder()

				recovered := panicValue(func() {
					Recover(test.next).ServeHTTP(rec, req)
				})

				if want := test.wantRecovered; want != recovered {
					t.Fatalf("want recovered: %v; got: %v", want, recovered)
				}
				records := decodeLogRecords[recoverLogRecord](t, &logs)
				if want, got := 1, len(records); want != got {
					t.Fatalf("want len(records): %d; got: %d", want, got)
				}
				if want, got := slog.LevelError.String(), records[0].Level; want != got {
					t.Errorf("want records[0].Level: %q; got: %q", want, got)
				}
				if want, got := "panic serving request", records[0].Msg; want != got {
					t.Errorf("want records[0].Msg: %q; got: %q", want, got)
				}
				if want, got := "handler failed", records[0].Err; want != got {
					t.Errorf("want records[0].Err: %q; got: %q", want, got)
				}
				// The stack must reach the code that panicked, not stop at the recovery.
				if want, got := "recover_test.go", records[0].Stack; !strings.Contains(got, want) {
					t.Errorf("want records[0].Stack: containing %q; got: %q", want, got)
				}
			})
		}
	})

	t.Run("logs nothing when the handler panics with http.ErrAbortHandler", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic(http.ErrAbortHandler)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		recovered := panicValue(func() {
			Recover(next).ServeHTTP(rec, req)
		})

		if want := http.ErrAbortHandler; want != recovered {
			t.Fatalf("want recovered: %v; got: %v", want, recovered)
		}
		if got := logs.String(); got != "" {
			t.Errorf("want logs: empty; got: %q", got)
		}
	})

	t.Run("logs the request with the 500 when chained after RequestLog", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("handler failed")
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		recovered := panicValue(func() {
			Chain(next, RequestLog, Recover).ServeHTTP(rec, req)
		})

		if recovered != nil {
			t.Fatalf("want ServeHTTP: no panic; got: %v", recovered)
		}
		records := decodeLogRecords[recoverLogRecord](t, &logs)
		if want, got := 2, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := "request", records[1].Msg; want != got {
			t.Errorf("want records[1].Msg: %q; got: %q", want, got)
		}
		if want, got := http.StatusInternalServerError, records[1].Status; want != got {
			t.Errorf("want records[1].Status: %d; got: %d", want, got)
		}
	})
}

// recoverLogRecord is one record written by [Recover], or by [RequestLog] when
// chained before it.
type recoverLogRecord struct {
	Level  string `json:"level"`
	Msg    string `json:"msg"`
	Err    string `json:"err"`
	Stack  string `json:"stack"`
	Status int    `json:"status"`
}
