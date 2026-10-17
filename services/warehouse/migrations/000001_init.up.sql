-- MVP: one warehouse serves each sales region.
CREATE TABLE warehouses (
    id         BIGSERIAL PRIMARY KEY,
    code       TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    region_id  BIGINT      NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Quantities are in each product's base unit. The CHECKs are the last line of
-- defence against overselling: even a buggy query cannot reserve stock that
-- is not on the shelf.
CREATE TABLE stocks (
    warehouse_id BIGINT      NOT NULL REFERENCES warehouses (id),
    product_id   BIGINT      NOT NULL,
    on_hand      BIGINT      NOT NULL DEFAULT 0 CHECK (on_hand >= 0),
    reserved     BIGINT      NOT NULL DEFAULT 0 CHECK (reserved >= 0),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (warehouse_id, product_id),
    CHECK (reserved <= on_hand)
);

CREATE SEQUENCE delivery_number_seq;

CREATE TABLE deliveries (
    id                BIGSERIAL PRIMARY KEY,
    number            TEXT        NOT NULL UNIQUE DEFAULT ('DO-' || lpad(nextval('delivery_number_seq')::text, 6, '0')),
    order_id          BIGINT      NOT NULL UNIQUE,
    order_number      TEXT        NOT NULL,
    warehouse_id      BIGINT      NOT NULL REFERENCES warehouses (id),
    customer_id       BIGINT      NOT NULL,
    customer_name     TEXT        NOT NULL,
    payment_term_days INT         NOT NULL,
    status            TEXT        NOT NULL CHECK (status IN ('PENDING', 'SHIPPED', 'DELIVERED')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    shipped_at        TIMESTAMPTZ,
    delivered_at      TIMESTAMPTZ
);
CREATE INDEX deliveries_status_idx ON deliveries (status);

CREATE TABLE delivery_lines (
    delivery_id       BIGINT        NOT NULL REFERENCES deliveries (id) ON DELETE CASCADE,
    line_no           INT           NOT NULL,
    product_id        BIGINT        NOT NULL,
    sku               TEXT          NOT NULL,
    product_name      TEXT          NOT NULL,
    uom               TEXT          NOT NULL,
    conversion_factor BIGINT        NOT NULL CHECK (conversion_factor > 0),
    ordered_qty       BIGINT        NOT NULL CHECK (ordered_qty > 0),
    delivered_qty     BIGINT        CHECK (delivered_qty BETWEEN 0 AND ordered_qty),
    unit_price        NUMERIC(18,2) NOT NULL,
    PRIMARY KEY (delivery_id, line_no)
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
