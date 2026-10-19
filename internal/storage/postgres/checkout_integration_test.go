//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func cleanupPlacedOrder(t *testing.T, orderID int) {
	t.Helper()
	cleanup(t, `DELETE FROM orders WHERE id = $1`, orderID)
	cleanup(t, `DELETE FROM order_items WHERE order_id = $1`, orderID)
	cleanup(t, `DELETE FROM transactions WHERE order_id = $1`, orderID)
}

func placeOrder(t *testing.T, email string, items ...models.OrderItem) int {
	t.Helper()
	id, err := st.PlaceOrder(context.Background(), email, items)
	if err != nil {
		t.Fatalf("PlaceOrder(%s, %+v): %v", email, items, err)
	}
	cleanupPlacedOrder(t, id)
	return id
}

func item(productID, quantity int) models.OrderItem {
	return models.OrderItem{ProductID: productID, Quantity: quantity}
}

func stockOf(t *testing.T, productID int) int {
	t.Helper()
	p, err := st.GetProductByID(context.Background(), productID)
	if err != nil {
		t.Fatalf("GetProductByID(%d): %v", productID, err)
	}
	return p.Stock
}

func writesOf(t *testing.T, userID int) (orders, items, transactions int) {
	t.Helper()
	err := db.QueryRow(context.Background(), `
		SELECT
			(SELECT count(*) FROM orders WHERE user_id = $1),
			(SELECT count(*) FROM order_items oi JOIN orders o ON o.id = oi.order_id WHERE o.user_id = $1),
			(SELECT count(*) FROM transactions t JOIN orders o ON o.id = t.order_id WHERE o.user_id = $1)`,
		userID).Scan(&orders, &items, &transactions)
	if err != nil {
		t.Fatalf("count writes: %v", err)
	}
	return orders, items, transactions
}

func assertNoWrites(t *testing.T, userID int) {
	t.Helper()
	if o, i, tr := writesOf(t, userID); o != 0 || i != 0 || tr != 0 {
		t.Fatalf("expected no partial writes, got orders=%d items=%d transactions=%d", o, i, tr)
	}
}

func transactionOf(t *testing.T, orderID int) (amount float64, status string, matchesTotal bool) {
	t.Helper()
	err := db.QueryRow(context.Background(), `
		SELECT t.amount, t.status, t.amount = o.total_price
		FROM transactions t JOIN orders o ON o.id = t.order_id
		WHERE t.order_id = $1`,
		orderID).Scan(&amount, &status, &matchesTotal)
	if err != nil {
		t.Fatalf("transaction of order %d: %v", orderID, err)
	}
	return amount, status, matchesTotal
}

func TestPlaceOrder_Success(t *testing.T) {
	ctx := context.Background()
	u := newUser(t)
	food := newProduct(t, "IT Dog Food", 10.5, 10)
	toy := newProduct(t, "IT Cat Toy", 3.25, 5)

	orderID := placeOrder(t, u.Email, item(toy.ID, 3), item(food.ID, 2))

	if got := stockOf(t, food.ID); got != 8 {
		t.Fatalf("food stock: expected 8, got %d", got)
	}
	if got := stockOf(t, toy.ID); got != 2 {
		t.Fatalf("toy stock: expected 2, got %d", got)
	}

	o := getOrder(t, orderID)
	if o.UserID != u.ID || o.TotalPrice != 30.75 || o.CreatedAt.IsZero() {
		t.Fatalf("unexpected order: %+v", o)
	}

	items := getItems(t, orderID)
	want := []models.OrderItem{
		{OrderID: orderID, ProductID: food.ID, ProductName: food.Name, Quantity: 2, UnitPrice: 10.5},
		{OrderID: orderID, ProductID: toy.ID, ProductName: toy.Name, Quantity: 3, UnitPrice: 3.25},
	}
	if len(items) != len(want) {
		t.Fatalf("expected %d items, got %+v", len(want), items)
	}
	for i := range items {
		want[i].ID = items[i].ID
		if items[i] != want[i] {
			t.Fatalf("item %d: expected %+v, got %+v", i, want[i], items[i])
		}
	}

	if _, _, tr := writesOf(t, u.ID); tr != 1 {
		t.Fatalf("expected 1 transaction, got %d", tr)
	}
	amount, status, matches := transactionOf(t, orderID)
	if amount != 30.75 || status != models.TransactionStatusPaid || !matches {
		t.Fatalf("transaction: amount=%v status=%q matches total=%v", amount, status, matches)
	}

	orders, err := st.GetOrdersByUserEmail(ctx, u.Email)
	if err != nil || len(orders) != 1 || orders[0].ID != orderID {
		t.Fatalf("expected user orders [%d], got %+v (err %v)", orderID, orders, err)
	}
}

