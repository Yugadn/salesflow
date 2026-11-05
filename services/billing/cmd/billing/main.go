// Command billing runs the billing service: it consumes delivery.completed,
// issues invoices, and publishes invoice.created through its outbox.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // embed zone data so Asia/Jakarta resolves in minimal images

	"golang.org/x/sync/errgroup"

	"github.com/yugadn/salesflow/pkg/auth"
	"github.com/yugadn/salesflow/pkg/config"
	"github.com/yugadn/salesflow/pkg/db"
	"github.com/yugadn/salesflow/pkg/httpx"
	"github.com/yugadn/salesflow/pkg/logger"
	"github.com/yugadn/salesflow/pkg/outbox"
	"github.com/yugadn/salesflow/pkg/rabbit"
	"github.com/yugadn/salesflow/services/billing/internal/domain"
	"github.com/yugadn/salesflow/services/billing/internal/handler"
	"github.com/yugadn/salesflow/services/billing/internal/repository"
	"github.com/yugadn/salesflow/services/billing/internal/usecase"
	"github.com/yugadn/salesflow/services/billing/migrations"
)

const queue = "billing-service"

func main() {
	addr := config.String("HTTP_ADDR", ":8084")
	httpx.HealthcheckIfRequested(addr)

	log := logger.New("billing-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := config.MustString("DATABASE_URL")
	amqpURL := config.MustString("RABBITMQ_URL")

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

	svc := usecase.NewService(repository.NewPostgres(pool), domain.MustLoadLocation(config.String("BUSINESS_TIMEZONE", "Asia/Jakarta")))

	publisher := rabbit.NewPublisher(amqpURL)
	defer publisher.Close()
	relay := outbox.NewRelay(pool, publisher, log, config.Duration("OUTBOX_INTERVAL", 0))
	consumer := rabbit.NewConsumer(rabbit.ConsumerConfig{
		URL:         amqpURL,
		Queue:       queue,
		RoutingKeys: usecase.SubscribedEvents,
		RetryDelay:  config.Duration("RETRY_DELAY", 0),
	}, handler.Events(svc), log)

	e := httpx.NewServer(log)
	httpx.RegisterHealth(e, pool)
	e.Use(auth.Middleware(auth.NewVerifier(config.MustString("JWKS_URL"))))
	handler.New(svc).Register(e)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { relay.Run(ctx); return nil })
	g.Go(func() error { return consumer.Run(ctx) })
	g.Go(func() error {
		log.Info("listening", "addr", addr)
		return httpx.Serve(ctx, e, addr)
	})
	if err := g.Wait(); err != nil {
		log.Error("service stopped with error", "err", err)
		os.Exit(1)
	}
	log.Info("stopped")
}
