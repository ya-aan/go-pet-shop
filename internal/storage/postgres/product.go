package postgres

import (
	"context"
	"errors"
	"fmt"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"

	"github.com/jackc/pgx/v5"
)

// ❗ Памятка - Контекст не должен создаваться через context.Background() внутри методов.
// Нужно пробросить ctx из main.go (или из вызывающего слоя) до уровня storage.
// Иначе тайм-ауты и отмены не будут работать — все запросы всегда будут выполняться
// с “вечным” background-контекстом.
// GetAllProducts - получает все продукты
func (s *Storage) GetAllProducts(ctx context.Context) ([]models.Product, error) {
	const fn = "storage.postgres.product.GetAllProducts"

	rows, err := s.db.Query(ctx, `SELECT id, name, price, stock FROM products ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}
	defer rows.Close()

	products := []models.Product{}
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Price, &p.Stock); err != nil {
			return nil, fmt.Errorf("%s: %w", fn, err)
		}
		products = append(products, p)
	}

	// Проверяем ошибки итерации
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	return products, nil
}

func (s *Storage) GetProductByID(ctx context.Context, id int) (models.Product, error) {
	const fn = "storage.postgres.product.GetProductByID"

	var p models.Product
	err := s.db.QueryRow(ctx,
		`SELECT id, name, price, stock FROM products WHERE id = $1`,
		id).Scan(&p.ID, &p.Name, &p.Price, &p.Stock)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Product{}, fmt.Errorf("%s: %w: id=%d", fn, storage.ErrNotFound, id)
		}
		return models.Product{}, fmt.Errorf("%s: %w", fn, err)
	}

	return p, nil
}

// CreateProduct - создает продукт и возвращает его ID
func (s *Storage) CreateProduct(ctx context.Context, p models.Product) (int, error) {
	const fn = "storage.postgres.product.CreateProduct"

	var id int
	err := s.db.QueryRow(ctx,
		`INSERT INTO products (name, price, stock) VALUES ($1, $2, $3) RETURNING id`,
		p.Name, p.Price, p.Stock).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("%s: %w", fn, err)
	}

	return id, nil
}

// DeleteProduct - удаляет продукт по ID
func (s *Storage) DeleteProduct(ctx context.Context, id int) error {
	const fn = "storage.postgres.product.DeleteProduct"

	cmd, err := s.db.Exec(ctx,
		`DELETE FROM products WHERE id = $1`,
		id)
	if err != nil {
		if _, ok := foreignKeyViolation(err); ok {
			return fmt.Errorf("%s: %w: id=%d", fn, storage.ErrInUse, id)
		}
		return fmt.Errorf("%s: %w", fn, err)
	}

	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w: id=%d", fn, storage.ErrNotFound, id)
	}

	return nil
}

// UpdateProduct - обновляет продукт
func (s *Storage) UpdateProduct(ctx context.Context, p models.Product) error {
	const fn = "storage.postgres.product.UpdateProduct"

	cmd, err := s.db.Exec(ctx,
		`UPDATE products SET name = $1, price = $2, stock = $3 WHERE id = $4`,
		p.Name, p.Price, p.Stock, p.ID)
	if err != nil {
		return fmt.Errorf("%s: %w", fn, err)
	}

	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w: id=%d", fn, storage.ErrNotFound, p.ID)
	}

	return nil
}
