package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"go-pet-shop/internal/handlers/user/mocks"
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

type userResponse struct {
	Status string      `json:"status"`
	ID     int         `json:"id"`
	User   models.User `json:"user"`
}

func newRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Get("/users", h.GetAllUsers)
	r.Post("/users", h.CreateUser)
	r.Get("/users/{email}", h.GetUserByEmail)
	return r
}

func serve(t *testing.T, usersMock *mocks.Users, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	w := httptest.NewRecorder()
	newRouter(New(slog.Default(), usersMock)).ServeHTTP(w, req)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("response body is not valid JSON: %v; body: %q", err, w.Body.String())
	}
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("expected status %d, got %d; body: %s", want, w.Code, w.Body.String())
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

func TestGetAllUsers_Success(t *testing.T) {
	want := []models.User{
		{ID: 1, Name: "Alice", Email: "alice@example.com"},
		{ID: 2, Name: "Bob", Email: "bob@example.com"},
	}
	usersMock := mocks.NewUsers(t)
	usersMock.On("GetAllUsers", mock.Anything).Return(want, nil)

	w := serve(t, usersMock, http.MethodGet, "/users", "")

	assertStatus(t, w, http.StatusOK)
	var got []models.User
	decodeBody(t, w, &got)
	if !slices.Equal(got, want) {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}

func TestGetAllUsers_Empty(t *testing.T) {
	usersMock := mocks.NewUsers(t)
	usersMock.On("GetAllUsers", mock.Anything).Return([]models.User{}, nil)

	w := serve(t, usersMock, http.MethodGet, "/users", "")

	assertStatus(t, w, http.StatusOK)
	if got := strings.TrimSpace(w.Body.String()); got != "[]" {
		t.Fatalf("expected empty JSON array, got %q", got)
	}
}

func TestGetAllUsers_Error(t *testing.T) {
	usersMock := mocks.NewUsers(t)
	usersMock.On("GetAllUsers", mock.Anything).Return(nil, errors.New("DB error"))

	w := serve(t, usersMock, http.MethodGet, "/users", "")

	assertStatus(t, w, http.StatusInternalServerError)
	assertErrorBody(t, w, "Internal server error", "Failed to retrieve users")
}

func TestGetUserByEmail_Success(t *testing.T) {
	want := models.User{ID: 1, Name: "Alice", Email: "alice@example.com"}

	for _, target := range []string{"/users/alice@example.com", "/users/alice%40example.com"} {
		t.Run(target, func(t *testing.T) {
			usersMock := mocks.NewUsers(t)
			usersMock.On("GetUserByEmail", mock.Anything, "alice@example.com").Return(want, nil)

			w := serve(t, usersMock, http.MethodGet, target, "")

			assertStatus(t, w, http.StatusOK)
			var got models.User
			decodeBody(t, w, &got)
			if got != want {
				t.Fatalf("expected body %+v, got %+v", want, got)
			}
		})
	}
}

func TestGetUserByEmail_NotFound(t *testing.T) {
	usersMock := mocks.NewUsers(t)
	usersMock.On("GetUserByEmail", mock.Anything, "nobody@example.com").
		Return(models.User{}, fmt.Errorf("storage.postgres.user: %w", storage.ErrNotFound))

	w := serve(t, usersMock, http.MethodGet, "/users/nobody@example.com", "")

	assertStatus(t, w, http.StatusNotFound)
	assertErrorBody(t, w, "Not found", "User with email nobody@example.com does not exist")
}

func TestGetUserByEmail_BadRequest(t *testing.T) {
	usersMock := mocks.NewUsers(t)

	w := serve(t, usersMock, http.MethodGet, "/users/not-an-email", "")

	assertStatus(t, w, http.StatusBadRequest)
	assertErrorBody(t, w, "Bad request", "Invalid email format")
}

func TestGetUserByEmail_Fail(t *testing.T) {
	usersMock := mocks.NewUsers(t)
	usersMock.On("GetUserByEmail", mock.Anything, "alice@example.com").
		Return(models.User{}, errors.New("DB error"))

	w := serve(t, usersMock, http.MethodGet, "/users/alice@example.com", "")

	assertStatus(t, w, http.StatusInternalServerError)
	assertErrorBody(t, w, "Internal server error", "Failed to retrieve user")
}

func TestCreateUser_Success(t *testing.T) {
	usersMock := mocks.NewUsers(t)
	usersMock.On("CreateUser", mock.Anything, models.User{Name: "Alice", Email: "alice@example.com"}).
		Return(10, nil)

	w := serve(t, usersMock, http.MethodPost, "/users", `{"name":" Alice ","email":" alice@example.com "}`)

	assertStatus(t, w, http.StatusCreated)
	var got userResponse
	decodeBody(t, w, &got)
	want := userResponse{
		Status: "User created successfully",
		ID:     10,
		User:   models.User{ID: 10, Name: "Alice", Email: "alice@example.com"},
	}
	if got != want {
		t.Fatalf("expected body %+v, got %+v", want, got)
	}
}

func TestCreateUser_BadRequest(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{"invalid json", `{invalid-json`, "Invalid JSON payload"},
		{"empty name", `{"name":"","email":"alice@example.com"}`, "User name is required"},
		{"empty email", `{"name":"Alice","email":""}`, "User email is required"},
		{"invalid email", `{"name":"Alice","email":"alice.example.com"}`, "Invalid email format"},
		{"email with display name", `{"name":"Alice","email":"Alice <alice@example.com>"}`, "Invalid email format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usersMock := mocks.NewUsers(t)

			w := serve(t, usersMock, http.MethodPost, "/users", tt.body)

			assertStatus(t, w, http.StatusBadRequest)
			assertErrorBody(t, w, "Bad request", tt.wantMessage)
		})
	}
}

func TestCreateUser_Conflict(t *testing.T) {
	usersMock := mocks.NewUsers(t)
	usersMock.On("CreateUser", mock.Anything, mock.Anything).
		Return(0, fmt.Errorf("storage.postgres.user: %w", storage.ErrAlreadyExists))

	w := serve(t, usersMock, http.MethodPost, "/users", `{"name":"Alice","email":"alice@example.com"}`)

	assertStatus(t, w, http.StatusConflict)
	assertErrorBody(t, w, "Conflict", "User with email alice@example.com already exists")
}

func TestCreateUser_Fail(t *testing.T) {
	usersMock := mocks.NewUsers(t)
	usersMock.On("CreateUser", mock.Anything, mock.Anything).
		Return(0, errors.New("DB error"))

	w := serve(t, usersMock, http.MethodPost, "/users", `{"name":"Alice","email":"alice@example.com"}`)

	assertStatus(t, w, http.StatusInternalServerError)
	assertErrorBody(t, w, "Internal server error", "Failed to create user")
}
