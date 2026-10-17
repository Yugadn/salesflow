-- name: CreateInvoice :one
INSERT INTO invoices (order_id, order_number, delivery_id, delivery_number, customer_id, customer_name,
                      issue_date, due_date, total)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, number, created_at;

-- name: CreateInvoiceLine :exec
INSERT INTO invoice_lines (invoice_id, line_no, product_id, sku, product_name, uom, qty, unit_price, amount)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: InvoiceExistsForDelivery :one
SELECT EXISTS (SELECT 1 FROM invoices WHERE delivery_id = $1);

-- name: GetInvoice :one
SELECT * FROM invoices WHERE id = $1;

-- name: ListInvoices :many
SELECT * FROM invoices
WHERE sqlc.narg('customer_id')::bigint IS NULL OR customer_id = sqlc.narg('customer_id')::bigint
ORDER BY id DESC
LIMIT $1;

-- name: ListInvoiceLines :many
SELECT * FROM invoice_lines
WHERE invoice_id = ANY(@invoice_ids::bigint[])
ORDER BY invoice_id, line_no;
