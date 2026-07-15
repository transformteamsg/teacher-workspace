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

// Config is the server's configuration. Host-serving fields (DevServerURL,
// BuildDir) live directly on Config, not a nested struct, since this server
// only ever serves the one apps/host frontend.
type Config struct {
	Env      Environment  `dotenv:"TW_ENV"`
	LogLevel slog.Level   `dotenv:"TW_LOG_LEVEL"`
	Server   ServerConfig `dotenv:",squash"`

	// DevServerURL is where the Go server proxies to in development.
	DevServerURL *url.URL `dotenv:"TW_HOST_DEV_SERVER_URL"`
	// BuildDir is where the Go server reads apps/host's build output from in production.
	BuildDir string `dotenv:"TW_HOST_BUILD_DIR"`
}

type ServerConfig struct {
	Port              int           `dotenv:"TW_SERVER_PORT"`
	ReadHeaderTimeout time.Duration `dotenv:"TW_SERVER_READ_HEADER_TIMEOUT"`
	ReadTimeout       time.Duration `dotenv:"TW_SERVER_READ_TIMEOUT"`
	WriteTimeout      time.Duration `dotenv:"TW_SERVER_WRITE_TIMEOUT"`
	IdleTimeout       time.Duration `dotenv:"TW_SERVER_IDLE_TIMEOUT"`
}

// Default returns a Config populated with built-in defaults.
func Default() Config {
	return Config{
		Env:      EnvDevelopment,
		LogLevel: slog.LevelInfo,
		Server: ServerConfig{
			Port:              3000,
			ReadHeaderTimeout: 2 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		DevServerURL: must(url.Parse("http://127.0.0.1:3001")),
		BuildDir:     "apps/host/dist",
	}
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func (c Config) Validate() error {
	var errs []error

	if c.Env != EnvDevelopment && c.Env != EnvProduction {
		errs = append(errs, fmt.Errorf("TW_ENV must be %q or %q; got %q", EnvDevelopment, EnvProduction, c.Env))
	}

	switch c.Env {
	case EnvDevelopment:
		if c.DevServerURL.Scheme != "http" && c.DevServerURL.Scheme != "https" {
			errs = append(errs, fmt.Errorf("TW_HOST_DEV_SERVER_URL must use scheme http or https; got %q", c.DevServerURL))
		}
		if c.DevServerURL.Host == "" {
			errs = append(errs, fmt.Errorf("TW_HOST_DEV_SERVER_URL must include host[:port]; got %q", c.DevServerURL))
		}
	case EnvProduction:
		if c.BuildDir == "" {
			errs = append(errs, errors.New("TW_HOST_BUILD_DIR is required"))
		} else if _, err := os.Stat(c.BuildDir); os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("TW_HOST_BUILD_DIR does not exist: %q", c.BuildDir))
		}
	}

	return errors.Join(append(errs, c.Server.validate())...)
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
