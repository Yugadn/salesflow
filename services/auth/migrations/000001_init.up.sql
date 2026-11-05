CREATE TABLE users (
    id             BIGSERIAL PRIMARY KEY,
    email          TEXT        NOT NULL UNIQUE, -- stored lower-cased
    name           TEXT        NOT NULL,
    password_hash  TEXT        NOT NULL,        -- bcrypt
    role           TEXT        NOT NULL CHECK (role IN ('admin', 'sales', 'warehouse')),
    -- The master-data salesperson a sales user acts as.
    salesperson_id BIGINT,
    -- Users are deactivated, never deleted, so audit trails keep their author.
    active         BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((role = 'sales') = (salesperson_id IS NOT NULL))
);
-- One login per salesperson.
CREATE UNIQUE INDEX users_salesperson_idx ON users (salesperson_id) WHERE salesperson_id IS NOT NULL;

-- RSA keys that sign tokens. Kept so tokens survive restarts; the newest one
-- signs, all are published, which leaves room for rotation. A production
-- system would keep the private key in a KMS or secret manager instead.
CREATE TABLE signing_keys (
    kid             TEXT PRIMARY KEY,
    private_key_pem TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
