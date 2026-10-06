package config

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvProduction  Environment = "production"
)

// Config is the application configuration, read from TW_* environment variables.
type Config struct {
	Env      Environment `dotenv:"TW_ENV"`
	LogLevel slog.Level  `dotenv:"TW_LOG_LEVEL"`

	DevServerURL *url.URL `dotenv:"TW_DEV_SERVER_URL"`
	BuildDir     string   `dotenv:"TW_BUILD_DIR"`

	Server     ServerConfig     `dotenv:",squash"`
	Session    SessionConfig    `dotenv:",squash"`
	Edupass    EdupassConfig    `dotenv:",squash"`
	RemoteApps RemoteAppsConfig `dotenv:",squash"`
}

// Default returns a Config with only the fields that are safe in every
// environment set.
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
			Secure:           true,
			StoreProvider:    SessionStoreProviderMemory,
			Valkey: SessionValkeyConfig{
				Prefix: "session:",
			},
		},
		Edupass: EdupassConfig{
			ClientAuthMethod: EdupassClientAuthMethodClientSecretPost,
		},
		RemoteApps: RemoteAppsConfig{
			SignedTokenTTL: 1 * time.Minute,
		},
	}
}

// Validate returns an error describing every invalid field, or nil if the
// configuration is valid.
func (cfg *Config) Validate() error {
	var errs []error

	if cfg.Env != EnvDevelopment && cfg.Env != EnvProduction {
		errs = append(errs, fmt.Errorf("TW_ENV must be %q or %q; got %q", EnvDevelopment, EnvProduction, cfg.Env))
	}

	switch cfg.Env {
	case EnvDevelopment:
		if cfg.DevServerURL == nil {
			errs = append(errs, errors.New("TW_DEV_SERVER_URL is required"))
		} else {
			if cfg.DevServerURL.Scheme != "http" && cfg.DevServerURL.Scheme != "https" {
				errs = append(errs, fmt.Errorf("TW_DEV_SERVER_URL must use scheme http or https; got %q", cfg.DevServerURL))
			}
			if cfg.DevServerURL.Host == "" {
				errs = append(errs, fmt.Errorf("TW_DEV_SERVER_URL must include host[:port]; got %q", cfg.DevServerURL))
			}
		}
	case EnvProduction:
		if cfg.BuildDir == "" {
			errs = append(errs, errors.New("TW_BUILD_DIR is required"))
		} else if _, err := os.Stat(cfg.BuildDir); os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("TW_BUILD_DIR does not exist: %q", cfg.BuildDir))
		}
	}

	return errors.Join(append(
		errs,
		cfg.Server.validate(),
		cfg.Session.validate(),
		cfg.RemoteApps.validate(),
		cfg.Edupass.validate(),
	)...)
}

// ServerConfig represents the configuration for the HTTP server.
type ServerConfig struct {
	Port              int           `dotenv:"TW_SERVER_PORT"`
	ReadHeaderTimeout time.Duration `dotenv:"TW_SERVER_READ_HEADER_TIMEOUT"`
	ReadTimeout       time.Duration `dotenv:"TW_SERVER_READ_TIMEOUT"`
	WriteTimeout      time.Duration `dotenv:"TW_SERVER_WRITE_TIMEOUT"`
	IdleTimeout       time.Duration `dotenv:"TW_SERVER_IDLE_TIMEOUT"`
}

func (cfg ServerConfig) validate() error {
	var errs []error

	if cfg.Port < 1 || cfg.Port > 65535 {
		errs = append(errs, fmt.Errorf("TW_SERVER_PORT must be 1-65535; got %d", cfg.Port))
	}
	if cfg.ReadHeaderTimeout < 0 {
		errs = append(errs, fmt.Errorf("TW_SERVER_READ_HEADER_TIMEOUT must be >= 0; got %v", cfg.ReadHeaderTimeout))
	}
	if cfg.ReadTimeout < 0 {
		errs = append(errs, fmt.Errorf("TW_SERVER_READ_TIMEOUT must be >= 0; got %v", cfg.ReadTimeout))
	}
	if cfg.WriteTimeout < 0 {
		errs = append(errs, fmt.Errorf("TW_SERVER_WRITE_TIMEOUT must be >= 0; got %v", cfg.WriteTimeout))
	}
	if cfg.IdleTimeout < 0 {
		errs = append(errs, fmt.Errorf("TW_SERVER_IDLE_TIMEOUT must be >= 0; got %v", cfg.IdleTimeout))
	}

	return errors.Join(errs...)
}

type SessionStoreProvider string

