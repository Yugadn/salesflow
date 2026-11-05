// Command auth runs the auth service: sign-in, RS256 token issuing, the
// public JWKS every verifier reads, and user management.
package main

import (
	"context"
	"crypto/rsa"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yugadn/salesflow/pkg/auth"
	"github.com/yugadn/salesflow/pkg/config"
	"github.com/yugadn/salesflow/pkg/db"
	"github.com/yugadn/salesflow/pkg/httpx"
	"github.com/yugadn/salesflow/pkg/logger"
	"github.com/yugadn/salesflow/services/auth/internal/handler"
	"github.com/yugadn/salesflow/services/auth/internal/repository"
	"github.com/yugadn/salesflow/services/auth/internal/usecase"
	"github.com/yugadn/salesflow/services/auth/migrations"
)

func main() {
	addr := config.String("HTTP_ADDR", ":8085")
	httpx.HealthcheckIfRequested(addr)

	log := logger.New("auth-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := config.MustString("DATABASE_URL")
	adminEmail := config.MustString("ADMIN_EMAIL")
	adminPassword := config.MustString("ADMIN_PASSWORD")
	tokenTTL := config.Duration("TOKEN_TTL", 8*time.Hour)

	if err := db.EnsureDatabase(ctx, dbURL); err != nil {
		log.Error("ensure database", "err", err)
		os.Exit(1)
	}
	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		log.Error("connect database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := db.Migrate(dbURL, migrations.FS); err != nil {
		log.Error("migrate database", "err", err)
		os.Exit(1)
	}

	repo := repository.NewPostgres(pool)
	keys, err := repo.LoadKeys(ctx)
	if err != nil {
		log.Error("load signing keys", "err", err)
		os.Exit(1)
	}
	svc := usecase.NewService(repo, keys, tokenTTL)
	created, err := svc.EnsureAdmin(ctx, adminEmail, adminPassword)
	if err != nil {
		log.Error("create first admin", "err", err)
		os.Exit(1)
	}
	if created {
		log.Info("created first admin user", "email", adminEmail)
	}

	// auth-service verifies its own tokens from memory instead of fetching its own JWKS.
	local := auth.NewVerifierWithKeys(func(_ context.Context, kid string) (*rsa.PublicKey, error) {
		for _, k := range keys.Public().Keys {
			if k.Kid == kid {
				return k.PublicKey()
			}
		}
		return nil, fmt.Errorf("unknown signing key %q", kid)
	})

	e := httpx.NewServer(log)
	httpx.RegisterHealth(e, pool)
	handler.New(svc, local).Register(e)

	log.Info("listening", "addr", addr)
	if err := httpx.Serve(ctx, e, addr); err != nil {
		log.Error("http server", "err", err)
		os.Exit(1)
	}
	log.Info("stopped")
}
