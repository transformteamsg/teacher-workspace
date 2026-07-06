package config_test

import (
	"log/slog"
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			tc.mutate(&cfg)

			require.HasError(t, cfg.Validate())
		})
	}
}
