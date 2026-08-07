package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/handler"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/String-sg/teacher-workspace/server/internal/session"
	"github.com/String-sg/teacher-workspace/server/internal/session/memstore"
	"github.com/String-sg/teacher-workspace/server/internal/session/valkeystore"
	"github.com/String-sg/teacher-workspace/server/pkg/dotenv"
)

const shutdownTimeout = 30 * time.Second

func main() {
	cfg := config.Default()
	if err := dotenv.Load(&cfg); err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid config", "err", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	})))

	store, closeStore, err := newSessionStore(&cfg)
	if err != nil {
		slog.Error("failed to create session store", "provider", cfg.Session.StoreProvider, "err", err)
		os.Exit(1)
	}
	defer closeStore()

	sessionMiddleware := middleware.Session(store, middleware.SessionOptions{
		Name:             cfg.Session.Name,
		DefaultTTL:       cfg.Session.DefaultTTL,
		AuthenticatedTTL: cfg.Session.AuthenticatedTTL,
		Secure:           cfg.Env == config.EnvProduction,
	})

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	mux := http.NewServeMux()
	handler.New(&cfg).Register(mux, sessionMiddleware)
	srv := &http.Server{
		Addr:              addr,
		Handler:           middleware.RequestID(middleware.RequestLog(mux)),
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("listening", "addr", addr, "env", cfg.Env, "session_store", cfg.Session.StoreProvider)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown failed", "err", err)
		os.Exit(1)
	}
}

func newSessionStore(cfg *config.Config) (session.Store, func(), error) {
	switch cfg.Session.StoreProvider {
	case config.StoreProviderValkey:
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Session.ValkeyDialTimeout)
		defer cancel()

		client, err := valkeystore.Dial(ctx, cfg.Session.ValkeyURL)
		if err != nil {
			return nil, nil, err
		}

		return valkeystore.New(client, valkeystore.WithPrefix(cfg.Session.ValkeyPrefix)), client.Close, nil
	default:
		// Validate rejects any provider other than memory or valkey, so this
		// arm is only ever reached for memory.
		return memstore.New(), func() {}, nil
	}
}
