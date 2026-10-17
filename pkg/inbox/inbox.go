// Package inbox makes event consumers idempotent. RabbitMQ delivers
// at-least-once, so the same event can arrive twice; recording its ID in the
// same transaction as the side effects guarantees it is applied exactly once.
package inbox

import (
	"context"
	"fmt"

	"github.com/yugadn/salesflow/pkg/db"
)

// Schema is the table every consuming service includes in its migrations.
const Schema = `CREATE TABLE processed_events (
    event_id     UUID PRIMARY KEY,
    event_type   TEXT        NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`

// MarkProcessed records eventID and reports whether this is the first time it
// was seen. Call it inside the transaction that applies the event: if it
// returns false, skip the work — a previous delivery already committed it.
func MarkProcessed(ctx context.Context, q db.DBTX, eventID, eventType string) (bool, error) {
	tag, err := q.Exec(ctx,
		`INSERT INTO processed_events (event_id, event_type) VALUES ($1, $2)
		 ON CONFLICT (event_id) DO NOTHING`, eventID, eventType)
	if err != nil {
		return false, fmt.Errorf("mark event %s processed: %w", eventID, err)
	}
	return tag.RowsAffected() == 1, nil
}
