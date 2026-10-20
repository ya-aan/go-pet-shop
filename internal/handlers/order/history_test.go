package order

import (
	"errors"
	"fmt"
	"go-pet-shop/internal/handlers/order/mocks"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
)

func TestGetUserOrderHistory_Success(t *testing.T) {
	paidAt := time.Date(2026, 10, 19, 13, 0, 0, 0, time.UTC)
	want := []models.OrderDetail{
		{
			Order: models.Order{ID: 2, UserID: 1, TotalPrice: 30.75, CreatedAt: paidAt},
			Items: []models.OrderItem{
				{ID: 3, OrderID: 2, ProductID: 1, ProductName: "Dog Food", Quantity: 2, UnitPrice: 10.5},
				{ID: 4, OrderID: 2, ProductID: 2, ProductName: "Cat Toy", Quantity: 3, UnitPrice: 3.25},
			},
			Transactions: []models.Transaction{
				{ID: 1, OrderID: 2, Amount: 30.75, Status: models.TransactionStatusPaid, CreatedAt: paidAt},
			},
		},
		{
			Order:        models.Order{ID: 1, UserID: 1, TotalPrice: 0, CreatedAt: paidAt.Add(-time.Hour)},
			Items:        []models.OrderItem{},
			Transactions: []models.Transaction{},
		},
	}

	for _, target := range []string{"/users/alice@example.com/history", "/users/alice%40example.com/history"} {
		t.Run(target, func(t *testing.T) {
			ordersMock := mocks.NewOrders(t)
			ordersMock.On("GetUserOrderHistory", mock.Anything, "alice@example.com").Return(want, nil)

			w := serve(t, ordersMock, http.MethodGet, target, "")

			assertStatus(t, w, http.StatusOK)
			var got []models.OrderDetail
			decodeBody(t, w, &got)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("expected body %+v, got %+v", want, got)
			}
			if !strings.Contains(w.Body.String(), `"items":[],"transactions":[]`) {
				t.Fatalf("draft order must have empty items and transactions arrays, got %s", w.Body.String())
			}
		})
	}
}

func TestGetUserOrderHistory_NoOrders(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("GetUserOrderHistory", mock.Anything, "alice@example.com").Return([]models.OrderDetail{}, nil)

	w := serve(t, ordersMock, http.MethodGet, "/users/alice@example.com/history", "")

	assertStatus(t, w, http.StatusOK)
	if got := strings.TrimSpace(w.Body.String()); got != "[]" {
		t.Fatalf("expected empty JSON array, got %q", got)
	}
}

func TestGetUserOrderHistory_BadRequest(t *testing.T) {
	ordersMock := mocks.NewOrders(t)

	w := serve(t, ordersMock, http.MethodGet, "/users/not-an-email/history", "")

	assertStatus(t, w, http.StatusBadRequest)
	assertErrorBody(t, w, "Bad request", "Invalid email format")
}

func TestGetUserOrderHistory_NotFound(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("GetUserOrderHistory", mock.Anything, "nobody@example.com").
		Return(nil, fmt.Errorf("storage.postgres.history: %w", storage.ErrNotFound))

	w := serve(t, ordersMock, http.MethodGet, "/users/nobody@example.com/history", "")

	assertStatus(t, w, http.StatusNotFound)
	assertErrorBody(t, w, "Not found", "User with email nobody@example.com does not exist")
}

func TestGetUserOrderHistory_Fail(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("GetUserOrderHistory", mock.Anything, "alice@example.com").Return(nil, errors.New("DB error"))

	w := serve(t, ordersMock, http.MethodGet, "/users/alice@example.com/history", "")

	assertStatus(t, w, http.StatusInternalServerError)
	assertErrorBody(t, w, "Internal server error", "Failed to retrieve order history")
}
