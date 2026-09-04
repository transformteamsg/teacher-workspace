package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestHandler_authLogin(t *testing.T) {
	t.Run("redirects to the provider authorization endpoint", func(t *testing.T) {
		h, srv := newTestOIDCHandler(t)
		defer srv.Close()

		sess := session.New()
		req := httptest.NewRequest(http.MethodGet, "/auth/edupass", nil)
		req = req.WithContext(middleware.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()

		h.authLogin(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
		}

		loc := rec.Header().Get("Location")
		if loc == "" {
			t.Fatal("Location header is empty")
		}

		u, err := (&http.Request{}).URL.Parse(loc)
		if err != nil {
			t.Fatalf("parse Location: %v", err)
		}

		if u.Host != srv.Listener.Addr().String() {
			t.Errorf("Location host = %q, want %q", u.Host, srv.Listener.Addr().String())
		}
		if u.Path != "/authorize" {
			t.Errorf("Location path = %q, want /authorize", u.Path)
		}

		q := u.Query()
		if q.Get("client_id") != "test-client" {
			t.Errorf("client_id = %q, want test-client", q.Get("client_id"))
		}
		if q.Get("redirect_uri") != srv.URL+"/callback" {
			t.Errorf("redirect_uri = %q, want %s/callback", q.Get("redirect_uri"), srv.URL)
		}
		if q.Get("response_type") != "code" {
			t.Errorf("response_type = %q, want code", q.Get("response_type"))
		}
		if !strings.Contains(q.Get("scope"), "openid") {
			t.Errorf("scope = %q, want it to contain openid", q.Get("scope"))
		}
		if q.Get("state") == "" {
			t.Error("state is empty")
		}
		if q.Get("nonce") == "" {
			t.Error("nonce is empty")
		}
		if q.Get("code_challenge") == "" {
			t.Error("code_challenge is empty")
		}
		if q.Get("code_challenge_method") != "S256" {
			t.Errorf("code_challenge_method = %q, want S256", q.Get("code_challenge_method"))
		}
		if q.Has("response_mode") {
			t.Errorf("response_mode should not be set, got %q", q.Get("response_mode"))
		}
	})

	t.Run("stores OIDC values in the session", func(t *testing.T) {
		h, srv := newTestOIDCHandler(t)
		defer srv.Close()

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
			t.Fatal("session missing oidc_state")
		}
		if stateInSess != stateInURL {
			t.Errorf("session state = %q, want %q", stateInSess, stateInURL)
		}

		nonceInSess, ok := sess.Get(sessionKeyOIDCNonce)
		if !ok {
			t.Fatal("session missing oidc_nonce")
		}
		if nonceInSess != nonceInURL {
			t.Errorf("session nonce = %q, want %q", nonceInSess, nonceInURL)
		}

		verifier, ok := sess.Get(sessionKeyOIDCCodeVerifier)
		if !ok {
			t.Fatal("session missing oidc_code_verifier")
		}
		if v, _ := verifier.(string); v == "" {
			t.Error("session code verifier is empty")
		}
	})

	t.Run("generates different state, nonce, and challenge on each request", func(t *testing.T) {
		h, srv := newTestOIDCHandler(t)
		defer srv.Close()

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
			t.Error("state is identical across requests, want unique")
		}
		if n1 == n2 {
			t.Error("nonce is identical across requests, want unique")
		}
		if c1 == c2 {
			t.Error("code_challenge is identical across requests, want unique")
		}
	})
}
