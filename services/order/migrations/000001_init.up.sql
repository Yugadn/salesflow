CREATE SEQUENCE order_number_seq;

CREATE TABLE orders (
    id                BIGSERIAL PRIMARY KEY,
    number            TEXT          NOT NULL UNIQUE DEFAULT ('SO-' || lpad(nextval('order_number_seq')::text, 6, '0')),
    status            TEXT          NOT NULL CHECK (status IN ('DRAFT', 'CONFIRMED', 'FULFILLING', 'DELIVERED', 'INVOICED', 'REJECTED', 'CANCELLED')),
    -- Customer and salesperson are snapshotted: later master-data edits must not rewrite history.
    customer_id       BIGINT        NOT NULL,
    customer_code     TEXT          NOT NULL,
    customer_name     TEXT          NOT NULL,
    region_id         BIGINT        NOT NULL,
    payment_term_days INT           NOT NULL,
    salesperson_id    BIGINT        NOT NULL,
    salesperson_name  TEXT          NOT NULL,
    total             NUMERIC(18,2) NOT NULL CHECK (total >= 0),
    reject_reason     TEXT,
    invoice_number    TEXT,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),
    confirmed_at      TIMESTAMPTZ
);
CREATE INDEX orders_status_idx ON orders (status);

CREATE TABLE order_lines (
    order_id          BIGINT        NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    line_no           INT           NOT NULL,
    product_id        BIGINT        NOT NULL,
    sku               TEXT          NOT NULL,
    product_name      TEXT          NOT NULL,
    uom               TEXT          NOT NULL,
    conversion_factor BIGINT        NOT NULL CHECK (conversion_factor > 0),
    qty               BIGINT        NOT NULL CHECK (qty > 0),
    base_qty          BIGINT        NOT NULL CHECK (base_qty > 0),
    unit_price        NUMERIC(18,2) NOT NULL CHECK (unit_price >= 0),
    subtotal          NUMERIC(18,2) NOT NULL CHECK (subtotal >= 0),
    PRIMARY KEY (order_id, line_no)
);

-- Transactional outbox (see pkg/outbox).
CREATE TABLE outbox (
    id           UUID PRIMARY KEY,
    event_type   TEXT        NOT NULL,
    envelope     JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    attempts     INT         NOT NULL DEFAULT 0,
    last_error   TEXT
);
CREATE INDEX outbox_unpublished_idx ON outbox (created_at) WHERE published_at IS NULL;

-- Idempotent consumer inbox (see pkg/inbox).
CREATE TABLE processed_events (
    event_id     UUID PRIMARY KEY,
    event_type   TEXT        NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
