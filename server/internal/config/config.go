package config

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"
)

type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvProduction  Environment = "production"
)

// Config is the main configuration for the application.
type Config struct {
	Env      Environment `dotenv:"TW_ENV"`
	LogLevel slog.Level  `dotenv:"TW_LOG_LEVEL"`

	// DevServerURL is used in development to proxy requests to the frontend development server.
	DevServerURL *url.URL `dotenv:"TW_DEV_SERVER_URL"`
	// BuildDir is used in production to serve the frontend build output.
	BuildDir string `dotenv:"TW_BUILD_DIR"`

	Server   ServerConfig   `dotenv:",squash"`
	Session  SessionConfig  `dotenv:",squash"`
	APIProxy APIProxyConfig `dotenv:",squash"`
	OIDC     OIDCConfig     `dotenv:",squash"`
	Remote   RemoteConfig   `dotenv:",squash"`
}

// RemoteConfig holds the Module Federation remotes the host registers at
// runtime. An empty URL means that remote is not registered.
type RemoteConfig struct {
	// PostsManifestURL is the mf-manifest.json URL of the Posts and Groups remote.
	PostsManifestURL string `dotenv:"TW_REMOTE_POSTS_MURL"`
	// StudentInsightsManifestURL is the mf-manifest.json URL of the Student Insights remote.
	StudentInsightsManifestURL string `dotenv:"TW_REMOTE_STUDENT_INSIGHTS_MURL"`
}

// ServerConfig represents the configuration for the HTTP server.
type ServerConfig struct {
	Port              int           `dotenv:"TW_SERVER_PORT"`
	ReadHeaderTimeout time.Duration `dotenv:"TW_SERVER_READ_HEADER_TIMEOUT"`
	ReadTimeout       time.Duration `dotenv:"TW_SERVER_READ_TIMEOUT"`
	WriteTimeout      time.Duration `dotenv:"TW_SERVER_WRITE_TIMEOUT"`
	IdleTimeout       time.Duration `dotenv:"TW_SERVER_IDLE_TIMEOUT"`
}

type SessionStoreProvider string

const (
	SessionStoreProviderMemory SessionStoreProvider = "memory"
	SessionStoreProviderValkey SessionStoreProvider = "valkey"
)

// SessionConfig represents the configuration for the session.
type SessionConfig struct {
	Name             string               `dotenv:"TW_SESSION_NAME"`
	DefaultTTL       time.Duration        `dotenv:"TW_SESSION_DEFAULT_TTL"`
	AuthenticatedTTL time.Duration        `dotenv:"TW_SESSION_AUTHENTICATED_TTL"`
	StoreProvider    SessionStoreProvider `dotenv:"TW_SESSION_STORE_PROVIDER"`

	Valkey SessionValkeyConfig `dotenv:",squash"`
	Memory SessionMemoryConfig `dotenv:",squash"`
}
type SessionValkeyConfig struct {
	URL    *url.URL `dotenv:"TW_SESSION_VALKEY_URL"`
	Prefix string   `dotenv:"TW_SESSION_VALKEY_PREFIX"`
}

// SessionMemoryConfig bounds the in-memory session store, which drops expired
// sessions first and never evicts a signed-in one to make room.
type SessionMemoryConfig struct {
	// MaxEntries is how many sessions the store holds.
	MaxEntries int `dotenv:"TW_SESSION_MEMORY_MAX_ENTRIES"`
	// MaxBytes is the total size of the sessions the store holds.
	MaxBytes int `dotenv:"TW_SESSION_MEMORY_MAX_BYTES"`
}

// OIDCConfig represents the configuration for the Edupass OIDC relying party.
type OIDCConfig struct {
	IssuerURL    *url.URL `dotenv:"TW_OIDC_ISSUER_URL"`
	AuthURL      *url.URL `dotenv:"TW_OIDC_AUTH_URL"`
	TokenURL     *url.URL `dotenv:"TW_OIDC_TOKEN_URL"`
	JWKSURI      *url.URL `dotenv:"TW_OIDC_JWKS_URI"`
	ClientID     string   `dotenv:"TW_OIDC_CLIENT_ID"`
	ClientSecret string   `dotenv:"TW_OIDC_CLIENT_SECRET"`
	RedirectURL  *url.URL `dotenv:"TW_OIDC_REDIRECT_URL"`
}

