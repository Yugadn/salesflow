-- name: CreateWarehouse :one
INSERT INTO warehouses (code, name, region_id) VALUES ($1, $2, $3)
RETURNING id, code, name, region_id;

-- name: ListWarehouses :many
SELECT id, code, name, region_id FROM warehouses ORDER BY code;

-- name: GetWarehouse :one
SELECT id, code, name, region_id FROM warehouses WHERE id = $1;

-- name: GetWarehouseByRegion :one
SELECT id, code, name, region_id FROM warehouses WHERE region_id = $1;

-- name: ReceiveStock :one
INSERT INTO stocks (warehouse_id, product_id, on_hand) VALUES ($1, $2, @qty)
ON CONFLICT (warehouse_id, product_id)
DO UPDATE SET on_hand = stocks.on_hand + EXCLUDED.on_hand, updated_at = now()
RETURNING warehouse_id, product_id, on_hand, reserved;

-- name: ListStocks :many
SELECT warehouse_id, product_id, on_hand, reserved FROM stocks
WHERE warehouse_id = $1
ORDER BY product_id;

-- Rows are locked in product_id order; every reservation does the same, so
-- two concurrent orders can never wait on each other in a cycle.
-- name: LockStocks :many
SELECT warehouse_id, product_id, on_hand, reserved FROM stocks
WHERE warehouse_id = $1 AND product_id = ANY(@product_ids::bigint[])
ORDER BY product_id
FOR UPDATE;

-- name: ReserveStock :execrows
UPDATE stocks SET reserved = reserved + @qty, updated_at = now()
WHERE warehouse_id = $1 AND product_id = $2 AND on_hand - reserved >= @qty;

-- name: ApplyDeliveryMovement :execrows
UPDATE stocks
SET on_hand = on_hand - @consume, reserved = reserved - @release, updated_at = now()
WHERE warehouse_id = $1 AND product_id = $2 AND on_hand >= @consume AND reserved >= @release;

-- name: CreateDelivery :one
INSERT INTO deliveries (order_id, order_number, warehouse_id, customer_id, customer_name, payment_term_days, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, number, created_at;

-- name: CreateDeliveryLine :exec
INSERT INTO delivery_lines (delivery_id, line_no, product_id, sku, product_name, uom, conversion_factor, ordered_qty, unit_price)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: DeliveryExistsForOrder :one
SELECT EXISTS (SELECT 1 FROM deliveries WHERE order_id = $1);

-- name: GetDelivery :one
SELECT * FROM deliveries WHERE id = $1;

-- name: GetDeliveryForUpdate :one
SELECT * FROM deliveries WHERE id = $1 FOR UPDATE;

-- name: ListDeliveries :many
SELECT * FROM deliveries
WHERE sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text
ORDER BY id DESC
LIMIT $1;

-- name: ListDeliveryLines :many
SELECT * FROM delivery_lines
WHERE delivery_id = ANY(@delivery_ids::bigint[])
ORDER BY delivery_id, line_no;

-- name: UpdateDelivery :exec
UPDATE deliveries SET status = $2, shipped_at = $3, delivered_at = $4 WHERE id = $1;

-- name: SetDeliveredQty :exec
UPDATE delivery_lines SET delivered_qty = $3 WHERE delivery_id = $1 AND line_no = $2;
