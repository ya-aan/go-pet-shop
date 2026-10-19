package postgres

import (
	"context"
	"errors"
	"fmt"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"
	"math"
	"sort"

	"github.com/jackc/pgx/v5"
)

func (s *Storage) PlaceOrder(ctx context.Context, userEmail string, items []models.OrderItem) (orderID int, err error) {
	const fn = "storage.postgres.checkout.PlaceOrder"

	merged, err := mergeOrderItems(items)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", fn, err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("%s: begin: %w", fn, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID int
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, userEmail).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("%s: user %w: email=%s", fn, storage.ErrNotFound, userEmail)
		}
		return 0, fmt.Errorf("%s: find user: %w", fn, err)
	}

	for _, item := range merged {
		if err := reserveStock(ctx, tx, item); err != nil {
			return 0, fmt.Errorf("%s: %w", fn, err)
		}
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO orders (user_id, total_price) VALUES ($1, 0) RETURNING id`,
		userID).Scan(&orderID)
	if err != nil {
		return 0, fmt.Errorf("%s: insert order: %w", fn, err)
	}

	for _, item := range merged {
		if _, err := tx.Exec(ctx, `
			INSERT INTO order_items (order_id, product_id, quantity, unit_price)
			SELECT $1, p.id, $3, p.price FROM products p WHERE p.id = $2`,
			orderID, item.ProductID, item.Quantity); err != nil {
			return 0, fmt.Errorf("%s: insert order item: %w", fn, err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE orders
		SET total_price = (SELECT SUM(unit_price * quantity) FROM order_items WHERE order_id = $1)
		WHERE id = $1`,
		orderID); err != nil {
		return 0, fmt.Errorf("%s: update total price: %w", fn, err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO transactions (order_id, amount, status)
		SELECT id, total_price, $2 FROM orders WHERE id = $1`,
		orderID, models.TransactionStatusPaid); err != nil {
		return 0, fmt.Errorf("%s: insert transaction: %w", fn, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("%s: commit: %w", fn, err)
	}

	return orderID, nil
}

func reserveStock(ctx context.Context, tx pgx.Tx, item models.OrderItem) error {
	cmd, err := tx.Exec(ctx,
		`UPDATE products SET stock = stock - $1 WHERE id = $2 AND stock >= $1`,
		item.Quantity, item.ProductID)
	if err != nil {
		return fmt.Errorf("reserve stock: %w", err)
	}
	if cmd.RowsAffected() == 1 {
		return nil
	}

	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM products WHERE id = $1)`,
		item.ProductID).Scan(&exists); err != nil {
		return fmt.Errorf("reserve stock: %w", err)
	}
	if !exists {
		return &storage.ProductError{ProductID: item.ProductID, Err: storage.ErrNotFound}
	}
	return &storage.ProductError{ProductID: item.ProductID, Err: storage.ErrInsufficientStock}
}

func mergeOrderItems(items []models.OrderItem) ([]models.OrderItem, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: order has no items", storage.ErrInvalidInput)
	}

	quantities := make(map[int]int64, len(items))
	for _, item := range items {
		if item.ProductID <= 0 || item.Quantity <= 0 || item.Quantity > math.MaxInt32 {
			return nil, &storage.ProductError{
				ProductID: item.ProductID,
				Err:       fmt.Errorf("%w: quantity %d is out of range", storage.ErrInvalidInput, item.Quantity),
			}
		}

		total := quantities[item.ProductID] + int64(item.Quantity)
		if total > math.MaxInt32 {
			return nil, &storage.ProductError{
				ProductID: item.ProductID,
				Err:       fmt.Errorf("%w: total quantity %d exceeds %d", storage.ErrInvalidInput, total, math.MaxInt32),
			}
		}
		quantities[item.ProductID] = total
	}

	merged := make([]models.OrderItem, 0, len(quantities))
	for productID, quantity := range quantities {
		merged = append(merged, models.OrderItem{ProductID: productID, Quantity: int(quantity)})
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].ProductID < merged[j].ProductID })

	return merged, nil
}
