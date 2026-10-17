-- name: CreateOrder :one
INSERT INTO orders (status, customer_id, customer_code, customer_name, region_id, payment_term_days,
                    salesperson_id, salesperson_name, total)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, number, created_at, updated_at;

-- name: CreateOrderLine :exec
INSERT INTO order_lines (order_id, line_no, product_id, sku, product_name, uom, conversion_factor,
                         qty, base_qty, unit_price, subtotal)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: GetOrder :one
SELECT * FROM orders WHERE id = $1;

-- name: GetOrderForUpdate :one
SELECT * FROM orders WHERE id = $1 FOR UPDATE;

-- name: ListOrders :many
SELECT * FROM orders
WHERE sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text
ORDER BY id DESC
LIMIT $1;

-- name: ListOrderLines :many
SELECT * FROM order_lines
WHERE order_id = ANY(@order_ids::bigint[])
ORDER BY order_id, line_no;

-- name: UpdateOrder :exec
UPDATE orders
SET status = $2, reject_reason = $3, invoice_number = $4, confirmed_at = $5, updated_at = now()
WHERE id = $1;