func TestPlaceOrder_ExactStock(t *testing.T) {
	u := newUser(t)
	p := newProduct(t, "IT Last Units", 2, 3)

	placeOrder(t, u.Email, item(p.ID, 3))

	if got := stockOf(t, p.ID); got != 0 {
		t.Fatalf("expected stock 0 after buying all units, got %d", got)
	}
}

func TestPlaceOrder_DuplicateItemsMerged(t *testing.T) {
	ctx := context.Background()

	t.Run("enough stock for merged quantity", func(t *testing.T) {
		u := newUser(t)
		p := newProduct(t, "IT Treat", 0.5, 5)

		orderID := placeOrder(t, u.Email, item(p.ID, 2), item(p.ID, 3))

		items := getItems(t, orderID)
		if len(items) != 1 || items[0].Quantity != 5 {
			t.Fatalf("expected one merged item with quantity 5, got %+v", items)
		}
		if got := stockOf(t, p.ID); got != 0 {
			t.Fatalf("expected stock 0, got %d", got)
		}
		if got := getOrder(t, orderID).TotalPrice; got != 2.5 {
			t.Fatalf("expected total 2.5, got %v", got)
		}
	})

	t.Run("merged quantity exceeds stock", func(t *testing.T) {
		u := newUser(t)
		p := newProduct(t, "IT Treat", 0.5, 4)

		_, err := st.PlaceOrder(ctx, u.Email, []models.OrderItem{item(p.ID, 2), item(p.ID, 3)})
		if !errors.Is(err, storage.ErrInsufficientStock) {
			t.Fatalf("expected ErrInsufficientStock, got %v", err)
		}
		if got := stockOf(t, p.ID); got != 4 {
			t.Fatalf("stock must stay 4, got %d", got)
		}
		assertNoWrites(t, u.ID)
	})

	t.Run("merged quantity overflows int32", func(t *testing.T) {
		u := newUser(t)
		p := newProduct(t, "IT Treat", 0.5, 4)

		_, err := st.PlaceOrder(ctx, u.Email, []models.OrderItem{item(p.ID, 2147483647), item(p.ID, 1)})
		var productErr *storage.ProductError
		if !errors.Is(err, storage.ErrInvalidInput) || !errors.As(err, &productErr) || productErr.ProductID != p.ID {
			t.Fatalf("expected ProductError(ErrInvalidInput) for product %d, got %v", p.ID, err)
		}
		assertNoWrites(t, u.ID)
	})
}

func TestPlaceOrder_PriceSnapshot(t *testing.T) {
	ctx := context.Background()
	u := newUser(t)
	p := newProduct(t, "IT Dog Food", 10.5, 10)
	orderID := placeOrder(t, u.Email, item(p.ID, 2))

	p.Price, p.Stock = 99, 8
	if err := st.UpdateProduct(ctx, p); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}

	if got := getOrder(t, orderID).TotalPrice; got != 21 {
		t.Fatalf("order total must stay 21 after price change, got %v", got)
	}
	if amount, _, matches := transactionOf(t, orderID); amount != 21 || !matches {
		t.Fatalf("transaction amount must stay 21 and match total, got %v (matches=%v)", amount, matches)
	}
}

func TestPlaceOrder_InsufficientStock_Rollback(t *testing.T) {
	u := newUser(t)
	first := newProduct(t, "IT First", 1, 10)
	second := newProduct(t, "IT Second", 1, 1)

	_, err := st.PlaceOrder(context.Background(), u.Email, []models.OrderItem{item(first.ID, 4), item(second.ID, 2)})

	var productErr *storage.ProductError
	if !errors.Is(err, storage.ErrInsufficientStock) || !errors.As(err, &productErr) || productErr.ProductID != second.ID {
		t.Fatalf("expected ErrInsufficientStock for product %d, got %v", second.ID, err)
	}
	if got := stockOf(t, first.ID); got != 10 {
		t.Fatalf("first product stock must be restored to 10, got %d", got)
	}
	if got := stockOf(t, second.ID); got != 1 {
		t.Fatalf("second product stock must stay 1, got %d", got)
	}
	assertNoWrites(t, u.ID)
}

