# 🐾 Go Pet Shop

**Go Pet Shop** — это учебный проект интернет-магазина товаров для питомцев, написанный на Go с использованием PostgreSQL.  
Он создан для отработки навыков работы с SQL-запросами, транзакциями и построения REST API.

---

## 🚀 Основная идея

Проект моделирует классический онлайн-магазин с товарами, пользователями, заказами и оплатами.  
Цель — показать полный цикл разработки от работы с базой данных и CRUD-операций до реализации транзакций и аналитики.

---

## 🔑 Возможности проекта

- 📦 Управление товарами (добавление, обновление, удаление, просмотр списка и конкретного товара).  
- 👤 Работа с пользователями (регистрация, получение списка и конкретного пользователя).  
- 🛒 Создание заказов и добавление позиций.  
- 💳 Оформление заказов в транзакции (уменьшение остатков, создание заказа, позиций и транзакции оплаты).  
- 📊 Просмотр истории заказов и аналитики (популярные товары, детали заказов и транзакций).  
- ✅ Health-check для проверки состояния сервера.

---

## ⚙️ Технологии

- **Go** — основной язык разработки.  
- **PostgreSQL** — база данных.  
- **golang-migrate** — управление миграциями БД.  
- **golangci-lint** — статический анализ кода.  
- **testify** и **mockery** — моки для unit-тестов.  
- **Taskfile** — автоматизация запуска миграций, линтера и других команд.

---

## 📂 Структура проекта

```
go-pet-shop/
├── cmd/ # Точки входа (app — сервер, migrator — миграции)
├── config/ # Конфигурация (yaml/json)
├── internal/ # Бизнес-логика, хендлеры, работа с БД
├── migrations/ # SQL-файлы миграций
├── models/ # Доменные модели и DTO
├── .env # Переменные окружения (не хранится в git)
├── Taskfile.yaml # Скрипты для работы с проектом
├── go.mod # Модуль Go
└── README.md # Документация
```

---

## 🛠 Как запустить

Клонировать репозиторий и создать `.env` из примера:

```bash
git clone https://github.com/ya-aan/go-pet-shop.git
cd go-pet-shop
cp .env.example .env
```

### Вариант 1: всё в Docker (одной командой)

```bash
docker compose up --build
```

Compose поднимает PostgreSQL, ждёт его healthcheck, затем контейнер приложения
применяет миграции (`./migrator --migrations-path ./migrations`) и запускает сервер.
API доступно на `http://localhost:8080`.

Контейнер приложения подключается к базе по имени сервиса `postgres`: его `DATABASE_URL`
задан в `docker-compose.yml`, а `DB_HOST`/`DB_PORT` из `.env` нужны только для запуска с хоста.
Если порт 5432 на хосте уже занят, задай в `.env` другой `DB_PORT`, например `5433`.

Остановить и удалить данные базы: `docker compose down -v`.

### Вариант 2: приложение локально, база в Docker

```bash
docker compose up -d postgres   # только PostgreSQL
task migrate                    # миграции, читает DATABASE_URL из .env
go run cmd/app/main.go          # сервер на localhost:8080
```

Без установленного Task миграции можно запустить напрямую:
`go run cmd/migrator/main.go --migrations-path ./migrations`.

---

## 📡 API

Ответы в JSON. Ошибки приходят в виде `{"error": "...", "message": "..."}`.

| Метод | URL | Описание | Коды ответа |
|---|---|---|---|
| GET | `/health` | Проверка состояния сервера | 200 |
| POST | `/users` | Создать пользователя: `{"name", "email"}` | 201, 400, 409 (email занят), 500 |
| GET | `/users` | Список пользователей | 200, 500 |
| GET | `/users/{email}` | Пользователь по email | 200, 400, 404, 500 |
| POST | `/products` | Добавить товар: `{"name", "price", "stock"}` | 201, 400, 500 |
| GET | `/products` | Список товаров | 200, 500 |
| GET | `/products/{id}` | Товар по ID | 200, 400, 404, 500 |
| PUT | `/products/{id}` | Обновить товар | 200, 400, 404, 500 |
| DELETE | `/products/{id}` | Удалить товар | 200, 400, 404, 500 |

Примеры:

```bash
curl -X POST localhost:8080/users -d '{"name":"Alice","email":"alice@example.com"}'
curl localhost:8080/users/alice@example.com
curl -X POST localhost:8080/products -d '{"name":"Dog Food","price":10.5,"stock":5}'
curl localhost:8080/products/1
```

---

## 🧪 Тестирование

Unit-тесты HTTP-хендлеров лежат рядом с хендлерами:

- `internal/handlers/product/products_test.go` — товары;
- `internal/handlers/user/users_test.go` — пользователи.

Тестируется только HTTP-слой: хранилище подменяется моками интерфейсов `Products` и `Users`,
поэтому PostgreSQL и Docker для тестов не нужны. В каждом сценарии проверяются **код и тело ответа**.

