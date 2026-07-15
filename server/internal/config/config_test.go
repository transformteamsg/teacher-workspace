package config_test

import (
	"log/slog"
	"net/url"
	"testing"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/pkg/require"
)

func TestDefault(t *testing.T) {
	cfg := config.Default()

	require.Equal(t, config.EnvDevelopment, cfg.Env)
	require.Equal(t, slog.LevelInfo, cfg.LogLevel)
	require.Equal(t, 3000, cfg.Server.Port)
	require.Equal(t, 2*time.Second, cfg.Server.ReadHeaderTimeout)
	require.Equal(t, 15*time.Second, cfg.Server.ReadTimeout)
	require.Equal(t, 30*time.Second, cfg.Server.WriteTimeout)
	require.Equal(t, 60*time.Second, cfg.Server.IdleTimeout)
	require.Equal(t, "http://127.0.0.1:3001", cfg.DevServerURL.String())
	require.Equal(t, "apps/host/dist", cfg.BuildDir)
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{
			name: "invalid env",
			mutate: func(c *config.Config) {
				c.Env = "staging"
			},
		},
		{
			name: "port too high",
			mutate: func(c *config.Config) {
				c.Server.Port = 70000
			},
		},
		{
			name: "port too low",
			mutate: func(c *config.Config) {
				c.Server.Port = 0
			},
		},
		{
			name: "negative read timeout",
			mutate: func(c *config.Config) {
				c.Server.ReadTimeout = -1
			},
		},
		{
			name: "dev server url missing scheme",
			mutate: func(c *config.Config) {
				c.DevServerURL = &url.URL{Host: "localhost:3001"}
			},
		},
		{
			name: "dev server url missing host",
			mutate: func(c *config.Config) {
				c.DevServerURL = &url.URL{Scheme: "http"}
			},
		},
		{
			name: "production build dir empty",
			mutate: func(c *config.Config) {
				c.Env = config.EnvProduction
				c.BuildDir = ""
			},
		},
		{
			name: "production build dir does not exist",
			mutate: func(c *config.Config) {
				c.Env = config.EnvProduction
				c.BuildDir = "/nonexistent/does-not-exist"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			tc.mutate(&cfg)

			require.HasError(t, cfg.Validate())
		})
	}
}
