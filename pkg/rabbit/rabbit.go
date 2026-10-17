// Package rabbit wraps RabbitMQ with the delivery guarantees the system
// relies on: confirmed publishes, at-least-once consumption, delayed retries
// and a dead-letter queue for messages that keep failing.
package rabbit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/yugadn/salesflow/pkg/events"
)

// Dial connects to RabbitMQ, retrying for up to 30s while the broker starts.
func Dial(ctx context.Context, url string) (*amqp.Connection, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		conn, err := amqp.Dial(url)
		if err == nil {
			return conn, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("rabbitmq not reachable: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// DeclareExchange declares the shared durable topic exchange.
func DeclareExchange(ch *amqp.Channel) error {
	return ch.ExchangeDeclare(events.Exchange, amqp.ExchangeTopic, true, false, false, false, nil)
}

// Publisher publishes with publisher confirms, so Publish only returns nil
// once the broker has taken responsibility for the message. It reconnects
// lazily after a failure.
type Publisher struct {
	url  string
	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
}

// NewPublisher returns a publisher that connects on first use.
func NewPublisher(url string) *Publisher { return &Publisher{url: url} }

// Publish sends body to exchange with routingKey and waits for the broker's confirm.
func (p *Publisher) Publish(ctx context.Context, exchange, routingKey, messageID string, body []byte, headers amqp.Table) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureChannel(ctx); err != nil {
		return err
	}
	dc, err := p.ch.PublishWithDeferredConfirmWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    messageID,
		Timestamp:    time.Now().UTC(),
		Headers:      headers,
		Body:         body,
	})
	if err != nil {
		p.reset()
		return fmt.Errorf("publish %s: %w", routingKey, err)
	}
	acked, err := dc.WaitContext(ctx)
	if err != nil {
		p.reset()
		return fmt.Errorf("await confirm for %s: %w", routingKey, err)
	}
	if !acked {
		return fmt.Errorf("broker nacked %s", routingKey)
	}
	return nil
}

func (p *Publisher) ensureChannel(ctx context.Context) error {
	if p.ch != nil && !p.ch.IsClosed() {
		return nil
	}
	p.reset()
	conn, err := Dial(ctx, p.url)
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("open channel: %w", err)
	}
	if err := ch.Confirm(false); err != nil {
		_ = conn.Close()
		return fmt.Errorf("enable confirms: %w", err)
	}
	if err := DeclareExchange(ch); err != nil {
		_ = conn.Close()
		return fmt.Errorf("declare exchange: %w", err)
	}
	p.conn, p.ch = conn, ch
	return nil
}

func (p *Publisher) reset() {
	if p.conn != nil {
		_ = p.conn.Close()
	}
	p.conn, p.ch = nil, nil
}

// Close releases the connection.
func (p *Publisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reset()
}

// permanentError marks a failure that retrying cannot fix.
type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent wraps err so the consumer dead-letters the message immediately
// instead of retrying (e.g. a malformed payload).
func Permanent(err error) error { return permanentError{err: err} }

// IsPermanent reports whether err was wrapped with Permanent.
func IsPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}
