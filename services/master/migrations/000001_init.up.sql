CREATE TABLE regions (
    id         BIGSERIAL PRIMARY KEY,
    code       TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE customers (
    id                BIGSERIAL PRIMARY KEY,
    code              TEXT        NOT NULL UNIQUE,
    name              TEXT        NOT NULL,
    region_id         BIGINT      NOT NULL REFERENCES regions (id),
    payment_term_days INT         NOT NULL CHECK (payment_term_days BETWEEN 0 AND 365),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE salespersons (
    id         BIGSERIAL PRIMARY KEY,
    code       TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    region_id  BIGINT      NOT NULL REFERENCES regions (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE uoms (
    code TEXT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE products (
    id         BIGSERIAL PRIMARY KEY,
    sku        TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    base_uom   TEXT        NOT NULL REFERENCES uoms (code),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- How many base units one unit of uom_code holds (e.g. 1 DUS = 24 PCS).
-- The base UoM itself is always stored with factor 1.
CREATE TABLE product_uoms (
    product_id BIGINT NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    uom_code   TEXT   NOT NULL REFERENCES uoms (code),
    factor     BIGINT NOT NULL CHECK (factor > 0),
    PRIMARY KEY (product_id, uom_code)
);

-- Price list: one price per product, region and selling UoM.
CREATE TABLE prices (
    product_id BIGINT        NOT NULL,
    region_id  BIGINT        NOT NULL REFERENCES regions (id),
    uom_code   TEXT          NOT NULL,
    price      NUMERIC(18,2) NOT NULL CHECK (price >= 0),
    updated_at TIMESTAMPTZ   NOT NULL DEFAULT now(),
    PRIMARY KEY (product_id, region_id, uom_code),
    FOREIGN KEY (product_id, uom_code) REFERENCES product_uoms (product_id, uom_code) ON DELETE CASCADE
);