// APIProxyConfig represents the configuration for the backend proxies.
type APIProxyConfig struct {
	StudentInsightsBaseURL    *url.URL      `dotenv:"TW_API_PROXY_STUDENT_INSIGHTS_BASE_URL"`
	StudentInsightsSigningKey string        `dotenv:"TW_API_PROXY_STUDENT_INSIGHTS_SIGNING_KEY"`
	PostsBaseURL              *url.URL      `dotenv:"TW_API_PROXY_POSTS_BASE_URL"`
	PostsSigningKey           string        `dotenv:"TW_API_PROXY_POSTS_SIGNING_KEY"`
	TokenTTL                  time.Duration `dotenv:"TW_API_PROXY_TOKEN_TTL"`
}

// Default returns the default configuration for the application. It only sets
// fields with a value that is safe in every environment.
func Default() Config {
	return Config{
		Env:      EnvDevelopment,
		LogLevel: slog.LevelInfo,

		DevServerURL: must(url.Parse("http://127.0.0.1:3001")),
		BuildDir:     "apps/host/dist",

		Server: ServerConfig{
			Port:              3000,
			ReadHeaderTimeout: 2 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		Session: SessionConfig{
			Name:             "tw_session",
			DefaultTTL:       3 * time.Hour,
			AuthenticatedTTL: 30 * time.Minute,
			StoreProvider:    SessionStoreProviderMemory,
			Valkey: SessionValkeyConfig{
				Prefix: "session:",
			},
			// ~64 MiB at either the typical or the worst-case session size.
			Memory: SessionMemoryConfig{
				MaxEntries: 50_000,
				MaxBytes:   64 << 20,
			},
		},
		APIProxy: APIProxyConfig{
			TokenTTL: 1 * time.Minute,
		},
	}
}

// Validate validates the configuration.
func (c Config) Validate() error {
	var errs []error

	if c.Env != EnvDevelopment && c.Env != EnvProduction {
		errs = append(errs, fmt.Errorf("TW_ENV must be %q or %q; got %q", EnvDevelopment, EnvProduction, c.Env))
	}

	switch c.Env {
	case EnvDevelopment:
		if c.DevServerURL == nil {
			errs = append(errs, errors.New("TW_DEV_SERVER_URL is required"))
		} else {
			if c.DevServerURL.Scheme != "http" && c.DevServerURL.Scheme != "https" {
				errs = append(errs, fmt.Errorf("TW_DEV_SERVER_URL must use scheme http or https; got %q", c.DevServerURL))
			}
			if c.DevServerURL.Host == "" {
				errs = append(errs, fmt.Errorf("TW_DEV_SERVER_URL must include host[:port]; got %q", c.DevServerURL))
			}
		}
	case EnvProduction:
		if c.BuildDir == "" {
			errs = append(errs, errors.New("TW_BUILD_DIR is required"))
		} else if _, err := os.Stat(c.BuildDir); os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("TW_BUILD_DIR does not exist: %q", c.BuildDir))
		}
	}

	return errors.Join(append(errs, c.Server.validate(), c.Session.validate(), c.APIProxy.validate(), c.Remote.validate(), c.OIDC.validate())...)
}

func (c ServerConfig) validate() error {
	var errs []error

	if c.Port < 1 || c.Port > 65535 {
		errs = append(errs, fmt.Errorf("TW_SERVER_PORT must be 1-65535; got %d", c.Port))
	}
	if c.ReadHeaderTimeout < 0 {
		errs = append(errs, fmt.Errorf("TW_SERVER_READ_HEADER_TIMEOUT must be >= 0; got %v", c.ReadHeaderTimeout))
	}
	if c.ReadTimeout < 0 {
		errs = append(errs, fmt.Errorf("TW_SERVER_READ_TIMEOUT must be >= 0; got %v", c.ReadTimeout))
	}
	if c.WriteTimeout < 0 {
		errs = append(errs, fmt.Errorf("TW_SERVER_WRITE_TIMEOUT must be >= 0; got %v", c.WriteTimeout))
	}
	if c.IdleTimeout < 0 {
		errs = append(errs, fmt.Errorf("TW_SERVER_IDLE_TIMEOUT must be >= 0; got %v", c.IdleTimeout))
	}

	return errors.Join(errs...)
}

