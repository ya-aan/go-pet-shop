package product

import (
	"bytes"
	"context"
	"errors"
	"go-pet-shop/internal/handlers/product/mocks"
	"go-pet-shop/internal/models"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi"
	"github.com/stretchr/testify/mock"
)

func withURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// Get Product - Ready
func TestGetAllProducts_Success(t *testing.T) {
	// Мокаем storage — он вернёт один продукт.
	productsMock := mocks.NewProducts(t)
	productsMock.On("GetAllProducts", mock.Anything).
		Return([]models.Product{{ID: 1, Name: "Dog Food"}}, nil)

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

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
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
}