| Хендлер | Успех | Некорректный вход (400) | Не найдено (404) | Конфликт (409) | Ошибка сервиса (500) |
|---|---|---|---|---|---|
| `GetAllProducts` | 200 | не применимо, см. ниже | — | — | да |
| `GetProductByID` | 200 | `id` не число | да | — | да |
| `CreateProduct` | 201 | битый JSON, пустое имя, отрицательные цена и остаток | — | — | да |
| `UpdateProduct` | 200 | битый JSON | да | — | да |
| `DeleteProduct` | 200 | пустой `id` | да | — | да |
| `GetAllUsers` | 200, в том числе пустой список `[]` | не применимо | — | — | да |
| `GetUserByEmail` | 200, email как `a@b.com` и как `a%40b.com` | некорректный email | да | — | да |
| `CreateUser` | 201 | битый JSON, пустые имя или email, некорректный email | — | email уже занят | да |

`GetAllProducts` и `GetAllUsers` не принимают входных данных: хендлеры не читают ни тело запроса,
ни параметры пути или запроса. Поэтому сценария «некорректный вход» для них нет.

Тесты пользователей вызывают хендлеры через настоящий роутер chi. Так проверяется, что
email с точкой в домене и с закодированным `@` доходит до хранилища целиком.

### Моки

Моки сгенерированы mockery: `internal/handlers/product/mocks/Products.go` (тип `mocks.Products`)
и `internal/handlers/user/mocks/Users.go` (тип `mocks.Users`).
В тесте мок настраивается так: `m := mocks.NewProducts(t)`, затем `m.On("GetAllProducts", mock.Anything).Return(...)`.
`NewProducts(t)` проверяет, что ожидаемые вызовы были, а неожиданный вызов валит тест.

### Запуск тестов

```bash
go test ./...                                                    # все тесты
go test ./internal/handlers/product/ -v                          # хендлеры товаров, подробный вывод
go test ./internal/handlers/product/ -run TestCreateProduct -v   # тесты одного хендлера
```

### Перегенерация моков

Директивы лежат над интерфейсами `Products` в `internal/handlers/product/products.go`
и `Users` в `internal/handlers/user/users.go`:

```go
//go:generate go run github.com/vektra/mockery/v2 --name=Products
//go:generate go run github.com/vektra/mockery/v2 --name=Users
```

Чтобы перегенерировать мок:

```bash
go generate ./...
```

Запускать после любого изменения интерфейсов. Устанавливать mockery отдельно не нужно,
`go run` берёт его версию из `go.mod`. Файлы в `mocks/` генерируются, вручную их не правим.

### Форматирование и линтер

```bash
gofmt -l .                # пустой вывод — всё отформатировано
golangci-lint run ./...   # или: task linter
```

Конфиг `.golangci.yaml` написан в формате golangci-lint v2.

---

## 🧩 Версии проекта
Проект развивается по шагам, каждая часть закреплена в отдельной ветке:

### v1 — товары и пользователи

Зачем: освоить CRUD-операции, простые `SELECT` и `INSERT` по таблицам `products` и `users`.

Что добавлено:

- **Пользователи.** Модель `User`, методы хранилища `CreateUser`, `GetUserByEmail`, `GetAllUsers`
  (`internal/storage/postgres/user.go`) и ручки `POST /users`, `GET /users`, `GET /users/{email}`
  (`internal/handlers/user`). Повторный email ловится по коду ошибки PostgreSQL `23505`
  (нарушение `UNIQUE`) и возвращается как 409 Conflict.
- **Товар по ID.** Метод `GetProductByID` и ручка `GET /products/{id}`.
- **Честные HTTP-статусы.** Создание отвечает 201. «Не найдено» определяется через
  `errors.Is(err, storage.ErrNotFound)`, а не по тексту ошибки. Общие ошибки хранилища
  (`ErrNotFound`, `ErrAlreadyExists`) объявлены в одном месте, `internal/storage/storage.go`.
  Исправлен баг: при отрицательном `stock` `POST /products` отвечал 200 вместо 400.
- **JSON-поля в нижнем регистре.** У моделей `Product` и `User` появились JSON-теги:
  `{"id", "name", ...}` вместо `{"ID", "Name", ...}`.
- **Убран `middleware.URLFormat`.** Он считал всё после последней точки в пути расширением файла,
  и `/users/alice@example.com` доходил до хендлера как `alice@example`.
- **Docker.** `docker compose up` работает с `.env`, скопированным из `.env.example`:
  контейнер приложения подключается к базе по имени сервиса `postgres`.
  Хост-порт базы настраивается через `DB_PORT`.

Отклонения от сигнатур из ТЗ:

- Все методы хранилища первым аргументом принимают `context.Context`, чтобы тайм-ауты
  и отмена запроса доходили до базы.
- `CreateUser` и `CreateProduct` возвращают `(int, error)` вместо `error`: ответ 201
  содержит ID созданной записи. Так `CreateProduct` работал и раньше, `CreateUser` сделан так же.

### Дальнейшие версии

v2 — Добавлены заказы и позиции заказа.
v3 — Оформление заказа в транзакции.
v4 — История заказов и аналитика (популярные товары).
