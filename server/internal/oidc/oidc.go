package oidc

import (
	"context"
	"fmt"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// RelyingParty holds the OIDC provider and OAuth2 config needed for the
// authorization code flow with PKCE.
type RelyingParty struct {
	Provider *coreoidc.Provider
	OAuth2   oauth2.Config
	Verifier *coreoidc.IDTokenVerifier
}

// New discovers the OIDC provider at issuerURL and returns a configured
// RelyingParty. It contacts the issuer's .well-known/openid-configuration
// endpoint, so the provider must be reachable when this is called.
func New(ctx context.Context, issuerURL, clientID, clientSecret, redirectURL string) (*RelyingParty, error) {
	provider, err := coreoidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery failed for %s: %w", issuerURL, err)
	}

	endpoint := provider.Endpoint()
	// mock-edupass uses client_secret_post; set AuthStyleInParams so x/oauth2
	// sends credentials in the POST body rather than an Authorization header.
	endpoint.AuthStyle = oauth2.AuthStyleInParams

	cfg := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     endpoint,
		Scopes:       []string{coreoidc.ScopeOpenID},
	}

	verifier := provider.Verifier(&coreoidc.Config{ClientID: clientID})

	return &RelyingParty{
		Provider: provider,
		OAuth2:   cfg,
		Verifier: verifier,
	}, nil
}
