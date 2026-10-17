//go:build integration

package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/db"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/pkg/outbox"
	"github.com/yugadn/salesflow/pkg/rabbit"
	"github.com/yugadn/salesflow/pkg/testinfra"
)

// TestRelay_PublishesOnlyCommittedEventsInOrder is the reason the outbox
// exists: an event whose transaction rolled back must never reach the broker,
// and committed events must arrive in the order they were written.
func TestRelay_PublishesOnlyCommittedEventsInOrder(t *testing.T) {
	ctx := context.Background()
	pool, err := db.Connect(ctx, testinfra.Postgres(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, outbox.Schema)
	require.NoError(t, err)

	amqpURL := testinfra.RabbitMQ(t)
	deliveries := bindTestQueue(t, amqpURL, "order.confirmed")

	write := func(n int) events.Envelope {
		env, err := events.New("test", events.OrderConfirmed, map[string]int{"n": n})
		require.NoError(t, err)
		return env
	}
	committed := []events.Envelope{write(1), write(2), write(3)}
	for _, env := range committed {
		require.NoError(t, db.InTx(ctx, pool, func(tx pgx.Tx) error { return outbox.Write(ctx, tx, env) }))
	}
	errBoom := errors.New("business rule failed after writing the event")
	err = db.InTx(ctx, pool, func(tx pgx.Tx) error {
		require.NoError(t, outbox.Write(ctx, tx, write(99)))
		return errBoom
	})
	require.ErrorIs(t, err, errBoom)

	pub := rabbit.NewPublisher(amqpURL)
	t.Cleanup(pub.Close)
	relay := outbox.NewRelay(pool, pub, slog.New(slog.NewTextHandler(io.Discard, nil)), 0)
	n, err := relay.PublishBatch(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	for _, want := range committed {
		select {
		case d := <-deliveries:
			assert.Equal(t, want.ID, d.MessageId)
			assert.Equal(t, "order.confirmed", d.RoutingKey)
		case <-time.After(5 * time.Second):
			t.Fatalf("event %s was not delivered", want.ID)
		}
	}
	select {
	case d := <-deliveries:
		t.Fatalf("unexpected extra message %s (the rolled-back event leaked?)", d.MessageId)
	case <-time.After(500 * time.Millisecond):
	}

	pending, err := outbox.Pending(ctx, pool)
	require.NoError(t, err)
	assert.Zero(t, pending)

	n, err = relay.PublishBatch(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "published rows are not sent again")
}

// bindTestQueue declares an exclusive queue bound to routingKey and consumes it.
func bindTestQueue(t *testing.T, url, routingKey string) <-chan amqp.Delivery {
	t.Helper()
	conn, err := rabbit.Dial(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	ch, err := conn.Channel()
	require.NoError(t, err)
	require.NoError(t, rabbit.DeclareExchange(ch))
	q, err := ch.QueueDeclare("", false, true, true, false, nil)
	require.NoError(t, err)
	require.NoError(t, ch.QueueBind(q.Name, routingKey, events.Exchange, false, nil))
	msgs, err := ch.Consume(q.Name, "", true, true, false, false, nil)
	require.NoError(t, err)
	return msgs
}
