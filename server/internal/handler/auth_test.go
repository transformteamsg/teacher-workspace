package handler

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/String-sg/teacher-workspace/server/internal/oidc"
	"github.com/String-sg/teacher-workspace/server/internal/session"
)

// newTestOIDCHandler spins up a minimal OIDC test server and returns a
// Handler with a real RelyingParty pointed at it.
func newTestOIDCHandler(t *testing.T) (*Handler, *httptest.Server) {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"keys":[]}`)) //nolint:errcheck
	})

	rp := oidc.New(srv.URL, "test-client", srv.URL+"/callback", srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/jwks", testClientKey(t))

	cfg := config.Default()
	h, err := New(&cfg, rp)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h, srv
}

// callbackTestEnv holds the test OIDC server and handler for callback tests.
// Set the pointer fields before each request to control what the mock /token
// endpoint returns.
type callbackTestEnv struct {
	h            *Handler
	srv          *httptest.Server
	tokenNonce   *string
	tokenEmail   *string
	tokenErr     *string // when non-empty, mock returns {"error": <value>} with 400
	skipIDToken  *bool   // when true, mock omits id_token from the response
	tokenExpired *bool   // when true, mock sets exp to the past
}

func newCallbackTestEnv(t *testing.T) *callbackTestEnv {
	t.Helper()

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}

	var tokenNonce, tokenEmail, tokenErr string
	var skipIDToken, tokenExpired bool
	env := &callbackTestEnv{
		tokenNonce:   &tokenNonce,
		tokenEmail:   &tokenEmail,
		tokenErr:     &tokenErr,
		skipIDToken:  &skipIDToken,
		tokenExpired: &tokenExpired,
	}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	env.srv = srv
	t.Cleanup(srv.Close)

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		jwk := jose.JSONWebKey{
			Key:       &rsaKey.PublicKey,
			Algorithm: string(jose.RS256),
			Use:       "sig",
		}
		keySet := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(keySet) //nolint:errcheck
	})

	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if *env.tokenErr != "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": *env.tokenErr}) //nolint:errcheck
			return
		}

		expiry := time.Now().Add(time.Hour).Unix()
		if *env.tokenExpired {
			expiry = time.Now().Add(-time.Hour).Unix()
		}

		claims := map[string]any{
			"iss":   srv.URL,
			"aud":   []string{"test-client"},
			"sub":   "test-subject",
			"email": *env.tokenEmail,
			"nonce": *env.tokenNonce,
			"iat":   time.Now().Unix(),
			"exp":   expiry,
		}
		claimsJSON, err := json.Marshal(claims)
		if err != nil {
			http.Error(w, fmt.Sprintf("marshal claims: %v", err), http.StatusInternalServerError)
			return
		}

		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: rsaKey}, nil)
		if err != nil {
			http.Error(w, fmt.Sprintf("new signer: %v", err), http.StatusInternalServerError)
			return
		}

		jws, err := signer.Sign(claimsJSON)
		if err != nil {
			http.Error(w, fmt.Sprintf("sign: %v", err), http.StatusInternalServerError)
			return
		}

		rawIDToken, err := jws.CompactSerialize()
		if err != nil {
			http.Error(w, fmt.Sprintf("serialize: %v", err), http.StatusInternalServerError)
			return
		}

		resp := map[string]any{
			"access_token": "test-access-token",
			"token_type":   "Bearer",
		}
		if !*env.skipIDToken {
			resp["id_token"] = rawIDToken
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})

	rp := oidc.New(srv.URL, "test-client", srv.URL+"/callback", srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/jwks", testClientKey(t))

	cfg := config.Default()
	h, err := New(&cfg, rp)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	env.h = h
	return env
}

func newSessionWithOIDC(state, nonce, codeVerifier string) *session.Session {
	sess := session.New()
	sess.Set(sessionKeyOIDCState, state)
	sess.Set(sessionKeyOIDCNonce, nonce)
	sess.Set(sessionKeyOIDCCodeVerifier, codeVerifier)
	return sess
}

func newSessionWithOIDCAndReturnTo(state, nonce, codeVerifier, returnTo string) *session.Session {
	sess := newSessionWithOIDC(state, nonce, codeVerifier)
	// mirrors authEdupass: empty return_to is never stored
	if returnTo != "" {
		sess.Set(sessionKeyReturnTo, returnTo)
	}
	return sess
}

func TestHandler_authEdupass(t *testing.T) {
	t.Run("redirects to the provider authorization endpoint", func(t *testing.T) {
		h, srv := newTestOIDCHandler(t)

		sess := session.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		if want, got := http.StatusFound, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}

		loc := rec.Header().Get("Location")
		if loc == "" {
			t.Fatal("want: non-empty; got: empty")
		}

		u, err := url.Parse(loc)
		if err != nil {
			t.Fatalf("parse Location: %v", err)
		}

		if want, got := srv.Listener.Addr().String(), u.Host; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "/authorize", u.Path; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}

		q := u.Query()
		if want, got := "test-client", q.Get("client_id"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := srv.URL+"/callback", q.Get("redirect_uri"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "code", q.Get("response_type"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if got := q.Get("scope"); !strings.Contains(got, "openid") {
			t.Errorf("want: containing %q; got: %q", "openid", got)
		}
		if got := q.Get("state"); got == "" {
			t.Error("want: non-empty; got: empty")
		}
		if got := q.Get("nonce"); got == "" {
			t.Error("want: non-empty; got: empty")
		}
		if got := q.Get("code_challenge"); got == "" {
			t.Error("want: non-empty; got: empty")
		}
		if want, got := "S256", q.Get("code_challenge_method"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if q.Has("response_mode") {
			t.Errorf("want: response_mode absent; got: %q", q.Get("response_mode"))
		}
	})

	t.Run("stores OIDC values in the session", func(t *testing.T) {
		h, _ := newTestOIDCHandler(t)

		sess := session.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		loc := rec.Header().Get("Location")
		u, err := url.Parse(loc)
		if err != nil {
			t.Fatalf("parse Location: %v", err)
		}
		q := u.Query()

		stateInURL := q.Get("state")
		nonceInURL := q.Get("nonce")

		stateInSess, ok := sess.Get(sessionKeyOIDCState)
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if stateInSess != stateInURL {
			t.Errorf("want: %q; got: %q", stateInURL, stateInSess)
		}

		nonceInSess, ok := sess.Get(sessionKeyOIDCNonce)
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if nonceInSess != nonceInURL {
			t.Errorf("want: %q; got: %q", nonceInURL, nonceInSess)
		}

		verifier, ok := sess.Get(sessionKeyOIDCCodeVerifier)
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if v, _ := verifier.(string); v == "" {
			t.Error("want: non-empty; got: empty")
		}
	})

	t.Run("returns 500 when session is missing from context", func(t *testing.T) {
		h, _ := newTestOIDCHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/auth/edupass", nil)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
	})

	t.Run("generates different state, nonce, and challenge on each request", func(t *testing.T) {
		h, _ := newTestOIDCHandler(t)

		extract := func() (state, nonce, challenge string) {
			sess := session.New()
			req := httptest.NewRequest(http.MethodGet, "/auth/edupass", nil)
			req = req.WithContext(middleware.WithSession(req.Context(), sess))
			rec := httptest.NewRecorder()
			h.authEdupass(rec, req)

			u, err := url.Parse(rec.Header().Get("Location"))
			if err != nil {
				t.Fatalf("parse Location: %v", err)
			}
			q := u.Query()
			return q.Get("state"), q.Get("nonce"), q.Get("code_challenge")
		}

		s1, n1, c1 := extract()
		s2, n2, c2 := extract()

		if s1 == s2 {
			t.Errorf("want: != %q; got: %q", s1, s2)
		}
		if n1 == n2 {
			t.Errorf("want: != %q; got: %q", n1, n2)
		}
		if c1 == c2 {
			t.Errorf("want: != %q; got: %q", c1, c2)
		}
	})

	t.Run("stores return_to in the session when valid", func(t *testing.T) {
		h, _ := newTestOIDCHandler(t)

		sess := session.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass?return_to=%2Fposts%3Ftab%3Ddrafts", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		if want, got := http.StatusFound, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}

		val, ok := sess.Get(sessionKeyReturnTo)
		if !ok {
			t.Fatal("want ok: true; got: false")
		}
		if want, got := "/posts?tab=drafts", val.(string); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("does not store return_to when the value is refused", func(t *testing.T) {
		h, _ := newTestOIDCHandler(t)

		var logBuf bytes.Buffer
		testLogger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		sess := session.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass?return_to=https%3A%2F%2Fevil.example", nil)
		ctx := middleware.WithSession(req.Context(), sess)
		ctx = middleware.WithLogger(ctx, testLogger)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		if want, got := http.StatusFound, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}

		if _, ok := sess.Get(sessionKeyReturnTo); ok {
			t.Error("want ok: false; got: true")
		}

		var entry struct {
			Level string `json:"level"`
			Msg   string `json:"msg"`
			Raw   string `json:"raw"`
		}
		if err := json.NewDecoder(&logBuf).Decode(&entry); err != nil {
			t.Fatalf("want a log entry; got decode error: %v", err)
		}
		if want, got := "WARN", entry.Level; want != got {
			t.Errorf("log level: want %q; got %q", want, got)
		}
		if want, got := "refused return_to destination", entry.Msg; want != got {
			t.Errorf("log msg: want %q; got %q", want, got)
		}
		if want, got := "https://evil.example", entry.Raw; want != got {
			t.Errorf("log raw: want %q; got %q", want, got)
		}
	})

	t.Run("does not store return_to when the parameter is absent", func(t *testing.T) {
		h, _ := newTestOIDCHandler(t)

		sess := session.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		if want, got := http.StatusFound, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}

		if _, ok := sess.Get(sessionKeyReturnTo); ok {
			t.Error("want ok: false; got: true")
		}
	})

	t.Run("does not store return_to when the value targets /auth/", func(t *testing.T) {
		h, _ := newTestOIDCHandler(t)

		sess := session.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass?return_to=%2Fauth%2Fedupass", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		if want, got := http.StatusFound, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}

		if _, ok := sess.Get(sessionKeyReturnTo); ok {
			t.Error("want ok: false; got: true")
		}
	})

	t.Run("does not store return_to when the value targets /api/", func(t *testing.T) {
		h, _ := newTestOIDCHandler(t)

		sess := session.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass?return_to=%2Fapi%2Fposts", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		if want, got := http.StatusFound, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}

		if _, ok := sess.Get(sessionKeyReturnTo); ok {
			t.Error("want ok: false; got: true")
		}
	})

	t.Run("does not store return_to when the value is an empty string", func(t *testing.T) {
		h, _ := newTestOIDCHandler(t)

		sess := session.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass?return_to=", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		h.authEdupass(rec, req)

		if want, got := http.StatusFound, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}

		if _, ok := sess.Get(sessionKeyReturnTo); ok {
			t.Error("want ok: false; got: true")
		}
	})

}

func TestHandler_authEdupassCallback(t *testing.T) {
	t.Run("authenticates the session and redirects to /", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		nonce := "test-nonce"
		*env.tokenNonce = nonce
		*env.tokenEmail = "jane@example.com"

		sess := newSessionWithOIDC(state, nonce, "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusSeeOther, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		if want, got := "/", rec.Header().Get("Location"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if sess.User() == nil {
			t.Fatal("want: non-nil; got: nil")
		}
		if want, got := "jane@example.com", sess.User().Email; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("rejects state mismatch", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		sess := newSessionWithOIDC("known-state", "test-nonce", "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state=unknown-state", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		if sess.User() != nil {
			t.Error("want: nil; got: non-nil")
		}
	})

	t.Run("rejects missing code verifier", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		sess := newSessionWithOIDC(state, "test-nonce", "")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
	})

	t.Run("rejects when no OIDC values in session", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		sess := session.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state=some-state", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
	})

	t.Run("rejects provider error response", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		sess := newSessionWithOIDC("test-state", "test-nonce", "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?error=access_denied&error_description=user+denied", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		for _, key := range []string{sessionKeyOIDCState, sessionKeyOIDCNonce, sessionKeyOIDCCodeVerifier, sessionKeyReturnTo} {
			if _, ok := sess.Get(key); ok {
				t.Errorf("want %q ok: false; got: true", key)
			}
		}
	})

	t.Run("rejects missing code", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		sess := newSessionWithOIDC(state, "test-nonce", "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusBadRequest, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		for _, key := range []string{sessionKeyOIDCState, sessionKeyOIDCNonce, sessionKeyOIDCCodeVerifier, sessionKeyReturnTo} {
			if _, ok := sess.Get(key); ok {
				t.Errorf("want %q ok: false; got: true", key)
			}
		}
	})

	t.Run("rejects nonce mismatch", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		*env.tokenNonce = "token-nonce-B"
		*env.tokenEmail = "jane@example.com"

		sess := newSessionWithOIDC(state, "session-nonce-A", "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
	})

	t.Run("rejects missing nonce", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		*env.tokenNonce = "token-nonce"
		*env.tokenEmail = "jane@example.com"

		sess := newSessionWithOIDC(state, "", "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
	})

	t.Run("rejects missing email claim", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		nonce := "test-nonce"
		*env.tokenNonce = nonce
		*env.tokenEmail = ""

		sess := newSessionWithOIDC(state, nonce, "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
	})

	t.Run("returns 400 when state is missing from callback URL", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		sess := newSessionWithOIDC("test-state", "test-nonce", "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusBadRequest, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		for _, key := range []string{sessionKeyOIDCState, sessionKeyOIDCNonce, sessionKeyOIDCCodeVerifier, sessionKeyReturnTo} {
			if _, ok := sess.Get(key); ok {
				t.Errorf("want %q ok: false; got: true", key)
			}
		}
	})

	t.Run("returns 500 when session is missing from context", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state=test-state", nil)
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
	})

	t.Run("returns 403 when token exchange fails", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		*env.tokenErr = "invalid_grant"

		sess := newSessionWithOIDC(state, "test-nonce", "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
	})

	t.Run("returns 500 when token response is missing id_token", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		nonce := "test-nonce"
		*env.tokenNonce = nonce
		*env.tokenEmail = "jane@example.com"
		*env.skipIDToken = true

		sess := newSessionWithOIDC(state, nonce, "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusInternalServerError, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
	})

	t.Run("returns 403 when ID token is expired", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		nonce := "test-nonce"
		*env.tokenNonce = nonce
		*env.tokenEmail = "jane@example.com"
		*env.tokenExpired = true

		sess := newSessionWithOIDC(state, nonce, "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
	})

	t.Run("redirects to the return_to destination after authentication", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		nonce := "test-nonce"
		*env.tokenNonce = nonce
		*env.tokenEmail = "jane@example.com"

		sess := newSessionWithOIDCAndReturnTo(state, nonce, "test-verifier", "/posts?tab=drafts")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusSeeOther, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		if want, got := "/posts?tab=drafts", rec.Header().Get("Location"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if sess.User() == nil {
			t.Fatal("want: non-nil; got: nil")
		}
		if want, got := "jane@example.com", sess.User().Email; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("ignores return_to on the callback request URL", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		nonce := "test-nonce"
		*env.tokenNonce = nonce
		*env.tokenEmail = "jane@example.com"

		sess := newSessionWithOIDC(state, nonce, "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state="+state+"&return_to=/evil", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusSeeOther, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		if want, got := "/", rec.Header().Get("Location"); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("clears all OIDC session keys after successful authentication", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		nonce := "test-nonce"
		*env.tokenNonce = nonce
		*env.tokenEmail = "jane@example.com"

		sess := newSessionWithOIDCAndReturnTo(state, nonce, "test-verifier", "/posts")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusSeeOther, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}

		for _, key := range []string{sessionKeyOIDCState, sessionKeyOIDCNonce, sessionKeyOIDCCodeVerifier, sessionKeyReturnTo} {
			if _, ok := sess.Get(key); ok {
				t.Errorf("session key %q should be absent after callback; got present", key)
			}
		}
	})

	t.Run("clears OIDC session keys and does not set user on nonce mismatch", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		*env.tokenNonce = "token-nonce-B"
		*env.tokenEmail = "jane@example.com"

		sess := newSessionWithOIDCAndReturnTo(state, "session-nonce-A", "test-verifier", "/posts")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		if sess.User() != nil {
			t.Error("want: nil; got: non-nil")
		}
		for _, key := range []string{sessionKeyOIDCState, sessionKeyOIDCNonce, sessionKeyOIDCCodeVerifier, sessionKeyReturnTo} {
			if _, ok := sess.Get(key); ok {
				t.Errorf("session key %q should be absent after failed callback; got present", key)
			}
		}
	})

	t.Run("clears OIDC session keys and does not set user on missing email", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		nonce := "test-nonce"
		*env.tokenNonce = nonce
		*env.tokenEmail = ""

		sess := newSessionWithOIDCAndReturnTo(state, nonce, "test-verifier", "/posts")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authEdupassCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		if sess.User() != nil {
			t.Error("want: nil; got: non-nil")
		}
		for _, key := range []string{sessionKeyOIDCState, sessionKeyOIDCNonce, sessionKeyOIDCCodeVerifier, sessionKeyReturnTo} {
			if _, ok := sess.Get(key); ok {
				t.Errorf("session key %q should be absent after failed callback; got present", key)
			}
		}
	})
}
