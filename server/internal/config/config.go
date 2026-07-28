package config

import (
	"errors"
	"fmt"
	"log/slog"
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

	Server  ServerConfig  `dotenv:",squash"`
	Session SessionConfig `dotenv:",squash"`
}

// ServerConfig represents the configuration for the HTTP server.
type ServerConfig struct {
	Port              int           `dotenv:"TW_SERVER_PORT"`
	ReadHeaderTimeout time.Duration `dotenv:"TW_SERVER_READ_HEADER_TIMEOUT"`
	ReadTimeout       time.Duration `dotenv:"TW_SERVER_READ_TIMEOUT"`
	WriteTimeout      time.Duration `dotenv:"TW_SERVER_WRITE_TIMEOUT"`
	IdleTimeout       time.Duration `dotenv:"TW_SERVER_IDLE_TIMEOUT"`
}

// SessionConfig represents the configuration for the session.
type SessionConfig struct {
	Name             string        `dotenv:"TW_SESSION_NAME"`
	DefaultTTL       time.Duration `dotenv:"TW_SESSION_DEFAULT_TTL"`
	AuthenticatedTTL time.Duration `dotenv:"TW_SESSION_AUTHENTICATED_TTL"`
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
			Name:             "tw_session",
			DefaultTTL:       3 * time.Hour,
			AuthenticatedTTL: 30 * time.Minute,
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
		if c.DevServerURL.Scheme != "http" && c.DevServerURL.Scheme != "https" {
			errs = append(errs, fmt.Errorf("TW_DEV_SERVER_URL must use scheme http or https; got %q", c.DevServerURL))
		}
		if c.DevServerURL.Host == "" {
			errs = append(errs, fmt.Errorf("TW_DEV_SERVER_URL must include host[:port]; got %q", c.DevServerURL))
		}
	case EnvProduction:
		if c.BuildDir == "" {
			errs = append(errs, errors.New("TW_BUILD_DIR is required"))
		} else if _, err := os.Stat(c.BuildDir); os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("TW_BUILD_DIR does not exist: %q", c.BuildDir))
		}
	}

	return errors.Join(append(errs, c.Server.validate(), c.Session.validate())...)
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

	if c.Name == "" {
		errs = append(errs, errors.New("TW_SESSION_NAME is required"))
	}
	if c.DefaultTTL <= 0 {
		errs = append(errs, fmt.Errorf("TW_SESSION_DEFAULT_TTL must be positive duration; got %v", c.DefaultTTL))
	}
	if c.AuthenticatedTTL <= 0 {
		errs = append(errs, fmt.Errorf("TW_SESSION_AUTHENTICATED_TTL must be positive duration; got %v", c.AuthenticatedTTL))
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
