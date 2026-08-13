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
		if want, got := "valkey://127.0.0.1:6379", cfg.Session.Valkey.URL.String(); want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := "session:", cfg.Session.Valkey.Prefix; want != got {
			t.Errorf("want: %q; got: %q", want, got)
		}
		if want, got := 5*time.Second, cfg.Session.Valkey.DialTimeout; want != got {
			t.Errorf("want: %v; got: %v", want, got)
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

func TestConfig_Validate(t *testing.T) {
	t.Run("accepts http and https dev server urls", func(t *testing.T) {
		for _, scheme := range []string{"http", "https"} {
			t.Run(scheme, func(t *testing.T) {
				cfg := Default()
				cfg.DevServerURL = &url.URL{Scheme: scheme, Host: "127.0.0.1:3001"}

				if err := cfg.Validate(); err != nil {
					t.Errorf("want err: nil; got: %v", err)
				}
			})
		}
	})

	t.Run("accepts production pointing at an existing build dir", func(t *testing.T) {
		cfg := Default()
		cfg.Env = EnvProduction
		cfg.BuildDir = t.TempDir()

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
		cfg.Env = EnvProduction
		cfg.BuildDir = t.TempDir()
		cfg.DevServerURL = &url.URL{}

		if err := cfg.Validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("skips the build dir outside production", func(t *testing.T) {
		cfg := Default()
		cfg.BuildDir = "testdata/does-not-exist"

		if err := cfg.Validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("reports multiple invalid fields in one error", func(t *testing.T) {
		cfg := Default()
		cfg.Env = "staging"
		cfg.Server.Port = 0
		cfg.Session.Name = ""
		cfg.APIProxy.PostsBaseURL = nil

		err := cfg.Validate()

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		for _, want := range []string{"TW_ENV", "TW_SERVER_PORT", "TW_SESSION_NAME", "TW_API_PROXY_POSTS_BASE_URL"} {
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

	t.Run("accepts the valkey provider with the platform url format", func(t *testing.T) {
		cfg := Default().Session
		cfg.StoreProvider = SessionStoreProviderValkey
		cfg.Valkey.URL = &url.URL{
			Scheme:   "valkey",
			Host:     "cache.example.com:6379",
			Path:     "/0",
			RawQuery: "tls=true",
		}

		if err := cfg.validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("accepts the valkey provider without credentials or tls", func(t *testing.T) {
		cfg := Default().Session
		cfg.StoreProvider = SessionStoreProviderValkey
		cfg.Valkey.URL = &url.URL{Scheme: "valkey", Host: "127.0.0.1:6379"}

		if err := cfg.validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("skips the valkey settings when the provider is memory", func(t *testing.T) {
		cfg := Default().Session
		cfg.Valkey.URL = &url.URL{Scheme: "nonsense"}
		cfg.Valkey.Prefix = ""
		cfg.Valkey.DialTimeout = 0

		if err := cfg.validate(); err != nil {
			t.Errorf("want err: nil; got: %v", err)
		}
	})

	t.Run("rejects invalid valkey settings", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			mutate func(*SessionConfig)
			want   string
		}{
			{
				name:   "non-valkey scheme",
				mutate: func(c *SessionConfig) { c.Valkey.URL.Scheme = "rediss" },
				want:   `TW_SESSION_VALKEY_URL must use scheme "valkey"; got "rediss"`,
			},
			{
				name:   "missing port",
				mutate: func(c *SessionConfig) { c.Valkey.URL.Host = "cache.example.com" },
				want:   "TW_SESSION_VALKEY_URL must include a port",
			},
			{
				name:   "missing hostname",
				mutate: func(c *SessionConfig) { c.Valkey.URL.Host = "" },
				want:   "TW_SESSION_VALKEY_URL must include a hostname",
			},
			{
				name:   "non-boolean tls value",
				mutate: func(c *SessionConfig) { c.Valkey.URL.RawQuery = "tls=1" },
				want:   `TW_SESSION_VALKEY_URL "tls" must be "true" or "false"; got "1"`,
			},
			{
				name:   "zero dial timeout",
				mutate: func(c *SessionConfig) { c.Valkey.DialTimeout = 0 },
				want:   "TW_SESSION_VALKEY_DIAL_TIMEOUT must be positive",
			},
			{
				name:   "empty key prefix",
				mutate: func(c *SessionConfig) { c.Valkey.Prefix = "" },
				want:   "TW_SESSION_VALKEY_PREFIX is required",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				cfg := Default().Session
				cfg.StoreProvider = SessionStoreProviderValkey
				cfg.Valkey.URL = &url.URL{
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

	t.Run("keeps valkey credentials out of the error", func(t *testing.T) {
		// The validation error is logged at startup, so a malformed URL must not
		// carry the password into the logs with it.
		cfg := Default().Session
		cfg.StoreProvider = SessionStoreProviderValkey
		cfg.Valkey.URL = &url.URL{Scheme: "valkey", Host: "cache.example.com"}
		cfg.Valkey.URL.User = url.UserPassword("someone", "s3cret")

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