const (
	SessionStoreProviderMemory SessionStoreProvider = "memory"
	SessionStoreProviderValkey SessionStoreProvider = "valkey"
)

// SessionConfig represents the configuration for sessions.
type SessionConfig struct {
	Name             string               `dotenv:"TW_SESSION_NAME"`
	DefaultTTL       time.Duration        `dotenv:"TW_SESSION_DEFAULT_TTL"`
	AuthenticatedTTL time.Duration        `dotenv:"TW_SESSION_AUTHENTICATED_TTL"`
	Secure           bool                 `dotenv:"TW_SESSION_SECURE"`
	StoreProvider    SessionStoreProvider `dotenv:"TW_SESSION_STORE_PROVIDER"`

	Valkey SessionValkeyConfig `dotenv:",squash"`
}

type SessionValkeyConfig struct {
	URL    *url.URL `dotenv:"TW_SESSION_VALKEY_URL"`
	Prefix string   `dotenv:"TW_SESSION_VALKEY_PREFIX"`
}

func (cfg SessionConfig) validate() error {
	var errs []error

	if cfg.Name == "" {
		errs = append(errs, errors.New("TW_SESSION_NAME is required"))
	} else if (&http.Cookie{Name: cfg.Name}).String() == "" {
		// http.SetCookie drops a cookie whose name falls outside the token charset
		// of RFC 6265, section 4.1.1, serializing it to "" instead.
		errs = append(errs, fmt.Errorf("TW_SESSION_NAME must be a valid cookie name; got %q", cfg.Name))
	}

	// A sub-second TTL truncates to Max-Age=0, which net/http omits rather than
	// expires, shipping a cookie that outlives its store entry.
	if cfg.DefaultTTL < time.Second {
		errs = append(errs, fmt.Errorf("TW_SESSION_DEFAULT_TTL must be at least 1s; got %v", cfg.DefaultTTL))
	}
	if cfg.AuthenticatedTTL < time.Second {
		errs = append(errs, fmt.Errorf("TW_SESSION_AUTHENTICATED_TTL must be at least 1s; got %v", cfg.AuthenticatedTTL))
	}

	switch cfg.StoreProvider {
	case SessionStoreProviderMemory:
	case SessionStoreProviderValkey:
		errs = append(errs, cfg.Valkey.validate())
	default:
		errs = append(errs, fmt.Errorf("TW_SESSION_STORE_PROVIDER must be %q or %q; got %q",
			SessionStoreProviderMemory, SessionStoreProviderValkey, cfg.StoreProvider))
	}

	return errors.Join(errs...)
}

func (cfg SessionValkeyConfig) validate() error {
	var errs []error

	if cfg.Prefix == "" {
		errs = append(errs, errors.New("TW_SESSION_VALKEY_PREFIX is required"))
	}

	if cfg.URL == nil {
		errs = append(errs, errors.New("TW_SESSION_VALKEY_URL is required"))
	} else {
		if cfg.URL.Scheme != "valkey" {
			errs = append(errs, fmt.Errorf(`TW_SESSION_VALKEY_URL must use scheme "valkey"; got %q`, cfg.URL.Scheme))
		}
		if cfg.URL.Hostname() == "" {
			errs = append(errs, fmt.Errorf("TW_SESSION_VALKEY_URL must include a hostname; got %q", cfg.URL.Redacted()))
		}
		if cfg.URL.Port() == "" {
			errs = append(errs, fmt.Errorf("TW_SESSION_VALKEY_URL must include a port; got %q", cfg.URL.Redacted()))
		}
		if cfg.URL.Query().Has("tls") {
			if tls := cfg.URL.Query().Get("tls"); tls != "true" && tls != "false" {
				errs = append(errs, fmt.Errorf(`TW_SESSION_VALKEY_URL "tls" must be "true" or "false"; got %q`, tls))
			}
		}
	}

	return errors.Join(errs...)
}

type EdupassClientAuthMethod string

const (
	EdupassClientAuthMethodClientSecretPost EdupassClientAuthMethod = "client_secret_post"
)

type EdupassClientCredentials struct {
	Secret string
}

