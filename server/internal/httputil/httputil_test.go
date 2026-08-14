package httputil_test

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
)

// errResponseWriter is an [httptest.ResponseRecorder] whose Write always fails,
// so tests can reach a renderer's write-error branch. The embedded recorder
// cannot: it writes to a buffer and never returns an error.
type errResponseWriter struct {
	*httptest.ResponseRecorder
}

func (w *errResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("connection reset")
}

func TestRenderPlain(t *testing.T) {
	t.Run("renders plain text", func(t *testing.T) {
		tests := []struct {
			name                   string
			status                 int
			wantStatus             int
			wantContentType        string
			wantContentTypeOptions string
			wantBody               string
		}{
			{
				name:                   "known status",
				status:                 http.StatusNotFound,
				wantStatus:             http.StatusNotFound,
				wantContentType:        "text/plain; charset=UTF-8",
				wantContentTypeOptions: "nosniff",
				wantBody:               "Not Found",
			},
			{
				name:                   "unknown status",
				status:                 599,
				wantStatus:             599,
				wantContentType:        "text/plain; charset=UTF-8",
				wantContentTypeOptions: "nosniff",
				wantBody:               "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))

				rec := httptest.NewRecorder()

				httputil.RenderPlain(rec, logger, tt.status)

				// Result reports the headers snapshotted at WriteHeader, so it
				// catches headers set too late to reach a real client.
				res := rec.Result()
				t.Cleanup(func() { _ = res.Body.Close() })

				if want, got := tt.wantStatus, res.StatusCode; want != got {
					t.Errorf("want: %d; got: %d", want, got)
				}
				if want, got := tt.wantContentType, res.Header.Get("Content-Type"); want != got {
					t.Errorf("want: %q; got: %q", want, got)
				}
				if want, got := tt.wantContentTypeOptions, res.Header.Get("X-Content-Type-Options"); want != got {
					t.Errorf("want: %q; got: %q", want, got)
				}
				if want, got := tt.wantBody, rec.Body.String(); want != got {
					t.Errorf("want: %q; got: %q", want, got)
				}
				if got := logs.String(); got != "" {
					t.Errorf("want: empty; got: %q", got)
				}
			})
		}
	})

	t.Run("logs write failures", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))

		w := &errResponseWriter{ResponseRecorder: httptest.NewRecorder()}

		httputil.RenderPlain(w, logger, http.StatusInternalServerError)

		// The level is the contract (it is what pages someone); the message
		// wording is not, so it is deliberately not asserted.
		logged := logs.String()
		if want := `"level":"ERROR"`; !strings.Contains(logged, want) {
			t.Errorf("want logged: containing %q; got: %q", want, logged)
		}
		if want := `"renderer":"plain"`; !strings.Contains(logged, want) {
			t.Errorf("want logged: containing %q; got: %q", want, logged)
		}
		if want := "connection reset"; !strings.Contains(logged, want) {
			t.Errorf("want logged: containing %q; got: %q", want, logged)
		}
	})
}
