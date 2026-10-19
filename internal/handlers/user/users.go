package user

import (
	"context"
	"errors"
	"fmt"
	"go-pet-shop/internal/lib/validate"
	"go-pet-shop/internal/models"
	"go-pet-shop/internal/storage"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
)

//go:generate go run github.com/vektra/mockery/v2 --name=Users
type Users interface {
	CreateUser(ctx context.Context, user models.User) (int, error)
	GetUserByEmail(ctx context.Context, email string) (models.User, error)
	GetAllUsers(ctx context.Context) ([]models.User, error)
}

type Handler struct {
	log     *slog.Logger
	storage Users
}

func New(log *slog.Logger, storage Users) *Handler {
	return &Handler{
		log:     log,
		storage: storage,
	}
}

func (h *Handler) GetAllUsers(w http.ResponseWriter, r *http.Request) {
	const fn = "handlers.users.GetAllUsers"
	log := h.log.With(
		slog.String("fn", fn),
		slog.String("request_id", middleware.GetReqID(r.Context())),
	)

	users, err := h.storage.GetAllUsers(r.Context())
	if err != nil {
		log.Error("failed to get users", slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{
			"error":   "Internal server error",
			"message": "Failed to retrieve users",
		})
		return
	}

	log.Info("Retrieved users successfully",
		slog.String("url", r.URL.String()),
		slog.Int("count", len(users)),
	)

	render.JSON(w, r, users)
}

func (h *Handler) GetUserByEmail(w http.ResponseWriter, r *http.Request) {
	const fn = "handlers.users.GetUserByEmail"
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

	user, err := h.storage.GetUserByEmail(r.Context(), email)
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

		log.Error("failed to get user", slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{
			"error":   "Internal server error",
			"message": "Failed to retrieve user",
		})
		return
	}

	log.Info("Retrieved user successfully",
		slog.String("email", email),
		slog.String("url", r.URL.String()),
	)

	render.JSON(w, r, user)
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	const fn = "handlers.users.CreateUser"
	log := h.log.With(
		slog.String("fn", fn),
		slog.String("request_id", middleware.GetReqID(r.Context())),
	)

	log.Info("Creating new user", slog.String("url", r.URL.String()))

	var user models.User
	if err := render.DecodeJSON(r.Body, &user); err != nil {
		log.Error("failed to decode request body", slog.Any("error", err))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "Invalid JSON payload",
		})
		return
	}

	user.Name = strings.TrimSpace(user.Name)
	user.Email = strings.TrimSpace(user.Email)

	if user.Name == "" {
		log.Error("user name is empty")
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "User name is required",
		})
		return
	}

	if user.Email == "" {
		log.Error("user email is empty")
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "User email is required",
		})
		return
	}

	if !validate.Email(user.Email) {
		log.Error("invalid email format", slog.String("email", user.Email))
		w.WriteHeader(http.StatusBadRequest)
		render.JSON(w, r, map[string]string{
			"error":   "Bad request",
			"message": "Invalid email format",
		})
		return
	}

	userID, err := h.storage.CreateUser(r.Context(), user)
	if err != nil {
		if errors.Is(err, storage.ErrAlreadyExists) {
			log.Warn("user already exists", slog.String("email", user.Email))
			w.WriteHeader(http.StatusConflict)
			render.JSON(w, r, map[string]string{
				"error":   "Conflict",
				"message": fmt.Sprintf("User with email %s already exists", user.Email),
			})
			return
		}

		log.Error("failed to create user", slog.Any("error", err))
		w.WriteHeader(http.StatusInternalServerError)
		render.JSON(w, r, map[string]string{
			"error":   "Internal server error",
			"message": "Failed to create user",
		})
		return
	}

	log.Info("User created successfully",
		slog.Int("id", userID),
		slog.String("email", user.Email),
		slog.String("url", r.URL.String()),
	)

	user.ID = userID
	w.WriteHeader(http.StatusCreated)
	render.JSON(w, r, map[string]interface{}{
		"status": "User created successfully",
		"id":     userID,
		"user":   user,
	})
}
