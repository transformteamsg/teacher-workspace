package config

import (
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	t.Run("returns the default config", func(t *testing.T) {
		cfg := Default()

		if want, got := EnvDevelopment, cfg.Env; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := slog.LevelInfo, cfg.LogLevel; want != got {
			t.Errorf("want: %v; got: %v", want, got)
		}
		if want, got := "http://127.0.0.1:3001", cfg.DevServerURL.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "apps/host/dist", cfg.BuildDir; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}

		if got := cfg.Remote.PostsManifestURL; got != "" {
			t.Errorf("want: empty; got: %q", got)
		}
		if got := cfg.Remote.StudentInsightsManifestURL; got != "" {
			t.Errorf("want: empty; got: %q", got)
		}

		if want, got := 3000, cfg.Server.Port; want != got {
			t.Errorf("want: %d; got: %d", want, got)
		}
		if want, got := 2*time.Second, cfg.Server.ReadHeaderTimeout; want != got {
			t.Errorf("want: %v; got: %v", want, got)
		}
		if want, got := 15*time.Second, cfg.Server.ReadTimeout; want != got {
			t.Errorf("want: %v; got: %v", want, got)
		}
		if want, got := 30*time.Second, cfg.Server.WriteTimeout; want != got {
			t.Errorf("want: %v; got: %v", want, got)
		}
		if want, got := 60*time.Second, cfg.Server.IdleTimeout; want != got {
			t.Errorf("want: %v; got: %v", want, got)
		}

		if want, got := "tw_session", cfg.Session.Name; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := 3*time.Hour, cfg.Session.DefaultTTL; want != got {
			t.Errorf("want: %v; got: %v", want, got)
		}
		if want, got := 30*time.Minute, cfg.Session.AuthenticatedTTL; want != got {
			t.Errorf("want: %v; got: %v", want, got)
		}
		if want, got := SessionStoreProviderMemory, cfg.Session.StoreProvider; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if got := cfg.Session.Valkey.URL; got != nil {
			t.Errorf("want: nil; got: %q", got)
		}
		if want, got := "session:", cfg.Session.Valkey.Prefix; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}

		if want, got := "http://127.0.0.1:3002", cfg.APIProxy.StudentInsightsBaseURL.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "http://127.0.0.1:3003", cfg.APIProxy.PostsBaseURL.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "a-string-secret-at-least-256-bits-long", cfg.APIProxy.StudentInsightsSigningKey; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "a-string-secret-at-least-256-bits-long", cfg.APIProxy.PostsSigningKey; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := time.Minute, cfg.APIProxy.TokenTTL; want != got {
			t.Errorf("want: %v; got: %v", want, got)
		}
	})
}

func validOIDCConfig() OIDCConfig {
	return OIDCConfig{
		IssuerURL:    &url.URL{Scheme: "http", Host: "localhost:9000"},
		AuthURL:      &url.URL{Scheme: "http", Host: "localhost:9000", Path: "/authorize"},
		TokenURL:     &url.URL{Scheme: "http", Host: "localhost:9000", Path: "/token"},
		JWKSURI:      &url.URL{Scheme: "http", Host: "localhost:9000", Path: "/jwks"},
		ClientID:     "teacher-workspace",
		ClientSecret: "teacher-workspace-secret",
		RedirectURL:  &url.URL{Scheme: "http", Host: "localhost:3000", Path: "/auth/edupass/callback"},
	}
}

