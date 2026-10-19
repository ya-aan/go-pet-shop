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
| DELETE | `/products/{id}` | Удалить товар | 200, 400, 404, 409 (товар есть в заказах), 500 |
| POST | `/orders` | Создать пустой заказ: `{"user_id"}` | 201, 400 (в том числе пользователя нет), 500 |
| POST | `/orders/{id}/items` | Добавить позицию: `{"product_id", "quantity"}` | 201, 400 (в том числе товара нет), 404 (заказа нет), 500 |
| GET | `/orders/{id}` | Заказ с позициями | 200, 400, 404, 500 |
| GET | `/users/{email}/orders` | Заказы пользователя, новые первыми | 200, 400, 404 (пользователя нет), 500 |

404 означает, что нет ресурса из URL. Если в теле запроса указан несуществующий пользователь
или товар, ответ 400: сам адрес существует, ошибка во входных данных.

Примеры:

```bash
curl -X POST localhost:8080/users -d '{"name":"Alice","email":"alice@example.com"}'
curl localhost:8080/users/alice@example.com
curl -X POST localhost:8080/products -d '{"name":"Dog Food","price":10.5,"stock":5}'
curl localhost:8080/products/1
curl -X POST localhost:8080/orders -d '{"user_id":1}'
curl -X POST localhost:8080/orders/1/items -d '{"product_id":1,"quantity":2}'
curl localhost:8080/orders/1
curl localhost:8080/users/alice@example.com/orders
```

Ответ `GET /orders/{id}`:

```json
{
  "id": 1, "user_id": 1, "total_price": 41.25, "created_at": "2026-10-19T15:46:55.757887Z",
  "items": [
    {"id": 1, "order_id": 1, "product_id": 1, "product_name": "Dog Food", "quantity": 2, "unit_price": 10.5},
    {"id": 2, "order_id": 1, "product_id": 2, "product_name": "Cat Toy", "quantity": 3, "unit_price": 3.25},
    {"id": 3, "order_id": 1, "product_id": 1, "product_name": "Dog Food", "quantity": 1, "unit_price": 10.5}
  ]
}
```

---

## 🧪 Тестирование

Unit-тесты HTTP-хендлеров лежат рядом с хендлерами:

- `internal/handlers/product/products_test.go` — товары;
- `internal/handlers/user/users_test.go` — пользователи;
- `internal/handlers/order/orders_test.go` — заказы.

Тестируется только HTTP-слой: хранилище подменяется моками интерфейсов `Products`, `Users` и `Orders`,
поэтому PostgreSQL и Docker для unit-тестов не нужны. В каждом сценарии проверяются **код и тело ответа**.

| Хендлер | Успех | Некорректный вход (400) | Не найдено (404) | Конфликт (409) | Ошибка сервиса (500) |
|---|---|---|---|---|---|
| `GetAllProducts` | 200 | не применимо, см. ниже | — | — | да |
| `GetProductByID` | 200 | `id` не число | да | — | да |
| `CreateProduct` | 201 | битый JSON, пустое имя, отрицательные цена и остаток | — | — | да |
| `UpdateProduct` | 200 | битый JSON | да | — | да |
| `DeleteProduct` | 200 | пустой `id` | да | товар есть в заказах | да |
| `GetAllUsers` | 200, в том числе пустой список `[]` | не применимо | — | — | да |
| `GetUserByEmail` | 200, email как `a@b.com` и как `a%40b.com` | некорректный email | да | — | да |
| `CreateUser` | 201 | битый JSON, пустые имя или email, некорректный email | — | email уже занят | да |
| `CreateOrder` | 201, `total_price` из тела игнорируется | битый JSON, `user_id <= 0`, пользователя нет | — | — | да |
| `AddOrderItem` | 201, `order_id` и `unit_price` из тела игнорируются | `id` не число, битый JSON, `product_id <= 0`, `quantity <= 0`, товара нет | заказа нет | — | да |
| `GetOrderByID` | 200, в том числе пустой `items: []` | `id` не число | да | — | ошибка заказа и ошибка позиций |
| `GetOrdersByUserEmail` | 200, в том числе `[]`, email как `a%40b.com` | некорректный email | пользователя нет | — | да |

`GetAllProducts` и `GetAllUsers` не принимают входных данных: хендлеры не читают ни тело запроса,
ни параметры пути или запроса. Поэтому сценария «некорректный вход» для них нет.

Тесты пользователей и заказов вызывают хендлеры через настоящий роутер chi. Так проверяется, что
email с точкой в домене и с закодированным `@` доходит до хранилища целиком.

Проверка email вынесена в `internal/lib/validate` и покрыта своим тестом.

### Интеграционные тесты PostgreSQL

`internal/storage/postgres/integration_test.go` проверяет SQL на настоящей базе:
JOIN и LEFT JOIN, CTE в `AddOrderItem`, расчёт `total_price`, фиксацию цены позиции,
отсутствие частичных записей при ошибках, параллельное добавление позиций в один заказ,
409 при удалении товара из заказа. Файл собирается только с тегом `integration`,
поэтому обычный `go test ./...` его не запускает и базы не требует.

Тесты запускаются **только на отдельной тестовой базе**. Её нужно один раз создать рядом с основной:

