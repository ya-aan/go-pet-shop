BEGIN;

ALTER TABLE order_items ADD COLUMN unit_price NUMERIC NOT NULL DEFAULT 0;

UPDATE order_items oi
SET unit_price = p.price
FROM products p
WHERE p.id = oi.product_id;

ALTER TABLE order_items ALTER COLUMN unit_price DROP DEFAULT;

COMMIT;