func TestPlaceOrder_ProductNotFound_Rollback(t *testing.T) {
	u := newUser(t)
	p := newProduct(t, "IT Dog Food", 10.5, 10)

	_, err := st.PlaceOrder(context.Background(), u.Email, []models.OrderItem{item(p.ID, 3), item(2147483647, 1)})

	var productErr *storage.ProductError
	if !errors.Is(err, storage.ErrNotFound) || !errors.As(err, &productErr) || productErr.ProductID != 2147483647 {
		t.Fatalf("expected ProductError(ErrNotFound) for missing product, got %v", err)
	}
	if got := stockOf(t, p.ID); got != 10 {
		t.Fatalf("stock must be restored to 10, got %d", got)
	}
	assertNoWrites(t, u.ID)
}

func TestPlaceOrder_UserNotFound(t *testing.T) {
	p := newProduct(t, "IT Dog Food", 10.5, 10)

	_, err := st.PlaceOrder(context.Background(), "nobody-integration@example.com", []models.OrderItem{item(p.ID, 1)})

	var productErr *storage.ProductError
	if !errors.Is(err, storage.ErrNotFound) || errors.As(err, &productErr) {
		t.Fatalf("expected ErrNotFound for user (not product), got %v", err)
	}
	if got := stockOf(t, p.ID); got != 10 {
		t.Fatalf("stock must stay 10, got %d", got)
	}
}

func TestPlaceOrder_FailureOnLastStep_Rollback(t *testing.T) {
	ctx := context.Background()
	u := newUser(t)
	p := newProduct(t, "IT Dog Food", 10.5, 10)

	if _, err := db.Exec(ctx, fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION it_fail_transaction_insert() RETURNS trigger AS $$
		BEGIN
			IF EXISTS (SELECT 1 FROM orders WHERE id = NEW.order_id AND user_id = %d) THEN
				RAISE EXCEPTION 'injected failure on transactions insert';
			END IF;
			RETURN NEW;
		END
		$$ LANGUAGE plpgsql;
		DROP TRIGGER IF EXISTS it_fail_transaction_insert ON transactions;
		CREATE TRIGGER it_fail_transaction_insert BEFORE INSERT ON transactions
			FOR EACH ROW EXECUTE FUNCTION it_fail_transaction_insert();`, u.ID)); err != nil {
		t.Fatalf("create failing trigger: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS it_fail_transaction_insert ON transactions;
			DROP FUNCTION IF EXISTS it_fail_transaction_insert();`); err != nil {
			t.Errorf("drop failing trigger: %v", err)
		}
	})

	_, err := st.PlaceOrder(ctx, u.Email, []models.OrderItem{item(p.ID, 3)})

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Message != "injected failure on transactions insert" {
		t.Fatalf("expected injected failure, got %v", err)
	}
	if got := stockOf(t, p.ID); got != 10 {
		t.Fatalf("stock must be restored to 10, got %d", got)
	}
	assertNoWrites(t, u.ID)
}

func TestPlaceOrder_ConcurrentLastUnits(t *testing.T) {
	const stock, buyers = 5, 30

	u := newUser(t)
	p := newProduct(t, "IT Last Units", 1.25, stock)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		orderIDs []int
		soldOut  int
		other    []error
	)
	for range buyers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := st.PlaceOrder(context.Background(), u.Email, []models.OrderItem{item(p.ID, 1)})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				orderIDs = append(orderIDs, id)
			case errors.Is(err, storage.ErrInsufficientStock):
				soldOut++
			default:
				other = append(other, err)
			}
		}()
	}
	wg.Wait()
	for _, id := range orderIDs {
		cleanupPlacedOrder(t, id)
	}

	if len(other) > 0 {
		t.Fatalf("unexpected errors: %v", other)
	}
	if len(orderIDs) != stock || soldOut != buyers-stock {
		t.Fatalf("expected %d orders and %d sold out, got %d and %d", stock, buyers-stock, len(orderIDs), soldOut)
	}
	if got := stockOf(t, p.ID); got != 0 {
		t.Fatalf("expected stock 0, got %d", got)
	}
	if o, i, tr := writesOf(t, u.ID); o != stock || i != stock || tr != stock {
		t.Fatalf("expected %d orders, items and transactions, got %d, %d, %d", stock, o, i, tr)
	}
}

