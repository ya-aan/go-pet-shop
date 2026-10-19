package product

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go-pet-shop/internal/handlers/product/mocks"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi"
	"github.com/stretchr/testify/mock"
)

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type productResponse struct {
	Status  string         `json:"status"`
	ID      int            `json:"id"`
	Product models.Product `json:"product"`
}

// notFoundResponse - тело ошибки с ID товара (404 и 409)
type notFoundResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	ID      int    `json:"id"`
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

// errNotFound - ошибка в том виде, в каком её возвращает storage: обёрнутый storage.ErrNotFound
func errNotFound(id int) error {
	return fmt.Errorf("storage.postgres.product: %w: id=%d", storage.ErrNotFound, id)
}

func assertNotFoundBody(t *testing.T, w *httptest.ResponseRecorder, id int) {
	t.Helper()
	var got notFoundResponse
	decodeBody(t, w, &got)
	want := notFoundResponse{
		Error:   "Not found",
		Message: fmt.Sprintf("Product with ID %d does not exist", id),
		ID:      id,
	}
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
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
	productsMock := mocks.NewProducts(t)
	productsMock.On("GetAllProducts", mock.Anything).
		Return(want, nil)

	// Создаем HTTP-запрос GET /products
	req := httptest.NewRequest(http.MethodGet, "/products", nil)
	w := httptest.NewRecorder()

	// Создаем хендлер с мок-хранилищем
	handler := New(slog.Default(), productsMock)

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
	productsMock := mocks.NewProducts(t)
	productsMock.On("GetAllProducts", mock.Anything).
		Return(nil, errors.New("DB error"))

	// Создаем запрос
	req := httptest.NewRequest(http.MethodGet, "/products", nil)
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
	handler.GetAllProducts(w, req)

	// Ожидаем HTTP 500
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	assertErrorBody(t, w, "Internal server error", "Failed to retrieve products")
}

func TestGetProductByID_Success(t *testing.T) {
	want := models.Product{ID: 7, Name: "Cat Toy", Price: 3.25, Stock: 12}
	productsMock := mocks.NewProducts(t)
	productsMock.On("GetProductByID", mock.Anything, 7).
		Return(want, nil)

	req := httptest.NewRequest(http.MethodGet, "/products/7", nil)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
	handler.GetProductByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var got models.Product
	decodeBody(t, w, &got)
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}

func TestGetProductByID_NotFound(t *testing.T) {
	productsMock := mocks.NewProducts(t)
	productsMock.On("GetProductByID", mock.Anything, 99).
		Return(models.Product{}, errNotFound(99))

	req := httptest.NewRequest(http.MethodGet, "/products/99", nil)
	req = withURLParam(req, "id", "99")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
	handler.GetProductByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}

	assertNotFoundBody(t, w, 99)
}

func TestGetProductByID_BadRequest(t *testing.T) {
	// Storage не должен вызываться: мок без ожиданий завалит тест на любом вызове.
	productsMock := mocks.NewProducts(t)

	req := httptest.NewRequest(http.MethodGet, "/products/abc", nil)
	req = withURLParam(req, "id", "abc")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
	handler.GetProductByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	assertErrorBody(t, w, "Bad request", "Product ID must be a number")
}

func TestGetProductByID_Fail(t *testing.T) {
	productsMock := mocks.NewProducts(t)
	productsMock.On("GetProductByID", mock.Anything, 1).
		Return(models.Product{}, errors.New("DB error"))

	req := httptest.NewRequest(http.MethodGet, "/products/1", nil)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
	handler.GetProductByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	assertErrorBody(t, w, "Internal server error", "Failed to retrieve product")
}

// =======================
// Create Product
// =======================

func TestCreateProduct_Success(t *testing.T) {
	productsMock := mocks.NewProducts(t)
	productsMock.On("CreateProduct", mock.Anything, mock.Anything).
		Return(42, nil)

	body := bytes.NewBufferString(`{"name":"Dog Food","price":10.5,"stock":5}`)
	req := httptest.NewRequest(http.MethodPost, "/products", body)
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
	handler.CreateProduct(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
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
	productsMock := mocks.NewProducts(t)

	body := strings.NewReader(`{invalid-json`)
	req := httptest.NewRequest(http.MethodPost, "/products", body)
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
	handler.CreateProduct(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	assertErrorBody(t, w, "Bad request", "Invalid JSON payload")
}

func TestCreateProduct_Validation(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{"empty name", `{"name":"","price":10.5,"stock":5}`, "Product name is required"},
		{"negative price", `{"name":"Dog Food","price":-1,"stock":5}`, "Product price cannot be negative"},
		{"negative stock", `{"name":"Dog Food","price":10.5,"stock":-1}`, "Product stock cannot be negative"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			productsMock := mocks.NewProducts(t)

			req := httptest.NewRequest(http.MethodPost, "/products", strings.NewReader(tt.body))
			w := httptest.NewRecorder()

			handler := New(slog.Default(), productsMock)
			handler.CreateProduct(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", w.Code)
			}

			assertErrorBody(t, w, "Bad request", tt.wantMessage)
		})
	}
}

