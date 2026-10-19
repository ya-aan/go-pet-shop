-- Цена товара на момент добавления позиции в заказ.
-- Позиция хранит свою цену, поэтому изменение цены товара не меняет стоимость старых заказов.
BEGIN;

ALTER TABLE order_items ADD COLUMN unit_price NUMERIC NOT NULL DEFAULT 0;

-- Позиции, созданные до этой миграции, получают текущую цену товара: другой у нас нет
UPDATE order_items oi
SET unit_price = p.price
FROM products p
WHERE p.id = oi.product_id;

-- Дефолт нужен только для заполнения старых строк, новые позиции обязаны передавать цену явно
ALTER TABLE order_items ALTER COLUMN unit_price DROP DEFAULT;

COMMIT;
