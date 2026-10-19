package main

import (
	"context"
	"go-pet-shop/internal/config"
	"go-pet-shop/internal/handlers"
	"go-pet-shop/internal/handlers/order"
	"go-pet-shop/internal/handlers/product"
	"go-pet-shop/internal/handlers/user"
	"go-pet-shop/internal/lib/logger"
	"go-pet-shop/internal/storage/postgres"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

func main() {
	cfg := config.MustLoad()

	// Settings logger
	log := logger.SetupLogger(cfg.Env)
	log.Info("starting the project...", slog.String("env", cfg.Env))

	// Settings and started database
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabaseTimeout)
	defer cancel()

	storage, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("failed to init storage", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() {
		if err := storage.Close(); err != nil {
			log.Error("failed to close storage", slog.String("error", err.Error()))
		}
		log.Info("storage closed")
	}()

	// Init router
	router := chi.NewRouter()

	// Middlewares
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)

	router.Use(logger.CustomLogger(log))

	// Handlers
	productHandler := product.New(log, storage)
	userHandler := user.New(log, storage)
	orderHandler := order.New(log, storage)
	router.Get("/health", handlers.StatusHandler)

	router.Get("/users", userHandler.GetAllUsers)
	router.Post("/users", userHandler.CreateUser)
	router.Get("/users/{email}", userHandler.GetUserByEmail)
	router.Get("/users/{email}/orders", orderHandler.GetOrdersByUserEmail)

	router.Get("/products", productHandler.GetAllProducts)
	router.Post("/products", productHandler.CreateProduct)
	router.Get("/products/{id}", productHandler.GetProductByID)
	router.Delete("/products/{id}", productHandler.DeleteProduct)
	router.Put("/products/{id}", productHandler.UpdateProduct)

	router.Post("/orders", orderHandler.CreateOrder)
	router.Get("/orders/{id}", orderHandler.GetOrderByID)
	router.Post("/orders/{id}/items", orderHandler.AddOrderItem)

	// Settings and started server
	srv := &http.Server{
		Addr:         cfg.HTTPServer.Address,
		Handler:      router,
		ReadTimeout:  cfg.HTTPServer.Timeout,
		WriteTimeout: cfg.HTTPServer.Timeout,
		IdleTimeout:  cfg.HTTPServer.IdleTimeout,
	}

	// Запуск сервера в горутине для graceful shutdown
	go func() {
		log.Info("Starting server on", slog.String("address", cfg.HTTPServer.Address))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop // Ждем сигнал завершения

	log.Info("Shutting down server...")

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("Server shutdown error", slog.String("error", err.Error()))
	}

	log.Info("Server stopped gracefully")
}
