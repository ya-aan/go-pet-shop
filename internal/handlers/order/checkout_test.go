package order

import (
	"errors"
	"fmt"
	"go-pet-shop/internal/handlers/order/mocks"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"
	"math"
	"net/http"
	"testing"

	"github.com/stretchr/testify/mock"
)

type checkoutResponse struct {
	Status  string `json:"status"`
	OrderID int    `json:"order_id"`
}

type conflictResponse struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	ProductID int    `json:"product_id"`
}

func TestCheckout_Success(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	wantItems := []models.OrderItem{
		{ProductID: 2, Quantity: 1},
		{ProductID: 1, Quantity: 2},
		{ProductID: 2, Quantity: 3},
	}
	ordersMock.On("PlaceOrder", mock.Anything, "alice@example.com", wantItems).Return(17, nil)

	w := serve(t, ordersMock, http.MethodPost, "/checkout", `{
		"user_email": " alice@example.com ",
		"items": [
			{"product_id": 2, "quantity": 1, "unit_price": 0.01, "order_id": 99},
			{"product_id": 1, "quantity": 2},
			{"product_id": 2, "quantity": 3}
		]
	}`)

	assertStatus(t, w, http.StatusCreated)
	var got checkoutResponse
	decodeBody(t, w, &got)
	want := checkoutResponse{Status: "Order placed successfully", OrderID: 17}
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}

func TestCheckout_BadRequest(t *testing.T) {
	maxQty := fmt.Sprintf("Quantity must be between 1 and %d", math.MaxInt32)

	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{"invalid json", `{invalid-json`, "Invalid JSON payload"},
		{"quantity overflows int", `{"user_email":"a@example.com","items":[{"product_id":1,"quantity":99999999999999999999}]}`, "Invalid JSON payload"},
		{"missing email", `{"items":[{"product_id":1,"quantity":1}]}`, "User email is required"},
		{"invalid email", `{"user_email":"not-an-email","items":[{"product_id":1,"quantity":1}]}`, "Invalid email format"},
		{"missing items", `{"user_email":"a@example.com"}`, "Order must contain at least one item"},
		{"empty items", `{"user_email":"a@example.com","items":[]}`, "Order must contain at least one item"},
		{"missing product_id", `{"user_email":"a@example.com","items":[{"quantity":1}]}`, "Product ID must be a positive number"},
		{"negative product_id", `{"user_email":"a@example.com","items":[{"product_id":-3,"quantity":1}]}`, "Product ID must be a positive number"},
		{"zero quantity", `{"user_email":"a@example.com","items":[{"product_id":1,"quantity":0}]}`, maxQty},
		{"negative quantity", `{"user_email":"a@example.com","items":[{"product_id":1,"quantity":-1}]}`, maxQty},
		{"quantity above int32", `{"user_email":"a@example.com","items":[{"product_id":1,"quantity":2147483648}]}`, maxQty},
		{"bad item after good one", `{"user_email":"a@example.com","items":[{"product_id":1,"quantity":1},{"product_id":2,"quantity":0}]}`, maxQty},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ordersMock := mocks.NewOrders(t)

			w := serve(t, ordersMock, http.MethodPost, "/checkout", tt.body)

			assertStatus(t, w, http.StatusBadRequest)
			assertErrorBody(t, w, "Bad request", tt.wantMessage)
		})
	}
}

func TestCheckout_StorageErrors(t *testing.T) {
	const body = `{"user_email":"alice@example.com","items":[{"product_id":5,"quantity":2}]}`

	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantError   string
		wantMessage string
	}{
		{
			name:        "user not found",
			err:         fmt.Errorf("storage.postgres.checkout.PlaceOrder: user %w", storage.ErrNotFound),
			wantStatus:  http.StatusBadRequest,
			wantError:   "Bad request",
			wantMessage: "User with email alice@example.com does not exist",
		},
		{
			name:        "product not found",
			err:         fmt.Errorf("storage.postgres.checkout.PlaceOrder: %w", &storage.ProductError{ProductID: 5, Err: storage.ErrNotFound}),
			wantStatus:  http.StatusBadRequest,
			wantError:   "Bad request",
			wantMessage: "Product with ID 5 does not exist",
		},
		{
			name: "merged quantity overflow",
			err: fmt.Errorf("storage.postgres.checkout.PlaceOrder: %w", &storage.ProductError{
				ProductID: 5, Err: fmt.Errorf("%w: total quantity exceeds", storage.ErrInvalidInput),
			}),
			wantStatus:  http.StatusBadRequest,
			wantError:   "Bad request",
			wantMessage: fmt.Sprintf("Total quantity for product with ID 5 must be between 1 and %d", math.MaxInt32),
		},
		{
			name:        "database error",
			err:         errors.New("DB error"),
			wantStatus:  http.StatusInternalServerError,
			wantError:   "Internal server error",
			wantMessage: "Failed to place order",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ordersMock := mocks.NewOrders(t)
			ordersMock.On("PlaceOrder", mock.Anything, "alice@example.com", mock.Anything).Return(0, tt.err)

			w := serve(t, ordersMock, http.MethodPost, "/checkout", body)

			assertStatus(t, w, tt.wantStatus)
			assertErrorBody(t, w, tt.wantError, tt.wantMessage)
		})
	}
}

func TestCheckout_InsufficientStock(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("PlaceOrder", mock.Anything, "alice@example.com", mock.Anything).
		Return(0, fmt.Errorf("storage.postgres.checkout.PlaceOrder: %w",
			&storage.ProductError{ProductID: 5, Err: storage.ErrInsufficientStock}))

	w := serve(t, ordersMock, http.MethodPost, "/checkout",
		`{"user_email":"alice@example.com","items":[{"product_id":5,"quantity":2}]}`)

	assertStatus(t, w, http.StatusConflict)
	var got conflictResponse
	decodeBody(t, w, &got)
	want := conflictResponse{Error: "Conflict", Message: "Not enough stock for product with ID 5", ProductID: 5}
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}
