package product

import (
	"errors"
	"go-pet-shop/internal/handlers/product/mocks"
	"go-pet-shop/internal/models"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi"
	"github.com/stretchr/testify/mock"
)

func servePopular(t *testing.T, productsMock *mocks.Products) *httptest.ResponseRecorder {
	t.Helper()
	h := New(slog.Default(), productsMock)
	r := chi.NewRouter()
	r.Get("/products/{id}", h.GetProductByID)
	r.Get("/products/popular", h.GetPopularProducts)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/products/popular", nil))
	return w
}

func TestGetPopularProducts_Success(t *testing.T) {
	want := []models.PopularProduct{
		{ProductID: 2, Name: "Cat Toy", TotalSold: 7, OrdersCount: 1, Revenue: 22.75},
		{ProductID: 1, Name: "Dog Food", TotalSold: 5, OrdersCount: 2, Revenue: 52.5},
	}
	productsMock := mocks.NewProducts(t)
	productsMock.On("GetPopularProducts", mock.Anything).Return(want, nil)

	w := servePopular(t, productsMock)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body: %s", w.Code, w.Body.String())
	}
	var got []models.PopularProduct
	decodeBody(t, w, &got)
	if !slices.Equal(got, want) {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}

func TestGetPopularProducts_NoSales(t *testing.T) {
	productsMock := mocks.NewProducts(t)
	productsMock.On("GetPopularProducts", mock.Anything).Return([]models.PopularProduct{}, nil)

	w := servePopular(t, productsMock)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "[]" {
		t.Fatalf("expected empty JSON array, got %q", got)
	}
}

func TestGetPopularProducts_Fail(t *testing.T) {
	productsMock := mocks.NewProducts(t)
	productsMock.On("GetPopularProducts", mock.Anything).Return(nil, errors.New("DB error"))

	w := servePopular(t, productsMock)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	assertErrorBody(t, w, "Internal server error", "Failed to retrieve popular products")
}
