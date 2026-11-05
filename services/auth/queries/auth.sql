-- name: CreateUser :one
INSERT INTO users (email, name, password_hash, role, salesperson_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY role, name;

-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: UpdateUser :one
UPDATE users
SET name = $2, role = $3, salesperson_id = $4, active = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdatePassword :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: InsertSigningKey :exec
INSERT INTO signing_keys (kid, private_key_pem) VALUES ($1, $2);

-- name: ListSigningKeys :many
SELECT kid, private_key_pem FROM signing_keys ORDER BY created_at DESC;
