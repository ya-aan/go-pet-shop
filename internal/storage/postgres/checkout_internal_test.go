package postgres

import (
	"errors"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"
	"math"
	"slices"
	"testing"
)

func TestMergeOrderItems(t *testing.T) {
	got, err := mergeOrderItems([]models.OrderItem{
		{ProductID: 3, Quantity: 1},
		{ProductID: 1, Quantity: 2},
		{ProductID: 3, Quantity: 4},
		{ProductID: 2, Quantity: 1, UnitPrice: 0.01, OrderID: 99},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []models.OrderItem{
		{ProductID: 1, Quantity: 2},
		{ProductID: 2, Quantity: 1},
		{ProductID: 3, Quantity: 5},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

func TestMergeOrderItems_MaxQuantity(t *testing.T) {
	got, err := mergeOrderItems([]models.OrderItem{
		{ProductID: 1, Quantity: math.MaxInt32 - 1},
		{ProductID: 1, Quantity: 1},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Quantity != math.MaxInt32 {
		t.Fatalf("expected one item with quantity %d, got %+v", math.MaxInt32, got)
	}
}

func TestMergeOrderItems_Errors(t *testing.T) {
	tests := []struct {
		name          string
		items         []models.OrderItem
		wantProductID int
	}{
		{"no items", nil, 0},
		{"zero quantity", []models.OrderItem{{ProductID: 1, Quantity: 0}}, 1},
		{"negative quantity", []models.OrderItem{{ProductID: 1, Quantity: -1}}, 1},
		{"zero product id", []models.OrderItem{{ProductID: 0, Quantity: 1}}, 0},
		{"single quantity above int32", []models.OrderItem{{ProductID: 4, Quantity: math.MaxInt32 + 1}}, 4},
		{"merged quantity overflows int32", []models.OrderItem{
			{ProductID: 2, Quantity: 1},
			{ProductID: 7, Quantity: math.MaxInt32},
			{ProductID: 7, Quantity: 1},
		}, 7},
		{"many large duplicates", []models.OrderItem{
			{ProductID: 9, Quantity: math.MaxInt32},
			{ProductID: 9, Quantity: math.MaxInt32},
			{ProductID: 9, Quantity: math.MaxInt32},
		}, 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mergeOrderItems(tt.items)
			if !errors.Is(err, storage.ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}

			var productErr *storage.ProductError
			if tt.wantProductID == 0 {
				if errors.As(err, &productErr) && productErr.ProductID != 0 {
					t.Fatalf("expected error without product, got %v", err)
				}
				return
			}
			if !errors.As(err, &productErr) || productErr.ProductID != tt.wantProductID {
				t.Fatalf("expected ProductError for product %d, got %v", tt.wantProductID, err)
			}
		})
	}
}