```bash
docker compose up -d postgres
docker compose exec postgres createdb -U postgres petshop_test
TEST_DATABASE_URL="postgres://postgres:pass@localhost:5432/petshop_test?sslmode=disable" \
  go test -tags integration -count=1 -v ./internal/storage/postgres/
# или: раскомментировать TEST_DATABASE_URL в .env и запустить task test-integration
```

Защита от запуска на рабочих данных (`checkTestDatabaseURL`, покрыта unit-тестом):

- без `TEST_DATABASE_URL` интеграционные тесты пропускаются с сообщением;
- если в имени базы нет `test`, тесты падают сразу, ещё до миграций и запросов;
- если `TEST_DATABASE_URL` указывает на ту же базу, что и `DATABASE_URL` (тот же хост, порт и имя),
  тесты тоже падают сразу.

Перед запуском тесты сами применяют миграции к тестовой базе. Каждый тест создаёт свои данные
с уникальными email и удаляет после себя только их: по ID созданных строк, без `TRUNCATE`
и массовых `DELETE`.

### Моки

Моки сгенерированы mockery: `internal/handlers/product/mocks/Products.go` (тип `mocks.Products`),
`internal/handlers/user/mocks/Users.go` (тип `mocks.Users`) и `internal/handlers/order/mocks/Orders.go` (тип `mocks.Orders`).
В тесте мок настраивается так: `m := mocks.NewProducts(t)`, затем `m.On("GetAllProducts", mock.Anything).Return(...)`.
`NewProducts(t)` проверяет, что ожидаемые вызовы были, а неожиданный вызов валит тест.

### Запуск тестов

```bash
go test ./...                                                    # все unit-тесты (или: task test)
go test ./internal/handlers/product/ -v                          # хендлеры товаров, подробный вывод
go test ./internal/handlers/product/ -run TestCreateProduct -v   # тесты одного хендлера
```

### Перегенерация моков

Директивы лежат над интерфейсами `Products` в `internal/handlers/product/products.go`,
`Users` в `internal/handlers/user/users.go` и `Orders` в `internal/handlers/order/orders.go`:

```go
//go:generate go run github.com/vektra/mockery/v2 --name=Products
//go:generate go run github.com/vektra/mockery/v2 --name=Users
//go:generate go run github.com/vektra/mockery/v2 --name=Orders
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

Конфиг `.golangci.yaml` написан в формате golangci-lint v2. В нём включён тег сборки `integration`,
поэтому линтер проверяет и интеграционные тесты.

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

### v2 — заказы и позиции заказа

Зачем: научиться создавать заказ, добавлять в него позиции и выводить информацию по заказам
через `SELECT` с `JOIN`.

Что добавлено:

- **Заказы.** Модели `Order` (`user_id`, `total_price`, `created_at`) и `OrderItem`, методы хранилища
  `CreateOrder`, `AddOrderItem`, `GetOrderByID`, `GetOrdersByUserEmail`, `GetOrderItemsByOrderID`
  (`internal/storage/postgres/order.go`) и ручки `POST /orders`, `POST /orders/{id}/items`,
  `GET /orders/{id}`, `GET /users/{email}/orders` (`internal/handlers/order`).
- **Миграция `0002`: цена позиции `order_items.unit_price`.** Цена товара фиксируется в момент
  добавления позиции, поэтому изменение цены товара не меняет стоимость старых заказов.
  Позиции, созданные до миграции, получают текущую цену товара.
- **Сумма заказа считается сервером.** Заказ создаётся с `total_price = 0`. `AddOrderItem` одним
  SQL-запросом с CTE (`WITH item AS (INSERT ... RETURNING ...) UPDATE orders ...`) вставляет позицию
  с ценой из `products` и увеличивает `total_price` на `unit_price * quantity`. Один запрос атомарен:
  при любой ошибке не остаётся ни позиции, ни изменения суммы. Параллельные добавления в один заказ
  не теряют обновлений.
- **JOIN.** `GetOrderItemsByOrderID` соединяет `order_items` с `products`, чтобы отдать название товара.
  `GetOrdersByUserEmail` строится через `users LEFT JOIN orders`: за один запрос отличает
  «пользователя нет» (404) от «пользователь есть, заказов нет» (`[]`).
- **409 при удалении товара из заказа.** Нарушение внешнего ключа (код PostgreSQL `23503`)
  превращается в `storage.ErrInUse`, `DELETE /products/{id}` отвечает 409 вместо 500.
- **Остатки (`stock`) в v2 не меняются**, их уменьшает оформление заказа в v3.
- **Тесты.** Unit-тесты ручек заказов, интеграционные тесты хранилища на PostgreSQL
  (тег `integration`), задачи `task test` и `task test-integration`.

Отклонения от ТЗ:

- В таблице `order_items` появилось поле `unit_price` (миграция `0002`): без него стоимость старых
  заказов менялась бы вместе с ценой товара.
- `POST /orders/{id}/items` отвечает `{"status", "order_id", "product_id", "quantity"}` без ID позиции:
  `AddOrderItem` по ТЗ возвращает только `error`. Позиции с ID и ценой видны в `GET /orders/{id}`.

### Дальнейшие версии

v3 — Оформление заказа в транзакции.
v4 — История заказов и аналитика (популярные товары).
