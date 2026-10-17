// Command warehouse runs the warehouse service: the HTTP API, the outbox relay
// that publishes stock and delivery events, and the consumer that reserves
// stock for confirmed orders.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/yugadn/salesflow/pkg/config"
	"github.com/yugadn/salesflow/pkg/db"
	"github.com/yugadn/salesflow/pkg/httpx"
	"github.com/yugadn/salesflow/pkg/logger"
	"github.com/yugadn/salesflow/pkg/outbox"
	"github.com/yugadn/salesflow/pkg/rabbit"
	"github.com/yugadn/salesflow/services/warehouse/internal/handler"
	"github.com/yugadn/salesflow/services/warehouse/internal/repository"
	"github.com/yugadn/salesflow/services/warehouse/internal/usecase"
	"github.com/yugadn/salesflow/services/warehouse/migrations"
)

const queue = "warehouse-service"

func main() {
	addr := config.String("HTTP_ADDR", ":8083")
	httpx.HealthcheckIfRequested(addr)

	log := logger.New("warehouse-service")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := config.MustString("DATABASE_URL")
	amqpURL := config.MustString("RABBITMQ_URL")

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
