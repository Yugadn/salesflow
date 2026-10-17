// Package outbox implements the transactional outbox pattern.
//
// Writing to the database and publishing to RabbitMQ are two systems, so doing
// both directly can lose an event (crash after commit) or emit a phantom one
// (publish, then rollback). Instead, services Write the event into an outbox
// table in the same transaction as their state change, and a Relay publishes
// committed rows afterwards. Delivery is at-least-once; consumers dedupe with
// the inbox package.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/yugadn/salesflow/pkg/db"
	"github.com/yugadn/salesflow/pkg/events"
)

// Schema is the table every publishing service includes in its migrations.
const Schema = `CREATE TABLE outbox (
    id           UUID PRIMARY KEY,
    event_type   TEXT        NOT NULL,
    envelope     JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    attempts     INT         NOT NULL DEFAULT 0,
    last_error   TEXT
);
CREATE INDEX outbox_unpublished_idx ON outbox (created_at) WHERE published_at IS NULL;`

// Write stores env in the outbox. q must be the transaction that also
// carries the state change the event describes.
func Write(ctx context.Context, q db.DBTX, env events.Envelope) error {
	raw, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	if _, err := q.Exec(ctx,
		`INSERT INTO outbox (id, event_type, envelope) VALUES ($1, $2, $3)`,
		env.ID, env.Type, raw); err != nil {
		return fmt.Errorf("write outbox %s: %w", env.Type, err)
	}
	return nil
}

// Publisher is the subset of rabbit.Publisher the relay needs.
type Publisher interface {
	Publish(ctx context.Context, exchange, routingKey, messageID string, body []byte, headers amqp.Table) error
}

// Relay polls the outbox and publishes unpublished rows in creation order.
// Rows are claimed with FOR UPDATE SKIP LOCKED, so several replicas of a
// service can run relays concurrently without double-publishing a batch.
type Relay struct {
	pool     *pgxpool.Pool
	pub      Publisher
	log      *slog.Logger
	interval time.Duration
	batch    int
}

// NewRelay builds a relay polling every interval.
func NewRelay(pool *pgxpool.Pool, pub Publisher, log *slog.Logger, interval time.Duration) *Relay {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	return &Relay{pool: pool, pub: pub, log: log.With("component", "outbox-relay"), interval: interval, batch: 50}
}

// Run publishes until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) {
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Drain full batches back-to-back; wait for the next tick otherwise.
			for {
				n, err := r.PublishBatch(ctx)
				if err != nil {
					r.log.Error("relay batch failed", "err", err)
					break
				}
				if n < r.batch {
					break
				}
			}
		}
	}
}

type outboxRow struct {
	id        string
	eventType string
	envelope  []byte
}

// PublishBatch publishes up to one batch and returns how many rows it published.
// It stops at the first failure so events keep their order.
func (r *Relay) PublishBatch(ctx context.Context) (int, error) {
	published := 0
	err := db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id::text, event_type, envelope FROM outbox
			 WHERE published_at IS NULL
			 ORDER BY created_at
			 LIMIT $1
			 FOR UPDATE SKIP LOCKED`, r.batch)
		if err != nil {
			return fmt.Errorf("claim outbox rows: %w", err)
		}
		batch, err := pgx.CollectRows(rows, func(cr pgx.CollectableRow) (outboxRow, error) {
			var o outboxRow
			err := cr.Scan(&o.id, &o.eventType, &o.envelope)
			return o, err
		})
		if err != nil {
			return fmt.Errorf("read outbox rows: %w", err)
		}

		for _, o := range batch {
			if err := r.pub.Publish(ctx, events.Exchange, o.eventType, o.id, o.envelope, nil); err != nil {
				if _, uerr := tx.Exec(ctx,
					`UPDATE outbox SET attempts = attempts + 1, last_error = $2 WHERE id = $1`,
					o.id, err.Error()); uerr != nil {
					return fmt.Errorf("record publish failure: %w", uerr)
				}
				r.log.Warn("publish failed, will retry", "event_id", o.id, "event_type", o.eventType, "err", err)
				return nil // commit the attempt counter; the next tick retries
			}
			if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = $1`, o.id); err != nil {
				return fmt.Errorf("mark published: %w", err)
			}
			published++
		}
		return nil
	})
	return published, err
}

// Pending counts events not yet published.
func Pending(ctx context.Context, q db.DBTX) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE published_at IS NULL`).Scan(&n)
	return n, err
}
