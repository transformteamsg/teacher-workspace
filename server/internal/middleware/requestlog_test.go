package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/String-sg/teacher-workspace/server/pkg/require"
)

func TestRequestLog(t *testing.T) {
	t.Run("logs method, path, status, and duration", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := context.WithValue(t.Context(), ctxKeyLogger{}, logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})

		req := httptest.NewRequest(http.MethodPost, "/users", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequestLog(next).ServeHTTP(rec, req)

		var record struct {
			Level      string `json:"level"`
			Msg        string `json:"msg"`
			Method     string `json:"method"`
			Path       string `json:"path"`
			Status     int    `json:"status"`
			DurationMS int64  `json:"duration_ms"`
		}
		if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
			t.Fatalf("failed to unmarshal log entry: %v", err)
		}

		require.Equal(t, "INFO", record.Level)
		require.Equal(t, "request", record.Msg)
		require.Equal(t, http.MethodPost, record.Method)
		require.Equal(t, "/users", record.Path)
		require.Equal(t, http.StatusCreated, record.Status)
		if record.DurationMS < 0 {
			t.Errorf("want: >= 0; got: %d", record.DurationMS)
		}
	})

	t.Run("logs status 200 when handler never calls WriteHeader", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := context.WithValue(t.Context(), ctxKeyLogger{}, logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequestLog(next).ServeHTTP(rec, req)

		var record struct {
			Status int `json:"status"`
		}
		if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
			t.Fatalf("failed to unmarshal log entry: %v", err)
		}

		require.Equal(t, http.StatusOK, record.Status)
	})

	t.Run("not-found responses are logged with status 404", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := context.WithValue(t.Context(), ctxKeyLogger{}, logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})

		req := httptest.NewRequest(http.MethodGet, "/unknown", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequestLog(next).ServeHTTP(rec, req)

		var record struct {
			Path   string `json:"path"`
			Status int    `json:"status"`
		}
		if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
			t.Fatalf("failed to unmarshal log entry: %v", err)
		}

		require.Equal(t, "/unknown", record.Path)
		require.Equal(t, http.StatusNotFound, record.Status)
	})

	t.Run("records status from first WriteHeader call only", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := context.WithValue(t.Context(), ctxKeyLogger{}, logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			w.WriteHeader(http.StatusInternalServerError)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequestLog(next).ServeHTTP(rec, req)

		var record struct {
			Status int `json:"status"`
		}
		if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
			t.Fatalf("failed to unmarshal log entry: %v", err)
		}

		require.Equal(t, http.StatusTeapot, record.Status)
	})

	t.Run("records implicit 200 when handler writes body without WriteHeader", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := context.WithValue(t.Context(), ctxKeyLogger{}, logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := w.Write([]byte("hello")); err != nil {
				t.Fatalf("failed to write: %v", err)
			}
		})

		req := httptest.NewRequest(http.MethodGet, "/hello", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequestLog(next).ServeHTTP(rec, req)

		var record struct {
			Status int `json:"status"`
		}
		if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
			t.Fatalf("failed to unmarshal log entry: %v", err)
		}

		require.Equal(t, http.StatusOK, record.Status)
	})

	t.Run("unwraps requestLogResponseWriter for ResponseController", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := context.WithValue(t.Context(), ctxKeyLogger{}, logger)

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Errorf("want err: nil; got: %v", err)
			}
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		RequestLog(next).ServeHTTP(rec, req)
	})

	t.Run("falls back to default logger when no request logger in context", func(t *testing.T) {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		// must not panic
		RequestLog(next).ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("concurrent requests get independent loggers with no data race", func(t *testing.T) {
		const n = 50
		var wg sync.WaitGroup
		wg.Add(n)

		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()

				var logs bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))
				ctx := context.WithValue(t.Context(), ctxKeyLogger{}, logger)

				next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				})

				req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
				rec := httptest.NewRecorder()

				RequestID(RequestLog(next)).ServeHTTP(rec, req)
			}()
		}

		wg.Wait()
	})
}
