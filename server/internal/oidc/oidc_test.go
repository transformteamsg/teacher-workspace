package oidc_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"

	"github.com/String-sg/teacher-workspace/server/internal/oidc"
)

func newTestOIDCServer(t *testing.T) *httptest.Server {
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

	return srv
}

func TestNew(t *testing.T) {
	t.Run("discovers the provider and returns a configured RelyingParty", func(t *testing.T) {
		srv := newTestOIDCServer(t)
		defer srv.Close()

		rp, err := oidc.New(t.Context(), srv.URL, "test-client-id", "test-client-secret", "http://localhost/callback")
		if err != nil {
			t.Fatalf("New() returned unexpected error: %v", err)
		}

		if rp.OAuth2.ClientID != "test-client-id" {
			t.Errorf("OAuth2.ClientID = %q, want %q", rp.OAuth2.ClientID, "test-client-id")
		}
		if rp.OAuth2.RedirectURL != "http://localhost/callback" {
			t.Errorf("OAuth2.RedirectURL = %q, want %q", rp.OAuth2.RedirectURL, "http://localhost/callback")
		}
		if rp.OAuth2.Endpoint.AuthStyle != oauth2.AuthStyleInParams {
			t.Errorf("OAuth2.Endpoint.AuthStyle = %v, want AuthStyleInParams", rp.OAuth2.Endpoint.AuthStyle)
		}
		if rp.Verifier == nil {
			t.Error("Verifier is nil, want non-nil")
		}
	})

	t.Run("returns an error when the issuer is unreachable", func(t *testing.T) {
		srv := newTestOIDCServer(t)
		srv.Close()

		_, err := oidc.New(t.Context(), srv.URL, "test-client-id", "test-client-secret", "http://localhost/callback")
		if err == nil {
			t.Fatal("New() returned nil error, want non-nil")
		}
	})
}