func (c SessionConfig) validate() error {
	var errs []error

	switch {
	case c.Name == "":
		errs = append(errs, errors.New("TW_SESSION_NAME is required"))
	// http.SetCookie drops a cookie whose name falls outside the token charset
	// of RFC 6265, section 4.1.1, serializing it to "" instead.
	case (&http.Cookie{Name: c.Name}).String() == "":
		errs = append(errs, fmt.Errorf("TW_SESSION_NAME must be a valid cookie name; got %q", c.Name))
	}
	// A sub-second TTL truncates to Max-Age=0, which net/http omits rather than
	// expires, shipping a cookie that outlives its store entry.
	if c.DefaultTTL < time.Second {
		errs = append(errs, fmt.Errorf("TW_SESSION_DEFAULT_TTL must be at least 1s; got %v", c.DefaultTTL))
	}
	if c.AuthenticatedTTL < time.Second {
		errs = append(errs, fmt.Errorf("TW_SESSION_AUTHENTICATED_TTL must be at least 1s; got %v", c.AuthenticatedTTL))
	}

	if c.StoreProvider != SessionStoreProviderMemory && c.StoreProvider != SessionStoreProviderValkey {
		errs = append(errs, fmt.Errorf("TW_SESSION_STORE_PROVIDER must be %q or %q; got %q",
			SessionStoreProviderMemory, SessionStoreProviderValkey, c.StoreProvider))
	}
	if c.StoreProvider == SessionStoreProviderValkey {
		errs = append(errs, c.Valkey.validate())
	}
	if c.StoreProvider == SessionStoreProviderMemory {
		errs = append(errs, c.Memory.validate())
	}

	return errors.Join(errs...)
}

func (c SessionMemoryConfig) validate() error {
	var errs []error

	if c.MaxEntries < 1 {
		errs = append(errs, fmt.Errorf("TW_SESSION_MEMORY_MAX_ENTRIES must be at least 1; got %d", c.MaxEntries))
	}
	if c.MaxBytes < 1 {
		errs = append(errs, fmt.Errorf("TW_SESSION_MEMORY_MAX_BYTES must be at least 1; got %d", c.MaxBytes))
	}

	return errors.Join(errs...)
}

func (c SessionValkeyConfig) validate() error {
	var errs []error

	if c.Prefix == "" {
		errs = append(errs, errors.New("TW_SESSION_VALKEY_PREFIX is required"))
	}

	if c.URL == nil {
		errs = append(errs, errors.New("TW_SESSION_VALKEY_URL is required"))
	} else {
		if c.URL.Scheme != "valkey" {
			errs = append(errs, fmt.Errorf(`TW_SESSION_VALKEY_URL must use scheme "valkey"; got %q`, c.URL.Scheme))
		}
		if c.URL.Hostname() == "" {
			errs = append(errs, fmt.Errorf("TW_SESSION_VALKEY_URL must include a hostname; got %q", c.URL.Redacted()))
		}
		if c.URL.Port() == "" {
			errs = append(errs, fmt.Errorf("TW_SESSION_VALKEY_URL must include a port; got %q", c.URL.Redacted()))
		}
		if c.URL.Query().Has("tls") {
			if tls := c.URL.Query().Get("tls"); tls != "true" && tls != "false" {
				errs = append(errs, fmt.Errorf(`TW_SESSION_VALKEY_URL "tls" must be "true" or "false"; got %q`, tls))
			}
		}
	}

	return errors.Join(errs...)
}

