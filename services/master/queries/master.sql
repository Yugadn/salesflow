-- name: CreateRegion :one
INSERT INTO regions (code, name) VALUES ($1, $2) RETURNING id, code, name;

-- name: ListRegions :many
SELECT id, code, name FROM regions ORDER BY code;

-- name: CreateCustomer :one
INSERT INTO customers (code, name, region_id, payment_term_days)
VALUES ($1, $2, $3, $4)
RETURNING id, code, name, region_id, payment_term_days;

-- name: GetCustomer :one
SELECT id, code, name, region_id, payment_term_days FROM customers WHERE id = $1;

-- name: ListCustomers :many
SELECT id, code, name, region_id, payment_term_days FROM customers ORDER BY code;

-- name: CreateSalesperson :one
INSERT INTO salespersons (code, name, region_id) VALUES ($1, $2, $3)
RETURNING id, code, name, region_id;

-- name: GetSalesperson :one
SELECT id, code, name, region_id FROM salespersons WHERE id = $1;

-- name: ListSalespersons :many
SELECT id, code, name, region_id FROM salespersons ORDER BY code;

-- name: CreateUoM :one
INSERT INTO uoms (code, name) VALUES ($1, $2) RETURNING code, name;

-- name: ListUoMs :many
SELECT code, name FROM uoms ORDER BY code;

-- name: CreateProduct :one
INSERT INTO products (sku, name, base_uom) VALUES ($1, $2, $3)
RETURNING id, sku, name, base_uom;

-- name: GetProduct :one
SELECT id, sku, name, base_uom FROM products WHERE id = $1;

-- name: ListProducts :many
SELECT id, sku, name, base_uom FROM products ORDER BY sku;

-- name: UpsertProductUoM :exec
INSERT INTO product_uoms (product_id, uom_code, factor) VALUES ($1, $2, $3)
ON CONFLICT (product_id, uom_code) DO UPDATE SET factor = EXCLUDED.factor;

-- name: GetProductUoM :one
SELECT product_id, uom_code, factor FROM product_uoms WHERE product_id = $1 AND uom_code = $2;

-- name: ListProductUoMs :many
SELECT product_id, uom_code, factor FROM product_uoms
WHERE product_id = ANY(@product_ids::bigint[])
ORDER BY product_id, factor;

-- name: UpsertPrice :exec
INSERT INTO prices (product_id, region_id, uom_code, price) VALUES ($1, $2, $3, $4)
ON CONFLICT (product_id, region_id, uom_code)
DO UPDATE SET price = EXCLUDED.price, updated_at = now();

-- name: GetPrice :one
SELECT price FROM prices WHERE product_id = $1 AND region_id = $2 AND uom_code = $3;

-- name: ListPricesByProduct :many
SELECT product_id, region_id, uom_code, price FROM prices
WHERE product_id = $1
ORDER BY region_id, uom_code;