func TestCreateProduct_Fail(t *testing.T) {
	productsMock := mocks.NewProducts(t)
	productsMock.On("CreateProduct", mock.Anything, mock.Anything).
		Return(0, errors.New("DB error"))

	body := bytes.NewBufferString(`{"name":"Dog Food","price":10.5,"stock":5}`)
	req := httptest.NewRequest(http.MethodPost, "/products", body)
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
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
	productsMock := mocks.NewProducts(t)
	productsMock.On("UpdateProduct", mock.Anything, mock.Anything).
		Return(nil)

	body := bytes.NewBufferString(`{"name":"Dog Food","price":10.5,"stock":5}`)
	req := httptest.NewRequest(http.MethodPut, "/products/1", body)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
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
	productsMock := mocks.NewProducts(t)

	body := strings.NewReader(`{invalid-json`)
	req := httptest.NewRequest(http.MethodPut, "/products/1", body)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
	handler.UpdateProduct(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	assertErrorBody(t, w, "Bad request", "Invalid JSON payload")
}

func TestUpdateProduct_NotFound(t *testing.T) {
	productsMock := mocks.NewProducts(t)
	productsMock.On("UpdateProduct", mock.Anything, mock.Anything).
		Return(errNotFound(5))

	body := bytes.NewBufferString(`{"name":"Dog Food","price":10.5,"stock":5}`)
	req := httptest.NewRequest(http.MethodPut, "/products/5", body)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
	handler.UpdateProduct(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}

	assertNotFoundBody(t, w, 5)
}

func TestUpdateProduct_Fail(t *testing.T) {
	productsMock := mocks.NewProducts(t)
	productsMock.On("UpdateProduct", mock.Anything, mock.Anything).
		Return(errors.New("DB error"))

	body := bytes.NewBufferString(`{"name":"Dog Food","price":10.5,"stock":5}`)
	req := httptest.NewRequest(http.MethodPut, "/products/1", body)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
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
	productsMock := mocks.NewProducts(t)
	productsMock.On("DeleteProduct", mock.Anything, mock.Anything).
		Return(nil)

	req := httptest.NewRequest(http.MethodDelete, "/products/1", nil)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
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
	productsMock := mocks.NewProducts(t)

	// Пустой id: без chi route context chi.URLParam вернёт "".
	req := httptest.NewRequest(http.MethodDelete, "/products/", nil)
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
	handler.DeleteProduct(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	assertErrorBody(t, w, "Bad request", "Product ID is required")
}

func TestDeleteProduct_NotFound(t *testing.T) {
	productsMock := mocks.NewProducts(t)
	productsMock.On("DeleteProduct", mock.Anything, 5).
		Return(errNotFound(5))

	req := httptest.NewRequest(http.MethodDelete, "/products/5", nil)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
	handler.DeleteProduct(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", w.Code)
	}

	assertNotFoundBody(t, w, 5)
}

func TestDeleteProduct_InUse(t *testing.T) {
	productsMock := mocks.NewProducts(t)
	productsMock.On("DeleteProduct", mock.Anything, 3).
		Return(fmt.Errorf("storage.postgres.product: %w: id=3", storage.ErrInUse))

	req := httptest.NewRequest(http.MethodDelete, "/products/3", nil)
	req = withURLParam(req, "id", "3")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
	handler.DeleteProduct(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", w.Code)
	}

	var got notFoundResponse
	decodeBody(t, w, &got)
	want := notFoundResponse{
		Error:   "Conflict",
		Message: "Product with ID 3 is used in orders and cannot be deleted",
		ID:      3,
	}
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}

func TestDeleteProduct_Fail(t *testing.T) {
	productsMock := mocks.NewProducts(t)
	productsMock.On("DeleteProduct", mock.Anything, mock.Anything).
		Return(errors.New("DB error"))

	req := httptest.NewRequest(http.MethodDelete, "/products/1", nil)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()

	handler := New(slog.Default(), productsMock)
	handler.DeleteProduct(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	assertErrorBody(t, w, "Internal server error", "Failed to delete product")
}
