-- A sequence can leave gaps when a transaction rolls back. That is fine for an
-- internal invoice number; legally numbered tax invoices (e-Faktur) are
-- allocated by the tax authority, not by this sequence.
CREATE SEQUENCE invoice_number_seq;

CREATE TABLE invoices (
    id              BIGSERIAL PRIMARY KEY,
    number          TEXT          NOT NULL UNIQUE DEFAULT ('INV-' || lpad(nextval('invoice_number_seq')::text, 6, '0')),
    order_id        BIGINT        NOT NULL,
    order_number    TEXT          NOT NULL,
    -- One invoice per delivery: the unique key makes double-billing impossible.
    delivery_id     BIGINT        NOT NULL UNIQUE,
    delivery_number TEXT          NOT NULL,
    customer_id     BIGINT        NOT NULL,
    customer_name   TEXT          NOT NULL,
    issue_date      DATE          NOT NULL,
    due_date        DATE          NOT NULL CHECK (due_date >= issue_date),
    total           NUMERIC(18,2) NOT NULL CHECK (total >= 0),
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX invoices_customer_idx ON invoices (customer_id);

CREATE TABLE invoice_lines (
    invoice_id   BIGINT        NOT NULL REFERENCES invoices (id) ON DELETE CASCADE,
    line_no      INT           NOT NULL,
    product_id   BIGINT        NOT NULL,
    sku          TEXT          NOT NULL,
    product_name TEXT          NOT NULL,
    uom          TEXT          NOT NULL,
    qty          BIGINT        NOT NULL CHECK (qty > 0),
    unit_price   NUMERIC(18,2) NOT NULL CHECK (unit_price >= 0),
    amount       NUMERIC(18,2) NOT NULL CHECK (amount >= 0),
    PRIMARY KEY (invoice_id, line_no)
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
