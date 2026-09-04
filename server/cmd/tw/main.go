package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	glide "github.com/valkey-io/valkey-glide/go/v2"
	glideconfig "github.com/valkey-io/valkey-glide/go/v2/config"

	"github.com/String-sg/teacher-workspace/server/internal/config"
	"github.com/String-sg/teacher-workspace/server/internal/handler"
	"github.com/String-sg/teacher-workspace/server/internal/middleware"
	"github.com/String-sg/teacher-workspace/server/internal/oidc"
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

	var store session.Store
	switch cfg.Session.StoreProvider {
	case config.SessionStoreProviderValkey:
		host := cfg.Session.Valkey.URL.Hostname()
		port, err := strconv.Atoi(cfg.Session.Valkey.URL.Port())
		if err != nil {
			slog.Error("failed to parse valkey port", "port", cfg.Session.Valkey.URL.Port(), "err", err)
			os.Exit(1)
		}

		vcfg := glideconfig.NewClientConfiguration().
			WithAddress(&glideconfig.NodeAddress{Host: host, Port: port}).
			WithUseTLS(cfg.Session.Valkey.URL.Query().Get("tls") == "true")

		if cfg.Session.Valkey.URL.User != nil {
			username := cfg.Session.Valkey.URL.User.Username()
			password, _ := cfg.Session.Valkey.URL.User.Password()
			vcfg = vcfg.WithCredentials(glideconfig.NewServerCredentials(username, password))
		}

		client, err := glide.NewClient(vcfg)
		if err != nil {
			slog.Error("failed to create valkey client", "err", err)
			os.Exit(1)
		}
		defer client.Close()

		store = valkeystore.New(client, valkeystore.WithPrefix(cfg.Session.Valkey.Prefix))
	case config.SessionStoreProviderMemory:
		store = memstore.New()
	default:
		slog.Error("unsupported session store provider", "provider", cfg.Session.StoreProvider)
		os.Exit(1)
	}

	sessionMiddleware := middleware.Session(store, middleware.SessionOptions{
		Name:             cfg.Session.Name,
		DefaultTTL:       cfg.Session.DefaultTTL,
		AuthenticatedTTL: cfg.Session.AuthenticatedTTL,
		Secure:           cfg.Env == config.EnvProduction,
	})

	rp, err := oidc.New(context.Background(), cfg.OIDC.IssuerURL.String(), cfg.OIDC.ClientID, cfg.OIDC.ClientSecret, cfg.OIDC.RedirectURL.String())
	if err != nil {
		slog.Error("failed to initialize OIDC relying party", "err", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	mux := http.NewServeMux()
	handler.New(&cfg, rp).Register(mux, sessionMiddleware)
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