// EdupassConfig represents the configuration for the Edupass identity provider.
type EdupassConfig struct {
	IssuerURL *url.URL `dotenv:"TW_EDUPASS_ISSUER_URL"`
	AuthURL   *url.URL `dotenv:"TW_EDUPASS_AUTH_URL"`
	TokenURL  *url.URL `dotenv:"TW_EDUPASS_TOKEN_URL"`
	JWKSURL   *url.URL `dotenv:"TW_EDUPASS_JWKS_URL"`

	ClientID    string   `dotenv:"TW_EDUPASS_CLIENT_ID"`
	RedirectURL *url.URL `dotenv:"TW_EDUPASS_REDIRECT_URL"`

	ClientAuthMethod EdupassClientAuthMethod `dotenv:"TW_EDUPASS_CLIENT_AUTH_METHOD"`
	ClientSecret     string                  `dotenv:"TW_EDUPASS_CLIENT_SECRET"`
	ClientSecretFile string                  `dotenv:"TW_EDUPASS_CLIENT_SECRET_FILE"`

	ClientCredentials EdupassClientCredentials `dotenv:"-"`
}

func (cfg *EdupassConfig) validate() error {
	var errs []error

	if cfg.IssuerURL == nil {
		errs = append(errs, errors.New("TW_EDUPASS_ISSUER_URL is required"))
	} else {
		if cfg.IssuerURL.Scheme != "http" && cfg.IssuerURL.Scheme != "https" {
			errs = append(errs, fmt.Errorf("TW_EDUPASS_ISSUER_URL must use scheme http or https; got %q", cfg.IssuerURL))
		}
		if cfg.IssuerURL.Host == "" {
			errs = append(errs, fmt.Errorf("TW_EDUPASS_ISSUER_URL must include host[:port]; got %q", cfg.IssuerURL))
		}
	}
	if cfg.AuthURL == nil {
		errs = append(errs, errors.New("TW_EDUPASS_AUTH_URL is required"))
	} else {
		if cfg.AuthURL.Scheme != "http" && cfg.AuthURL.Scheme != "https" {
			errs = append(errs, fmt.Errorf("TW_EDUPASS_AUTH_URL must use scheme http or https; got %q", cfg.AuthURL))
		}
		if cfg.AuthURL.Host == "" {
			errs = append(errs, fmt.Errorf("TW_EDUPASS_AUTH_URL must include host[:port]; got %q", cfg.AuthURL))
		}
	}
	if cfg.TokenURL == nil {
		errs = append(errs, errors.New("TW_EDUPASS_TOKEN_URL is required"))
	} else {
		if cfg.TokenURL.Scheme != "http" && cfg.TokenURL.Scheme != "https" {
			errs = append(errs, fmt.Errorf("TW_EDUPASS_TOKEN_URL must use scheme http or https; got %q", cfg.TokenURL))
		}
		if cfg.TokenURL.Host == "" {
			errs = append(errs, fmt.Errorf("TW_EDUPASS_TOKEN_URL must include host[:port]; got %q", cfg.TokenURL))
		}
	}
	if cfg.JWKSURL == nil {
		errs = append(errs, errors.New("TW_EDUPASS_JWKS_URI is required"))
	} else {
		if cfg.JWKSURL.Scheme != "http" && cfg.JWKSURL.Scheme != "https" {
			errs = append(errs, fmt.Errorf("TW_EDUPASS_JWKS_URI must use scheme http or https; got %q", cfg.JWKSURL))
		}
		if cfg.JWKSURL.Host == "" {
			errs = append(errs, fmt.Errorf("TW_EDUPASS_JWKS_URI must include host[:port]; got %q", cfg.JWKSURL))
		}
	}

	if cfg.ClientID == "" {
		errs = append(errs, errors.New("TW_EDUPASS_CLIENT_ID is required"))
	}
	if cfg.RedirectURL == nil {
		errs = append(errs, errors.New("TW_EDUPASS_REDIRECT_URL is required"))
	} else {
		if cfg.RedirectURL.Scheme != "http" && cfg.RedirectURL.Scheme != "https" {
			errs = append(errs, fmt.Errorf("TW_EDUPASS_REDIRECT_URL must use scheme http or https; got %q", cfg.RedirectURL))
		}
		if cfg.RedirectURL.Host == "" {
			errs = append(errs, fmt.Errorf("TW_EDUPASS_REDIRECT_URL must include host[:port]; got %q", cfg.RedirectURL))
		}
	}

	switch cfg.ClientAuthMethod {
	case EdupassClientAuthMethodClientSecretPost:
		if cfg.ClientSecret != "" && cfg.ClientSecretFile != "" {
			errs = append(errs, errors.New("TW_EDUPASS_CLIENT_SECRET and TW_EDUPASS_CLIENT_SECRET_FILE are both set; set only one"))
		}
		if cfg.ClientSecret == "" && cfg.ClientSecretFile == "" {
			errs = append(errs, errors.New("TW_EDUPASS_CLIENT_SECRET or TW_EDUPASS_CLIENT_SECRET_FILE is required for client_secret_post"))
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}

		clientSecret := cfg.ClientSecret
		if cfg.ClientSecretFile != "" {
			clientSecretFileContents, err := os.ReadFile(cfg.ClientSecretFile)
			if err != nil {
				return fmt.Errorf("TW_EDUPASS_CLIENT_SECRET_FILE: %w", err)
			}
			clientSecret = strings.TrimRight(string(clientSecretFileContents), "\r\n")
			if clientSecret == "" {
				return fmt.Errorf("TW_EDUPASS_CLIENT_SECRET_FILE: %s is empty", cfg.ClientSecretFile)
			}
		}
		cfg.ClientCredentials = EdupassClientCredentials{Secret: clientSecret}

	default:
		errs = append(errs, fmt.Errorf("TW_EDUPASS_CLIENT_AUTH_METHOD must be %q; got %q",
			EdupassClientAuthMethodClientSecretPost, cfg.ClientAuthMethod))
	}

	return errors.Join(errs...)
}

