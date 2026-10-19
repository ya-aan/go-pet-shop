package order

import (
	"context"
	"errors"
	"fmt"
	"go-pet-shop/internal/lib/validate"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
)

//go:generate go run github.com/vektra/mockery/v2 --name=Orders
type Orders interface {
	CreateOrder(ctx context.Context, order models.Order) (int, error)
	AddOrderItem(ctx context.Context, orderItem models.OrderItem) error
	GetOrderByID(ctx context.Context, id int) (models.Order, error)
	GetOrdersByUserEmail(ctx context.Context, email string) ([]models.Order, error)
	GetOrderItemsByOrderID(ctx context.Context, orderID int) ([]models.OrderItem, error)
	PlaceOrder(ctx context.Context, userEmail string, items []models.OrderItem) (int, error)
}

type Handler struct {
	log     *slog.Logger
	storage Orders
}

func New(log *slog.Logger, storage Orders) *Handler {
	return &Handler{
		log:     log,
		storage: storage,
	}
}

type orderDetailsResponse struct {
	models.Order
	Items []models.OrderItem `json:"items"`
}

func parseOrderID(w http.ResponseWriter, r *http.Request, log *slog.Logger) (int, bool) {
	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		log.Error("empty id")
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "Order ID is required",
		})
		return 0, false
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Error("invalid id format", slog.Any("error", err), slog.String("id", idStr))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "Order ID must be a number",
		})
		return 0, false
	}

	return id, true
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	const fn = "handlers.orders.CreateOrder"
	log := h.log.With(
		slog.String("fn", fn),
		slog.String("request_id", middleware.GetReqID(r.Context())),
	)

	log.Info("Creating new order", slog.String("url", r.URL.String()))

	var order models.Order
	if err := render.DecodeJSON(r.Body, &order); err != nil {
		log.Error("failed to decode request body", slog.Any("error", err))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "Invalid JSON payload",
		})
		return
	}

	if order.UserID <= 0 {
		log.Error("user id is not positive", slog.Int("user_id", order.UserID))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "User ID must be a positive number",
		})
		return
	}

	order = models.Order{UserID: order.UserID}

	orderID, err := h.storage.CreateOrder(r.Context(), order)
	if err != nil {
		if errors.Is(err, storage.ErrInvalidInput) {
			log.Warn("user for order does not exist", slog.Int("user_id", order.UserID))
			w.WriteHeader(http.StatusBadRequest)
			render.JSON(w, r, map[string]string{
				"error":   "Bad request",
				"message": fmt.Sprintf("User with ID %d does not exist", order.UserID),
			})
			return
		}

		log.Error("failed to create order", slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{
			"error":   "Internal server error",
			"message": "Failed to create order",
		})
		return
	}

	log.Info("Order created successfully",
		slog.Int("id", orderID),
		slog.Int("user_id", order.UserID),
		slog.String("url", r.URL.String()),
	)

	order.ID = orderID
	w.WriteHeader(http.StatusCreated)
	render.JSON(w, r, map[string]interface{}{
		"status": "Order created successfully",
		"id":     orderID,
		"order":  order,
	})
}

