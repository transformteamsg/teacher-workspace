package handler

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/String-sg/teacher-workspace/server/internal/session"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

func TestHandler_authEdupass(t *testing.T) {
	t.Run("responds with 500 when the request has no session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want status: %d; got: %d", want, got)
		}
		if got := rec.Header().Get("Location"); got != "" {
			t.Errorf("want Location: empty; got: %q", got)
		}
	})

	t.Run("logs an error when the request has no session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := middleware.WithLogger(t.Context(), logger)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "no session found in context", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := "edupass", records[0].Provider; want != got {
			t.Errorf("want records[0].Provider: %q; got: %q", want, got)
		}
	})

	t.Run("redirects to the Edupass authorization endpoint", func(t *testing.T) {
		cfg := newEdupassConfig()
		h, err := New(cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, session.New())

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		if want, got := http.StatusFound, rec.Code; want != got {
			t.Errorf("want status: %d; got: %d", want, got)
		}

		location, err := rec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := cfg.Edupass.AuthURL.Scheme, location.Scheme; want != got {
			t.Errorf("want location.Scheme: %q; got: %q", want, got)
		}
		if want, got := cfg.Edupass.AuthURL.Host, location.Host; want != got {
			t.Errorf("want location.Host: %q; got: %q", want, got)
		}
		if want, got := cfg.Edupass.AuthURL.Path, location.Path; want != got {
			t.Errorf("want location.Path: %q; got: %q", want, got)
		}
	})

	t.Run("requests an authorization code for the configured client", func(t *testing.T) {
		cfg := newEdupassConfig()
		h, err := New(cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, session.New())

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		location, err := rec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		query := location.Query()
		if want, got := "code", query.Get("response_type"); want != got {
			t.Errorf("want response_type: %q; got: %q", want, got)
		}
		if want, got := cfg.Edupass.ClientID, query.Get("client_id"); want != got {
			t.Errorf("want client_id: %q; got: %q", want, got)
		}
		if want, got := cfg.Edupass.RedirectURL.String(), query.Get("redirect_uri"); want != got {
			t.Errorf("want redirect_uri: %q; got: %q", want, got)
		}
		if want, got := "openid", query.Get("scope"); want != got {
			t.Errorf("want scope: %q; got: %q", want, got)
		}
	})

	t.Run("requests an S256 code challenge", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, session.New())

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		location, err := rec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if want, got := "S256", location.Query().Get("code_challenge_method"); want != got {
			t.Errorf("want code_challenge_method: %q; got: %q", want, got)
		}
	})

	t.Run("records the state it sends to Edupass in the session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		sess := session.New()
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, sess)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		location, err := rec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		state, ok := sess.Get[string](sessionKeyEdupassState)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassState) ok: true; got: false")
		}
		if state == "" {
			t.Fatal("want state: non-empty; got: empty")
		}
		if got := location.Query().Get("state"); state != got {
			t.Errorf("want state: %q; got: %q", state, got)
		}
	})

	t.Run("records the nonce it sends to Edupass in the session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		sess := session.New()
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, sess)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		location, err := rec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		nonce, ok := sess.Get[string](sessionKeyEdupassNonce)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassNonce) ok: true; got: false")
		}
		if nonce == "" {
			t.Fatal("want nonce: non-empty; got: empty")
		}
		if got := location.Query().Get("nonce"); nonce != got {
			t.Errorf("want nonce: %q; got: %q", nonce, got)
		}
	})

	t.Run("records the code verifier for the code challenge it sends to Edupass in the session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		sess := session.New()
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, sess)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		location, err := rec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		codeVerifier, ok := sess.Get[string](sessionKeyEdupassCodeVerifier)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassCodeVerifier) ok: true; got: false")
		}
		if codeVerifier == "" {
			t.Fatal("want codeVerifier: non-empty; got: empty")
		}
		if want, got := oauth2.S256ChallengeFromVerifier(codeVerifier), location.Query().Get("code_challenge"); want != got {
			t.Errorf("want code_challenge: %q; got: %q", want, got)
		}
	})

	t.Run("records in the session the path the user lands on after logging in", func(t *testing.T) {
		for _, test := range []struct {
			name   string
			target string
			want   string
		}{
			{
				name:   "as the given return_to when it is safe",
				target: "/auth/edupass?return_to=" + url.QueryEscape("/announcements?id=1"),
				want:   "/announcements?id=1",
			},
			{
				name:   `as "/" when return_to is missing`,
				target: "/auth/edupass",
				want:   "/",
			},
			{
				name:   `as "/" when return_to is unsafe`,
				target: "/auth/edupass?return_to=" + url.QueryEscape("//evil.example"),
				want:   "/",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				h, err := New(newEdupassConfig())
				if err != nil {
					t.Fatalf("New: %v", err)
				}

				sess := session.New()
				ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
				ctx = middleware.WithSession(ctx, sess)

				req := httptest.NewRequestWithContext(ctx, http.MethodGet, test.target, nil)
				rec := httptest.NewRecorder()

				h.authEdupass(rec, req)

				returnTo, ok := sess.Get[string](sessionKeyEdupassReturnTo)
				if !ok {
					t.Fatal("want sess.Get(sessionKeyEdupassReturnTo) ok: true; got: false")
				}
				if want := test.want; want != returnTo {
					t.Errorf("want returnTo: %q; got: %q", want, returnTo)
				}
			})
		}
	})

	t.Run("replaces the pending login when another login starts on the same session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		sess := session.New()
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, sess)

		firstReq := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass?return_to=%2Ffirst", nil)
		firstRec := httptest.NewRecorder()
		h.authEdupass(firstRec, firstReq)

		secondReq := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass?return_to=%2Fsecond", nil)
		secondRec := httptest.NewRecorder()
		h.authEdupass(secondRec, secondReq)

		firstLocation, err := firstRec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		secondLocation, err := secondRec.Result().Location()
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		firstQuery := firstLocation.Query()
		secondQuery := secondLocation.Query()
		for _, param := range []string{"state", "nonce", "code_challenge"} {
			if want, got := firstQuery.Get(param), secondQuery.Get(param); want == got {
				t.Fatalf("want secondQuery.Get(%q): != %q; got: %q", param, want, got)
			}
		}

		state, ok := sess.Get[string](sessionKeyEdupassState)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassState) ok: true; got: false")
		}
		if want := secondQuery.Get("state"); want != state {
			t.Errorf("want state: %q; got: %q", want, state)
		}

		nonce, ok := sess.Get[string](sessionKeyEdupassNonce)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassNonce) ok: true; got: false")
		}
		if want := secondQuery.Get("nonce"); want != nonce {
			t.Errorf("want nonce: %q; got: %q", want, nonce)
		}

		codeVerifier, ok := sess.Get[string](sessionKeyEdupassCodeVerifier)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassCodeVerifier) ok: true; got: false")
		}
		if want, got := secondQuery.Get("code_challenge"), oauth2.S256ChallengeFromVerifier(codeVerifier); want != got {
			t.Errorf("want code challenge of codeVerifier: %q; got: %q", want, got)
		}

		returnTo, ok := sess.Get[string](sessionKeyEdupassReturnTo)
		if !ok {
			t.Fatal("want sess.Get(sessionKeyEdupassReturnTo) ok: true; got: false")
		}
		if want := "/second"; want != returnTo {
			t.Errorf("want returnTo: %q; got: %q", want, returnTo)
		}
	})
}

