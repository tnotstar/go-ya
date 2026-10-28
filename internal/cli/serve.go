// Package cli defines all CLI command implementations for the gateway binary.
// Commands call the same application services used by the HTTP server,
// ensuring no duplicated business logic.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/tnotstar/go-ya/internal/config"
	"github.com/tnotstar/go-ya/internal/persistence"
	"github.com/tnotstar/go-ya/internal/providers"
	"github.com/tnotstar/go-ya/internal/server"
	"github.com/tnotstar/go-ya/internal/services"
	gormlog "gorm.io/gorm/logger"
)

// ServeCommand returns the `serve` subcommand that starts the AI gateway.
func ServeCommand() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Start the AI Gateway server",
		Action: func(c *cli.Context) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("serve: config: %w", err)
			}

			masterKey, err := cfg.MasterKeyBytes()
			if err != nil {
				return err
			}

			// Configure logger.
			var handler slog.Handler
			if cfg.LogFormat == "json" {
				handler = slog.NewJSONHandler(os.Stdout, nil)
			} else {
				handler = slog.NewTextHandler(os.Stdout, nil)
			}
			logger := slog.New(handler)
			slog.SetDefault(logger)

			// Open database.
			logLevel := gormlog.Warn
			if cfg.LogLevel == "debug" {
				logLevel = gormlog.Info
			}
			db, err := persistence.Open(cfg.DBDriver, cfg.DBDSN, logLevel)
			if err != nil {
				return fmt.Errorf("serve: db: %w", err)
			}

			// Build service graph.
			provRepo := persistence.NewProviderRepo(db)
			keyRepo := persistence.NewKeyRepo(db)
			sessionRepo := persistence.NewSessionRepo(db)
			messageRepo := persistence.NewMessageRepo(db)

			keyStore := services.NewKeyStore(keyRepo, masterKey)
			sessionSvc := services.NewSessionService(sessionRepo, messageRepo)
			registry := providers.NewRegistry(cfg)
			routerSvc := services.NewRouterService(provRepo, keyStore, registry, cfg.DefaultProvider, cfg.MaxRetries)

			svc := &server.Services{
				Router:   routerSvc,
				Session:  sessionSvc,
				KeyStore: keyStore,
				ProvRepo: provRepo,
				Registry: registry,
			}

			e := server.New(cfg, svc, logger)

			// Graceful shutdown.
			addr := fmt.Sprintf(":%d", cfg.Port)
			logger.Info("gateway starting", slog.String("addr", addr))

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			go func() {
				if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
					logger.Error("server error", slog.String("error", err.Error()))
				}
			}()

			<-ctx.Done()
			logger.Info("shutdown signal received")

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return e.Shutdown(shutdownCtx)
		},
	}
}