func TestConfig_Validate(t *testing.T) {
	t.Run("accepts http and https dev server urls", func(t *testing.T) {
		for _, scheme := range []string{"http", "https"} {
			t.Run(scheme, func(t *testing.T) {
				cfg := Default()
				cfg.OIDC = validOIDCConfig()
				cfg.DevServerURL = &url.URL{Scheme: scheme, Host: "127.0.0.1:3001"}

				if err := cfg.Validate(); err != nil {
					t.Errorf("want err: nil; got: %v", err)
				}
			})
		}
	})

	t.Run("accepts production pointing at an existing build dir", func(t *testing.T) {
		cfg := Default()
		cfg.OIDC = validOIDCConfig()
		cfg.Env = EnvProduction
		cfg.BuildDir = t.TempDir()
		cfg.Session.StoreProvider = SessionStoreProviderValkey
		cfg.Session.Valkey.URL = &url.URL{Scheme: "valkey", Host: "127.0.0.1:6379"}

		if err := cfg.Validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("rejects invalid values", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			mutate func(*Config)
			want   string
		}{
			{
				name:   "unknown env",
				mutate: func(c *Config) { c.Env = "staging" },
				want:   `TW_ENV must be "development" or "production"; got "staging"`,
			},
			{
				name:   "missing dev server url",
				mutate: func(c *Config) { c.DevServerURL = nil },
				want:   "TW_DEV_SERVER_URL is required",
			},
			{
				name:   "dev server url with a non-http scheme",
				mutate: func(c *Config) { c.DevServerURL = &url.URL{Scheme: "ftp", Host: "127.0.0.1:3001"} },
				want:   `TW_DEV_SERVER_URL must use scheme http or https; got "ftp://127.0.0.1:3001"`,
			},
			{
				name:   "dev server url without a host",
				mutate: func(c *Config) { c.DevServerURL = &url.URL{Scheme: "http"} },
				want:   `TW_DEV_SERVER_URL must include host[:port]; got "http:"`,
			},
			{
				name: "empty build dir in production",
				mutate: func(c *Config) {
					c.Env = EnvProduction
					c.BuildDir = ""
				},
				want: "TW_BUILD_DIR is required",
			},
			{
				name: "missing build dir in production",
				mutate: func(c *Config) {
					c.Env = EnvProduction
					c.BuildDir = "testdata/does-not-exist"
				},
				want: `TW_BUILD_DIR does not exist: "testdata/does-not-exist"`,
			},
			{
				name: "memory session store in production",
				mutate: func(c *Config) {
					c.Env = EnvProduction
					c.Session.StoreProvider = SessionStoreProviderMemory
				},
				want: `TW_SESSION_STORE_PROVIDER must be "valkey" when TW_ENV is "production"; got "memory"`,
			},
			{
				name:   "remote manifest url with a non-http scheme",
				mutate: func(c *Config) { c.Remote.PostsManifestURL = "ftp://pg.test/mf-manifest.json" },
				want:   `TW_REMOTE_POSTS_MURL must use scheme http or https; got "ftp://pg.test/mf-manifest.json"`,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				cfg := Default()
				tt.mutate(&cfg)

				err := cfg.Validate()

				if err == nil {
					t.Fatal("want err: non-nil; got: nil")
				}
				if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("want err: containing %q; got: %q", tt.want, err)
				}
			})
		}
	})

	t.Run("skips the dev server url outside development", func(t *testing.T) {
		cfg := Default()
		cfg.OIDC = validOIDCConfig()
		cfg.Env = EnvProduction
		cfg.BuildDir = t.TempDir()
		cfg.Session.StoreProvider = SessionStoreProviderValkey
		cfg.Session.Valkey.URL = &url.URL{Scheme: "valkey", Host: "127.0.0.1:6379"}
		cfg.DevServerURL = &url.URL{}

		if err := cfg.Validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("skips the build dir outside production", func(t *testing.T) {
		cfg := Default()
		cfg.OIDC = validOIDCConfig()
		cfg.BuildDir = "testdata/does-not-exist"

		if err := cfg.Validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("accepts the memory session store outside production", func(t *testing.T) {
		cfg := Default()
		cfg.OIDC = validOIDCConfig()
		cfg.Session.StoreProvider = SessionStoreProviderMemory

		if err := cfg.Validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("reports multiple invalid fields in one error", func(t *testing.T) {
		cfg := Default()
		cfg.Env = "staging"
		cfg.Remote.PostsManifestURL = "ftp://pg.test/mf-manifest.json"
		cfg.Server.Port = 0
		cfg.Session.Name = ""
		cfg.APIProxy.PostsBaseURL = nil
		cfg.OIDC = validOIDCConfig()
		cfg.OIDC.ClientID = ""

		err := cfg.Validate()

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		for _, want := range []string{"TW_ENV", "TW_REMOTE_POSTS_MURL", "TW_SERVER_PORT", "TW_SESSION_NAME", "TW_API_PROXY_POSTS_BASE_URL", "TW_OIDC_CLIENT_ID"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("want err: containing %q; got: %q", want, err)
			}
		}
	})
}

func TestServerConfig_validate(t *testing.T) {
	t.Run("accepts zero timeouts", func(t *testing.T) {
		// net/http reads a zero timeout as "no timeout", so it is a valid choice
		// rather than an unset field.
		cfg := ServerConfig{Port: 3000}

		if err := cfg.validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("rejects invalid values", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			mutate func(*ServerConfig)
			want   string
		}{
			{
				name:   "port below range",
				mutate: func(c *ServerConfig) { c.Port = 0 },
				want:   "TW_SERVER_PORT must be 1-65535; got 0",
			},
			{
				name:   "port above range",
				mutate: func(c *ServerConfig) { c.Port = 65536 },
				want:   "TW_SERVER_PORT must be 1-65535; got 65536",
			},
			{
				name:   "negative read header timeout",
				mutate: func(c *ServerConfig) { c.ReadHeaderTimeout = -time.Second },
				want:   "TW_SERVER_READ_HEADER_TIMEOUT must be >= 0; got -1s",
			},
			{
				name:   "negative read timeout",
				mutate: func(c *ServerConfig) { c.ReadTimeout = -time.Second },
				want:   "TW_SERVER_READ_TIMEOUT must be >= 0; got -1s",
			},
			{
				name:   "negative write timeout",
				mutate: func(c *ServerConfig) { c.WriteTimeout = -time.Second },
				want:   "TW_SERVER_WRITE_TIMEOUT must be >= 0; got -1s",
			},
			{
				name:   "negative idle timeout",
				mutate: func(c *ServerConfig) { c.IdleTimeout = -time.Second },
				want:   "TW_SERVER_IDLE_TIMEOUT must be >= 0; got -1s",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				cfg := Default().Server
				tt.mutate(&cfg)

				err := cfg.validate()

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

func TestSessionConfig_validate(t *testing.T) {
	t.Run("rejects invalid values", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			mutate func(*SessionConfig)
			want   string
		}{
			{
				name:   "empty name",
				mutate: func(c *SessionConfig) { c.Name = "" },
				want:   "TW_SESSION_NAME is required",
			},
			{
				name:   "name with a space",
				mutate: func(c *SessionConfig) { c.Name = "tw session" },
				want:   `TW_SESSION_NAME must be a valid cookie name; got "tw session"`,
			},
			{
				name:   "name with an equals sign",
				mutate: func(c *SessionConfig) { c.Name = "tw=session" },
				want:   `TW_SESSION_NAME must be a valid cookie name; got "tw=session"`,
			},
			{
				name:   "name with a semicolon",
				mutate: func(c *SessionConfig) { c.Name = "tw;session" },
				want:   `TW_SESSION_NAME must be a valid cookie name; got "tw;session"`,
			},
			{
				name:   "zero default TTL",
				mutate: func(c *SessionConfig) { c.DefaultTTL = 0 },
				want:   "TW_SESSION_DEFAULT_TTL must be at least 1s; got 0s",
			},
			{
				name:   "negative default TTL",
				mutate: func(c *SessionConfig) { c.DefaultTTL = -time.Second },
				want:   "TW_SESSION_DEFAULT_TTL must be at least 1s; got -1s",
			},
			{
				name:   "sub-second default TTL",
				mutate: func(c *SessionConfig) { c.DefaultTTL = 500 * time.Millisecond },
				want:   "TW_SESSION_DEFAULT_TTL must be at least 1s; got 500ms",
			},
			{
				name:   "zero authenticated TTL",
				mutate: func(c *SessionConfig) { c.AuthenticatedTTL = 0 },
				want:   "TW_SESSION_AUTHENTICATED_TTL must be at least 1s; got 0s",
			},
			{
				name:   "negative authenticated TTL",
				mutate: func(c *SessionConfig) { c.AuthenticatedTTL = -time.Second },
				want:   "TW_SESSION_AUTHENTICATED_TTL must be at least 1s; got -1s",
			},
			{
				name:   "sub-second authenticated TTL",
				mutate: func(c *SessionConfig) { c.AuthenticatedTTL = 500 * time.Millisecond },
				want:   "TW_SESSION_AUTHENTICATED_TTL must be at least 1s; got 500ms",
			},
			{
				name:   "unknown store provider",
				mutate: func(c *SessionConfig) { c.StoreProvider = "postgres" },
				want:   `TW_SESSION_STORE_PROVIDER must be "memory" or "valkey"; got "postgres"`,
			},
			{
				name: "valkey provider without a url",
				mutate: func(c *SessionConfig) {
					c.StoreProvider = SessionStoreProviderValkey
					c.Valkey.URL = nil
				},
				want: "TW_SESSION_VALKEY_URL is required",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				cfg := Default().Session
				tt.mutate(&cfg)

				err := cfg.validate()

				if err == nil {
					t.Fatal("want err: non-nil; got: nil")
				}
				if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("want err: containing %q; got: %q", tt.want, err)
				}
			})
		}
	})

	t.Run("validates the valkey settings when the provider is valkey", func(t *testing.T) {
		cfg := Default().Session
		cfg.StoreProvider = SessionStoreProviderValkey
		cfg.Valkey.Prefix = ""

		err := cfg.validate()

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if want := "TW_SESSION_VALKEY_PREFIX is required"; !strings.Contains(err.Error(), want) {
			t.Errorf("want err: containing %q; got: %q", want, err)
		}
	})

	t.Run("skips the valkey settings when the provider is memory", func(t *testing.T) {
		cfg := Default().Session
		cfg.Valkey.URL = &url.URL{Scheme: "nonsense"}
		cfg.Valkey.Prefix = ""

		if err := cfg.validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})
}

func TestSessionValkeyConfig_validate(t *testing.T) {
	t.Run("accepts the platform url format", func(t *testing.T) {
		cfg := Default().Session.Valkey
		cfg.URL = &url.URL{
			Scheme:   "valkey",
			Host:     "cache.example.com:6379",
			Path:     "/0",
			RawQuery: "tls=true",
		}

		if err := cfg.validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("accepts a url without credentials or tls", func(t *testing.T) {
		cfg := Default().Session.Valkey
		cfg.URL = &url.URL{Scheme: "valkey", Host: "127.0.0.1:6379"}

		if err := cfg.validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("rejects invalid values", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			mutate func(*SessionValkeyConfig)
			want   string
		}{
			{
				name:   "missing url",
				mutate: func(c *SessionValkeyConfig) { c.URL = nil },
				want:   "TW_SESSION_VALKEY_URL is required",
			},
			{
				name:   "non-valkey scheme",
				mutate: func(c *SessionValkeyConfig) { c.URL.Scheme = "rediss" },
				want:   `TW_SESSION_VALKEY_URL must use scheme "valkey"; got "rediss"`,
			},
			{
				name:   "missing port",
				mutate: func(c *SessionValkeyConfig) { c.URL.Host = "cache.example.com" },
				want:   "TW_SESSION_VALKEY_URL must include a port",
			},
			{
				name:   "missing hostname",
				mutate: func(c *SessionValkeyConfig) { c.URL.Host = "" },
				want:   "TW_SESSION_VALKEY_URL must include a hostname",
			},
			{
				name:   "non-boolean tls value",
				mutate: func(c *SessionValkeyConfig) { c.URL.RawQuery = "tls=1" },
				want:   `TW_SESSION_VALKEY_URL "tls" must be "true" or "false"; got "1"`,
			},
			{
				name:   "empty key prefix",
				mutate: func(c *SessionValkeyConfig) { c.Prefix = "" },
				want:   "TW_SESSION_VALKEY_PREFIX is required",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				cfg := Default().Session.Valkey
				cfg.URL = &url.URL{
					Scheme:   "valkey",
					Host:     "cache.example.com:6379",
					Path:     "/0",
					RawQuery: "tls=true",
				}
				tt.mutate(&cfg)

				err := cfg.validate()

				if err == nil {
					t.Fatal("want err: non-nil; got: nil")
				}
				if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("want err: containing %q; got: %q", tt.want, err)
				}
			})
		}
	})

	t.Run("keeps credentials out of the error", func(t *testing.T) {
		// The validation error is logged at startup, so a malformed URL must not
		// carry the password into the logs with it.
		cfg := Default().Session.Valkey
		cfg.URL = &url.URL{Scheme: "valkey", Host: "cache.example.com"}
		cfg.URL.User = url.UserPassword("someone", "s3cret")

		err := cfg.validate()

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		if strings.Contains(err.Error(), "s3cret") {
			t.Errorf("want err: without the password; got: %q", err)
		}
	})
}

func TestAPIProxyConfig_validate(t *testing.T) {
	t.Run("accepts http and https base urls", func(t *testing.T) {
		for _, scheme := range []string{"http", "https"} {
			t.Run(scheme, func(t *testing.T) {
				cfgAPIProxy := Default().APIProxy
				cfgAPIProxy.StudentInsightsBaseURL = &url.URL{Scheme: scheme, Host: "student-insights.example.com"}
				cfgAPIProxy.PostsBaseURL = &url.URL{Scheme: scheme, Host: "posts.example.com"}

				if err := cfgAPIProxy.validate(); err != nil {
					t.Errorf("want err: nil; got: %v", err)
				}
			})
		}
	})

	t.Run("rejects invalid values", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			mutate func(*APIProxyConfig)
			want   string
		}{
			{
				name:   "missing student insights",
				mutate: func(c *APIProxyConfig) { c.StudentInsightsBaseURL = nil },
				want:   "TW_API_PROXY_STUDENT_INSIGHTS_BASE_URL is required",
			},
			{
				name:   "student insights with a non-http scheme",
				mutate: func(c *APIProxyConfig) { c.StudentInsightsBaseURL = &url.URL{Scheme: "ftp", Host: "127.0.0.1:3002"} },
				want:   `TW_API_PROXY_STUDENT_INSIGHTS_BASE_URL must use scheme http or https; got "ftp://127.0.0.1:3002"`,
			},
			{
				name:   "student insights without a host",
				mutate: func(c *APIProxyConfig) { c.StudentInsightsBaseURL = &url.URL{Scheme: "http"} },
				want:   `TW_API_PROXY_STUDENT_INSIGHTS_BASE_URL must include host[:port]; got "http:"`,
			},
			{
				name:   "missing posts",
				mutate: func(c *APIProxyConfig) { c.PostsBaseURL = nil },
				want:   "TW_API_PROXY_POSTS_BASE_URL is required",
			},
			{
				name:   "posts with a non-http scheme",
				mutate: func(c *APIProxyConfig) { c.PostsBaseURL = &url.URL{Scheme: "ftp", Host: "127.0.0.1:3003"} },
				want:   `TW_API_PROXY_POSTS_BASE_URL must use scheme http or https; got "ftp://127.0.0.1:3003"`,
			},
			{
				name:   "posts without a host",
				mutate: func(c *APIProxyConfig) { c.PostsBaseURL = &url.URL{Scheme: "http"} },
				want:   `TW_API_PROXY_POSTS_BASE_URL must include host[:port]; got "http:"`,
			},
			{
				name:   "missing student insights signing key",
				mutate: func(c *APIProxyConfig) { c.StudentInsightsSigningKey = "" },
				want:   "TW_API_PROXY_STUDENT_INSIGHTS_SIGNING_KEY is required",
			},
			{
				name:   "short student insights signing key",
				mutate: func(c *APIProxyConfig) { c.StudentInsightsSigningKey = "a-short-secret" },
				want:   "TW_API_PROXY_STUDENT_INSIGHTS_SIGNING_KEY must be at least 32 bytes; got 14",
			},
			{
				name:   "missing posts signing key",
				mutate: func(c *APIProxyConfig) { c.PostsSigningKey = "" },
				want:   "TW_API_PROXY_POSTS_SIGNING_KEY is required",
			},
			{
				name:   "short posts signing key",
				mutate: func(c *APIProxyConfig) { c.PostsSigningKey = "a-short-secret" },
				want:   "TW_API_PROXY_POSTS_SIGNING_KEY must be at least 32 bytes; got 14",
			},
			{
				name:   "zero token TTL",
				mutate: func(c *APIProxyConfig) { c.TokenTTL = 0 },
				want:   "TW_API_PROXY_TOKEN_TTL must be at least 1s; got 0s",
			},
			{
				name:   "negative token TTL",
				mutate: func(c *APIProxyConfig) { c.TokenTTL = -time.Second },
				want:   "TW_API_PROXY_TOKEN_TTL must be at least 1s; got -1s",
			},
			{
				name:   "sub-second token TTL",
				mutate: func(c *APIProxyConfig) { c.TokenTTL = 500 * time.Millisecond },
				want:   "TW_API_PROXY_TOKEN_TTL must be at least 1s; got 500ms",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				cfg := Default().APIProxy
				tt.mutate(&cfg)

				err := cfg.validate()

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

func TestOIDCConfig_validate(t *testing.T) {
	t.Run("accepts http and https urls", func(t *testing.T) {
		for _, scheme := range []string{"http", "https"} {
			t.Run(scheme, func(t *testing.T) {
				cfg := validOIDCConfig()
				cfg.IssuerURL = &url.URL{Scheme: scheme, Host: "localhost:9000"}
				cfg.AuthURL = &url.URL{Scheme: scheme, Host: "localhost:9000", Path: "/authorize"}
				cfg.TokenURL = &url.URL{Scheme: scheme, Host: "localhost:9000", Path: "/token"}
				cfg.JWKSURI = &url.URL{Scheme: scheme, Host: "localhost:9000", Path: "/jwks"}
				cfg.RedirectURL = &url.URL{Scheme: scheme, Host: "localhost:3000", Path: "/auth/edupass/callback"}

				if err := cfg.validate(); err != nil {
					t.Errorf("want err: nil; got: %v", err)
				}
			})
		}
	})
}
func TestRemoteConfig_validate(t *testing.T) {
	t.Run("accepts empty manifest urls", func(t *testing.T) {
		cfg := RemoteConfig{}

		if err := cfg.validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("accepts http and https manifest urls", func(t *testing.T) {
		for _, scheme := range []string{"http", "https"} {
			t.Run(scheme, func(t *testing.T) {
				cfg := RemoteConfig{
					PostsManifestURL:           scheme + "://pg.test/mf-manifest.json",
					StudentInsightsManifestURL: scheme + "://si.test/mf-manifest.json",
				}

				if err := cfg.validate(); err != nil {
					t.Errorf("want err: nil; got: %v", err)
				}
			})
		}
	})

	t.Run("rejects invalid values", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			mutate func(*OIDCConfig)
			want   string
		}{
			{
				name:   "missing issuer url",
				mutate: func(c *OIDCConfig) { c.IssuerURL = nil },
				want:   "TW_OIDC_ISSUER_URL is required",
			},
			{
				name:   "issuer url with a non-http scheme",
				mutate: func(c *OIDCConfig) { c.IssuerURL = &url.URL{Scheme: "ftp", Host: "localhost:9000"} },
				want:   "TW_OIDC_ISSUER_URL must use scheme http or https",
			},
			{
				name:   "issuer url without a host",
				mutate: func(c *OIDCConfig) { c.IssuerURL = &url.URL{Scheme: "http"} },
				want:   "TW_OIDC_ISSUER_URL must include host",
			},
			{
				name:   "missing auth url",
				mutate: func(c *OIDCConfig) { c.AuthURL = nil },
				want:   "TW_OIDC_AUTH_URL is required",
			},
			{
				name:   "auth url with a non-http scheme",
				mutate: func(c *OIDCConfig) { c.AuthURL = &url.URL{Scheme: "ftp", Host: "localhost:9000"} },
				want:   "TW_OIDC_AUTH_URL must use scheme http or https",
			},
			{
				name:   "auth url without a host",
				mutate: func(c *OIDCConfig) { c.AuthURL = &url.URL{Scheme: "http"} },
				want:   "TW_OIDC_AUTH_URL must include host",
			},
			{
				name:   "missing token url",
				mutate: func(c *OIDCConfig) { c.TokenURL = nil },
				want:   "TW_OIDC_TOKEN_URL is required",
			},
			{
				name:   "token url with a non-http scheme",
				mutate: func(c *OIDCConfig) { c.TokenURL = &url.URL{Scheme: "ftp", Host: "localhost:9000"} },
				want:   "TW_OIDC_TOKEN_URL must use scheme http or https",
			},
			{
				name:   "token url without a host",
				mutate: func(c *OIDCConfig) { c.TokenURL = &url.URL{Scheme: "http"} },
				want:   "TW_OIDC_TOKEN_URL must include host",
			},
			{
				name:   "missing jwks uri",
				mutate: func(c *OIDCConfig) { c.JWKSURI = nil },
				want:   "TW_OIDC_JWKS_URI is required",
			},
			{
				name:   "jwks uri with a non-http scheme",
				mutate: func(c *OIDCConfig) { c.JWKSURI = &url.URL{Scheme: "ftp", Host: "localhost:9000"} },
				want:   "TW_OIDC_JWKS_URI must use scheme http or https",
			},
			{
				name:   "jwks uri without a host",
				mutate: func(c *OIDCConfig) { c.JWKSURI = &url.URL{Scheme: "http"} },
				want:   "TW_OIDC_JWKS_URI must include host",
			},
			{
				name:   "empty client id",
				mutate: func(c *OIDCConfig) { c.ClientID = "" },
				want:   "TW_OIDC_CLIENT_ID is required",
			},
			{
				name:   "empty client secret",
				mutate: func(c *OIDCConfig) { c.ClientSecret = "" },
				want:   "TW_OIDC_CLIENT_SECRET is required",
			},
			{
				name:   "missing redirect url",
				mutate: func(c *OIDCConfig) { c.RedirectURL = nil },
				want:   "TW_OIDC_REDIRECT_URL is required",
			},
			{
				name:   "redirect url with a non-http scheme",
				mutate: func(c *OIDCConfig) { c.RedirectURL = &url.URL{Scheme: "ftp", Host: "localhost:3000"} },
				want:   "TW_OIDC_REDIRECT_URL must use scheme http or https",
			},
			{
				name:   "redirect url without a host",
				mutate: func(c *OIDCConfig) { c.RedirectURL = &url.URL{Scheme: "http"} },
				want:   "TW_OIDC_REDIRECT_URL must include host",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				cfg := validOIDCConfig()
				tt.mutate(&cfg)

				err := cfg.validate()

				if err == nil {
					t.Fatal("want err: non-nil; got: nil")
				}
				if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("want err: containing %q; got: %q", tt.want, err)
				}
			})
		}
	})

	t.Run("rejects invalid values", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			mutate func(*RemoteConfig)
			want   string
		}{
			{
				name:   "unparseable posts url",
				mutate: func(c *RemoteConfig) { c.PostsManifestURL = "http://bad host/mf-manifest.json" },
				want:   `TW_REMOTE_POSTS_MURL must be a valid url; got "http://bad host/mf-manifest.json"`,
			},
			{
				name:   "posts url with a non-http scheme",
				mutate: func(c *RemoteConfig) { c.PostsManifestURL = "ftp://pg.test/mf-manifest.json" },
				want:   `TW_REMOTE_POSTS_MURL must use scheme http or https; got "ftp://pg.test/mf-manifest.json"`,
			},
			{
				name:   "posts url without a host",
				mutate: func(c *RemoteConfig) { c.PostsManifestURL = "https:///mf-manifest.json" },
				want:   `TW_REMOTE_POSTS_MURL must include host[:port]; got "https:///mf-manifest.json"`,
			},
			{
				name:   "unparseable student insights url",
				mutate: func(c *RemoteConfig) { c.StudentInsightsManifestURL = "http://bad host/mf-manifest.json" },
				want:   `TW_REMOTE_STUDENT_INSIGHTS_MURL must be a valid url; got "http://bad host/mf-manifest.json"`,
			},
			{
				name:   "student insights url with a non-http scheme",
				mutate: func(c *RemoteConfig) { c.StudentInsightsManifestURL = "ftp://si.test/mf-manifest.json" },
				want:   `TW_REMOTE_STUDENT_INSIGHTS_MURL must use scheme http or https; got "ftp://si.test/mf-manifest.json"`,
			},
			{
				name:   "student insights url without a host",
				mutate: func(c *RemoteConfig) { c.StudentInsightsManifestURL = "https:///mf-manifest.json" },
				want:   `TW_REMOTE_STUDENT_INSIGHTS_MURL must include host[:port]; got "https:///mf-manifest.json"`,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				cfg := RemoteConfig{
					PostsManifestURL:           "https://pg.test/mf-manifest.json",
					StudentInsightsManifestURL: "https://si.test/mf-manifest.json",
				}
				tt.mutate(&cfg)

				err := cfg.validate()

				if err == nil {
					t.Fatal("want err: non-nil; got: nil")
				}
				if !strings.Contains(err.Error(), tt.want) {
					t.Errorf("want err: containing %q; got: %q", tt.want, err)
				}
			})
		}
	})

	t.Run("reports both invalid manifest urls in one error", func(t *testing.T) {
		cfg := RemoteConfig{
			PostsManifestURL:           "ftp://pg.test/mf-manifest.json",
			StudentInsightsManifestURL: "https:///mf-manifest.json",
		}

		err := cfg.validate()

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		for _, want := range []string{"TW_REMOTE_POSTS_MURL", "TW_REMOTE_STUDENT_INSIGHTS_MURL"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("want err: containing %q; got: %q", want, err)
			}
		}
	})
}
