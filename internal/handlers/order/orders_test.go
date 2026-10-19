package order

import (
	"encoding/json"
	"errors"
	"fmt"
	"go-pet-shop/internal/handlers/order/mocks"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi"
	"github.com/stretchr/testify/mock"
)

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type notFoundResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	ID      int    `json:"id"`
}

type createOrderResponse struct {
	Status string       `json:"status"`
	ID     int          `json:"id"`
	Order  models.Order `json:"order"`
}

type addItemResponse struct {
	Status    string `json:"status"`
	OrderID   int    `json:"order_id"`
	ProductID int    `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

func newRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Post("/orders", h.CreateOrder)
	r.Get("/orders/{id}", h.GetOrderByID)
	r.Post("/orders/{id}/items", h.AddOrderItem)
	r.Get("/users/{email}/orders", h.GetOrdersByUserEmail)
	r.Post("/checkout", h.Checkout)
	return r
}

func serve(t *testing.T, ordersMock *mocks.Orders, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	w := httptest.NewRecorder()
	newRouter(New(slog.Default(), ordersMock)).ServeHTTP(w, req)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("response body is not valid JSON: %v; body: %q", err, w.Body.String())
	}
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("expected status %d, got %d; body: %s", want, w.Code, w.Body.String())
	}
}

func assertErrorBody(t *testing.T, w *httptest.ResponseRecorder, wantError, wantMessage string) {
	t.Helper()
	var got errorResponse
	decodeBody(t, w, &got)
	want := errorResponse{Error: wantError, Message: wantMessage}
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}

func assertOrderNotFoundBody(t *testing.T, w *httptest.ResponseRecorder, id int) {
	t.Helper()
	var got notFoundResponse
	decodeBody(t, w, &got)
	want := notFoundResponse{
		Error:   "Not found",
		Message: fmt.Sprintf("Order with ID %d does not exist", id),
		ID:      id,
	}
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}

func TestCreateOrder_Success(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("CreateOrder", mock.Anything, models.Order{UserID: 3}).Return(15, nil)

	w := serve(t, ordersMock, http.MethodPost, "/orders",
		`{"user_id":3,"total_price":999,"created_at":"2020-01-01T00:00:00Z"}`)

	assertStatus(t, w, http.StatusCreated)
	var got createOrderResponse
	decodeBody(t, w, &got)
	want := createOrderResponse{
		Status: "Order created successfully",
		ID:     15,
		Order:  models.Order{ID: 15, UserID: 3, TotalPrice: 0},
	}
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
	if strings.Contains(w.Body.String(), "created_at") {
		t.Fatalf("created_at must be omitted until the order is read from storage; body: %s", w.Body.String())
	}
}

func TestCreateOrder_BadRequest(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{"invalid json", `{invalid-json`, "Invalid JSON payload"},
		{"missing user_id", `{}`, "User ID must be a positive number"},
		{"negative user_id", `{"user_id":-1}`, "User ID must be a positive number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ordersMock := mocks.NewOrders(t)

			w := serve(t, ordersMock, http.MethodPost, "/orders", tt.body)

			assertStatus(t, w, http.StatusBadRequest)
			assertErrorBody(t, w, "Bad request", tt.wantMessage)
		})
	}
}

func TestCreateOrder_UserNotFound(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("CreateOrder", mock.Anything, models.Order{UserID: 404}).
		Return(0, fmt.Errorf("storage.postgres.order: %w", storage.ErrInvalidInput))

	w := serve(t, ordersMock, http.MethodPost, "/orders", `{"user_id":404}`)

	assertStatus(t, w, http.StatusBadRequest)
	assertErrorBody(t, w, "Bad request", "User with ID 404 does not exist")
}

func TestCreateOrder_Fail(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("CreateOrder", mock.Anything, mock.Anything).Return(0, errors.New("DB error"))

	w := serve(t, ordersMock, http.MethodPost, "/orders", `{"user_id":1}`)

	assertStatus(t, w, http.StatusInternalServerError)
	assertErrorBody(t, w, "Internal server error", "Failed to create order")
}

func TestAddOrderItem_Success(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("AddOrderItem", mock.Anything, models.OrderItem{OrderID: 7, ProductID: 2, Quantity: 3}).
		Return(nil)

	w := serve(t, ordersMock, http.MethodPost, "/orders/7/items",
		`{"order_id":100,"product_id":2,"quantity":3,"unit_price":0.01}`)

	assertStatus(t, w, http.StatusCreated)
	var got addItemResponse
	decodeBody(t, w, &got)
	want := addItemResponse{Status: "Order item added successfully", OrderID: 7, ProductID: 2, Quantity: 3}
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}

func TestAddOrderItem_BadRequest(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		body        string
		wantMessage string
	}{
		{"invalid order id", "/orders/abc/items", `{"product_id":2,"quantity":1}`, "Order ID must be a number"},
		{"invalid json", "/orders/1/items", `{invalid-json`, "Invalid JSON payload"},
		{"missing product_id", "/orders/1/items", `{"quantity":1}`, "Product ID must be a positive number"},
		{"zero quantity", "/orders/1/items", `{"product_id":2,"quantity":0}`, "Quantity must be a positive number"},
		{"negative quantity", "/orders/1/items", `{"product_id":2,"quantity":-5}`, "Quantity must be a positive number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ordersMock := mocks.NewOrders(t)

			w := serve(t, ordersMock, http.MethodPost, tt.target, tt.body)

			assertStatus(t, w, http.StatusBadRequest)
			assertErrorBody(t, w, "Bad request", tt.wantMessage)
		})
	}
}

func TestAddOrderItem_OrderNotFound(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("AddOrderItem", mock.Anything, mock.Anything).
		Return(fmt.Errorf("storage.postgres.order: %w", storage.ErrNotFound))

	w := serve(t, ordersMock, http.MethodPost, "/orders/9/items", `{"product_id":2,"quantity":1}`)

	assertStatus(t, w, http.StatusNotFound)
	assertOrderNotFoundBody(t, w, 9)
}

func TestAddOrderItem_ProductNotFound(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("AddOrderItem", mock.Anything, mock.Anything).
		Return(fmt.Errorf("storage.postgres.order: %w", storage.ErrInvalidInput))

	w := serve(t, ordersMock, http.MethodPost, "/orders/1/items", `{"product_id":77,"quantity":1}`)

	assertStatus(t, w, http.StatusBadRequest)
	assertErrorBody(t, w, "Bad request", "Product with ID 77 does not exist")
}

func TestAddOrderItem_OrderPaid(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("AddOrderItem", mock.Anything, mock.Anything).
		Return(fmt.Errorf("storage.postgres.order: %w: order id=4", storage.ErrOrderPaid))

	w := serve(t, ordersMock, http.MethodPost, "/orders/4/items", `{"product_id":2,"quantity":1}`)

	assertStatus(t, w, http.StatusConflict)
	var got notFoundResponse
	decodeBody(t, w, &got)
	want := notFoundResponse{
		Error:   "Conflict",
		Message: "Order with ID 4 is already paid and cannot be changed",
		ID:      4,
	}
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}

func TestAddOrderItem_Fail(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("AddOrderItem", mock.Anything, mock.Anything).Return(errors.New("DB error"))

	w := serve(t, ordersMock, http.MethodPost, "/orders/1/items", `{"product_id":2,"quantity":1}`)

	assertStatus(t, w, http.StatusInternalServerError)
	assertErrorBody(t, w, "Internal server error", "Failed to add item to order")
}

func TestGetOrderByID_Success(t *testing.T) {
	createdAt := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	order := models.Order{ID: 5, UserID: 1, TotalPrice: 24.5, CreatedAt: createdAt}
	items := []models.OrderItem{
		{ID: 1, OrderID: 5, ProductID: 2, ProductName: "Dog Food", Quantity: 2, UnitPrice: 10.5},
		{ID: 2, OrderID: 5, ProductID: 3, ProductName: "Cat Toy", Quantity: 1, UnitPrice: 3.5},
	}
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("GetOrderByID", mock.Anything, 5).Return(order, nil)
	ordersMock.On("GetOrderItemsByOrderID", mock.Anything, 5).Return(items, nil)

	w := serve(t, ordersMock, http.MethodGet, "/orders/5", "")

	assertStatus(t, w, http.StatusOK)
	var got orderDetailsResponse
	decodeBody(t, w, &got)
	if got.Order != order || !slices.Equal(got.Items, items) {
		t.Fatalf("expected %+v with items %+v, got %+v", order, items, got)
	}
}

func TestGetOrderByID_EmptyItems(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("GetOrderByID", mock.Anything, 5).Return(models.Order{ID: 5, UserID: 1}, nil)
	ordersMock.On("GetOrderItemsByOrderID", mock.Anything, 5).Return([]models.OrderItem{}, nil)

	w := serve(t, ordersMock, http.MethodGet, "/orders/5", "")

	assertStatus(t, w, http.StatusOK)
	if !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("expected empty items array, got %s", w.Body.String())
	}
}

func TestGetOrderByID_BadRequest(t *testing.T) {
	ordersMock := mocks.NewOrders(t)

	w := serve(t, ordersMock, http.MethodGet, "/orders/abc", "")

	assertStatus(t, w, http.StatusBadRequest)
	assertErrorBody(t, w, "Bad request", "Order ID must be a number")
}

func TestGetOrderByID_NotFound(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("GetOrderByID", mock.Anything, 9).
		Return(models.Order{}, fmt.Errorf("storage.postgres.order: %w", storage.ErrNotFound))

	w := serve(t, ordersMock, http.MethodGet, "/orders/9", "")

	assertStatus(t, w, http.StatusNotFound)
	assertOrderNotFoundBody(t, w, 9)
}

func TestGetOrderByID_Fail(t *testing.T) {
	t.Run("order query fails", func(t *testing.T) {
		ordersMock := mocks.NewOrders(t)
		ordersMock.On("GetOrderByID", mock.Anything, 1).Return(models.Order{}, errors.New("DB error"))

		w := serve(t, ordersMock, http.MethodGet, "/orders/1", "")

		assertStatus(t, w, http.StatusInternalServerError)
		assertErrorBody(t, w, "Internal server error", "Failed to retrieve order")
	})

	t.Run("items query fails", func(t *testing.T) {
		ordersMock := mocks.NewOrders(t)
		ordersMock.On("GetOrderByID", mock.Anything, 1).Return(models.Order{ID: 1, UserID: 1}, nil)
		ordersMock.On("GetOrderItemsByOrderID", mock.Anything, 1).Return(nil, errors.New("DB error"))

		w := serve(t, ordersMock, http.MethodGet, "/orders/1", "")

		assertStatus(t, w, http.StatusInternalServerError)
		assertErrorBody(t, w, "Internal server error", "Failed to retrieve order")
	})
}

func TestGetOrdersByUserEmail_Success(t *testing.T) {
	want := []models.Order{
		{ID: 2, UserID: 1, TotalPrice: 7, CreatedAt: time.Date(2026, 10, 19, 13, 0, 0, 0, time.UTC)},
		{ID: 1, UserID: 1, TotalPrice: 21, CreatedAt: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)},
	}

	for _, target := range []string{"/users/alice@example.com/orders", "/users/alice%40example.com/orders"} {
		t.Run(target, func(t *testing.T) {
			ordersMock := mocks.NewOrders(t)
			ordersMock.On("GetOrdersByUserEmail", mock.Anything, "alice@example.com").Return(want, nil)

			w := serve(t, ordersMock, http.MethodGet, target, "")

			assertStatus(t, w, http.StatusOK)
			var got []models.Order
			decodeBody(t, w, &got)
			if !slices.Equal(got, want) {
				t.Fatalf("expected body %+v, got %+v", want, got)
			}
		})
	}
}

func TestGetOrdersByUserEmail_NoOrders(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("GetOrdersByUserEmail", mock.Anything, "alice@example.com").Return([]models.Order{}, nil)

	w := serve(t, ordersMock, http.MethodGet, "/users/alice@example.com/orders", "")

	assertStatus(t, w, http.StatusOK)
	if got := strings.TrimSpace(w.Body.String()); got != "[]" {
		t.Fatalf("expected empty JSON array, got %q", got)
	}
}

func TestGetOrdersByUserEmail_BadRequest(t *testing.T) {
	ordersMock := mocks.NewOrders(t)

	w := serve(t, ordersMock, http.MethodGet, "/users/not-an-email/orders", "")

	assertStatus(t, w, http.StatusBadRequest)
	assertErrorBody(t, w, "Bad request", "Invalid email format")
}

func TestGetOrdersByUserEmail_NotFound(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("GetOrdersByUserEmail", mock.Anything, "nobody@example.com").
		Return(nil, fmt.Errorf("storage.postgres.order: %w", storage.ErrNotFound))

	w := serve(t, ordersMock, http.MethodGet, "/users/nobody@example.com/orders", "")

	assertStatus(t, w, http.StatusNotFound)
	assertErrorBody(t, w, "Not found", "User with email nobody@example.com does not exist")
}

func TestGetOrdersByUserEmail_Fail(t *testing.T) {
	ordersMock := mocks.NewOrders(t)
	ordersMock.On("GetOrdersByUserEmail", mock.Anything, "alice@example.com").Return(nil, errors.New("DB error"))

	w := serve(t, ordersMock, http.MethodGet, "/users/alice@example.com/orders", "")

	assertStatus(t, w, http.StatusInternalServerError)
	assertErrorBody(t, w, "Internal server error", "Failed to retrieve user orders")
}
