package oidc

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	coreoidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"

	"github.com/String-sg/teacher-workspace/server/pkg/random"
)

const (
	clientAssertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
	clientAssertionTTL  = 5 * time.Minute
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

type ClientKey struct {
	Key  *rsa.PrivateKey
	Cert *x509.Certificate
}

// LoadClientKey returns the ClientKey.
func LoadClientKey(keyPEM, certPEM string) (ClientKey, error) {
	keyBytes, err := readPEM("TW_OIDC_CLIENT_PRIVATE_KEY", keyPEM)
	if err != nil {
		return ClientKey{}, err
	}
	keyBlock, _ := pem.Decode(keyBytes)
	if keyBlock == nil {
		return ClientKey{}, errors.New("TW_OIDC_CLIENT_PRIVATE_KEY: not a PEM block")
	}

	parsed, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return ClientKey{}, fmt.Errorf("TW_OIDC_CLIENT_PRIVATE_KEY: not a PKCS#8 key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return ClientKey{}, fmt.Errorf("TW_OIDC_CLIENT_PRIVATE_KEY: not an RSA key, got %T", parsed)
	}

	certBytes, err := readPEM("TW_OIDC_CLIENT_PUBLIC_KEY", certPEM)
	if err != nil {
		return ClientKey{}, err
	}
	certBlock, _ := pem.Decode(certBytes)
	if certBlock == nil {
		return ClientKey{}, errors.New("TW_OIDC_CLIENT_PUBLIC_KEY: not a PEM block")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return ClientKey{}, fmt.Errorf("TW_OIDC_CLIENT_PUBLIC_KEY: not an X.509 certificate: %w", err)
	}

	return ClientKey{Key: key, Cert: cert}, nil
}

// RelyingParty holds the OAuth2 config and ID-token verifier needed for the
// authorization code flow with PKCE.
type RelyingParty struct {
	OAuth2   oauth2.Config
	Verifier *coreoidc.IDTokenVerifier

	clientKey ClientKey
}

// New constructs a RelyingParty from explicit endpoint URLs. No network call
// is made; JWKS keys are fetched lazily on the first Verify() call.
func New(issuerURL, clientID, redirectURL, authURL, tokenURL, jwksURI string, clientKey ClientKey) *RelyingParty {
	keySet := coreoidc.NewRemoteKeySet(coreoidc.ClientContext(context.Background(), httpClient), jwksURI)
	verifier := coreoidc.NewVerifier(issuerURL, keySet, &coreoidc.Config{ClientID: clientID})

	cfg := oauth2.Config{
		ClientID:    clientID,
		RedirectURL: redirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:   authURL,
			TokenURL:  tokenURL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
		Scopes: []string{coreoidc.ScopeOpenID},
	}

	return &RelyingParty{
		OAuth2:    cfg,
		Verifier:  verifier,
		clientKey: clientKey,
	}
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
	IDToken     string `json:"id_token"`
}

type TokenError struct {
	ErrorCode        string `json:"error"`
	ErrorDescription string `json:"error_description"`
	ErrorURI         string `json:"error_uri"`
	TraceID          string `json:"trace_id"`
	CorrelationID    string `json:"correlation_id"`
}

// Exchange trades an authorisation code for ID token.
func (rp *RelyingParty) Exchange(ctx context.Context, code, codeVerifier string) (*TokenResponse, error) {
	assertion, err := rp.clientAssertion()
	if err != nil {
		return nil, fmt.Errorf("sign client assertion: %w", err)
	}

	form := url.Values{
		"grant_type":            {"authorization_code"},
		"code":                  {code},
		"redirect_uri":          {rp.OAuth2.RedirectURL},
		"code_verifier":         {codeVerifier},
		"client_id":             {rp.OAuth2.ClientID},
		"client_assertion_type": {clientAssertionType},
		"client_assertion":      {assertion},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rp.OAuth2.Endpoint.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var tokenErr TokenError
		if err := json.Unmarshal(body, &tokenErr); err != nil || tokenErr.ErrorCode == "" {
			return nil, fmt.Errorf("token endpoint responded %d: %q", resp.StatusCode, body)
		}
		return nil, fmt.Errorf("token endpoint responded %d: %s: %s (error_uri=%q trace_id=%q correlation_id=%q)",
			resp.StatusCode, tokenErr.ErrorCode, tokenErr.ErrorDescription,
			tokenErr.ErrorURI, tokenErr.TraceID, tokenErr.CorrelationID)
	}

	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("decode response body: %w", err)
	}
	return &token, nil
}

// readPEM returns the PEM a value holds directly, or the contents of the file it names.
func readPEM(envName, value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("%s is required", envName)
	}
	if strings.HasPrefix(value, "-----BEGIN") {
		return []byte(value), nil
	}

	pemBytes, err := os.ReadFile(value)
	if err != nil {
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			err = pathErr.Err
		}
		return nil, fmt.Errorf("%s: %w", envName, err)
	}
	return pemBytes, nil
}

// clientAssertion mints the JWT that authenticates this client at the token endpoint.
func (rp *RelyingParty) clientAssertion() (string, error) {
	now := time.Now()

	token := jwt.NewWithClaims(jwt.SigningMethodPS256, jwt.RegisteredClaims{
		Issuer:    rp.OAuth2.ClientID,
		Subject:   rp.OAuth2.ClientID,
		Audience:  jwt.ClaimStrings{rp.OAuth2.Endpoint.TokenURL},
		ID:        random.Base62(32),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(clientAssertionTTL)),
	})
	sum := sha256.Sum256(rp.clientKey.Cert.Raw)
	token.Header["x5t#S256"] = base64.RawURLEncoding.EncodeToString(sum[:])

	return token.SignedString(rp.clientKey.Key)
}
