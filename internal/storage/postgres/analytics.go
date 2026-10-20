package postgres

import (
	"context"
	"fmt"
	"go-pet-shop/internal/models"
)

const popularProductsQuery = `
	SELECT p.id, p.name,
	       SUM(oi.quantity) AS total_sold,
	       COUNT(DISTINCT oi.order_id) AS orders_count,
	       SUM(oi.unit_price * oi.quantity) AS revenue
	FROM order_items oi
	JOIN products p ON p.id = oi.product_id
	WHERE EXISTS (
	    SELECT 1 FROM transactions t
	    WHERE t.order_id = oi.order_id AND t.status = $1
	)
	GROUP BY p.id, p.name
	ORDER BY total_sold DESC, p.id`

func (s *Storage) GetPopularProducts(ctx context.Context) ([]models.PopularProduct, error) {
	const fn = "storage.postgres.analytics.GetPopularProducts"

	rows, err := s.db.Query(ctx, popularProductsQuery, models.TransactionStatusPaid)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}
	defer rows.Close()

	products := []models.PopularProduct{}
	for rows.Next() {
		var p models.PopularProduct
		if err := rows.Scan(&p.ProductID, &p.Name, &p.TotalSold, &p.OrdersCount, &p.Revenue); err != nil {
			return nil, fmt.Errorf("%s: %w", fn, err)
		}
		products = append(products, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	return products, nil
}