func TestPlaceOrder_NoDeadlock(t *testing.T) {
	const buyers = 40

	u := newUser(t)
	a := newProduct(t, "IT A", 1, buyers)
	b := newProduct(t, "IT B", 1, buyers)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		orderIDs []int
		errs     []error
	)
	for i := range buyers {
		items := []models.OrderItem{item(a.ID, 1), item(b.ID, 1)}
		if i%2 == 1 {
			items = []models.OrderItem{item(b.ID, 1), item(a.ID, 1)}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := st.PlaceOrder(context.Background(), u.Email, items)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			orderIDs = append(orderIDs, id)
		}()
	}
	wg.Wait()
	for _, id := range orderIDs {
		cleanupPlacedOrder(t, id)
	}

	if len(errs) > 0 {
		t.Fatalf("%d of %d orders failed, first error: %v", len(errs), buyers, errs[0])
	}
	if stockOf(t, a.ID) != 0 || stockOf(t, b.ID) != 0 {
		t.Fatalf("expected both stocks 0, got %d and %d", stockOf(t, a.ID), stockOf(t, b.ID))
	}
}

func TestAddOrderItem_PaidOrder(t *testing.T) {
	ctx := context.Background()
	u := newUser(t)
	food := newProduct(t, "IT Dog Food", 10.5, 10)
	toy := newProduct(t, "IT Cat Toy", 3.25, 10)
	orderID := placeOrder(t, u.Email, item(food.ID, 2))

	tests := []struct {
		name    string
		item    models.OrderItem
		wantErr error
	}{
		{"existing product", models.OrderItem{OrderID: orderID, ProductID: toy.ID, Quantity: 1}, storage.ErrOrderPaid},
		{"same product again", models.OrderItem{OrderID: orderID, ProductID: food.ID, Quantity: 1}, storage.ErrOrderPaid},
		{"missing product", models.OrderItem{OrderID: orderID, ProductID: 2147483647, Quantity: 1}, storage.ErrOrderPaid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := st.AddOrderItem(ctx, tt.item); !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}

	if got := getOrder(t, orderID).TotalPrice; got != 21 {
		t.Fatalf("total must stay 21, got %v", got)
	}
	if got := len(getItems(t, orderID)); got != 1 {
		t.Fatalf("expected 1 item, got %d", got)
	}
	if amount, _, matches := transactionOf(t, orderID); amount != 21 || !matches {
		t.Fatalf("transaction must stay 21 and match total, got %v (matches=%v)", amount, matches)
	}
	if stockOf(t, food.ID) != 8 || stockOf(t, toy.ID) != 10 {
		t.Fatalf("stocks must not change, got food=%d toy=%d", stockOf(t, food.ID), stockOf(t, toy.ID))
	}

	draftID := newOrder(t, u.ID)
	addItem(t, draftID, toy.ID, 2)
	if got := getOrder(t, draftID).TotalPrice; got != 6.5 {
		t.Fatalf("draft total: expected 6.5, got %v", got)
	}
}

func TestAddOrderItem_PaidOrder_Concurrent(t *testing.T) {
	const attempts = 20

	u := newUser(t)
	p := newProduct(t, "IT Dog Food", 10.5, 10)
	orderID := placeOrder(t, u.Email, item(p.ID, 1))

	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- st.AddOrderItem(context.Background(), models.OrderItem{OrderID: orderID, ProductID: p.ID, Quantity: 1})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, storage.ErrOrderPaid) {
			t.Fatalf("expected ErrOrderPaid, got %v", err)
		}
	}

	if got := len(getItems(t, orderID)); got != 1 {
		t.Fatalf("expected 1 item, got %d", got)
	}
	if amount, _, matches := transactionOf(t, orderID); amount != 10.5 || !matches {
		t.Fatalf("transaction must stay 10.5 and match total, got %v (matches=%v)", amount, matches)
	}
}
