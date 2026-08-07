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
}

// ServerConfig represents the configuration for the HTTP server.
type ServerConfig struct {
	Port              int           `dotenv:"TW_SERVER_PORT"`
	ReadHeaderTimeout time.Duration `dotenv:"TW_SERVER_READ_HEADER_TIMEOUT"`
	ReadTimeout       time.Duration `dotenv:"TW_SERVER_READ_TIMEOUT"`
	WriteTimeout      time.Duration `dotenv:"TW_SERVER_WRITE_TIMEOUT"`
	IdleTimeout       time.Duration `dotenv:"TW_SERVER_IDLE_TIMEOUT"`
}

type StoreProvider string

const (
	StoreProviderMemory StoreProvider = "memory"
	StoreProviderValkey StoreProvider = "valkey"
)

// SessionConfig represents the configuration for the session.
type SessionConfig struct {
	Name              string        `dotenv:"TW_SESSION_NAME"`
	DefaultTTL        time.Duration `dotenv:"TW_SESSION_DEFAULT_TTL"`
	AuthenticatedTTL  time.Duration `dotenv:"TW_SESSION_AUTHENTICATED_TTL"`
	StoreProvider     StoreProvider `dotenv:"TW_SESSION_STORE_PROVIDER"`
	ValkeyURL         *url.URL      `dotenv:"TW_SESSION_VALKEY_URL"`
	ValkeyPrefix      string        `dotenv:"TW_SESSION_VALKEY_PREFIX"`
	ValkeyDialTimeout time.Duration `dotenv:"TW_SESSION_VALKEY_DIAL_TIMEOUT"`
}

// APIProxyConfig represents the configuration for the backend proxies.
type APIProxyConfig struct {
	StudentInsightsBaseURL    *url.URL      `dotenv:"TW_API_PROXY_STUDENT_INSIGHTS_BASE_URL"`
	StudentInsightsSigningKey string        `dotenv:"TW_API_PROXY_STUDENT_INSIGHTS_SIGNING_KEY"`
	PostsBaseURL              *url.URL      `dotenv:"TW_API_PROXY_POSTS_BASE_URL"`
	PostsSigningKey           string        `dotenv:"TW_API_PROXY_POSTS_SIGNING_KEY"`
	TokenTTL                  time.Duration `dotenv:"TW_API_PROXY_TOKEN_TTL"`
}

// Default returns the default configuration for the application.
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
			Name:              "tw_session",
			DefaultTTL:        3 * time.Hour,
			AuthenticatedTTL:  30 * time.Minute,
			StoreProvider:     StoreProviderMemory,
			ValkeyPrefix:      "session:",
			ValkeyDialTimeout: 5 * time.Second,
		},
		APIProxy: APIProxyConfig{
			StudentInsightsBaseURL:    must(url.Parse("http://127.0.0.1:3002")),
			PostsBaseURL:              must(url.Parse("http://127.0.0.1:3003")),
			StudentInsightsSigningKey: "a-string-secret-at-least-256-bits-long",
			PostsSigningKey:           "a-string-secret-at-least-256-bits-long",
			TokenTTL:                  1 * time.Minute,
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

	return errors.Join(append(errs, c.Server.validate(), c.Session.validate(), c.APIProxy.validate())...)
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

	switch c.StoreProvider {
	case StoreProviderMemory:
	case StoreProviderValkey:
		errs = append(errs, c.validateValkey()...)
	default:
		errs = append(errs, fmt.Errorf("TW_SESSION_STORE_PROVIDER must be %q or %q; got %q",
			StoreProviderMemory, StoreProviderValkey, c.StoreProvider))
	}

	return errors.Join(errs...)
}

func (c SessionConfig) validateValkey() []error {
	var errs []error

	if c.ValkeyDialTimeout <= 0 {
		errs = append(errs, fmt.Errorf("TW_SESSION_VALKEY_DIAL_TIMEOUT must be positive; got %v", c.ValkeyDialTimeout))
	}
	if c.ValkeyPrefix == "" {
		errs = append(errs, errors.New("TW_SESSION_VALKEY_PREFIX is required"))
	}

	if c.ValkeyURL == nil {
		return append(errs, fmt.Errorf("TW_SESSION_VALKEY_URL is required when TW_SESSION_STORE_PROVIDER is %q", StoreProviderValkey))
	}

	if c.ValkeyURL.Scheme != "valkey" {
		errs = append(errs, fmt.Errorf(`TW_SESSION_VALKEY_URL must use scheme "valkey"; got %q`, c.ValkeyURL.Scheme))
	}
	// Redacted, not the URL itself: %q on a *url.URL calls String(), which
	// prints the password, and this error is logged at startup.
	if c.ValkeyURL.Hostname() == "" {
		errs = append(errs, fmt.Errorf("TW_SESSION_VALKEY_URL must include a host; got %q", c.ValkeyURL.Redacted()))
	}
	if c.ValkeyURL.Port() == "" {
		errs = append(errs, fmt.Errorf("TW_SESSION_VALKEY_URL must include a port; got %q", c.ValkeyURL.Redacted()))
	}

	for key, vals := range c.ValkeyURL.Query() {
		if key != "tls" {
			errs = append(errs, fmt.Errorf("TW_SESSION_VALKEY_URL has unknown query parameter %q; only \"tls\" is supported", key))
			continue
		}
		// The store reads the first value, so a repeated parameter is ambiguous
		// rather than last-wins: accepting it risks validating "true" while the
		// connection is made in plaintext.
		if len(vals) > 1 {
			errs = append(errs, fmt.Errorf(`TW_SESSION_VALKEY_URL has %d "tls" values; specify it once`, len(vals)))
			continue
		}
		if v := vals[0]; v != "true" && v != "false" {
			errs = append(errs, fmt.Errorf(`TW_SESSION_VALKEY_URL tls must be "true" or "false"; got %q`, v))
		}
	}

	return errs
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

// must is a helper function to panic if an error is not nil.
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
