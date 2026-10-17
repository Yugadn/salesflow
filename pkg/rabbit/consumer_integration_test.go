//go:build integration

package rabbit_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/pkg/rabbit"
	"github.com/yugadn/salesflow/pkg/testinfra"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type harness struct {
	url string
	cfg rabbit.ConsumerConfig
	pub *rabbit.Publisher
	ch  *amqp.Channel
}

func newHarness(t *testing.T, queue string) harness {
	t.Helper()
	url := testinfra.RabbitMQ(t)
	cfg := rabbit.ConsumerConfig{
		URL: url, Queue: queue, RoutingKeys: []string{"thing.happened"},
		MaxRetries: 2, RetryDelay: 200 * time.Millisecond,
	}
	conn, err := rabbit.Dial(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	ch, err := conn.Channel()
	require.NoError(t, err)
	// Declare the topology before publishing so the first message is routed.
	require.NoError(t, rabbit.DeclareTopology(ch, cfg))

	pub := rabbit.NewPublisher(url)
	t.Cleanup(pub.Close)
	return harness{url: url, cfg: cfg, pub: pub, ch: ch}
}

func (h harness) run(t *testing.T, handler rabbit.Handler) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = rabbit.NewConsumer(h.cfg, handler, quiet).Run(ctx) }()
}

func (h harness) publish(t *testing.T) events.Envelope {
	t.Helper()
	env, err := events.New("test", "thing.happened", map[string]string{"k": "v"})
	require.NoError(t, err)
	body, err := json.Marshal(env)
	require.NoError(t, err)
	require.NoError(t, h.pub.Publish(context.Background(), events.Exchange, "thing.happened", env.ID, body, nil))
	return env
}

// dlqDepth reports how many messages sit in the dead-letter queue.
func (h harness) dlqDepth(t *testing.T) int {
	t.Helper()
	q, err := h.ch.QueueDeclarePassive(rabbit.DeadLetterQueue(h.cfg.Queue), true, false, false, false, nil)
	require.NoError(t, err)
	return q.Messages
}

func TestConsumer_RetriesTransientFailureThenSucceeds(t *testing.T) {
	h := newHarness(t, "retry-then-ok")
	var attempts atomic.Int32
	done := make(chan struct{})
	h.run(t, func(context.Context, events.Envelope) error {
		if attempts.Add(1) < 3 {
			return errors.New("database briefly unavailable")
		}
		close(done)
		return nil
	})

	h.publish(t)

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("message was never handled successfully")
	}
	assert.Equal(t, int32(3), attempts.Load(), "first try + 2 retries")
	assert.Zero(t, h.dlqDepth(t))
}

func TestConsumer_DeadLettersAfterMaxRetries(t *testing.T) {
	h := newHarness(t, "always-fails")
	var attempts atomic.Int32
	h.run(t, func(context.Context, events.Envelope) error {
		attempts.Add(1)
		return errors.New("still broken")
	})

	h.publish(t)

	require.Eventually(t, func() bool { return h.dlqDepth(t) == 1 }, 10*time.Second, 100*time.Millisecond)
	assert.Equal(t, int32(1+h.cfg.MaxRetries), attempts.Load())
}

func TestConsumer_PermanentErrorSkipsRetries(t *testing.T) {
	h := newHarness(t, "permanent")
	var attempts atomic.Int32
	h.run(t, func(context.Context, events.Envelope) error {
		attempts.Add(1)
		return rabbit.Permanent(errors.New("payload can never be valid"))
	})

	h.publish(t)

	require.Eventually(t, func() bool { return h.dlqDepth(t) == 1 }, 10*time.Second, 100*time.Millisecond)
	assert.Equal(t, int32(1), attempts.Load())
}
