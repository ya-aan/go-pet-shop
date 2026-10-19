//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"
	"go-pet-shop/internal/storage/postgres"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	st  *postgres.Storage
	db  *pgxpool.Pool
	seq atomic.Int64
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		fmt.Println("TEST_DATABASE_URL is not set, skipping PostgreSQL integration tests")
		os.Exit(0)
	}

	if err := checkTestDatabaseURL(dsn, os.Getenv("DATABASE_URL")); err != nil {
		fmt.Println("refusing to run integration tests:", err)
		os.Exit(1)
	}

	if err := migrateUp(dsn); err != nil {
		fmt.Println("failed to apply migrations:", err)
		os.Exit(1)
	}

	ctx := context.Background()
	var err error
	if st, err = postgres.New(ctx, dsn); err != nil {
		fmt.Println("failed to init storage:", err)
		os.Exit(1)
	}
	if db, err = pgxpool.New(ctx, dsn); err != nil {
		fmt.Println("failed to connect:", err)
		os.Exit(1)
	}

	code := m.Run()

	db.Close()
	_ = st.Close()
	os.Exit(code)
}

func migrateUp(dsn string) error {
	m, err := migrate.New("file://../../../migrations", dsn)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

func cleanup(t *testing.T, query string, id int) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := db.Exec(context.Background(), query, id); err != nil {
			t.Errorf("cleanup %q (id=%d): %v", query, id, err)
		}
	})
}

func newUser(t *testing.T) models.User {
	t.Helper()
	u := models.User{
		Name:  "Integration User",
		Email: fmt.Sprintf("it-%d-%d@example.com", time.Now().UnixNano(), seq.Add(1)),
	}
	id, err := st.CreateUser(context.Background(), u)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	cleanup(t, `DELETE FROM users WHERE id = $1`, id)
	u.ID = id
	return u
}

func newProduct(t *testing.T, name string, price float64, stock int) models.Product {
	t.Helper()
	p := models.Product{Name: name, Price: price, Stock: stock}
	id, err := st.CreateProduct(context.Background(), p)
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	cleanup(t, `DELETE FROM products WHERE id = $1`, id)
	p.ID = id
	return p
}

func newOrder(t *testing.T, userID int) int {
	t.Helper()
	id, err := st.CreateOrder(context.Background(), models.Order{UserID: userID})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	cleanup(t, `DELETE FROM orders WHERE id = $1`, id)
	cleanup(t, `DELETE FROM order_items WHERE order_id = $1`, id)
	return id
}

func addItem(t *testing.T, orderID, productID, quantity int) {
	t.Helper()
	item := models.OrderItem{OrderID: orderID, ProductID: productID, Quantity: quantity}
	if err := st.AddOrderItem(context.Background(), item); err != nil {
		t.Fatalf("AddOrderItem(%+v): %v", item, err)
	}
}

func getOrder(t *testing.T, id int) models.Order {
	t.Helper()
	o, err := st.GetOrderByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetOrderByID(%d): %v", id, err)
	}
	return o
}