func (h *Handler) AddOrderItem(w http.ResponseWriter, r *http.Request) {
	const fn = "handlers.orders.AddOrderItem"
	log := h.log.With(
		slog.String("fn", fn),
		slog.String("request_id", middleware.GetReqID(r.Context())),
	)

	log.Info("Adding item to order", slog.String("url", r.URL.String()))

	orderID, ok := parseOrderID(w, r, log)
	if !ok {
		return
	}

	var item models.OrderItem
	if err := render.DecodeJSON(r.Body, &item); err != nil {
		log.Error("failed to decode request body", slog.Any("error", err))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "Invalid JSON payload",
		})
		return
	}

	if item.ProductID <= 0 {
		log.Error("product id is not positive", slog.Int("product_id", item.ProductID))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "Product ID must be a positive number",
		})
		return
	}

	if item.Quantity <= 0 {
		log.Error("quantity is not positive", slog.Int("quantity", item.Quantity))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "Quantity must be a positive number",
		})
		return
	}

	item = models.OrderItem{OrderID: orderID, ProductID: item.ProductID, Quantity: item.Quantity}

	if err := h.storage.AddOrderItem(r.Context(), item); err != nil {
		switch {
		case errors.Is(err, storage.ErrNotFound):
			log.Warn("order not found", slog.Int("order_id", orderID))
			w.WriteHeader(http.StatusNotFound)
			render.JSON(w, r, map[string]interface{}{
				"error":   "Not found",
				"message": fmt.Sprintf("Order with ID %d does not exist", orderID),
				"id":      orderID,
			})
		case errors.Is(err, storage.ErrOrderPaid):
			log.Warn("order is already paid", slog.Int("order_id", orderID))
			w.WriteHeader(http.StatusConflict)
			render.JSON(w, r, map[string]interface{}{
				"error":   "Conflict",
				"message": fmt.Sprintf("Order with ID %d is already paid and cannot be changed", orderID),
				"id":      orderID,
			})
		case errors.Is(err, storage.ErrInvalidInput):
			log.Warn("product for order item does not exist", slog.Int("product_id", item.ProductID))
			w.WriteHeader(http.StatusBadRequest)
			render.JSON(w, r, map[string]string{
				"error":   "Bad request",
				"message": fmt.Sprintf("Product with ID %d does not exist", item.ProductID),
			})
		default:
			log.Error("failed to add order item", slog.Any("error", err))
			w.WriteHeader(http.StatusInternalServerError)
			render.JSON(w, r, map[string]string{
				"error":   "Internal server error",
				"message": "Failed to add item to order",
			})
		}
		return
	}

	log.Info("Order item added successfully",
		slog.Int("order_id", orderID),
		slog.Int("product_id", item.ProductID),
		slog.Int("quantity", item.Quantity),
	)

	w.WriteHeader(http.StatusCreated)
	render.JSON(w, r, map[string]interface{}{
		"status":     "Order item added successfully",
		"order_id":   orderID,
		"product_id": item.ProductID,
		"quantity":   item.Quantity,
	})
}

func (h *Handler) GetOrderByID(w http.ResponseWriter, r *http.Request) {
	const fn = "handlers.orders.GetOrderByID"
	log := h.log.With(
		slog.String("fn", fn),
		slog.String("request_id", middleware.GetReqID(r.Context())),
	)

	orderID, ok := parseOrderID(w, r, log)
	if !ok {
		return
	}

	order, err := h.storage.GetOrderByID(r.Context(), orderID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			log.Warn("order not found", slog.Int("id", orderID))
			w.WriteHeader(http.StatusNotFound)
			render.JSON(w, r, map[string]interface{}{
				"error":   "Not found",
				"message": fmt.Sprintf("Order with ID %d does not exist", orderID),
				"id":      orderID,
			})
			return
		}

		log.Error("failed to get order", slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{
			"error":   "Internal server error",
			"message": "Failed to retrieve order",
		})
		return
	}

	items, err := h.storage.GetOrderItemsByOrderID(r.Context(), orderID)
	if err != nil {
		log.Error("failed to get order items", slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{
			"error":   "Internal server error",
			"message": "Failed to retrieve order",
		})
		return
	}

	log.Info("Retrieved order successfully",
		slog.Int("id", orderID),
		slog.Int("items", len(items)),
	)

	render.JSON(w, r, orderDetailsResponse{Order: order, Items: items})
}

func (h *Handler) GetOrdersByUserEmail(w http.ResponseWriter, r *http.Request) {
	const fn = "handlers.orders.GetOrdersByUserEmail"
	log := h.log.With(
		slog.String("fn", fn),
		slog.String("request_id", middleware.GetReqID(r.Context())),
	)

	email, err := url.PathUnescape(chi.URLParam(r, "email"))
	if err != nil || email == "" {
		log.Error("empty or malformed email in URL", slog.String("email", chi.URLParam(r, "email")))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "User email is required",
		})
		return
	}

	if !validate.Email(email) {
		log.Error("invalid email format", slog.String("email", email))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "Invalid email format",
		})
		return
	}

	orders, err := h.storage.GetOrdersByUserEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			log.Warn("user not found", slog.String("email", email))
			w.WriteHeader(http.StatusNotFound)
			render.JSON(w, r, map[string]string{
				"error":   "Not found",
				"message": fmt.Sprintf("User with email %s does not exist", email),
			})
			return
		}

		log.Error("failed to get user orders", slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{
			"error":   "Internal server error",
			"message": "Failed to retrieve user orders",
		})
		return
	}

	log.Info("Retrieved user orders successfully",
		slog.String("email", email),
		slog.Int("count", len(orders)),
	)

	render.JSON(w, r, orders)
}

