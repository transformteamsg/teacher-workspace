package oidc_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"

	"github.com/String-sg/teacher-workspace/server/internal/oidc"
)

const clientAssertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"

// newClientKey returns a fresh RSA key and a self-signed certificate for it.
func newClientKey(t *testing.T) oidc.ClientKey {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "teacher-workspace"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %v", err)
	}

	return oidc.ClientKey{Key: key, Cert: cert}
}

type tokenEndpoint struct {
	srv *httptest.Server

	mu     sync.Mutex
	forms  []url.Values
	status int
	body   string
}

// newTokenEndpoint starts a stub token endpoint that records the form of every
// request and answers 200 with an id_token until respondWith says otherwise.
func newTokenEndpoint(t *testing.T) *tokenEndpoint {
	t.Helper()

	endpoint := &tokenEndpoint{
		status: http.StatusOK,
		body:   `{"id_token":"test-id-token","access_token":"test-access-token"}`,
	}
	endpoint.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		endpoint.mu.Lock()
		endpoint.forms = append(endpoint.forms, r.PostForm)
		status, body := endpoint.status, endpoint.body
		endpoint.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(endpoint.srv.Close)

	return endpoint
}

// respondWith sets the status and body of every later response.
func (e *tokenEndpoint) respondWith(status int, body string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status, e.body = status, body
}

// form returns the form of the i-th request received, failing when fewer were
// made.
func (e *tokenEndpoint) form(t *testing.T, i int) url.Values {
	t.Helper()

	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.forms) <= i {
		t.Fatalf("want: more than %d requests; got: %d", i, len(e.forms))
	}
	return e.forms[i]
}

// newRelyingParty returns a RelyingParty pointed at the stub token endpoint.
func newRelyingParty(endpoint *tokenEndpoint, clientKey oidc.ClientKey) *oidc.RelyingParty {
	return oidc.New(
		"https://issuer.example.com",
		"test-client-id",
		"http://localhost/callback",
		"https://issuer.example.com/authorize",
		endpoint.srv.URL,
		"https://issuer.example.com/jwks",
		clientKey,
	)
}

func TestLoadClientKey(t *testing.T) {
	clientKey := newClientKey(t)

	pkcs8, err := x509.MarshalPKCS8PrivateKey(clientKey.Key)
	if err != nil {
		t.Fatalf("x509.MarshalPKCS8PrivateKey: %v", err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientKey.Cert.Raw}))

	t.Run("returns the key and certificate the values hold", func(t *testing.T) {
		loaded, err := oidc.LoadClientKey(keyPEM, certPEM)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if got := clientKey.Key.Equal(loaded.Key); !got {
			t.Error("want: true; got: false")
		}
		if got := clientKey.Cert.Equal(loaded.Cert); !got {
			t.Error("want: true; got: false")
		}
	})

	t.Run("reads the key and certificate from the files the values name", func(t *testing.T) {
		dir := t.TempDir()
		keyPath := filepath.Join(dir, "client.key")
		certPath := filepath.Join(dir, "client.cer")
		if err := os.WriteFile(keyPath, []byte(keyPEM), 0o600); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		if err := os.WriteFile(certPath, []byte(certPEM), 0o600); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}

		loaded, err := oidc.LoadClientKey(keyPath, certPath)

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if got := clientKey.Key.Equal(loaded.Key); !got {
			t.Error("want: true; got: false")
		}
		if got := clientKey.Cert.Equal(loaded.Cert); !got {
			t.Error("want: true; got: false")
		}
	})

	t.Run("accepts PEM material surrounded by whitespace", func(t *testing.T) {
		loaded, err := oidc.LoadClientKey("\n  "+keyPEM+"\n", "\n"+certPEM+"  \n")

		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		if got := clientKey.Key.Equal(loaded.Key); !got {
			t.Error("want: true; got: false")
		}
	})

	t.Run("keeps key material out of the unreadable path error", func(t *testing.T) {
		_, err := oidc.LoadClientKey("-brgin key\n"+keyPEM, certPEM)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if strings.Contains(err.Error(), "PRIVATE KEY") {
			t.Errorf("want err: without key material; got: %q", err)
		}
	})

	t.Run("rejects unusable key material", func(t *testing.T) {
		ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("ecdsa.GenerateKey: %v", err)
		}
		ecDER, err := x509.MarshalPKCS8PrivateKey(ecKey)
		if err != nil {
			t.Fatalf("x509.MarshalPKCS8PrivateKey: %v", err)
		}
		ecPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecDER}))

		for _, tt := range []struct {
			name string
			key  string
			cert string
			want string
		}{
			{
				name: "empty private key",
				key:  "",
				cert: certPEM,
				want: "TW_OIDC_CLIENT_PRIVATE_KEY is required",
			},
			{
				name: "unreadable private key path",
				key:  "does-not-exist.key",
				cert: certPEM,
				want: "TW_OIDC_CLIENT_PRIVATE_KEY: no such file",
			},
			{
				name: "private key that is not PEM",
				key:  "-----BEGIN PRIVATE KEY-----\nnot base64\n-----END PRIVATE KEY-----\n",
				cert: certPEM,
				want: "TW_OIDC_CLIENT_PRIVATE_KEY: not a PEM block",
			},
			{
				name: "private key holding a certificate",
				key:  certPEM,
				cert: certPEM,
				want: "TW_OIDC_CLIENT_PRIVATE_KEY: not a PKCS#8 key",
			},
			{
				name: "private key that is not an RSA key",
				key:  ecPEM,
				cert: certPEM,
				want: "TW_OIDC_CLIENT_PRIVATE_KEY: not an RSA key",
			},
			{
				name: "empty certificate",
				key:  keyPEM,
				cert: "",
				want: "TW_OIDC_CLIENT_PUBLIC_KEY is required",
			},
			{
				name: "certificate that is not PEM",
				key:  keyPEM,
				cert: "-----BEGIN CERTIFICATE-----\nnot base64\n-----END CERTIFICATE-----\n",
				want: "TW_OIDC_CLIENT_PUBLIC_KEY: not a PEM block",
			},
			{
				name: "certificate holding a private key",
				key:  keyPEM,
				cert: keyPEM,
				want: "TW_OIDC_CLIENT_PUBLIC_KEY: not an X.509 certificate",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				_, err := oidc.LoadClientKey(tt.key, tt.cert)

				if err == nil {
					t.Fatal("want err: non-nil; got: nil")
				}
				if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("want err: containing %q; got: %q", tt.want, err)
				}
			})
		}
	})
}

