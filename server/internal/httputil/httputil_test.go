package httputil_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
)

func TestRenderPlain(t *testing.T) {
	t.Run("writes the status text as a plain text response with the given status code", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderPlain(rec, logger, http.StatusServiceUnavailable)

		if want, got := http.StatusServiceUnavailable, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if want, got := "text/plain; charset=UTF-8", rec.Header().Get("Content-Type"); want != got {
			t.Errorf("want Content-Type: %q; got: %q", want, got)
		}
		if want, got := "nosniff", rec.Header().Get("X-Content-Type-Options"); want != got {
			t.Errorf("want X-Content-Type-Options: %q; got: %q", want, got)
		}
		if want, got := "Service Unavailable", rec.Body.String(); want != got {
			t.Errorf("want rec.Body.String(): %q; got: %q", want, got)
		}
	})

	t.Run("logs an error when the body cannot be written", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))

		writeErr := errors.New("connection reset")
		w := &failingWriter{
			ResponseRecorder: httptest.NewRecorder(),
			err:              writeErr,
		}

		httputil.RenderPlain(w, logger, http.StatusServiceUnavailable)

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "failed to write response body", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := "plain", records[0].Renderer; want != got {
			t.Errorf("want records[0].Renderer: %q; got: %q", want, got)
		}
		if want, got := writeErr.Error(), records[0].Err; !strings.Contains(got, want) {
			t.Errorf("want records[0].Err: containing %q; got: %q", want, got)
		}
	})
}

func TestRenderHTML(t *testing.T) {
	t.Run("writes the body as an HTML response with the given status code", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderHTML(rec, logger, http.StatusNotFound, []byte("<p>Not here</p>"))

		if want, got := http.StatusNotFound, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if want, got := "text/html; charset=UTF-8", rec.Header().Get("Content-Type"); want != got {
			t.Errorf("want Content-Type: %q; got: %q", want, got)
		}
		if want, got := "nosniff", rec.Header().Get("X-Content-Type-Options"); want != got {
			t.Errorf("want X-Content-Type-Options: %q; got: %q", want, got)
		}
		if want, got := "<p>Not here</p>", rec.Body.String(); want != got {
			t.Errorf("want rec.Body.String(): %q; got: %q", want, got)
		}
	})

	t.Run("logs an error when the body cannot be written", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))

		writeErr := errors.New("connection reset")
		w := &failingWriter{
			ResponseRecorder: httptest.NewRecorder(),
			err:              writeErr,
		}

		httputil.RenderHTML(w, logger, http.StatusNotFound, []byte("<p>Not here</p>"))

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "failed to write response body", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := "html", records[0].Renderer; want != got {
			t.Errorf("want records[0].Renderer: %q; got: %q", want, got)
		}
		if want, got := writeErr.Error(), records[0].Err; !strings.Contains(got, want) {
			t.Errorf("want records[0].Err: containing %q; got: %q", want, got)
		}
	})
}

func TestRenderJSON(t *testing.T) {
	t.Run("writes the value as a JSON response with the given status code", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderJSON(rec, logger, http.StatusCreated, map[string]string{"id": "post-1"})

		if want, got := http.StatusCreated, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if want, got := "application/json; charset=UTF-8", rec.Header().Get("Content-Type"); want != got {
			t.Errorf("want Content-Type: %q; got: %q", want, got)
		}
		if want, got := "nosniff", rec.Header().Get("X-Content-Type-Options"); want != got {
			t.Errorf("want X-Content-Type-Options: %q; got: %q", want, got)
		}
		if want, got := `{"id":"post-1"}`, rec.Body.String(); want != got {
			t.Errorf("want rec.Body.String(): %q; got: %q", want, got)
		}
	})

	t.Run("writes a 500 plain text response when the value cannot be encoded", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderJSON(rec, logger, http.StatusCreated, func() {})

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want rec.Code: %d; got: %d", want, got)
		}
		if want, got := "text/plain; charset=UTF-8", rec.Header().Get("Content-Type"); want != got {
			t.Errorf("want Content-Type: %q; got: %q", want, got)
		}
		if want, got := "nosniff", rec.Header().Get("X-Content-Type-Options"); want != got {
			t.Errorf("want X-Content-Type-Options: %q; got: %q", want, got)
		}
		if want, got := "Internal Server Error", rec.Body.String(); want != got {
			t.Errorf("want rec.Body.String(): %q; got: %q", want, got)
		}
	})

	t.Run("logs an error when the value cannot be encoded", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		rec := httptest.NewRecorder()

		httputil.RenderJSON(rec, logger, http.StatusCreated, func() {})

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "failed to encode response body", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := "json", records[0].Renderer; want != got {
			t.Errorf("want records[0].Renderer: %q; got: %q", want, got)
		}
		if want, got := "unsupported type", records[0].Err; !strings.Contains(got, want) {
			t.Errorf("want records[0].Err: containing %q; got: %q", want, got)
		}
	})

	t.Run("logs an error when the body cannot be written", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))

		writeErr := errors.New("connection reset")
		w := &failingWriter{
			ResponseRecorder: httptest.NewRecorder(),
			err:              writeErr,
		}

		httputil.RenderJSON(w, logger, http.StatusCreated, map[string]string{"id": "post-1"})

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "failed to write response body", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := "json", records[0].Renderer; want != got {
			t.Errorf("want records[0].Renderer: %q; got: %q", want, got)
		}
		if want, got := writeErr.Error(), records[0].Err; !strings.Contains(got, want) {
			t.Errorf("want records[0].Err: containing %q; got: %q", want, got)
		}
	})
}

