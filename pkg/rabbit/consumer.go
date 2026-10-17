package rabbit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/yugadn/salesflow/pkg/events"
)

const (
	headerRetryCount = "x-retry-count"
	headerLastError  = "x-last-error"
	headerRoutingKey = "x-original-routing-key"
)

// Handler processes one event. Returning nil acknowledges it; returning an
// error schedules a retry, or dead-letters it if the error is Permanent or
// retries are exhausted.
type Handler func(ctx context.Context, env events.Envelope) error

// ConsumerConfig describes one service's subscription.
type ConsumerConfig struct {
	URL         string
	Queue       string   // durable queue owned by the consuming service
	RoutingKeys []string // events the service subscribes to
	MaxRetries  int      // default 5
	RetryDelay  time.Duration
	Prefetch    int
}

// Consumer consumes a service's queue with this topology:
//
//	salesflow.events --(routing keys)--> <queue>
//	<queue>.retry : TTL = RetryDelay, dead-letters back into <queue> only
//	<queue>.dlq   : messages that failed permanently or ran out of retries
//
// Retries go through the default exchange straight to this service's queue,
// so other services subscribed to the same event never see the retry.
type Consumer struct {
	cfg     ConsumerConfig
	handler Handler
	log     *slog.Logger
}

// NewConsumer builds a consumer, filling in defaults.
func NewConsumer(cfg ConsumerConfig, handler Handler, log *slog.Logger) *Consumer {
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 5
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 5 * time.Second
	}
	if cfg.Prefetch <= 0 {
		cfg.Prefetch = 10
	}
	return &Consumer{cfg: cfg, handler: handler, log: log.With("queue", cfg.Queue)}
}

// RetryQueue and DeadLetterQueue name the auxiliary queues for queue.
func RetryQueue(queue string) string      { return queue + ".retry" }
func DeadLetterQueue(queue string) string { return queue + ".dlq" }

// Run consumes until ctx is cancelled, reconnecting after broker failures.
func (c *Consumer) Run(ctx context.Context) error {
	for {
		// runOnce only returns on shutdown or a broker failure; on failure, reconnect.
		if err := c.runOnce(ctx); err != nil && ctx.Err() == nil {
			c.log.Error("consumer stopped, reconnecting", "err", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
	}
}

// DeclareTopology declares the exchange, queues and bindings for cfg. It is
// idempotent and exported so tests can set up a queue before publishing.
func DeclareTopology(ch *amqp.Channel, cfg ConsumerConfig) error {
	if err := DeclareExchange(ch); err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}
	if _, err := ch.QueueDeclare(cfg.Queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare queue: %w", err)
	}
	for _, key := range cfg.RoutingKeys {
		if err := ch.QueueBind(cfg.Queue, key, events.Exchange, false, nil); err != nil {
			return fmt.Errorf("bind %s: %w", key, err)
		}
	}
	retryArgs := amqp.Table{
		"x-message-ttl":             int64(cfg.RetryDelay / time.Millisecond),
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": cfg.Queue,
	}
	if _, err := ch.QueueDeclare(RetryQueue(cfg.Queue), true, false, false, false, retryArgs); err != nil {
		return fmt.Errorf("declare retry queue: %w", err)
	}
	if _, err := ch.QueueDeclare(DeadLetterQueue(cfg.Queue), true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter queue: %w", err)
	}
	return nil
}

func (c *Consumer) runOnce(ctx context.Context) error {
	conn, err := Dial(ctx, c.cfg.URL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	if err := ch.Qos(c.cfg.Prefetch, 0, false); err != nil {
		return fmt.Errorf("set qos: %w", err)
	}
	if err := DeclareTopology(ch, c.cfg); err != nil {
		return err
	}

	// A separate confirm-mode channel republishes to the retry/dead-letter queues.
	pubCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open publish channel: %w", err)
	}
	if err := pubCh.Confirm(false); err != nil {
		return fmt.Errorf("enable confirms: %w", err)
	}

	deliveries, err := ch.ConsumeWithContext(ctx, c.cfg.Queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	c.log.Info("consuming", "routing_keys", c.cfg.RoutingKeys)

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("delivery channel closed")
			}
			c.handle(ctx, pubCh, d)
		}
	}
}

func (c *Consumer) handle(ctx context.Context, pubCh *amqp.Channel, d amqp.Delivery) {
	var env events.Envelope
	if err := json.Unmarshal(d.Body, &env); err != nil {
		c.deadLetter(ctx, pubCh, d, fmt.Errorf("malformed envelope: %w", err))
		return
	}
	log := c.log.With("event_id", env.ID, "event_type", env.Type)

	hctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err := c.handler(hctx, env)
	cancel()
	if err == nil {
		_ = d.Ack(false)
		log.Debug("event handled")
		return
	}

	retries := retryCount(d.Headers)
	if IsPermanent(err) || retries >= c.cfg.MaxRetries {
		log.Error("event dead-lettered", "err", err, "retries", retries)
		c.deadLetter(ctx, pubCh, d, err)
		return
	}
	log.Warn("event failed, scheduling retry", "err", err, "retry", retries+1)
	headers := copyHeaders(d.Headers)
	headers[headerRetryCount] = int32(retries + 1)
	headers[headerLastError] = err.Error()
	c.republish(ctx, pubCh, d, RetryQueue(c.cfg.Queue), headers)
}

func (c *Consumer) deadLetter(ctx context.Context, pubCh *amqp.Channel, d amqp.Delivery, cause error) {
	headers := copyHeaders(d.Headers)
	headers[headerLastError] = cause.Error()
	c.republish(ctx, pubCh, d, DeadLetterQueue(c.cfg.Queue), headers)
}

// republish copies d to queue via the default exchange, then acks the
// original. If the copy cannot be confirmed the original is requeued, so a
// message is never dropped (at worst it is seen twice, which the inbox absorbs).
func (c *Consumer) republish(ctx context.Context, pubCh *amqp.Channel, d amqp.Delivery, queue string, headers amqp.Table) {
	if _, ok := headers[headerRoutingKey]; !ok {
		headers[headerRoutingKey] = d.RoutingKey
	}
	dc, err := pubCh.PublishWithDeferredConfirmWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType:  d.ContentType,
		DeliveryMode: amqp.Persistent,
		MessageId:    d.MessageId,
		Timestamp:    d.Timestamp,
		Headers:      headers,
		Body:         d.Body,
	})
	if err == nil {
		var acked bool
		acked, err = dc.WaitContext(ctx)
		if err == nil && !acked {
			err = errors.New("broker nacked republish")
		}
	}
	if err != nil {
		c.log.Error("republish failed, requeueing original", "target", queue, "err", err)
		_ = d.Nack(false, true)
		return
	}
	_ = d.Ack(false)
}

func retryCount(h amqp.Table) int {
	switch v := h[headerRetryCount].(type) {
	case int32:
		return int(v)
	case int64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func copyHeaders(h amqp.Table) amqp.Table {
	out := amqp.Table{}
	for k, v := range h {
		out[k] = v
	}
	return out
}
