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
	"testing"
	"testing/synctest"
	"time"
)

func TestRequestLog(t *testing.T) {
	t.Run("leaves the handler's response unchanged", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Custom", "custom value")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("hello"))
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequestLog(next).ServeHTTP(rec, req)

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
			w.WriteHeader(http.StatusInternalServerError)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := &statusRecordingWriter{ResponseRecorder: httptest.NewRecorder()}

		RequestLog(next).ServeHTTP(rec, req)

		want := []int{http.StatusEarlyHints, http.StatusCreated, http.StatusInternalServerError}
		if got := rec.statuses; !slices.Equal(want, got) {
			t.Errorf("want rec.statuses: %v; got: %v", want, got)
		}
	})

	t.Run("flushes the response when the handler flushes", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = http.NewResponseController(w).Flush()
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequestLog(next).ServeHTTP(rec, req)

		if !rec.Flushed {
			t.Error("want rec.Flushed: true; got: false")
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

		RequestLog(next).ServeHTTP(rec, req)

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

		RequestLog(next).ServeHTTP(rec, req)

		if !errors.Is(hijackErr, http.ErrNotSupported) {
			t.Errorf("want err: %v; got: %v", http.ErrNotSupported, hijackErr)
		}
	})

	t.Run("logs the request once the handler returns", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			ctx := WithLogger(t.Context(), logger)

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
				time.Sleep(1500 * time.Millisecond)
			})

			req := httptest.NewRequest(http.MethodPost, "/users?page=2", nil).WithContext(ctx)
			rec := httptest.NewRecorder()

			RequestLog(next).ServeHTTP(rec, req)

			records := decodeLogRecords[requestLogRecord](t, &logs)
			if want, got := 1, len(records); want != got {
				t.Fatalf("want len(records): %d; got: %d", want, got)
			}
			if want, got := slog.LevelInfo.String(), records[0].Level; want != got {
				t.Errorf("want records[0].Level: %q; got: %q", want, got)
			}
			if want, got := "request", records[0].Msg; want != got {
				t.Errorf("want records[0].Msg: %q; got: %q", want, got)
			}
			if want, got := http.MethodPost, records[0].Method; want != got {
				t.Errorf("want records[0].Method: %q; got: %q", want, got)
			}
			if want, got := "/users", records[0].Path; want != got {
				t.Errorf("want records[0].Path: %q; got: %q", want, got)
			}
			if want, got := http.StatusCreated, records[0].Status; want != got {
				t.Errorf("want records[0].Status: %d; got: %d", want, got)
			}
			if want, got := int64(1500), records[0].DurationMS; want != got {
				t.Errorf("want records[0].DurationMS: %d; got: %d", want, got)
			}
		})
	})

	t.Run("logs the final status the handler committed", func(t *testing.T) {
		for _, test := range []struct {
			name string
			next http.HandlerFunc
			want int
		}{
			{
				name: "as 200 when the handler writes nothing",
				next: func(w http.ResponseWriter, r *http.Request) {},
				want: http.StatusOK,
			},
			{
				name: "as 200 when the handler writes without calling WriteHeader",
				next: func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte("hello"))
				},
				want: http.StatusOK,
			},
			{
				name: "as 200 when WriteHeader follows a write",
				next: func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte("hello"))
					w.WriteHeader(http.StatusInternalServerError)
				},
				want: http.StatusOK,
			},
			{
				name: "as 200 when WriteHeader follows a flush",
				next: func(w http.ResponseWriter, r *http.Request) {
					_ = http.NewResponseController(w).Flush()
					w.WriteHeader(http.StatusInternalServerError)
				},
				want: http.StatusOK,
			},
			{
				name: "as 200 when a write follows a 1xx status",
				next: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusEarlyHints)
					_, _ = w.Write([]byte("hello"))
				},
				want: http.StatusOK,
			},
			{
				name: "as the first status when WriteHeader is called twice",
				next: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusTeapot)
					w.WriteHeader(http.StatusInternalServerError)
				},
				want: http.StatusTeapot,
			},
			{
				name: "as the status that follows a 1xx status",
				next: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusEarlyHints)
					w.WriteHeader(http.StatusCreated)
				},
				want: http.StatusCreated,
			},
			{
				name: "as 101 when WriteHeader follows a 101 status",
				next: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusSwitchingProtocols)
					w.WriteHeader(http.StatusInternalServerError)
				},
				want: http.StatusSwitchingProtocols,
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))
				ctx := WithLogger(t.Context(), logger)

				req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
				rec := httptest.NewRecorder()

				RequestLog(test.next).ServeHTTP(rec, req)

				records := decodeLogRecords[requestLogRecord](t, &logs)
				if len(records) == 0 {
					t.Fatal("want records: non-empty; got: empty")
				}
				if got := records[0].Status; test.want != got {
					t.Errorf("want records[0].Status: %d; got: %d", test.want, got)
				}
			})
		}
	})

	t.Run("logs the status the handler committed before hijacking the connection", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _, _ = http.NewResponseController(w).Hijack()
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := newHijackableWriter(t)

		RequestLog(next).ServeHTTP(rec, req)

		if !rec.hijacked {
			t.Fatal("want rec.hijacked: true; got: false")
		}

		records := decodeLogRecords[requestLogRecord](t, &logs)
		if len(records) == 0 {
			t.Fatal("want records: non-empty; got: empty")
		}
		if want, got := http.StatusCreated, records[0].Status; want != got {
			t.Errorf("want records[0].Status: %d; got: %d", want, got)
		}
	})

	t.Run("logs no status when the handler hijacks the connection", func(t *testing.T) {
		for _, test := range []struct {
			name string
			next http.HandlerFunc
		}{
			{
				name: "before committing one",
				next: func(w http.ResponseWriter, r *http.Request) {
					_, _, _ = http.NewResponseController(w).Hijack()
				},
			},
			{
				name: "and then calls WriteHeader",
				next: func(w http.ResponseWriter, r *http.Request) {
					_, _, _ = http.NewResponseController(w).Hijack()
					w.WriteHeader(http.StatusBadGateway)
				},
			},
			{
				name: "and then writes",
				next: func(w http.ResponseWriter, r *http.Request) {
					_, _, _ = http.NewResponseController(w).Hijack()
					_, _ = w.Write([]byte("hello"))
				},
			},
			{
				name: "and then flushes",
				next: func(w http.ResponseWriter, r *http.Request) {
					_, _, _ = http.NewResponseController(w).Hijack()
					_ = http.NewResponseController(w).Flush()
				},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))
				ctx := WithLogger(t.Context(), logger)

				req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
				rec := newHijackableWriter(t)

				RequestLog(test.next).ServeHTTP(rec, req)

				if !rec.hijacked {
					t.Fatal("want rec.hijacked: true; got: false")
				}

				records := decodeLogRecords[map[string]any](t, &logs)
				if len(records) == 0 {
					t.Fatal("want records: non-empty; got: empty")
				}
				if _, ok := records[0]["status"]; ok {
					t.Error("want records[0][\"status\"] ok: false; got: true")
				}
			})
		}
	})

	t.Run("logs the status the handler commits after its hijack fails", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _, _ = http.NewResponseController(w).Hijack()
			w.WriteHeader(http.StatusBadGateway)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequestLog(next).ServeHTTP(rec, req)

		records := decodeLogRecords[requestLogRecord](t, &logs)
		if len(records) == 0 {
			t.Fatal("want records: non-empty; got: empty")
		}
		if want, got := http.StatusBadGateway, records[0].Status; want != got {
			t.Errorf("want records[0].Status: %d; got: %d", want, got)
		}
	})

	t.Run("logs hijacked as true when the handler hijacks the connection", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _, _ = http.NewResponseController(w).Hijack()
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := newHijackableWriter(t)

		RequestLog(next).ServeHTTP(rec, req)

		if !rec.hijacked {
			t.Fatal("want rec.hijacked: true; got: false")
		}

		records := decodeLogRecords[requestLogRecord](t, &logs)
		if len(records) == 0 {
			t.Fatal("want records: non-empty; got: empty")
		}
		if !records[0].Hijacked {
			t.Error("want records[0].Hijacked: true; got: false")
		}
	})

	t.Run("does not log hijacked", func(t *testing.T) {
		for _, test := range []struct {
			name string
			next http.HandlerFunc
		}{
			{
				name: "when the handler does not hijack the connection",
				next: func(w http.ResponseWriter, r *http.Request) {},
			},
			{
				name: "when the handler's hijack fails",
				next: func(w http.ResponseWriter, r *http.Request) {
					_, _, _ = http.NewResponseController(w).Hijack()
				},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))
				ctx := WithLogger(t.Context(), logger)

				req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
				rec := httptest.NewRecorder()

				RequestLog(test.next).ServeHTTP(rec, req)

				records := decodeLogRecords[map[string]any](t, &logs)
				if len(records) == 0 {
					t.Fatal("want records: non-empty; got: empty")
				}
				if _, ok := records[0]["hijacked"]; ok {
					t.Error("want records[0][\"hijacked\"] ok: false; got: true")
				}
			})
		}
	})

	t.Run("logs the request when the handler panics", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			panic("handler failed")
		})

		req := httptest.NewRequest(http.MethodPost, "/users", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("want ServeHTTP: panic; got: nil")
				}
			}()

			RequestLog(next).ServeHTTP(rec, req)
		}()

		records := decodeLogRecords[requestLogRecord](t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelInfo.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "request", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := http.MethodPost, records[0].Method; want != got {
			t.Errorf("want records[0].Method: %q; got: %q", want, got)
		}
		if want, got := "/users", records[0].Path; want != got {
			t.Errorf("want records[0].Path: %q; got: %q", want, got)
		}
		if want, got := http.StatusCreated, records[0].Status; want != got {
			t.Errorf("want records[0].Status: %d; got: %d", want, got)
		}
	})

	t.Run("logs the status as 200 when the handler panics after a write", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello"))
			panic("handler failed")
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("want ServeHTTP: panic; got: nil")
				}
			}()

			RequestLog(next).ServeHTTP(rec, req)
		}()

		records := decodeLogRecords[requestLogRecord](t, &logs)
		if len(records) == 0 {
			t.Fatal("want records: non-empty; got: empty")
		}
		if want, got := http.StatusOK, records[0].Status; want != got {
			t.Errorf("want records[0].Status: %d; got: %d", want, got)
		}
	})

	t.Run("logs no status when the handler panics", func(t *testing.T) {
		for _, test := range []struct {
			name string
			next http.HandlerFunc
		}{
			{
				name: "before committing one",
				next: func(w http.ResponseWriter, r *http.Request) {
					panic("handler failed")
				},
			},
			{
				name: "after only a 1xx status",
				next: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusEarlyHints)
					panic("handler failed")
				},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))
				ctx := WithLogger(t.Context(), logger)

				req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
				rec := httptest.NewRecorder()

				func() {
					defer func() {
						if recover() == nil {
							t.Fatal("want ServeHTTP: panic; got: nil")
						}
					}()

					RequestLog(test.next).ServeHTTP(rec, req)
				}()

				records := decodeLogRecords[map[string]any](t, &logs)
				if len(records) == 0 {
					t.Fatal("want records: non-empty; got: empty")
				}
				if _, ok := records[0]["status"]; ok {
					t.Error("want records[0][\"status\"] ok: false; got: true")
				}
			})
		}
	})

	t.Run("logs aborted as true when the handler panics", func(t *testing.T) {
		for _, test := range []struct {
			name string
			next http.HandlerFunc
		}{
			{
				name: "before committing a status",
				next: func(w http.ResponseWriter, r *http.Request) {
					panic("handler failed")
				},
			},
			{
				name: "after committing a status",
				next: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusCreated)
					panic("handler failed")
				},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))
				ctx := WithLogger(t.Context(), logger)

				req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
				rec := httptest.NewRecorder()

				func() {
					defer func() {
						if recover() == nil {
							t.Fatal("want ServeHTTP: panic; got: nil")
						}
					}()

					RequestLog(test.next).ServeHTTP(rec, req)
				}()

				records := decodeLogRecords[requestLogRecord](t, &logs)
				if len(records) == 0 {
					t.Fatal("want records: non-empty; got: empty")
				}
				if !records[0].Aborted {
					t.Error("want records[0].Aborted: true; got: false")
				}
			})
		}
	})

	t.Run("logs aborted as true when the handler panics after hijacking the connection", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _, _ = http.NewResponseController(w).Hijack()
			panic("handler failed")
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := newHijackableWriter(t)

		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("want ServeHTTP: panic; got: nil")
				}
			}()

			RequestLog(next).ServeHTTP(rec, req)
		}()

		if !rec.hijacked {
			t.Fatal("want rec.hijacked: true; got: false")
		}

		records := decodeLogRecords[requestLogRecord](t, &logs)
		if len(records) == 0 {
			t.Fatal("want records: non-empty; got: empty")
		}
		if !records[0].Aborted {
			t.Error("want records[0].Aborted: true; got: false")
		}
	})

	t.Run("does not log aborted when the handler returns", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequestLog(next).ServeHTTP(rec, req)

		records := decodeLogRecords[map[string]any](t, &logs)
		if len(records) == 0 {
			t.Fatal("want records: non-empty; got: empty")
		}
		if _, ok := records[0]["aborted"]; ok {
			t.Error("want records[0][\"aborted\"] ok: false; got: true")
		}
	})

	t.Run("does not log aborted when the handler returns after hijacking the connection", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := WithLogger(t.Context(), logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _, _ = http.NewResponseController(w).Hijack()
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := newHijackableWriter(t)

		RequestLog(next).ServeHTTP(rec, req)

		if !rec.hijacked {
			t.Fatal("want rec.hijacked: true; got: false")
		}

		records := decodeLogRecords[map[string]any](t, &logs)
		if len(records) == 0 {
			t.Fatal("want records: non-empty; got: empty")
		}
		if _, ok := records[0]["aborted"]; ok {
			t.Error("want records[0][\"aborted\"] ok: false; got: true")
		}
	})

	t.Run("continues the handler's panic", func(t *testing.T) {
		ctx := WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic(http.ErrAbortHandler)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		var recovered any
		func() {
			defer func() {
				recovered = recover()
			}()

			RequestLog(next).ServeHTTP(rec, req)
		}()

		if want := http.ErrAbortHandler; want != recovered {
			t.Errorf("want recovered: %v; got: %v", want, recovered)
		}
	})
}

// requestLogRecord is one record written by [RequestLog].
type requestLogRecord struct {
	Level      string `json:"level"`
	Msg        string `json:"msg"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	Hijacked   bool   `json:"hijacked"`
	Aborted    bool   `json:"aborted"`
	DurationMS int64  `json:"duration_ms"`
}
