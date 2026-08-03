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
	})
}

func TestConfig_Validate(t *testing.T) {
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

		err := cfg.Validate()

		if err == nil {
			t.Fatal("want err: non-nil; got: nil")
		}
		for _, want := range []string{"TW_ENV", "TW_SERVER_PORT", "TW_SESSION_NAME"} {
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
}