func TestHandler_authEdupassCallback(t *testing.T) {
	t.Run("responds with 500 when the request has no session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback", nil)
		rec := httptest.NewRecorder()

		h.authEdupassCallback(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Errorf("want status: %d; got: %d", want, got)
		}
		if got := rec.Header().Get("Location"); got != "" {
			t.Errorf("want Location: empty; got: %q", got)
		}
	})

	t.Run("logs an error when the request has no session", func(t *testing.T) {
		h, err := New(newEdupassConfig())
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		var logs bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logs, nil))
		ctx := middleware.WithLogger(t.Context(), logger)

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback", nil)
		rec := httptest.NewRecorder()

		h.authEdupassCallback(rec, req)

		records := decodeLogRecords(t, &logs)
		if want, got := 1, len(records); want != got {
			t.Fatalf("want len(records): %d; got: %d", want, got)
		}
		if want, got := slog.LevelError.String(), records[0].Level; want != got {
			t.Errorf("want records[0].Level: %q; got: %q", want, got)
		}
		if want, got := "no session found in context", records[0].Msg; want != got {
			t.Errorf("want records[0].Msg: %q; got: %q", want, got)
		}
		if want, got := "edupass", records[0].Provider; want != got {
			t.Errorf("want records[0].Provider: %q; got: %q", want, got)
		}
	})

	t.Run("sends the code and code verifier", func(t *testing.T) {
		var tokenPostForm url.Values
		edupass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			tokenPostForm = r.PostForm
			w.WriteHeader(http.StatusBadRequest)
		}))
		t.Cleanup(edupass.Close)

		cfg := newEdupassConfig()
		cfg.Edupass.TokenURL = mustParseURL(t, edupass.URL+"/token")
		h, err := New(cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		sess := session.New()
		sess.Set(sessionKeyEdupassState, "test-state")
		sess.Set(sessionKeyEdupassNonce, "test-nonce")
		sess.Set(sessionKeyEdupassCodeVerifier, "test-verifier")
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, sess)
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback?code=test-code&state=test-state", nil)

		h.authEdupassCallback(httptest.NewRecorder(), req)

		if tokenPostForm == nil {
			t.Fatal("want: a token request; got: none")
		}
		if want, got := "authorization_code", tokenPostForm.Get("grant_type"); want != got {
			t.Errorf("want grant_type: %q; got: %q", want, got)
		}
		if want, got := "test-code", tokenPostForm.Get("code"); want != got {
			t.Errorf("want code: %q; got: %q", want, got)
		}
		if want, got := cfg.Edupass.RedirectURL.String(), tokenPostForm.Get("redirect_uri"); want != got {
			t.Errorf("want redirect_uri: %q; got: %q", want, got)
		}
		if want, got := "test-verifier", tokenPostForm.Get("code_verifier"); want != got {
			t.Errorf("want code_verifier: %q; got: %q", want, got)
		}
		if want, got := cfg.Edupass.ClientID, tokenPostForm.Get("client_id"); want != got {
			t.Errorf("want client_id: %q; got: %q", want, got)
		}
	})

	for _, test := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "Edupass rejects the client",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				if _, err := w.Write([]byte(`{"error":"invalid_client"}`)); err != nil {
					t.Errorf("w.Write: %v", err)
				}
			},
		},
		{
			name: "Edupass rejects the code",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				if _, err := w.Write([]byte(`{"error":"invalid_grant"}`)); err != nil {
					t.Errorf("w.Write: %v", err)
				}
			},
		},
		{
			name: "the token response has no ID token",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"access_token":"test-access-token","token_type":"Bearer"}`)); err != nil {
					t.Errorf("w.Write: %v", err)
				}
			},
		},
	} {
		t.Run("redirects to the login page without logging in when "+test.name, func(t *testing.T) {
			edupass := httptest.NewServer(test.handler)
			t.Cleanup(edupass.Close)

			cfg := newEdupassConfig()
			cfg.Edupass.TokenURL = mustParseURL(t, edupass.URL+"/token")
			h, err := New(cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			sess := session.New()
			sess.Set(sessionKeyEdupassState, "test-state")
			sess.Set(sessionKeyEdupassNonce, "test-nonce")
			sess.Set(sessionKeyEdupassCodeVerifier, "test-verifier")
			ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
			ctx = middleware.WithSession(ctx, sess)
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback?code=test-code&state=test-state", nil)
			rec := httptest.NewRecorder()

			h.authEdupassCallback(rec, req)

			if want, got := http.StatusFound, rec.Code; want != got {
				t.Fatalf("want status: %d; got: %d", want, got)
			}
			if want, got := loginFailedURL("/"), rec.Header().Get("Location"); want != got {
				t.Errorf("want Location: %q; got: %q", want, got)
			}
			if sess.IsAuthenticated() {
				t.Error("want sess.IsAuthenticated(): false; got: true")
			}
		})
	}

	t.Run("redirects to the login page without logging in when the token request times out", func(t *testing.T) {
		edupass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				t.Errorf("r.ParseForm: %v", err)
			}
			<-r.Context().Done()
		}))
		t.Cleanup(edupass.Close)

		cfg := newEdupassConfig()
		cfg.Edupass.TokenURL = mustParseURL(t, edupass.URL+"/token")
		h, err := New(cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		h.edupassHTTPClient = &http.Client{Timeout: 200 * time.Millisecond}

		sess := session.New()
		sess.Set(sessionKeyEdupassState, "test-state")
		sess.Set(sessionKeyEdupassNonce, "test-nonce")
		sess.Set(sessionKeyEdupassCodeVerifier, "test-verifier")
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
		ctx = middleware.WithSession(ctx, sess)
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback?code=test-code&state=test-state", nil)
		rec := httptest.NewRecorder()

		h.authEdupassCallback(rec, req)

		if want, got := http.StatusFound, rec.Code; want != got {
			t.Fatalf("want status: %d; got: %d", want, got)
		}
		if want, got := loginFailedURL("/"), rec.Header().Get("Location"); want != got {
			t.Errorf("want Location: %q; got: %q", want, got)
		}
		if sess.IsAuthenticated() {
			t.Error("want sess.IsAuthenticated(): false; got: true")
		}
	})

	t.Run("logs the fields of a failed token response but not the client secret", func(t *testing.T) {
		edupass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			if _, err := w.Write([]byte(`{
				"error": "invalid_client",
				"error_description": "client authentication failed",
				"trace_id": "test-trace-id",
				"correlation_id": "test-correlation-id"
			}`)); err != nil {
				t.Errorf("w.Write: %v", err)
			}
		}))
		t.Cleanup(edupass.Close)

		cfg := newEdupassConfig()
		cfg.Edupass.TokenURL = mustParseURL(t, edupass.URL+"/token")
		cfg.Edupass.ClientCredentials = config.EdupassClientCredentials{Secret: "test-secret"}
		h, err := New(cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		var logs bytes.Buffer
		sess := session.New()
		sess.Set(sessionKeyEdupassState, "test-state")
		sess.Set(sessionKeyEdupassNonce, "test-nonce")
		sess.Set(sessionKeyEdupassCodeVerifier, "test-verifier")
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)))
		ctx = middleware.WithSession(ctx, sess)
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback?code=test-code&state=test-state", nil)

		h.authEdupassCallback(httptest.NewRecorder(), req)

		var record map[string]any
		if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
			t.Fatalf("json.Unmarshal: %v; logs: %s", err, logs.String())
		}
		for field, want := range map[string]any{
			"msg":               "failed to exchange code for token",
			"provider":          "edupass",
			"status":            float64(http.StatusUnauthorized),
			"error":             "invalid_client",
			"error_description": "client authentication failed",
			"trace_id":          "test-trace-id",
			"correlation_id":    "test-correlation-id",
		} {
			if got := record[field]; want != got {
				t.Errorf("want record[%q]: %v; got: %v", field, want, got)
			}
		}
		if got := logs.String(); strings.Contains(got, "test-secret") {
			t.Errorf("want logs: without the client secret; got: %s", got)
		}
	})

	t.Run("logs the decode error of a 400 or 401 token response that is not JSON", func(t *testing.T) {
		edupass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusUnauthorized)
			if _, err := w.Write([]byte("<html><body>401 Unauthorized</body></html>")); err != nil {
				t.Errorf("w.Write: %v", err)
			}
		}))
		t.Cleanup(edupass.Close)

		cfg := newEdupassConfig()
		cfg.Edupass.TokenURL = mustParseURL(t, edupass.URL+"/token")
		h, err := New(cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		var logs bytes.Buffer
		sess := session.New()
		sess.Set(sessionKeyEdupassState, "test-state")
		sess.Set(sessionKeyEdupassNonce, "test-nonce")
		sess.Set(sessionKeyEdupassCodeVerifier, "test-verifier")
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)))
		ctx = middleware.WithSession(ctx, sess)
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback?code=test-code&state=test-state", nil)

		h.authEdupassCallback(httptest.NewRecorder(), req)

		var record map[string]any
		if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
			t.Fatalf("json.Unmarshal: %v; logs: %s", err, logs.String())
		}
		if want, got := "failed to exchange code for token", record["msg"]; want != got {
			t.Errorf("want record[\"msg\"]: %q; got: %v", want, got)
		}
		if want, got := float64(http.StatusUnauthorized), record["status"]; want != got {
			t.Errorf("want record[\"status\"]: %v; got: %v", want, got)
		}
		if got, ok := record["err"].(string); !ok || got == "" {
			t.Errorf("want record[\"err\"]: non-empty; got: %v", record["err"])
		}
	})

	t.Run("logs only the status of a failed token response other than 400 or 401", func(t *testing.T) {
		edupass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			if _, err := w.Write([]byte(`{"error": "server_error", "trace_id": "test-trace-id"}`)); err != nil {
				t.Errorf("w.Write: %v", err)
			}
		}))
		t.Cleanup(edupass.Close)

		cfg := newEdupassConfig()
		cfg.Edupass.TokenURL = mustParseURL(t, edupass.URL+"/token")
		h, err := New(cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}

		var logs bytes.Buffer
		sess := session.New()
		sess.Set(sessionKeyEdupassState, "test-state")
		sess.Set(sessionKeyEdupassNonce, "test-nonce")
		sess.Set(sessionKeyEdupassCodeVerifier, "test-verifier")
		ctx := middleware.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)))
		ctx = middleware.WithSession(ctx, sess)
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback?code=test-code&state=test-state", nil)
		rec := httptest.NewRecorder()

		h.authEdupassCallback(rec, req)

		if want, got := loginFailedURL("/"), rec.Header().Get("Location"); want != got {
			t.Errorf("want Location: %q; got: %q", want, got)
		}
		var record map[string]any
		if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
			t.Fatalf("json.Unmarshal: %v; logs: %s", err, logs.String())
		}
		if want, got := "failed to exchange code for token", record["msg"]; want != got {
			t.Errorf("want record[\"msg\"]: %q; got: %v", want, got)
		}
		if want, got := float64(http.StatusBadGateway), record["status"]; want != got {
			t.Errorf("want record[\"status\"]: %v; got: %v", want, got)
		}
		for _, field := range []string{"error", "error_description", "trace_id", "correlation_id"} {
			if _, ok := record[field]; ok {
				t.Errorf("want record[%q]: absent; got: present", field)
			}
		}
	})

	t.Run("client_secret_post", func(t *testing.T) {
		t.Run("sends client_secret", func(t *testing.T) {
			var tokenPostForm url.Values
			edupass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				tokenPostForm = r.PostForm
				w.WriteHeader(http.StatusBadRequest)
			}))
			t.Cleanup(edupass.Close)

			cfg := newEdupassConfig()
			cfg.Edupass.TokenURL = mustParseURL(t, edupass.URL+"/token")
			cfg.Edupass.ClientAuthMethod = config.EdupassClientAuthMethodClientSecretPost
			cfg.Edupass.ClientCredentials = config.EdupassClientCredentials{Secret: "test-secret"}
			h, err := New(cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			sess := session.New()
			sess.Set(sessionKeyEdupassState, "test-state")
			sess.Set(sessionKeyEdupassNonce, "test-nonce")
			sess.Set(sessionKeyEdupassCodeVerifier, "test-verifier")
			ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
			ctx = middleware.WithSession(ctx, sess)
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback?code=test-code&state=test-state", nil)

			h.authEdupassCallback(httptest.NewRecorder(), req)

			if tokenPostForm == nil {
				t.Fatal("want: a token request; got: none")
			}
			if want, got := "test-secret", tokenPostForm.Get("client_secret"); want != got {
				t.Errorf("want client_secret: %q; got: %q", want, got)
			}
			if tokenPostForm.Has("client_assertion") {
				t.Error("want client_assertion: absent; got: present")
			}
		})
	})

	t.Run("private_key_jwt", func(t *testing.T) {
		t.Run("sends client_assertion", func(t *testing.T) {
			var tokenPostForm url.Values
			edupass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				tokenPostForm = r.PostForm
				w.WriteHeader(http.StatusBadRequest)
			}))
			t.Cleanup(edupass.Close)

			clientPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatalf("rsa.GenerateKey: %v", err)
			}
			cfg := newEdupassConfig()
			cfg.Edupass.TokenURL = mustParseURL(t, edupass.URL+"/token")
			cfg.Edupass.ClientAuthMethod = config.EdupassClientAuthMethodPrivateKeyJWT
			cfg.Edupass.ClientCredentials = config.EdupassClientCredentials{
				Secret:                "test-secret",
				Key:                   clientPrivateKey,
				CertificateThumbprint: "test-certificate-thumbprint",
			}
			h, err := New(cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			sess := session.New()
			sess.Set(sessionKeyEdupassState, "test-state")
			sess.Set(sessionKeyEdupassNonce, "test-nonce")
			sess.Set(sessionKeyEdupassCodeVerifier, "test-verifier")
			ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
			ctx = middleware.WithSession(ctx, sess)
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback?code=test-code&state=test-state", nil)

			h.authEdupassCallback(httptest.NewRecorder(), req)

			if tokenPostForm == nil {
				t.Fatal("want: a token request; got: none")
			}
			if got := tokenPostForm.Get("client_assertion"); got == "" {
				t.Error("want: non-empty; got: empty")
			}
			if want, got := "urn:ietf:params:oauth:client-assertion-type:jwt-bearer", tokenPostForm.Get("client_assertion_type"); want != got {
				t.Errorf("want: %q; got: %q", want, got)
			}
			if got := tokenPostForm.Has("client_secret"); got {
				t.Error("want: false; got: true")
			}
		})

		t.Run("signs client_assertion with PS256 and the certificate thumbprint", func(t *testing.T) {
			var tokenPostForm url.Values
			edupass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				tokenPostForm = r.PostForm
				w.WriteHeader(http.StatusBadRequest)
			}))
			t.Cleanup(edupass.Close)

			clientPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatalf("rsa.GenerateKey: %v", err)
			}
			cfg := newEdupassConfig()
			cfg.Edupass.TokenURL = mustParseURL(t, edupass.URL+"/token")
			cfg.Edupass.ClientAuthMethod = config.EdupassClientAuthMethodPrivateKeyJWT
			cfg.Edupass.ClientCredentials = config.EdupassClientCredentials{
				Key:                   clientPrivateKey,
				CertificateThumbprint: "test-certificate-thumbprint",
			}
			h, err := New(cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			sess := session.New()
			sess.Set(sessionKeyEdupassState, "test-state")
			sess.Set(sessionKeyEdupassNonce, "test-nonce")
			sess.Set(sessionKeyEdupassCodeVerifier, "test-verifier")
			ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
			ctx = middleware.WithSession(ctx, sess)
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback?code=test-code&state=test-state", nil)

			h.authEdupassCallback(httptest.NewRecorder(), req)

			if tokenPostForm == nil {
				t.Fatal("want: a token request; got: none")
			}
			clientAssertion, err := jwt.Parse(
				tokenPostForm.Get("client_assertion"),
				func(*jwt.Token) (any, error) { return &clientPrivateKey.PublicKey, nil },
				jwt.WithValidMethods([]string{"PS256"}),
			)
			if err != nil {
				t.Fatalf("want err: nil; got: %v", err)
			}
			if want, got := "test-certificate-thumbprint", clientAssertion.Header["x5t#S256"]; want != got {
				t.Errorf("want: %q; got: %v", want, got)
			}
		})

		t.Run("addresses client_assertion from the client to the token endpoint", func(t *testing.T) {
			var tokenPostForm url.Values
			edupass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				tokenPostForm = r.PostForm
				w.WriteHeader(http.StatusBadRequest)
			}))
			t.Cleanup(edupass.Close)

			clientPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatalf("rsa.GenerateKey: %v", err)
			}
			cfg := newEdupassConfig()
			cfg.Edupass.TokenURL = mustParseURL(t, edupass.URL+"/token")
			cfg.Edupass.ClientAuthMethod = config.EdupassClientAuthMethodPrivateKeyJWT
			cfg.Edupass.ClientCredentials = config.EdupassClientCredentials{
				Key:                   clientPrivateKey,
				CertificateThumbprint: "test-certificate-thumbprint",
			}
			h, err := New(cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			sess := session.New()
			sess.Set(sessionKeyEdupassState, "test-state")
			sess.Set(sessionKeyEdupassNonce, "test-nonce")
			sess.Set(sessionKeyEdupassCodeVerifier, "test-verifier")
			ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
			ctx = middleware.WithSession(ctx, sess)
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback?code=test-code&state=test-state", nil)

			h.authEdupassCallback(httptest.NewRecorder(), req)

			if tokenPostForm == nil {
				t.Fatal("want: a token request; got: none")
			}
			var clientAssertionClaims jwt.RegisteredClaims
			if _, _, err := jwt.NewParser().ParseUnverified(tokenPostForm.Get("client_assertion"), &clientAssertionClaims); err != nil {
				t.Fatalf("jwt.Parser.ParseUnverified: %v", err)
			}
			if want, got := cfg.Edupass.ClientID, clientAssertionClaims.Issuer; want != got {
				t.Errorf("want: %q; got: %q", want, got)
			}
			if want, got := cfg.Edupass.ClientID, clientAssertionClaims.Subject; want != got {
				t.Errorf("want: %q; got: %q", want, got)
			}
			if want, got := edupass.URL+"/token", clientAssertionClaims.Audience; len(got) != 1 || want != got[0] {
				t.Errorf("want: [%q]; got: %q", want, got)
			}
		})

		t.Run("expires client_assertion within 5 minutes", func(t *testing.T) {
			var tokenPostForm url.Values
			edupass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				tokenPostForm = r.PostForm
				w.WriteHeader(http.StatusBadRequest)
			}))
			t.Cleanup(edupass.Close)

			clientPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatalf("rsa.GenerateKey: %v", err)
			}
			cfg := newEdupassConfig()
			cfg.Edupass.TokenURL = mustParseURL(t, edupass.URL+"/token")
			cfg.Edupass.ClientAuthMethod = config.EdupassClientAuthMethodPrivateKeyJWT
			cfg.Edupass.ClientCredentials = config.EdupassClientCredentials{
				Key:                   clientPrivateKey,
				CertificateThumbprint: "test-certificate-thumbprint",
			}
			h, err := New(cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			sess := session.New()
			sess.Set(sessionKeyEdupassState, "test-state")
			sess.Set(sessionKeyEdupassNonce, "test-nonce")
			sess.Set(sessionKeyEdupassCodeVerifier, "test-verifier")
			ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
			ctx = middleware.WithSession(ctx, sess)
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback?code=test-code&state=test-state", nil)

			signedBefore := time.Now().Truncate(time.Second)
			h.authEdupassCallback(httptest.NewRecorder(), req)

			if tokenPostForm == nil {
				t.Fatal("want: a token request; got: none")
			}
			var clientAssertionClaims jwt.RegisteredClaims
			if _, _, err := jwt.NewParser().ParseUnverified(tokenPostForm.Get("client_assertion"), &clientAssertionClaims); err != nil {
				t.Fatalf("jwt.Parser.ParseUnverified: %v", err)
			}
			if clientAssertionClaims.ExpiresAt == nil {
				t.Fatal("want: non-nil; got: nil")
			}
			if want, got := signedBefore.Add(5*time.Minute+time.Second), clientAssertionClaims.ExpiresAt.Time; got.After(want) {
				t.Errorf("want: no later than %v; got: %v", want, got)
			}
		})

		t.Run("sends a new jti per request", func(t *testing.T) {
			var tokenPostForms []url.Values
			edupass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				tokenPostForms = append(tokenPostForms, r.PostForm)
				w.WriteHeader(http.StatusBadRequest)
			}))
			t.Cleanup(edupass.Close)

			clientPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatalf("rsa.GenerateKey: %v", err)
			}
			cfg := newEdupassConfig()
			cfg.Edupass.TokenURL = mustParseURL(t, edupass.URL+"/token")
			cfg.Edupass.ClientAuthMethod = config.EdupassClientAuthMethodPrivateKeyJWT
			cfg.Edupass.ClientCredentials = config.EdupassClientCredentials{Key: clientPrivateKey}
			h, err := New(cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			for range 2 {
				sess := session.New()
				sess.Set(sessionKeyEdupassState, "test-state")
				sess.Set(sessionKeyEdupassNonce, "test-nonce")
				sess.Set(sessionKeyEdupassCodeVerifier, "test-verifier")
				ctx := middleware.WithLogger(t.Context(), slog.New(slog.DiscardHandler))
				ctx = middleware.WithSession(ctx, sess)
				req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/edupass/callback?code=test-code&state=test-state", nil)

				h.authEdupassCallback(httptest.NewRecorder(), req)
			}

			if got := len(tokenPostForms); got != 2 {
				t.Fatalf("want: 2 token requests; got: %d", got)
			}
			clientAssertionIDs := make([]string, 0, len(tokenPostForms))
			for _, tokenPostForm := range tokenPostForms {
				var clientAssertionClaims jwt.RegisteredClaims
				if _, _, err := jwt.NewParser().ParseUnverified(tokenPostForm.Get("client_assertion"), &clientAssertionClaims); err != nil {
					t.Fatalf("jwt.Parser.ParseUnverified: %v", err)
				}
				clientAssertionIDs = append(clientAssertionIDs, clientAssertionClaims.ID)
			}
			if want, got := clientAssertionIDs[0], clientAssertionIDs[1]; want == got {
				t.Errorf("want: != %q; got: %q", want, got)
			}
		})
	})

}

func TestSafeReturnTo(t *testing.T) {
	t.Run("returns the given path", func(t *testing.T) {
		for _, test := range []struct {
			name      string
			candidate string
		}{
			{
				name:      "when it is the root path",
				candidate: "/",
			},
			{
				name:      "when it is a nested path",
				candidate: "/classes/123",
			},
			{
				name:      "when it has a query string",
				candidate: "/classes?tab=students",
			},
			{
				name:      "when it has a fragment",
				candidate: "/classes#roster",
			},
			{
				name:      "when it has a percent-encoded slash in a segment",
				candidate: "/groups/P5%2F3",
			},
			{
				name:      "when its query holds an encoded absolute URL",
				candidate: "/search?q=https%3A%2F%2Fevil.example.com",
			},
			{
				name:      "when its query holds a dot-dot",
				candidate: "/search?q=../classes",
			},
			{
				name:      "when its fragment holds a dot-dot",
				candidate: "/classes#../roster",
			},
			{
				name:      "when its query holds a backslash",
				candidate: `/search?q=a\b`,
			},
			{
				name:      "when it has a trailing slash",
				candidate: "/classes/",
			},
			{
				name:      "when a segment starts with a dot",
				candidate: "/.well-known/security.txt",
			},
			{
				name:      "when a segment starts with a dot-dot",
				candidate: "/files/..draft",
			},
			{
				name:      "when it is exactly 1024 bytes long",
				candidate: "/" + strings.Repeat("a", 1023),
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				returnTo := safeReturnTo(test.candidate, "/home")

				if want := test.candidate; want != returnTo {
					t.Errorf("want returnTo: %q; got: %q", want, returnTo)
				}
			})
		}
	})

	t.Run("returns the fallback", func(t *testing.T) {
		for _, test := range []struct {
			name      string
			candidate string
		}{
			{
				name:      "when the given path is empty",
				candidate: "",
			},
			{
				name:      "when the given path is relative",
				candidate: "classes/123",
			},
			{
				name:      "when the given path is a bare host name",
				candidate: "evil.example.com",
			},
			{
				name:      "when the given path is only a query string",
				candidate: "?tab=students",
			},
			{
				name:      "when the given path is an absolute URL",
				candidate: "https://evil.example.com/classes",
			},
			{
				name:      "when the given path has a javascript scheme",
				candidate: "javascript:alert(1)",
			},
			{
				name:      "when the given path is protocol-relative",
				candidate: "//evil.example.com/classes",
			},
			{
				name:      "when the given path is percent-encoded protocol-relative",
				candidate: "%2F%2Fevil.example.com/classes",
			},
			{
				name:      "when the given path starts with a slash and a backslash",
				candidate: `/\evil.example.com/classes`,
			},
			{
				name:      "when the given path has a backslash",
				candidate: `/classes\roster`,
			},
			{
				name:      "when the given path has a percent-encoded backslash",
				candidate: "/%5Cevil.example.com",
			},
			{
				name:      "when the given path is longer than 1024 bytes",
				candidate: "/" + strings.Repeat("a", 1024),
			},
			{
				name:      "when the given path has an invalid percent-encoding",
				candidate: "/classes/%zz",
			},
			{
				name:      "when the given path has a control character",
				candidate: "/\t/evil.example.com/classes",
			},
			{
				name:      "when the given path has a CRLF header injection",
				candidate: "/classes\r\nSet-Cookie: session=evil",
			},
			{
				name:      "when the given path has a dot-dot segment",
				candidate: "/x/../classes",
			},
			{
				name:      "when the given path has consecutive dot-dot segments",
				candidate: "/a/b/../../classes",
			},
			{
				name:      "when the given path has a percent-encoded dot-dot segment",
				candidate: "/x/%2e%2e/classes",
			},
			{
				name:      "when the given path has an uppercase percent-encoded dot-dot segment",
				candidate: "/x/%2E%2E/classes",
			},
			{
				name:      "when the given path has a partly percent-encoded dot-dot segment",
				candidate: "/x/.%2e/classes",
			},
			{
				name:      "when the given path resolves to a protocol-relative path through a dot-dot segment",
				candidate: "/a/..//evil.example.com",
			},
			{
				name:      "when the given path has a dot segment",
				candidate: "/./classes",
			},
			{
				name:      "when the given path resolves to a protocol-relative path through a dot segment",
				candidate: "/.//evil.example.com",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				returnTo := safeReturnTo(test.candidate, "/home")

				if want := "/home"; want != returnTo {
					t.Errorf("want returnTo: %q; got: %q", want, returnTo)
				}
			})
		}
	})
}

// newEdupassConfig returns a development config with every Edupass setting
// filled in, so [New] can build a handler from it.
func newEdupassConfig() *config.Config {
	return &config.Config{
		Env: config.EnvDevelopment,
		DevServerURL: &url.URL{
			Scheme: "http",
			Host:   "127.0.0.1:3001",
		},
		Edupass: config.EdupassConfig{
			IssuerURL: &url.URL{
				Scheme: "https",
				Host:   "edupass.example.com",
			},
			AuthURL: &url.URL{
				Scheme: "https",
				Host:   "edupass.example.com",
				Path:   "/oauth2/authorize",
			},
			TokenURL: &url.URL{
				Scheme: "https",
				Host:   "edupass.example.com",
				Path:   "/oauth2/token",
			},
			JWKSURL: &url.URL{
				Scheme: "https",
				Host:   "edupass.example.com",
				Path:   "/oauth2/jwks",
			},
			ClientID: "teacher-workspace",
			RedirectURL: &url.URL{
				Scheme: "https",
				Host:   "tw.example.com",
				Path:   "/auth/edupass/callback",
			},
			ClientAuthMethod:  config.EdupassClientAuthMethodClientSecretPost,
			ClientCredentials: config.EdupassClientCredentials{Secret: "teacher-workspace-secret"},
		},
	}
}

// mustParseURL returns rawURL parsed, failing t if it does not parse.
func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()

	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	return u
}

// logRecord is one record written by a [slog.JSONHandler].
type logRecord struct {
	Level    string `json:"level"`
	Msg      string `json:"msg"`
	Err      string `json:"err"`
	Provider string `json:"provider"`
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
