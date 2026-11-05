// Command master runs the master-data service: regions, customers,
// salespersons, products, UoM conversions and regional price lists.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/yugadn/salesflow/pkg/auth"
	"github.com/yugadn/salesflow/pkg/config"
	"github.com/yugadn/salesflow/pkg/db"
	"github.com/yugadn/salesflow/pkg/httpx"
	"github.com/yugadn/salesflow/pkg/logger"
	"github.com/yugadn/salesflow/services/master/internal/handler"
	"github.com/yugadn/salesflow/services/master/internal/repository"
	"github.com/yugadn/salesflow/services/master/internal/usecase"
	"github.com/yugadn/salesflow/services/master/migrations"
)

func main() {
	addr := config.String("HTTP_ADDR", ":8081")
	httpx.HealthcheckIfRequested(addr)

	log := logger.New("master-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := config.MustString("DATABASE_URL")

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

	svc := usecase.NewService(repository.NewPostgres(pool))
	e := httpx.NewServer(log)
	httpx.RegisterHealth(e, pool)
	e.Use(auth.Middleware(auth.NewVerifier(config.MustString("JWKS_URL"))))
	handler.New(svc).Register(e)

	log.Info("listening", "addr", addr)
	if err := httpx.Serve(ctx, e, addr); err != nil {
		log.Error("http server", "err", err)
		os.Exit(1)
	}
	log.Info("stopped")
}
