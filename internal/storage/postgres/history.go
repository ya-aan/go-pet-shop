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

const orderHistoryQuery = `
	SELECT o.id, o.total_price, o.created_at,
	       oi.id, oi.product_id, p.name, oi.quantity, oi.unit_price
	FROM orders o
	LEFT JOIN (order_items oi JOIN products p ON p.id = oi.product_id) ON oi.order_id = o.id
	WHERE o.user_id = $1
	ORDER BY o.created_at DESC, o.id DESC, oi.id`

const orderHistoryTransactionsQuery = `
	SELECT t.id, t.order_id, t.amount, t.status, t.created_at
	FROM transactions t
	JOIN orders o ON o.id = t.order_id
	WHERE o.user_id = $1
	ORDER BY t.created_at, t.id`

func (s *Storage) GetUserOrderHistory(ctx context.Context, email string) ([]models.OrderDetail, error) {
	const fn = "storage.postgres.history.GetUserOrderHistory"

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("%s: begin: %w", fn, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID int
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%s: %w: email=%s", fn, storage.ErrNotFound, email)
		}
		return nil, fmt.Errorf("%s: find user: %w", fn, err)
	}

	history, err := queryOrdersWithItems(ctx, tx, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	if err := attachTransactions(ctx, tx, userID, history); err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("%s: commit: %w", fn, err)
	}

	return history, nil
}

func queryOrdersWithItems(ctx context.Context, tx pgx.Tx, userID int) ([]models.OrderDetail, error) {
	rows, err := tx.Query(ctx, orderHistoryQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}
	defer rows.Close()

	history := []models.OrderDetail{}
	for rows.Next() {
		var (
			orderID     int
			totalPrice  float64
			createdAt   time.Time
			itemID      *int
			productID   *int
			productName *string
			quantity    *int
			unitPrice   *float64
		)
		if err := rows.Scan(&orderID, &totalPrice, &createdAt,
			&itemID, &productID, &productName, &quantity, &unitPrice); err != nil {
			return nil, fmt.Errorf("query orders: %w", err)
		}

		if n := len(history); n == 0 || history[n-1].ID != orderID {
			history = append(history, models.OrderDetail{
				Order:        models.Order{ID: orderID, UserID: userID, TotalPrice: totalPrice, CreatedAt: createdAt},
				Items:        []models.OrderItem{},
				Transactions: []models.Transaction{},
			})
		}

		if itemID == nil {
			continue
		}
		last := &history[len(history)-1]
		last.Items = append(last.Items, models.OrderItem{
			ID:          *itemID,
			OrderID:     orderID,
			ProductID:   *productID,
			ProductName: *productName,
			Quantity:    *quantity,
			UnitPrice:   *unitPrice,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}

	return history, nil
}

func attachTransactions(ctx context.Context, tx pgx.Tx, userID int, history []models.OrderDetail) error {
	byOrderID := make(map[int]int, len(history))
	for i, o := range history {
		byOrderID[o.ID] = i
	}

	rows, err := tx.Query(ctx, orderHistoryTransactionsQuery, userID)
	if err != nil {
		return fmt.Errorf("query transactions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t models.Transaction
		if err := rows.Scan(&t.ID, &t.OrderID, &t.Amount, &t.Status, &t.CreatedAt); err != nil {
			return fmt.Errorf("query transactions: %w", err)
		}
		if i, ok := byOrderID[t.OrderID]; ok {
			history[i].Transactions = append(history[i].Transactions, t)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("query transactions: %w", err)
	}

	return nil
}
