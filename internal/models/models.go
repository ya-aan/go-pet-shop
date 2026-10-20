package models

import "time"

const TransactionStatusPaid = "paid"

type Product struct {
	ID    int     `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Stock int     `json:"stock"` // количество на складе
}

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Order struct {
	ID         int       `json:"id"`
	UserID     int       `json:"user_id"`
	TotalPrice float64   `json:"total_price"`
	CreatedAt  time.Time `json:"created_at,omitzero"`
}

type OrderItem struct {
	ID          int     `json:"id"`
	OrderID     int     `json:"order_id"`
	ProductID   int     `json:"product_id"`
	ProductName string  `json:"product_name,omitempty"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

type Transaction struct {
	ID        int       `json:"id"`
	OrderID   int       `json:"order_id"`
	Amount    float64   `json:"amount"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type OrderDetail struct {
	Order
	Items        []OrderItem   `json:"items"`
	Transactions []Transaction `json:"transactions"`
}

type PopularProduct struct {
	ProductID   int     `json:"product_id"`
	Name        string  `json:"name"`
	TotalSold   int     `json:"total_sold"`   // сколько единиц продано
	OrdersCount int     `json:"orders_count"` // в скольких заказах
	Revenue     float64 `json:"revenue"`      // выручка по ценам позиций
}