func TestNew(t *testing.T) {
	t.Run("returns a configured RelyingParty", func(t *testing.T) {
		rp := oidc.New(
			"https://issuer.example.com",
			"test-client-id",
			"http://localhost/callback",
			"https://issuer.example.com/authorize",
			"https://issuer.example.com/token",
			"https://issuer.example.com/jwks",
			newClientKey(t),
		)

		if want, got := "test-client-id", rp.OAuth2.ClientID; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if got := rp.OAuth2.ClientSecret; got != "" {
			t.Errorf("want: empty; got: %q", got)
		}
		if want, got := "http://localhost/callback", rp.OAuth2.RedirectURL; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := oauth2.AuthStyleInParams, rp.OAuth2.Endpoint.AuthStyle; want != got {
			t.Errorf("want: %v; got: %v", want, got)
		}
		if want, got := "https://issuer.example.com/authorize", rp.OAuth2.Endpoint.AuthURL; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "https://issuer.example.com/token", rp.OAuth2.Endpoint.TokenURL; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("returns error when JWKS endpoint is slow", func(t *testing.T) {
		oidc.SetHTTPTimeout(t, 50*time.Millisecond)
		release := make(chan struct{})
		slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-release
		}))
		t.Cleanup(slow.Close)
		t.Cleanup(func() { close(release) })
		clientKey := newClientKey(t)
		rp := oidc.New(
			"https://issuer.example.com",
			"test-client-id",
			"http://localhost/callback",
			"https://issuer.example.com/authorize",
			"https://issuer.example.com/token",
			slow.URL,
			clientKey,
		)
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
			Issuer:    "https://issuer.example.com",
			Audience:  jwt.ClaimStrings{"test-client-id"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		})
		token.Header["kid"] = "unknown-kid"
		idToken, err := token.SignedString(clientKey.Key)
		if err != nil {
			t.Fatalf("jwt.Token.SignedString: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err = rp.Verifier.Verify(ctx, idToken)

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want, got := "Client.Timeout exceeded", err.Error(); !strings.Contains(got, want) {
			t.Errorf("want err: containing %q; got: %q", want, got)
		}
	})
}

