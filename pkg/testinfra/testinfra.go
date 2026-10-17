//go:build integration

package testinfra

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcrabbit "github.com/testcontainers/testcontainers-go/modules/rabbitmq"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Postgres starts a PostgreSQL container and returns its connection URL.
// The container is removed when the test ends.
func Postgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	c, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })

	url, err := c.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	return url
}

// RabbitMQ starts a RabbitMQ container and returns its AMQP URL.
func RabbitMQ(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	c, err := tcrabbit.Run(ctx, "rabbitmq:4-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(c) })

	url, err := c.AmqpURL(ctx)
	require.NoError(t, err)
	return url
}
