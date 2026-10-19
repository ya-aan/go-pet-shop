package postgres

import (
	"context"
	"errors"
	"fmt"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// CreateUser - создает пользователя и возвращает его ID
func (s *Storage) CreateUser(ctx context.Context, u models.User) (int, error) {
	const fn = "storage.postgres.user.CreateUser"

	var id int
	err := s.db.QueryRow(ctx,
		`INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id`,
		u.Name, u.Email).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return 0, fmt.Errorf("%s: %w: email=%s", fn, storage.ErrAlreadyExists, u.Email)
		}
		return 0, fmt.Errorf("%s: %w", fn, err)
	}

	return id, nil
}

// GetUserByEmail - получает пользователя по email
func (s *Storage) GetUserByEmail(ctx context.Context, email string) (models.User, error) {
	const fn = "storage.postgres.user.GetUserByEmail"

	var u models.User
	err := s.db.QueryRow(ctx,
		`SELECT id, name, email FROM users WHERE email = $1`,
		email).Scan(&u.ID, &u.Name, &u.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.User{}, fmt.Errorf("%s: %w: email=%s", fn, storage.ErrNotFound, email)
		}
		return models.User{}, fmt.Errorf("%s: %w", fn, err)
	}

	return u, nil
}

// GetAllUsers - получает всех пользователей
func (s *Storage) GetAllUsers(ctx context.Context) ([]models.User, error) {
	const fn = "storage.postgres.user.GetAllUsers"

	rows, err := s.db.Query(ctx, `SELECT id, name, email FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}
	defer rows.Close()

	// Пустой срез, а не nil: пустой список отдаётся в JSON как [], а не null
	users := []models.User{}
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email); err != nil {
			return nil, fmt.Errorf("%s: %w", fn, err)
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", fn, err)
	}

	return users, nil
}
