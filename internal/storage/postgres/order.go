package postgres

import (
	"context"
	"errors"
	"fmt"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	fkOrdersUserID        = "orders_user_id_fkey"
	fkOrderItemsOrderID   = "order_items_order_id_fkey"
	fkOrderItemsProductID = "order_items_product_id_fkey"
)

// CreateOrder - создает пустой заказ пользователя и возвращает его ID
func (s *Storage) CreateOrder(ctx context.Context, o models.Order) (int, error) {
	const fn = "storage.postgres.order.CreateOrder"

	// Сумма нового заказа всегда 0: она растёт по мере добавления позиций в AddOrderItem
	var id int
	err := s.db.QueryRow(ctx,
		`INSERT INTO orders (user_id, total_price) VALUES ($1, 0) RETURNING id`,
		o.UserID).Scan(&id)
	if err != nil {
		if constraint, ok := foreignKeyViolation(err); ok && constraint == fkOrdersUserID {
			return 0, fmt.Errorf("%s: %w: user id=%d does not exist", fn, storage.ErrInvalidInput, o.UserID)
		}
		return 0, fmt.Errorf("%s: %w", fn, err)
	}

	return id, nil
}

// addOrderItemQuery - добавляет позицию и увеличивает сумму заказа одним запросом.
// Один SQL-запрос выполняется атомарно: если позиция не вставилась, сумма заказа не меняется,
// и наоборот. Цена берётся из products в момент добавления и сохраняется в unit_price,
// поэтому последующее изменение цены товара не затрагивает уже оформленные позиции.
// Если товара нет, INSERT ... SELECT ничего не вставляет и UPDATE затрагивает 0 строк.
// Если нет заказа, INSERT падает на внешнем ключе order_items_order_id_fkey.
const addOrderItemQuery = `
WITH item AS (
    INSERT INTO order_items (order_id, product_id, quantity, unit_price)
    SELECT $1, p.id, $3, p.price
    FROM products p
    WHERE p.id = $2
    RETURNING order_id, quantity, unit_price
)
UPDATE orders o
SET total_price = o.total_price + item.unit_price * item.quantity
FROM item
WHERE o.id = item.order_id`

// AddOrderItem - добавляет позицию в заказ и пересчитывает сумму заказа
func (s *Storage) AddOrderItem(ctx context.Context, item models.OrderItem) error {
	const fn = "storage.postgres.order.AddOrderItem"

	cmd, err := s.db.Exec(ctx, addOrderItemQuery, item.OrderID, item.ProductID, item.Quantity)
	if err != nil {
		if constraint, ok := foreignKeyViolation(err); ok {
			switch constraint {
			case fkOrderItemsOrderID:
				return fmt.Errorf("%s: %w: order id=%d", fn, storage.ErrNotFound, item.OrderID)
			case fkOrderItemsProductID:
				// Товар удалили между SELECT и проверкой ключа
				return fmt.Errorf("%s: %w: product id=%d does not exist", fn, storage.ErrInvalidInput, item.ProductID)
			}
		}
		return fmt.Errorf("%s: %w", fn, err)
	}

	if cmd.RowsAffected() == 0 {
		// Ничего не вставилось, значит товара нет. Но если нет и заказа,
		// важнее сообщить об отсутствии заказа: он указан в URL
		var orderExists bool
		if err := s.db.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM orders WHERE id = $1)`,
			item.OrderID).Scan(&orderExists); err != nil {
			return fmt.Errorf("%s: %w", fn, err)
		}
		if !orderExists {
			return fmt.Errorf("%s: %w: order id=%d", fn, storage.ErrNotFound, item.OrderID)
		}
		return fmt.Errorf("%s: %w: product id=%d does not exist", fn, storage.ErrInvalidInput, item.ProductID)
	}

	return nil
}

// GetOrderByID - получает заказ по ID
func (s *Storage) GetOrderByID(ctx context.Context, id int) (models.Order, error) {
	const fn = "storage.postgres.order.GetOrderByID"

	var o models.Order
	err := s.db.QueryRow(ctx,
		`SELECT id, user_id, total_price, created_at FROM orders WHERE id = $1`,
		id).Scan(&o.ID, &o.UserID, &o.TotalPrice, &o.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Order{}, fmt.Errorf("%s: %w: id=%d", fn, storage.ErrNotFound, id)
		}
		return models.Order{}, fmt.Errorf("%s: %w", fn, err)
	}

	return o, nil
}

// GetOrdersByUserEmail - получает заказы пользователя по email, новые первыми.
// LEFT JOIN от users отличает два случая за один запрос:
// нет ни одной строки - пользователя нет (ErrNotFound);
// одна строка с o.id = NULL - пользователь есть, но заказов у него нет (пустой список).
func (s *Storage) GetOrdersByUserEmail(ctx context.Context, email string) ([]models.Order, error) {
	const fn = "storage.postgres.order.GetOrdersByUserEmail"

	rows, err := s.db.Query(ctx, `
		SELECT u.id, o.id, o.total_price, o.created_at
		FROM users u
		LEFT JOIN orders o ON o.user_id = u.id
		WHERE u.email = $1
		ORDER BY o.created_at DESC, o.id DESC`,
		email)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}
	defer rows.Close()

	userFound := false
	orders := []models.Order{}
	for rows.Next() {
		var (
			userID     int
			orderID    *int
			totalPrice *float64
			createdAt  *time.Time
		)
		if err := rows.Scan(&userID, &orderID, &totalPrice, &createdAt); err != nil {
			return nil, fmt.Errorf("%s: %w", fn, err)
		}
		userFound = true

		if orderID == nil {
			continue // пользователь без заказов
		}
		orders = append(orders, models.Order{
			ID:         *orderID,
			UserID:     userID,
			TotalPrice: *totalPrice,
			CreatedAt:  *createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	if !userFound {
		return nil, fmt.Errorf("%s: %w: email=%s", fn, storage.ErrNotFound, email)
	}

	return orders, nil
}

// GetOrderItemsByOrderID - получает позиции заказа вместе с названием товара (JOIN с products).
// Цена позиции берётся из order_items.unit_price, а не из текущей цены товара.
func (s *Storage) GetOrderItemsByOrderID(ctx context.Context, orderID int) ([]models.OrderItem, error) {
	const fn = "storage.postgres.order.GetOrderItemsByOrderID"

	rows, err := s.db.Query(ctx, `
		SELECT oi.id, oi.order_id, oi.product_id, p.name, oi.quantity, oi.unit_price
		FROM order_items oi
		JOIN products p ON p.id = oi.product_id
		WHERE oi.order_id = $1
		ORDER BY oi.id`,
		orderID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}
	defer rows.Close()

	items := []models.OrderItem{}
	for rows.Next() {
		var it models.OrderItem
		if err := rows.Scan(&it.ID, &it.OrderID, &it.ProductID, &it.ProductName, &it.Quantity, &it.UnitPrice); err != nil {
			return nil, fmt.Errorf("%s: %w", fn, err)
		}
		items = append(items, it)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	return items, nil
}