func getItems(t *testing.T, orderID int) []models.OrderItem {
	t.Helper()
	items, err := st.GetOrderItemsByOrderID(context.Background(), orderID)
	if err != nil {
		t.Fatalf("GetOrderItemsByOrderID(%d): %v", orderID, err)
	}
	return items
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	u := newUser(t)

	_, err := st.CreateUser(context.Background(), models.User{Name: "Other", Email: u.Email})
	if !errors.Is(err, storage.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	got, err := st.GetUserByEmail(context.Background(), u.Email)
	if err != nil || got != u {
		t.Fatalf("expected %+v, got %+v (err %v)", u, got, err)
	}
}

func TestGetUserByEmail_NotFound(t *testing.T) {
	_, err := st.GetUserByEmail(context.Background(), "nobody-integration@example.com")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetProductByID(t *testing.T) {
	p := newProduct(t, "IT Dog Food", 10.5, 5)

	got, err := st.GetProductByID(context.Background(), p.ID)
	if err != nil || got != p {
		t.Fatalf("expected %+v, got %+v (err %v)", p, got, err)
	}

	if _, err := st.GetProductByID(context.Background(), -1); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateOrder(t *testing.T) {
	u := newUser(t)
	before := time.Now().Add(-time.Minute)

	id := newOrder(t, u.ID)

	o := getOrder(t, id)
	if o.ID != id || o.UserID != u.ID || o.TotalPrice != 0 {
		t.Fatalf("unexpected new order: %+v", o)
	}
	if o.CreatedAt.Before(before) {
		t.Fatalf("created_at must be set by the database, got %v", o.CreatedAt)
	}
	if items := getItems(t, id); items == nil || len(items) != 0 {
		t.Fatalf("expected empty non-nil items, got %#v", items)
	}
}

func TestCreateOrder_UserNotFound(t *testing.T) {
	_, err := st.CreateOrder(context.Background(), models.Order{UserID: -1})
	if !errors.Is(err, storage.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestGetOrderByID_NotFound(t *testing.T) {
	if _, err := st.GetOrderByID(context.Background(), -1); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAddOrderItem_TotalPrice(t *testing.T) {
	u := newUser(t)
	food := newProduct(t, "IT Dog Food", 10.5, 100)
	toy := newProduct(t, "IT Cat Toy", 3.25, 100)
	orderID := newOrder(t, u.ID)

	addItem(t, orderID, food.ID, 2)
	addItem(t, orderID, toy.ID, 3)
	addItem(t, orderID, food.ID, 1)

	if got, want := getOrder(t, orderID).TotalPrice, 41.25; got != want {
		t.Fatalf("expected total_price %v, got %v", want, got)
	}

	items := getItems(t, orderID)
	want := []models.OrderItem{
		{OrderID: orderID, ProductID: food.ID, ProductName: food.Name, Quantity: 2, UnitPrice: 10.5},
		{OrderID: orderID, ProductID: toy.ID, ProductName: toy.Name, Quantity: 3, UnitPrice: 3.25},
		{OrderID: orderID, ProductID: food.ID, ProductName: food.Name, Quantity: 1, UnitPrice: 10.5},
	}
	if len(items) != len(want) {
		t.Fatalf("expected %d items, got %+v", len(want), items)
	}
	var sum float64
	for i := range items {
		if items[i].ID == 0 {
			t.Fatalf("item %d has no id: %+v", i, items[i])
		}
		want[i].ID = items[i].ID
		if items[i] != want[i] {
			t.Fatalf("item %d: expected %+v, got %+v", i, want[i], items[i])
		}
		sum += items[i].UnitPrice * float64(items[i].Quantity)
	}
	if sum != 41.25 {
		t.Fatalf("sum of items %v must match total_price 41.25", sum)
	}
}

func TestAddOrderItem_PriceSnapshot(t *testing.T) {
	ctx := context.Background()
	u := newUser(t)
	food := newProduct(t, "IT Dog Food", 10.5, 100)
	orderID := newOrder(t, u.ID)

	addItem(t, orderID, food.ID, 2)

	food.Price = 20
	if err := st.UpdateProduct(ctx, food); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}

	if got := getOrder(t, orderID).TotalPrice; got != 21 {
		t.Fatalf("old order total must stay 21 after price change, got %v", got)
	}
	if got := getItems(t, orderID)[0].UnitPrice; got != 10.5 {
		t.Fatalf("old item unit_price must stay 10.5, got %v", got)
	}

	addItem(t, orderID, food.ID, 1)
	if got := getOrder(t, orderID).TotalPrice; got != 41 {
		t.Fatalf("expected total 41 after adding item at new price, got %v", got)
	}
	if got := getItems(t, orderID)[1].UnitPrice; got != 20 {
		t.Fatalf("new item must use new price 20, got %v", got)
	}
}

func TestAddOrderItem_DoesNotChangeStock(t *testing.T) {
	u := newUser(t)
	food := newProduct(t, "IT Dog Food", 10.5, 5)
	orderID := newOrder(t, u.ID)

	addItem(t, orderID, food.ID, 3)

	got, err := st.GetProductByID(context.Background(), food.ID)
	if err != nil {
		t.Fatalf("GetProductByID: %v", err)
	}
	if got.Stock != 5 {
		t.Fatalf("stock must not change in v2, expected 5, got %d", got.Stock)
	}
}

func TestAddOrderItem_Errors(t *testing.T) {
	ctx := context.Background()
	u := newUser(t)
	food := newProduct(t, "IT Dog Food", 10.5, 100)
	orderID := newOrder(t, u.ID)
	addItem(t, orderID, food.ID, 1)

	tests := []struct {
		name    string
		item    models.OrderItem
		wantErr error
	}{
		{"product not found", models.OrderItem{OrderID: orderID, ProductID: -1, Quantity: 1}, storage.ErrInvalidInput},
		{"order not found", models.OrderItem{OrderID: -1, ProductID: food.ID, Quantity: 1}, storage.ErrNotFound},
		{"order and product not found", models.OrderItem{OrderID: -1, ProductID: -1, Quantity: 1}, storage.ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := st.AddOrderItem(ctx, tt.item); !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}

	if got := getOrder(t, orderID).TotalPrice; got != 10.5 {
		t.Fatalf("total_price must stay 10.5 after failed inserts, got %v", got)
	}
	if got := len(getItems(t, orderID)); got != 1 {
		t.Fatalf("expected 1 item after failed inserts, got %d", got)
	}
	var orphans int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM order_items WHERE order_id = -1`).Scan(&orphans); err != nil {
		t.Fatalf("count orphans: %v", err)
	}
	if orphans != 0 {
		t.Fatalf("expected no items for missing order, got %d", orphans)
	}
}

func TestAddOrderItem_Concurrent(t *testing.T) {
	u := newUser(t)
	p := newProduct(t, "IT Treat", 0.25, 1000)
	orderID := newOrder(t, u.ID)

	const workers = 40
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- st.AddOrderItem(context.Background(), models.OrderItem{OrderID: orderID, ProductID: p.ID, Quantity: 1})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("AddOrderItem: %v", err)
		}
	}

	if got, want := getOrder(t, orderID).TotalPrice, workers*0.25; got != want {
		t.Fatalf("expected total_price %v, got %v", want, got)
	}
	if got := len(getItems(t, orderID)); got != workers {
		t.Fatalf("expected %d items, got %d", workers, got)
	}
}

func TestGetOrdersByUserEmail(t *testing.T) {
	ctx := context.Background()
	alice := newUser(t)
	bob := newUser(t)
	food := newProduct(t, "IT Dog Food", 10.5, 100)

	first := newOrder(t, alice.ID)
	addItem(t, first, food.ID, 2)
	second := newOrder(t, alice.ID)
	newOrder(t, bob.ID)

	orders, err := st.GetOrdersByUserEmail(ctx, alice.Email)
	if err != nil {
		t.Fatalf("GetOrdersByUserEmail: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("expected 2 orders, got %+v", orders)
	}
	if orders[0].ID != second || orders[1].ID != first {
		t.Fatalf("expected orders [%d %d], got [%d %d]", second, first, orders[0].ID, orders[1].ID)
	}
	if orders[1].UserID != alice.ID || orders[1].TotalPrice != 21 || orders[1].CreatedAt.IsZero() {
		t.Fatalf("unexpected order data: %+v", orders[1])
	}

	t.Run("user without orders", func(t *testing.T) {
		carol := newUser(t)
		orders, err := st.GetOrdersByUserEmail(ctx, carol.Email)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if orders == nil || len(orders) != 0 {
			t.Fatalf("expected empty non-nil slice, got %#v", orders)
		}
	})

	t.Run("unknown user", func(t *testing.T) {
		_, err := st.GetOrdersByUserEmail(ctx, "nobody-integration@example.com")
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestDeleteProduct_UsedInOrder(t *testing.T) {
	ctx := context.Background()
	u := newUser(t)
	used := newProduct(t, "IT Dog Food", 10.5, 100)
	unused := newProduct(t, "IT Unused", 1, 1)
	orderID := newOrder(t, u.ID)
	addItem(t, orderID, used.ID, 1)

	if err := st.DeleteProduct(ctx, used.ID); !errors.Is(err, storage.ErrInUse) {
		t.Fatalf("expected ErrInUse, got %v", err)
	}
	if _, err := st.GetProductByID(ctx, used.ID); err != nil {
		t.Fatalf("product used in order must still exist: %v", err)
	}

	if err := st.DeleteProduct(ctx, unused.ID); err != nil {
		t.Fatalf("unused product must be deletable: %v", err)
	}
}