func TestRedirect(t *testing.T) {
	t.Run("redirects to the URL when the status is", func(t *testing.T) {
		tests := []struct {
			name   string
			status int
		}{
			{
				name:   "301 Moved Permanently",
				status: http.StatusMovedPermanently,
			},
			{
				name:   "302 Found",
				status: http.StatusFound,
			},
			{
				name:   "303 See Other",
				status: http.StatusSeeOther,
			},
			{
				name:   "307 Temporary Redirect",
				status: http.StatusTemporaryRedirect,
			},
			{
				name:   "308 Permanent Redirect",
				status: http.StatusPermanentRedirect,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				logger := slog.New(slog.DiscardHandler)
				rec := httptest.NewRecorder()

				httputil.Redirect(rec, logger, test.status, "https://example.com/login")

				if want, got := test.status, rec.Code; want != got {
					t.Errorf("want rec.Code: %d; got: %d", want, got)
				}
				if want, got := "https://example.com/login", rec.Header().Get("Location"); want != got {
					t.Errorf("want Location: %q; got: %q", want, got)
				}
			})
		}
	})

	t.Run("sends a relative URL in the Location header as given", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.Redirect(rec, logger, http.StatusSeeOther, "./login?next=%2Fhome")

		if want, got := "./login?next=%2Fhome", rec.Header().Get("Location"); want != got {
			t.Errorf("want Location: %q; got: %q", want, got)
		}
	})

	t.Run("writes a 500 plain text response instead of redirecting when the status is", func(t *testing.T) {
		tests := []struct {
			name   string
			status int
		}{
			{
				name:   "200 OK",
				status: http.StatusOK,
			},
			{
				name:   "300 Multiple Choices",
				status: http.StatusMultipleChoices,
			},
			{
				name:   "304 Not Modified",
				status: http.StatusNotModified,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				logger := slog.New(slog.DiscardHandler)
				rec := httptest.NewRecorder()

				httputil.Redirect(rec, logger, test.status, "https://example.com/login")

				if want, got := http.StatusInternalServerError, rec.Code; want != got {
					t.Errorf("want rec.Code: %d; got: %d", want, got)
				}
				if want, got := "text/plain; charset=UTF-8", rec.Header().Get("Content-Type"); want != got {
					t.Errorf("want Content-Type: %q; got: %q", want, got)
				}
				if want, got := "nosniff", rec.Header().Get("X-Content-Type-Options"); want != got {
					t.Errorf("want X-Content-Type-Options: %q; got: %q", want, got)
				}
				if want, got := "Internal Server Error", rec.Body.String(); want != got {
					t.Errorf("want rec.Body.String(): %q; got: %q", want, got)
				}
				if got := rec.Header().Get("Location"); got != "" {
					t.Errorf("want Location: empty; got: %q", got)
				}
			})
		}
	})

	t.Run("logs an error when the status is not 301, 302, 303, 307, or 308", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		rec := httptest.NewRecorder()

		httputil.Redirect(rec, logger, http.StatusNotModified, "https://example.com/login")

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "invalid status code for redirect", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := http.StatusNotModified, records[0].Status; want != got {
			t.Errorf("want records[0].Status: %d; got: %d", want, got)
		}
	})
}

// failingWriter is an [http.ResponseWriter] whose Write always fails with err.
type failingWriter struct {
	*httptest.ResponseRecorder
	err error
}

func (w *failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

// logRecord is one record written by a [slog.JSONHandler].
type logRecord struct {
	Level    string `json:"level"`
	Msg      string `json:"msg"`
	Renderer string `json:"renderer"`
	Status   int    `json:"status"`
	Err      string `json:"err"`
}

// decodeLogRecords returns the records in logs, the output of a
// [slog.JSONHandler], in the order they were written.
func decodeLogRecords(t *testing.T, logs *bytes.Buffer) []logRecord {
	t.Helper()

	var records []logRecord
	for line := range bytes.Lines(logs.Bytes()) {
		var record logRecord
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		records = append(records, record)
	}

	return records
}
