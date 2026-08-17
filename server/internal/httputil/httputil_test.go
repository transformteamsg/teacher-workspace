package httputil_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/String-sg/teacher-workspace/server/internal/httputil"
)

func TestRenderPlain(t *testing.T) {
	t.Run("sets Content-Type header", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderPlain(rec, logger, http.StatusInternalServerError)
		res := rec.Result()

		if want, got := "text/plain; charset=UTF-8", res.Header.Get("Content-Type"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("sets X-Content-Type-Options header", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderPlain(rec, logger, http.StatusInternalServerError)
		res := rec.Result()

		if want, got := "nosniff", res.Header.Get("X-Content-Type-Options"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("responds with the expected status code", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderPlain(rec, logger, http.StatusInternalServerError)
		res := rec.Result()

		if want, got := 500, res.StatusCode; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})

	t.Run("responds with the expected body", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderPlain(rec, logger, http.StatusInternalServerError)

		if want, got := "Internal Server Error", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("logs error message when it fails to write body", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		rec := httptest.NewRecorder()

		httputil.RenderPlain(rec, logger, http.StatusNoContent)

		type logRecord struct {
			Level    string `json:"level"`
			Renderer string `json:"renderer"`
			Err      string `json:"err"`
		}
		var lc logRecord
		if err := json.Unmarshal(logs.Bytes(), &lc); err != nil {
			t.Fatalf("decoding log output: %v", err)
		}

		if want, got := "ERROR", lc.Level; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "plain", lc.Renderer; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "http: request method or response status code does not allow body", lc.Err; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})
}

func TestRenderJSON(t *testing.T) {
	t.Run("sets Content-Type header", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderJSON(rec, logger, http.StatusInternalServerError, nil)
		res := rec.Result()

		if want, got := "application/json; charset=UTF-8", res.Header.Get("Content-Type"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("sets X-Content-Type-Options header", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderJSON(rec, logger, http.StatusInternalServerError, nil)
		res := rec.Result()

		if want, got := "nosniff", res.Header.Get("X-Content-Type-Options"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("responds with the expected status code", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderJSON(rec, logger, http.StatusInternalServerError, nil)
		res := rec.Result()

		if want, got := 500, res.StatusCode; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
	})

	t.Run("responds with the expected body", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		rec := httptest.NewRecorder()

		httputil.RenderJSON(rec, logger, http.StatusInternalServerError, httputil.ErrorResponse{Message: "Internal Server Error"})

		if want, got := `{"message":"Internal Server Error"}`+"\n", rec.Body.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("logs error message when it fails to encode the body to JSON", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		rec := httptest.NewRecorder()

		httputil.RenderJSON(rec, logger, http.StatusInternalServerError, func() {})

		type logLine struct {
			Level    string `json:"level"`
			Renderer string `json:"renderer"`
			Err      string `json:"err"`
		}
		var line logLine
		if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
			t.Fatalf("decoding log output: %v", err)
		}

		if want, got := "ERROR", line.Level; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "json", line.Renderer; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "json: unsupported type: func()", line.Err; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})
}
