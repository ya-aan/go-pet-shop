package storage

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrAlreadyExists     = errors.New("already exists")
	ErrInvalidInput      = errors.New("invalid input")
	ErrInUse             = errors.New("in use")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrOrderPaid         = errors.New("order is already paid")
)

type ProductError struct {
	ProductID int
	Err       error
}

func (e *ProductError) Error() string {
	return fmt.Sprintf("product id=%d: %v", e.ProductID, e.Err)
}

func (e *ProductError) Unwrap() error {
	return e.Err
}
