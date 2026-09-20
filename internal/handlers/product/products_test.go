package product

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go-pet-shop/internal/models"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi"
)

// Формы JSON-ответов хендлеров.
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type productResponse struct {
	Status  string         `json:"status"`
	ID      int            `json:"id"`
	Product models.Product `json:"product"`
}

type deleteResponse struct {
	Status  string `json:"status"`
	ID      int    `json:"id"`
	Message string `json:"message"`
}

func withURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("response body is not valid JSON: %v; body: %q", err, w.Body.String())
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

// Get Product - Ready
func TestGetAllProducts_Success(t *testing.T) {
	// Мокаем storage — он вернёт один продукт.
	want := []models.Product{{ID: 1, Name: "Dog Food", Price: 10.5, Stock: 5}}
	mock := &ProductsMock{
		GetAllProductsFunc: func(ctx context.Context) ([]models.Product, error) {
			return want, nil
		},
	}

	// Создаем HTTP-запрос GET /products
	req := httptest.NewRequest(http.MethodGet, "/products", nil)
	w := httptest.NewRecorder()

	// Создаем хендлер с мок-хранилищем
	handler := New(slog.Default(), mock)

	// Вызываем метод GetAllProducts, который является http.HandlerFunc
	handler.GetAllProducts(w, req)

	// Проверяем HTTP-код
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	// Проверяем тело ответа
	var got []models.Product
	decodeBody(t, w, &got)
	if !slices.Equal(got, want) {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}
func TestGetAllProducts_Error(t *testing.T) {
	// Мокаем storage — он будет возвращать ошибку
	mock := &ProductsMock{
		GetAllProductsFunc: func(ctx context.Context) ([]models.Product, error) {
			return nil, errors.New("DB error")
		},
	}

	// Создаем запрос
	req := httptest.NewRequest(http.MethodGet, "/products", nil)
	w := httptest.NewRecorder()

	handler := New(slog.Default(), mock)
	handler.GetAllProducts(w, req)

	// Ожидаем HTTP 500
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	assertErrorBody(t, w, "Internal server error", "Failed to retrieve products")
}

// =======================
// Create Product
// =======================

func TestCreateProduct_Success(t *testing.T) {
	mock := &ProductsMock{
		CreateProductFunc: func(ctx context.Context, product models.Product) (int, error) {
			return 42, nil
		},
	}

	body := bytes.NewBufferString(`{"name":"Dog Food","price":10.5,"stock":5}`)
	req := httptest.NewRequest(http.MethodPost, "/products", body)
	w := httptest.NewRecorder()

	handler := New(slog.Default(), mock)
	handler.CreateProduct(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var got productResponse
	decodeBody(t, w, &got)
	want := productResponse{
		Status:  "Product created successfully",
		ID:      42,
		Product: models.Product{ID: 42, Name: "Dog Food", Price: 10.5, Stock: 5},
	}
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}

func TestCreateProduct_BadRequest(t *testing.T) {
	mock := &ProductsMock{}

	body := strings.NewReader(`{invalid-json`)
	req := httptest.NewRequest(http.MethodPost, "/products", body)
	w := httptest.NewRecorder()

	handler := New(slog.Default(), mock)
	handler.CreateProduct(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	assertErrorBody(t, w, "Bad request", "Invalid JSON payload")
}

func TestCreateProduct_Fail(t *testing.T) {
	mock := &ProductsMock{
		CreateProductFunc: func(ctx context.Context, product models.Product) (int, error) {
			return 0, errors.New("DB error")
		},
	}

	body := bytes.NewBufferString(`{"name":"Dog Food","price":10.5,"stock":5}`)
	req := httptest.NewRequest(http.MethodPost, "/products", body)
	w := httptest.NewRecorder()

	handler := New(slog.Default(), mock)
	handler.CreateProduct(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	assertErrorBody(t, w, "Internal server error", "Failed to create product")
}

// =======================
// Update Product
// =======================

func TestUpdateProduct_Success(t *testing.T) {
	mock := &ProductsMock{
		UpdateProductFunc: func(ctx context.Context, product models.Product) error {
			return nil
		},
	}

	body := bytes.NewBufferString(`{"name":"Dog Food","price":10.5,"stock":5}`)
	req := httptest.NewRequest(http.MethodPut, "/products/1", body)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), mock)
	handler.UpdateProduct(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	// ID в теле ответа берётся из URL, в JSON запроса его нет.
	var got productResponse
	decodeBody(t, w, &got)
	want := productResponse{
		Status:  "Product updated successfully",
		ID:      1,
		Product: models.Product{ID: 1, Name: "Dog Food", Price: 10.5, Stock: 5},
	}
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}

func TestUpdateProduct_BadRequest(t *testing.T) {
	mock := &ProductsMock{}

	body := strings.NewReader(`{invalid-json`)
	req := httptest.NewRequest(http.MethodPut, "/products/1", body)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), mock)
	handler.UpdateProduct(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	assertErrorBody(t, w, "Bad request", "Invalid JSON payload")
}

func TestUpdateProduct_Fail(t *testing.T) {
	mock := &ProductsMock{
		UpdateProductFunc: func(ctx context.Context, product models.Product) error {
			return errors.New("DB error")
		},
	}

	body := bytes.NewBufferString(`{"name":"Dog Food","price":10.5,"stock":5}`)
	req := httptest.NewRequest(http.MethodPut, "/products/1", body)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), mock)
	handler.UpdateProduct(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	assertErrorBody(t, w, "Internal server error", "Failed to update product")
}

// =======================
// Delete Product
// =======================

func TestDeleteProduct_Success(t *testing.T) {
	mock := &ProductsMock{
		DeleteProductFunc: func(ctx context.Context, id int) error {
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/products/1", nil)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), mock)
	handler.DeleteProduct(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var got deleteResponse
	decodeBody(t, w, &got)
	want := deleteResponse{
		Status:  "Product deleted successfully",
		ID:      1,
		Message: "Product with ID 1 has been deleted",
	}
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}

func TestDeleteProduct_BadRequest(t *testing.T) {
	mock := &ProductsMock{}

	// Пустой id: без chi route context chi.URLParam вернёт "".
	req := httptest.NewRequest(http.MethodDelete, "/products/", nil)
	w := httptest.NewRecorder()

	handler := New(slog.Default(), mock)
	handler.DeleteProduct(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	assertErrorBody(t, w, "Bad request", "Product ID is required")
}

func TestDeleteProduct_Fail(t *testing.T) {
	mock := &ProductsMock{
		DeleteProductFunc: func(ctx context.Context, id int) error {
			return errors.New("DB error")
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/products/1", nil)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), mock)
	handler.DeleteProduct(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	assertErrorBody(t, w, "Internal server error", "Failed to delete product")
}