type checkoutRequest struct {
	UserEmail string             `json:"user_email"`
	Items     []models.OrderItem `json:"items"`
}

func validateCheckout(req checkoutRequest) ([]models.OrderItem, string) {
	if req.UserEmail == "" {
		return nil, "User email is required"
	}
	if !validate.Email(req.UserEmail) {
		return nil, "Invalid email format"
	}
	if len(req.Items) == 0 {
		return nil, "Order must contain at least one item"
	}

	items := make([]models.OrderItem, 0, len(req.Items))
	for _, item := range req.Items {
		if item.ProductID <= 0 {
			return nil, "Product ID must be a positive number"
		}
		if item.Quantity <= 0 || item.Quantity > math.MaxInt32 {
			return nil, fmt.Sprintf("Quantity must be between 1 and %d", math.MaxInt32)
		}
		items = append(items, models.OrderItem{ProductID: item.ProductID, Quantity: item.Quantity})
	}

	return items, ""
}

func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	const fn = "handlers.orders.Checkout"
	log := h.log.With(
		slog.String("fn", fn),
		slog.String("request_id", middleware.GetReqID(r.Context())),
	)

	log.Info("Placing order", slog.String("url", r.URL.String()))

	var req checkoutRequest
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		log.Error("failed to decode request body", slog.Any("error", err))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "Invalid JSON payload",
		})
		return
	}

	req.UserEmail = strings.TrimSpace(req.UserEmail)
	items, msg := validateCheckout(req)
	if msg != "" {
		log.Error("invalid checkout request", slog.String("reason", msg))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": msg,
		})
		return
	}

	orderID, err := h.storage.PlaceOrder(r.Context(), req.UserEmail, items)
	if err != nil {
		h.writeCheckoutError(w, r, log, req.UserEmail, err)
		return
	}

	log.Info("Order placed successfully",
		slog.Int("order_id", orderID),
		slog.String("email", req.UserEmail),
		slog.Int("items", len(items)),
	)

	w.WriteHeader(http.StatusCreated)
	render.JSON(w, r, map[string]interface{}{
		"status":   "Order placed successfully",
		"order_id": orderID,
	})
}

func (h *Handler) writeCheckoutError(w http.ResponseWriter, r *http.Request, log *slog.Logger, email string, err error) {
	var productErr *storage.ProductError
	isProductErr := errors.As(err, &productErr)

	switch {
	case isProductErr && errors.Is(err, storage.ErrInsufficientStock):
		log.Warn("insufficient stock", slog.Int("product_id", productErr.ProductID))
		w.WriteHeader(http.StatusConflict)
		render.JSON(w, r, map[string]interface{}{
			"error":      "Conflict",
			"message":    fmt.Sprintf("Not enough stock for product with ID %d", productErr.ProductID),
			"product_id": productErr.ProductID,
		})
	case isProductErr && errors.Is(err, storage.ErrNotFound):
		log.Warn("product does not exist", slog.Int("product_id", productErr.ProductID))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": fmt.Sprintf("Product with ID %d does not exist", productErr.ProductID),
		})
	case isProductErr && errors.Is(err, storage.ErrInvalidInput):
		log.Warn("invalid quantity", slog.Int("product_id", productErr.ProductID), slog.Any("error", err))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": fmt.Sprintf("Total quantity for product with ID %d must be between 1 and %d", productErr.ProductID, math.MaxInt32),
		})
	case errors.Is(err, storage.ErrNotFound):
		log.Warn("user does not exist", slog.String("email", email))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": fmt.Sprintf("User with email %s does not exist", email),
		})
	default:
		log.Error("failed to place order", slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{
			"error":   "Internal server error",
			"message": "Failed to place order",
		})
	}
}
