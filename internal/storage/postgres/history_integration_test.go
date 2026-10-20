//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"
	"slices"
	"testing"
)

func getHistory(t *testing.T, email string) []models.OrderDetail {
	t.Helper()
	history, err := st.GetUserOrderHistory(context.Background(), email)
	if err != nil {
		t.Fatalf("GetUserOrderHistory(%s): %v", email, err)
	}
	return history
}

func popularOf(t *testing.T, products ...models.Product) []models.PopularProduct {
	t.Helper()
	all, err := st.GetPopularProducts(context.Background())
	if err != nil {
		t.Fatalf("GetPopularProducts: %v", err)
	}
	ids := make(map[int]bool, len(products))
	for _, p := range products {
		ids[p.ID] = true
	}
	got := []models.PopularProduct{}
	for _, p := range all {
		if ids[p.ProductID] {
			got = append(got, p)
		}
	}
	return got
}

func TestGetUserOrderHistory(t *testing.T) {
	alice := newUser(t)
	bob := newUser(t)
	food := newProduct(t, "IT Dog Food", 10.5, 10)
	toy := newProduct(t, "IT Cat Toy", 3.25, 10)

	emptyDraft := newOrder(t, alice.ID)
	draft := newOrder(t, alice.ID)
	addItem(t, draft, toy.ID, 1)
	paid := placeOrder(t, alice.Email, item(toy.ID, 3), item(food.ID, 2))
	placeOrder(t, bob.Email, item(food.ID, 1))

	history := getHistory(t, alice.Email)

	var ids []int
	for _, o := range history {
		ids = append(ids, o.ID)
	}
	if want := []int{paid, draft, emptyDraft}; !slices.Equal(ids, want) {
		t.Fatalf("expected alice's orders %v (newest first), got %v", want, ids)
	}

	for _, o := range history {
		if o.Order != getOrder(t, o.ID) {
			t.Fatalf("order %d: expected %+v, got %+v", o.ID, getOrder(t, o.ID), o.Order)
		}
		if items := getItems(t, o.ID); o.Items == nil || !slices.Equal(o.Items, items) {
			t.Fatalf("order %d: expected items %+v, got %#v", o.ID, items, o.Items)
		}
	}

	if got := history[0].Transactions; len(got) != 1 {
		t.Fatalf("paid order: expected 1 transaction, got %+v", got)
	}
	tr := history[0].Transactions[0]
	if tr.ID == 0 || tr.OrderID != paid || tr.Amount != 30.75 || tr.Status != models.TransactionStatusPaid || tr.CreatedAt.IsZero() {
		t.Fatalf("paid order: unexpected transaction %+v", tr)
	}
	for _, o := range history[1:] {
		if o.Transactions == nil || len(o.Transactions) != 0 {
			t.Fatalf("draft %d: expected empty non-nil transactions, got %#v", o.ID, o.Transactions)
		}
	}
	if len(history[2].Items) != 0 {
		t.Fatalf("empty draft: expected no items, got %+v", history[2].Items)
	}
}

func TestGetUserOrderHistory_NoOrders(t *testing.T) {
	u := newUser(t)

	history := getHistory(t, u.Email)
	if history == nil || len(history) != 0 {
		t.Fatalf("expected empty non-nil history, got %#v", history)
	}
}

func TestGetUserOrderHistory_UserNotFound(t *testing.T) {
	_, err := st.GetUserOrderHistory(context.Background(), "nobody-integration@example.com")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetPopularProducts(t *testing.T) {
	ctx := context.Background()
	alice := newUser(t)
	bob := newUser(t)
	food := newProduct(t, "IT Dog Food", 10.5, 100)
	toy := newProduct(t, "IT Cat Toy", 3.25, 100)
	draftOnly := newProduct(t, "IT Draft Only", 1, 100)
	unsold := newProduct(t, "IT Unsold", 1, 100)

	placeOrder(t, alice.Email, item(food.ID, 2), item(toy.ID, 7))

	food.Price, food.Stock = 20, stockOf(t, food.ID)
	if err := st.UpdateProduct(ctx, food); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	placeOrder(t, bob.Email, item(food.ID, 3))

	draft := newOrder(t, alice.ID)
	addItem(t, draft, draftOnly.ID, 50)
	addItem(t, draft, food.ID, 50)

	got := popularOf(t, food, toy, draftOnly, unsold)
	want := []models.PopularProduct{
		{ProductID: toy.ID, Name: toy.Name, TotalSold: 7, OrdersCount: 1, Revenue: 22.75},
		{ProductID: food.ID, Name: food.Name, TotalSold: 5, OrdersCount: 2, Revenue: 81},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

func TestOrderWithSeveralTransactions(t *testing.T) {
	ctx := context.Background()
	u := newUser(t)
	p := newProduct(t, "IT Dog Food", 10.5, 10)
	orderID := placeOrder(t, u.Email, item(p.ID, 2))

	if _, err := db.Exec(ctx,
		`INSERT INTO transactions (order_id, amount, status) VALUES ($1, 21, $2)`,
		orderID, models.TransactionStatusPaid); err != nil {
		t.Fatalf("insert second transaction: %v", err)
	}

	history := getHistory(t, u.Email)
	if len(history) != 1 || len(history[0].Items) != 1 || len(history[0].Transactions) != 2 {
		t.Fatalf("expected 1 order with 1 item and 2 transactions, got %+v", history)
	}

	got := popularOf(t, p)
	want := []models.PopularProduct{{ProductID: p.ID, Name: p.Name, TotalSold: 2, OrdersCount: 1, Revenue: 21}}
	if !slices.Equal(got, want) {
		t.Fatalf("second transaction must not double sales: expected %+v, got %+v", want, got)
	}
}
