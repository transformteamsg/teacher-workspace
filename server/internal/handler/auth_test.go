package handler

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/String-sg/teacher-workspace/server/internal/oidc"
	"github.com/String-sg/teacher-workspace/server/internal/session"
)

// newTestOIDCHandler spins up a minimal OIDC discovery server and returns a
// Handler with a real RelyingParty pointed at it.
func newTestOIDCHandler(t *testing.T) (*Handler, *httptest.Server) {
	t.Helper()

	mux := http.NewServeMux()
	var srv *httptest.Server
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		base := srv.URL
		doc := map[string]any{
			"issuer":                                base,
			"authorization_endpoint":                base + "/authorize",
			"token_endpoint":                        base + "/token",
			"jwks_uri":                              base + "/jwks",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(doc) //nolint:errcheck
	})

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"keys":[]}`)) //nolint:errcheck
	})

	rp, err := oidc.New(t.Context(), srv.URL, "test-client", "test-secret", srv.URL+"/callback")
	if err != nil {
		t.Fatalf("oidc.New: %v", err)
	}

	cfg := config.Default()
	h := New(&cfg, rp)
	return h, srv
}

// callbackTestEnv holds the test OIDC server and handler for callback tests.
// Set *tokenNonce and *tokenEmail before each request to control what the mock
// /token endpoint returns in the signed ID token.
type callbackTestEnv struct {
	h          *Handler
	srv        *httptest.Server
	tokenNonce *string
	tokenEmail *string
}

func newCallbackTestEnv(t *testing.T) *callbackTestEnv {
	t.Helper()

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}

	var tokenNonce, tokenEmail string
	env := &callbackTestEnv{
		tokenNonce: &tokenNonce,
		tokenEmail: &tokenEmail,
	}

	mux := http.NewServeMux()
	var srv *httptest.Server
	srv = httptest.NewServer(mux)
	env.srv = srv
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		base := srv.URL
		doc := map[string]any{
			"issuer":                                base,
			"authorization_endpoint":                base + "/authorize",
			"token_endpoint":                        base + "/token",
			"jwks_uri":                              base + "/jwks",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(doc) //nolint:errcheck
	})

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
		claims := map[string]any{
			"iss":   srv.URL,
			"aud":   []string{"test-client"},
			"sub":   "test-subject",
			"email": tokenEmail,
			"nonce": tokenNonce,
			"iat":   time.Now().Unix(),
			"exp":   time.Now().Add(time.Hour).Unix(),
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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"access_token": "test-access-token",
			"token_type":   "Bearer",
			"id_token":     rawIDToken,
		})
	})

	rp, err := oidc.New(t.Context(), srv.URL, "test-client", "test-secret", srv.URL+"/callback")
	if err != nil {
		t.Fatalf("oidc.New: %v", err)
	}

	cfg := config.Default()
	env.h = New(&cfg, rp)
	return env
}

func newSessionWithOIDC(state, nonce, codeVerifier string) *session.Session {
	sess := session.New()
	sess.Set(sessionKeyOIDCState, state)
	sess.Set(sessionKeyOIDCNonce, nonce)
	sess.Set(sessionKeyOIDCCodeVerifier, codeVerifier)
	return sess
}

func TestHandler_authLogin(t *testing.T) {
	t.Run("redirects to the provider authorization endpoint", func(t *testing.T) {
		h, srv := newTestOIDCHandler(t)

		sess := session.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		h.authLogin(rec, req)

		if want, got := http.StatusFound, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}

		loc := rec.Header().Get("Location")
		if loc == "" {
			t.Fatal("want: non-empty; got: empty")
		}

		u, err := (&http.Request{}).URL.Parse(loc)
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

		h.authLogin(rec, req)

		loc := rec.Header().Get("Location")
		u, err := (&http.Request{}).URL.Parse(loc)
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

	t.Run("generates different state, nonce, and challenge on each request", func(t *testing.T) {
		h, _ := newTestOIDCHandler(t)

		extract := func() (state, nonce, challenge string) {
			sess := session.New()
			req := httptest.NewRequest(http.MethodGet, "/auth/edupass", nil)
			req = req.WithContext(middleware.WithSession(req.Context(), sess))
			rec := httptest.NewRecorder()
			h.authLogin(rec, req)

			u, err := (&http.Request{}).URL.Parse(rec.Header().Get("Location"))
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
}

func TestHandler_authCallback(t *testing.T) {
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

		env.h.authCallback(rec, req)

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

	t.Run("rejects unknown state", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		sess := newSessionWithOIDC("known-state", "test-nonce", "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state=unknown-state", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
		if sess.User() != nil {
			t.Error("want: nil; got: non-nil")
		}
	})

	t.Run("rejects when no OIDC values in session", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		sess := session.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?code=test-code&state=some-state", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
	})

	t.Run("rejects missing code", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		state := "test-state"
		sess := newSessionWithOIDC(state, "test-nonce", "test-verifier")
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?state="+state, nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authCallback(rec, req)

		if want, got := http.StatusBadRequest, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
	})

	t.Run("rejects provider error response", func(t *testing.T) {
		env := newCallbackTestEnv(t)

		sess := session.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass/callback?error=access_denied&error_description=user+denied", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		env.h.authCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
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

		env.h.authCallback(rec, req)

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

		env.h.authCallback(rec, req)

		if want, got := http.StatusForbidden, rec.Code; want != got {
			t.Fatalf("want: %d; got: %d", want, got)
		}
	})
}