// RemoteAppsConfig represents the configuration for the remote apps. Each app
// must have either all of its values set, which registers it, or none.
type RemoteAppsConfig struct {
	SignedTokenTTL time.Duration `dotenv:"TW_REMOTE_SIGNED_TOKEN_TTL"`

	PostsManifestURL       *url.URL `dotenv:"TW_REMOTE_POSTS_MANIFEST_URL"`
	PostsBackendBaseURL    *url.URL `dotenv:"TW_REMOTE_POSTS_BACKEND_BASE_URL"`
	PostsBackendSigningKey string   `dotenv:"TW_REMOTE_POSTS_BACKEND_SIGNING_KEY"`

	StudentInsightsManifestURL       *url.URL `dotenv:"TW_REMOTE_STUDENT_INSIGHTS_MANIFEST_URL"`
	StudentInsightsBackendBaseURL    *url.URL `dotenv:"TW_REMOTE_STUDENT_INSIGHTS_BACKEND_BASE_URL"`
	StudentInsightsBackendSigningKey string   `dotenv:"TW_REMOTE_STUDENT_INSIGHTS_BACKEND_SIGNING_KEY"`
}

func (cfg RemoteAppsConfig) validate() error {
	var errs []error

	if cfg.SignedTokenTTL < time.Second {
		errs = append(errs, fmt.Errorf("TW_REMOTE_SIGNED_TOKEN_TTL must be at least 1s; got %v", cfg.SignedTokenTTL))
	}

	if cfg.PostsManifestURL != nil || cfg.PostsBackendBaseURL != nil || cfg.PostsBackendSigningKey != "" {
		if cfg.PostsManifestURL == nil {
			errs = append(errs, errors.New("TW_REMOTE_POSTS_MANIFEST_URL is required to register the posts remote"))
		} else {
			if cfg.PostsManifestURL.Scheme != "http" && cfg.PostsManifestURL.Scheme != "https" {
				errs = append(errs, fmt.Errorf("TW_REMOTE_POSTS_MANIFEST_URL must use scheme http or https; got %q", cfg.PostsManifestURL))
			}
			if cfg.PostsManifestURL.Host == "" {
				errs = append(errs, fmt.Errorf("TW_REMOTE_POSTS_MANIFEST_URL must include host[:port]; got %q", cfg.PostsManifestURL))
			}
		}
		if cfg.PostsBackendBaseURL == nil {
			errs = append(errs, errors.New("TW_REMOTE_POSTS_BACKEND_BASE_URL is required to register the posts remote"))
		} else {
			if cfg.PostsBackendBaseURL.Scheme != "http" && cfg.PostsBackendBaseURL.Scheme != "https" {
				errs = append(errs, fmt.Errorf("TW_REMOTE_POSTS_BACKEND_BASE_URL must use scheme http or https; got %q", cfg.PostsBackendBaseURL))
			}
			if cfg.PostsBackendBaseURL.Host == "" {
				errs = append(errs, fmt.Errorf("TW_REMOTE_POSTS_BACKEND_BASE_URL must include host[:port]; got %q", cfg.PostsBackendBaseURL))
			}
			if cfg.PostsBackendBaseURL.RawQuery != "" {
				errs = append(errs, fmt.Errorf("TW_REMOTE_POSTS_BACKEND_BASE_URL must not include a query; got %q", cfg.PostsBackendBaseURL))
			}
			if cfg.PostsBackendBaseURL.Fragment != "" {
				errs = append(errs, fmt.Errorf("TW_REMOTE_POSTS_BACKEND_BASE_URL must not include a fragment; got %q", cfg.PostsBackendBaseURL))
			}
		}
		if cfg.PostsBackendSigningKey == "" {
			errs = append(errs, errors.New("TW_REMOTE_POSTS_BACKEND_SIGNING_KEY is required to register the posts remote"))
		} else {
			// RFC 7518, section 3.2 requires a key at least as big as hash output (HS256).
			if keyLength := len(cfg.PostsBackendSigningKey); keyLength < sha256.Size {
				errs = append(errs, fmt.Errorf("TW_REMOTE_POSTS_BACKEND_SIGNING_KEY must be at least %d bytes; got %d", sha256.Size, keyLength))
			}
		}
	}

	if cfg.StudentInsightsManifestURL != nil || cfg.StudentInsightsBackendBaseURL != nil || cfg.StudentInsightsBackendSigningKey != "" {
		if cfg.StudentInsightsManifestURL == nil {
			errs = append(errs, errors.New("TW_REMOTE_STUDENT_INSIGHTS_MANIFEST_URL is required to register the student insights remote"))
		} else {
			if cfg.StudentInsightsManifestURL.Scheme != "http" && cfg.StudentInsightsManifestURL.Scheme != "https" {
				errs = append(errs, fmt.Errorf("TW_REMOTE_STUDENT_INSIGHTS_MANIFEST_URL must use scheme http or https; got %q", cfg.StudentInsightsManifestURL))
			}
			if cfg.StudentInsightsManifestURL.Host == "" {
				errs = append(errs, fmt.Errorf("TW_REMOTE_STUDENT_INSIGHTS_MANIFEST_URL must include host[:port]; got %q", cfg.StudentInsightsManifestURL))
			}
		}
		if cfg.StudentInsightsBackendBaseURL == nil {
			errs = append(errs, errors.New("TW_REMOTE_STUDENT_INSIGHTS_BACKEND_BASE_URL is required to register the student insights remote"))
		} else {
			if cfg.StudentInsightsBackendBaseURL.Scheme != "http" && cfg.StudentInsightsBackendBaseURL.Scheme != "https" {
				errs = append(errs, fmt.Errorf("TW_REMOTE_STUDENT_INSIGHTS_BACKEND_BASE_URL must use scheme http or https; got %q", cfg.StudentInsightsBackendBaseURL))
			}
			if cfg.StudentInsightsBackendBaseURL.Host == "" {
				errs = append(errs, fmt.Errorf("TW_REMOTE_STUDENT_INSIGHTS_BACKEND_BASE_URL must include host[:port]; got %q", cfg.StudentInsightsBackendBaseURL))
			}
			if cfg.StudentInsightsBackendBaseURL.RawQuery != "" {
				errs = append(errs, fmt.Errorf("TW_REMOTE_STUDENT_INSIGHTS_BACKEND_BASE_URL must not include a query; got %q", cfg.StudentInsightsBackendBaseURL))
			}
			if cfg.StudentInsightsBackendBaseURL.Fragment != "" {
				errs = append(errs, fmt.Errorf("TW_REMOTE_STUDENT_INSIGHTS_BACKEND_BASE_URL must not include a fragment; got %q", cfg.StudentInsightsBackendBaseURL))
			}
		}
		if cfg.StudentInsightsBackendSigningKey == "" {
			errs = append(errs, errors.New("TW_REMOTE_STUDENT_INSIGHTS_BACKEND_SIGNING_KEY is required to register the student insights remote"))
		} else {
			// RFC 7518, section 3.2 requires a key at least as big as hash output (HS256).
			if keyLength := len(cfg.StudentInsightsBackendSigningKey); keyLength < sha256.Size {
				errs = append(errs, fmt.Errorf("TW_REMOTE_STUDENT_INSIGHTS_BACKEND_SIGNING_KEY must be at least %d bytes; got %d", sha256.Size, keyLength))
			}
		}
	}

	return errors.Join(errs...)
}

// IsPostsRegistered reports whether the posts remote app has all of its values set.
func (cfg RemoteAppsConfig) IsPostsRegistered() bool {
	return cfg.PostsManifestURL != nil && cfg.PostsBackendBaseURL != nil && cfg.PostsBackendSigningKey != ""
}

// IsStudentInsightsRegistered reports whether the student insights remote app
// has all of its values set.
func (cfg RemoteAppsConfig) IsStudentInsightsRegistered() bool {
	return cfg.StudentInsightsManifestURL != nil && cfg.StudentInsightsBackendBaseURL != nil && cfg.StudentInsightsBackendSigningKey != ""
}

// must is a helper function to panic if an error is not nil.
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