func (c APIProxyConfig) validate() error {
	var errs []error

	if c.StudentInsightsBaseURL == nil {
		errs = append(errs, errors.New("TW_API_PROXY_STUDENT_INSIGHTS_BASE_URL is required"))
	} else {
		if c.StudentInsightsBaseURL.Scheme != "http" && c.StudentInsightsBaseURL.Scheme != "https" {
			errs = append(errs, fmt.Errorf("TW_API_PROXY_STUDENT_INSIGHTS_BASE_URL must use scheme http or https; got %q", c.StudentInsightsBaseURL))
		}
		if c.StudentInsightsBaseURL.Host == "" {
			errs = append(errs, fmt.Errorf("TW_API_PROXY_STUDENT_INSIGHTS_BASE_URL must include host[:port]; got %q", c.StudentInsightsBaseURL))
		}
	}
	if c.StudentInsightsSigningKey == "" {
		errs = append(errs, errors.New("TW_API_PROXY_STUDENT_INSIGHTS_SIGNING_KEY is required"))
	} else {
		// RFC 7518, section 3.2 requires a key at least as big as hash output (HS256).
		if keyLength := len(c.StudentInsightsSigningKey); keyLength < sha256.Size {
			errs = append(errs, fmt.Errorf("TW_API_PROXY_STUDENT_INSIGHTS_SIGNING_KEY must be at least %d bytes; got %d", sha256.Size, keyLength))
		}
	}
	if c.PostsBaseURL == nil {
		errs = append(errs, errors.New("TW_API_PROXY_POSTS_BASE_URL is required"))
	} else {
		if c.PostsBaseURL.Scheme != "http" && c.PostsBaseURL.Scheme != "https" {
			errs = append(errs, fmt.Errorf("TW_API_PROXY_POSTS_BASE_URL must use scheme http or https; got %q", c.PostsBaseURL))
		}
		if c.PostsBaseURL.Host == "" {
			errs = append(errs, fmt.Errorf("TW_API_PROXY_POSTS_BASE_URL must include host[:port]; got %q", c.PostsBaseURL))
		}
	}
	if c.PostsSigningKey == "" {
		errs = append(errs, errors.New("TW_API_PROXY_POSTS_SIGNING_KEY is required"))
	} else {
		// RFC 7518, section 3.2 requires a key at least as big as hash output (HS256).
		if keyLength := len(c.PostsSigningKey); keyLength < sha256.Size {
			errs = append(errs, fmt.Errorf("TW_API_PROXY_POSTS_SIGNING_KEY must be at least %d bytes; got %d", sha256.Size, keyLength))
		}
	}
	if c.TokenTTL < time.Second {
		errs = append(errs, fmt.Errorf("TW_API_PROXY_TOKEN_TTL must be at least 1s; got %v", c.TokenTTL))
	}

	return errors.Join(errs...)
}

func validateHTTPURL(envName string, u *url.URL) []error {
	if u == nil {
		return []error{fmt.Errorf("%s is required", envName)}
	}
	var errs []error
	if u.Scheme != "http" && u.Scheme != "https" {
		errs = append(errs, fmt.Errorf("%s must use scheme http or https; got %q", envName, u))
	}
	if u.Host == "" {
		errs = append(errs, fmt.Errorf("%s must include host; got %q", envName, u))
	}
	return errs
}

func (c OIDCConfig) validate() error {
	var errs []error

	errs = append(errs, validateHTTPURL("TW_OIDC_ISSUER_URL", c.IssuerURL)...)
	errs = append(errs, validateHTTPURL("TW_OIDC_AUTH_URL", c.AuthURL)...)
	errs = append(errs, validateHTTPURL("TW_OIDC_TOKEN_URL", c.TokenURL)...)
	errs = append(errs, validateHTTPURL("TW_OIDC_JWKS_URI", c.JWKSURI)...)
	if c.ClientID == "" {
		errs = append(errs, errors.New("TW_OIDC_CLIENT_ID is required"))
	}
	if c.ClientSecret == "" {
		errs = append(errs, errors.New("TW_OIDC_CLIENT_SECRET is required"))
	}
	errs = append(errs, validateHTTPURL("TW_OIDC_REDIRECT_URL", c.RedirectURL)...)

	return errors.Join(errs...)
}

func (c RemoteConfig) validate() error {
	var errs []error

	for _, remote := range []struct{ name, value string }{
		{name: "TW_REMOTE_POSTS_MURL", value: c.PostsManifestURL},
		{name: "TW_REMOTE_STUDENT_INSIGHTS_MURL", value: c.StudentInsightsManifestURL},
	} {
		// An unset remote is not registered, so only a value is checked.
		if remote.value == "" {
			continue
		}

		u, err := url.Parse(remote.value)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("%s must be a valid url; got %q", remote.name, remote.value))
		case u.Scheme != "http" && u.Scheme != "https":
			errs = append(errs, fmt.Errorf("%s must use scheme http or https; got %q", remote.name, remote.value))
		case u.Host == "":
			errs = append(errs, fmt.Errorf("%s must include host[:port]; got %q", remote.name, remote.value))
		}
	}

	return errors.Join(errs...)
}

// must is a helper function to panic if an error is not nil.
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