func TestRelyingParty_Exchange(t *testing.T) {
	clientKey := newClientKey(t)

	t.Run("posts the code, the verifier, and a client assertion", func(t *testing.T) {
		endpoint := newTokenEndpoint(t)
		rp := newRelyingParty(endpoint, clientKey)

		if _, err := rp.Exchange(context.Background(), "test-code", "test-verifier"); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		form := endpoint.form(t, 0)
		for _, tt := range []struct {
			key  string
			want string
		}{
			{key: "grant_type", want: "authorization_code"},
			{key: "code", want: "test-code"},
			{key: "redirect_uri", want: "http://localhost/callback"},
			{key: "code_verifier", want: "test-verifier"},
			{key: "client_id", want: "test-client-id"},
			{key: "client_assertion_type", want: clientAssertionType},
		} {
			t.Run(tt.key, func(t *testing.T) {
				if got := form.Get(tt.key); tt.want != got {
					t.Errorf("want: %q; got: %q", tt.want, got)
				}
			})
		}

		if got := form.Get("client_assertion"); got == "" {
			t.Error("want: non-empty; got: empty")
		}
		if got := form.Has("client_secret"); got {
			t.Error("want: false; got: true")
		}
	})

	t.Run("signs the assertion with the client key", func(t *testing.T) {
		endpoint := newTokenEndpoint(t)
		rp := newRelyingParty(endpoint, clientKey)

		before := time.Now()
		if _, err := rp.Exchange(context.Background(), "test-code", "test-verifier"); err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		var claims jwt.RegisteredClaims
		assertion, err := jwt.ParseWithClaims(
			endpoint.form(t, 0).Get("client_assertion"),
			&claims,
			func(*jwt.Token) (any, error) { return &clientKey.Key.PublicKey, nil },
			jwt.WithValidMethods([]string{"PS256"}),
		)
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}

		sum := sha256.Sum256(clientKey.Cert.Raw)
		wantThumbprint := base64.RawURLEncoding.EncodeToString(sum[:])

		for _, tt := range []struct {
			key  string
			want string
		}{
			{key: "alg", want: "PS256"},
			{key: "typ", want: "JWT"},
			{key: "x5t#S256", want: wantThumbprint},
		} {
			t.Run(tt.key, func(t *testing.T) {
				got, ok := assertion.Header[tt.key].(string)
				if !ok {
					t.Fatalf("want %q ok: true; got: false", tt.key)
				}
				if tt.want != got {
					t.Errorf("want: %q; got: %q", tt.want, got)
				}
			})
		}

		if want, got := "test-client-id", claims.Issuer; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "test-client-id", claims.Subject; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := endpoint.srv.URL, claims.Audience; len(got) != 1 || want != got[0] {
			t.Errorf("want: [%q]; got: %q", want, got)
		}
		if got := claims.ID; got == "" {
			t.Error("want: non-empty; got: empty")
		}
		if claims.IssuedAt == nil || claims.NotBefore == nil || claims.ExpiresAt == nil {
			t.Fatalf("want: non-nil; got: iat %v, nbf %v, exp %v", claims.IssuedAt, claims.NotBefore, claims.ExpiresAt)
		}
		if want, got := before.Add(5*time.Minute), claims.ExpiresAt.Time; got.After(want) {
			t.Errorf("want: no later than %v; got: %v", want, got)
		}

		parts := strings.Split(endpoint.form(t, 0).Get("client_assertion"), ".")
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		sig, err := base64.RawURLEncoding.DecodeString(parts[2])
		if err != nil {
			t.Fatalf("base64.RawURLEncoding.DecodeString: %v", err)
		}

		opts := &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256}
		err = rsa.VerifyPSS(&clientKey.Key.PublicKey, crypto.SHA256, digest[:], sig, opts)
		if err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("mints a different assertion id per request", func(t *testing.T) {
		endpoint := newTokenEndpoint(t)
		rp := newRelyingParty(endpoint, clientKey)

		for range 2 {
			if _, err := rp.Exchange(context.Background(), "test-code", "test-verifier"); err != nil {
				t.Fatalf("want err: nil; got: %v", err)
			}
		}

		ids := make([]string, 0, 2)
		for i := range 2 {
			var claims jwt.RegisteredClaims
			if _, _, err := jwt.NewParser().ParseUnverified(endpoint.form(t, i).Get("client_assertion"), &claims); err != nil {
				t.Fatalf("jwt.Parser.ParseUnverified: %v", err)
			}
			ids = append(ids, claims.ID)
		}

		if want, got := ids[0], ids[1]; want == got {
			t.Errorf("want: != %q; got: %q", want, got)
		}
	})

	t.Run("returns the token response", func(t *testing.T) {
		endpoint := newTokenEndpoint(t)
		endpoint.respondWith(http.StatusOK, `{
			"access_token": "test-access-token",
			"token_type": "Bearer",
			"expires_in": 3599,
			"scope": "openid",
			"id_token": "test-id-token"
		}`)
		rp := newRelyingParty(endpoint, clientKey)

		resp, err := rp.Exchange(context.Background(), "test-code", "test-verifier")
		if err != nil {
			t.Fatalf("want err: nil; got: %v", err)
		}
		for _, tt := range []struct{ name, want, got string }{
			{"AccessToken", "test-access-token", resp.AccessToken},
			{"TokenType", "Bearer", resp.TokenType},
			{"Scope", "openid", resp.Scope},
			{"IDToken", "test-id-token", resp.IDToken},
		} {
			if tt.want != tt.got {
				t.Errorf("%s: want: %q; got: %q", tt.name, tt.want, tt.got)
			}
		}
		if want, got := 3599, resp.ExpiresIn; want != got {
			t.Errorf("ExpiresIn: want: %d; got: %d", want, got)
		}
	})

	t.Run("names every field the error response holds", func(t *testing.T) {
		endpoint := newTokenEndpoint(t)
		endpoint.respondWith(http.StatusBadRequest, `{
			"error": "invalid_scope",
			"error_description": "AADSTS70011: The provided value is not valid.",
			"error_uri": "https://example.com/error",
			"error_codes": [70011],
			"timestamp": "2016-01-09 02:02:12Z",
			"trace_id": "0000aaaa-11bb-cccc-dd22-eeeeee333333",
			"correlation_id": "aaaa0000-bb11-2222-33cc-444444dddddd"
		}`)
		rp := newRelyingParty(endpoint, clientKey)

		resp, err := rp.Exchange(context.Background(), "test-code", "test-verifier")

		if resp != nil {
			t.Errorf("want: nil; got: %+v", resp)
		}
		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		want := `token endpoint responded 400: invalid_scope: AADSTS70011: The provided value is not valid. ` +
			`(error_uri="https://example.com/error" trace_id="0000aaaa-11bb-cccc-dd22-eeeeee333333" ` +
			`correlation_id="aaaa0000-bb11-2222-33cc-444444dddddd")`
		if got := err.Error(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
	})

	t.Run("keeps the body when it carries no error code", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			body string
		}{
			{name: "not JSON", body: "<html><body>502 Bad Gateway</body></html>"},
			{name: "JSON without an error field", body: `{"message":"gateway timeout"}`},
		} {
			t.Run(tt.name, func(t *testing.T) {
				endpoint := newTokenEndpoint(t)
				endpoint.respondWith(http.StatusBadGateway, tt.body)
				rp := newRelyingParty(endpoint, clientKey)

				_, err := rp.Exchange(context.Background(), "test-code", "test-verifier")

				if err == nil {
					t.Fatal("want err: non-nil; got: nil")
				}
				want := fmt.Sprintf("token endpoint responded 502: %q", tt.body)
				if got := err.Error(); want != got {
					t.Errorf("want: %q; got: %q", want, got)
				}
			})
		}
	})

	t.Run("returns error when token endpoint is slow", func(t *testing.T) {
		oidc.SetHTTPTimeout(t, 50*time.Millisecond)
		release := make(chan struct{})
		slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-release
		}))
		t.Cleanup(slow.Close)
		t.Cleanup(func() { close(release) })
		rp := oidc.New(
			"https://issuer.example.com",
			"test-client-id",
			"http://localhost/callback",
			"https://issuer.example.com/authorize",
			slow.URL,
			"https://issuer.example.com/jwks",
			clientKey,
		)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := rp.Exchange(ctx, "test-code", "test-verifier")

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want, got := "Client.Timeout exceeded", err.Error(); !strings.Contains(got, want) {
			t.Errorf("want err: containing %q; got: %q", want, got)
		}
	})
}
