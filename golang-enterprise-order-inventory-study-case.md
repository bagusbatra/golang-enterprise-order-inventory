# Enterprise Order & Inventory Management System

**Study Case / Technical Test — Backend Engineer (Golang)**

- **Level:** Advanced / Production-Oriented
- **Architecture:** Modular Monolith + Clean Architecture
- **Primary Language:** Go 1.25+
- **Database:** PostgreSQL 17
- **Cache:** Redis 8
- **Event/Queue:** Redis Streams
- **API:** REST API + WebSocket
- **Deployment:** Docker Compose + Nginx
- **Documentation:** OpenAPI / Swagger
- **Testing:** Go Testing + Testcontainers
- **Status:** Technical specification / implementation-ready

---

## 1. Tujuan

Bangun backend untuk perusahaan distribusi elektronik yang memiliki beberapa gudang dan melayani proses:

1. Authentication & authorization
2. Product management
3. Category management
4. Multi-warehouse management
5. Inventory management
6. Stock reservation
7. Customer order
8. Payment simulation
9. Shipment management
10. Notification
11. Realtime order tracking
12. Audit logging
13. Background processing
14. Caching
15. Rate limiting
16. Concurrency-safe transaction
17. Automated testing
18. Containerized deployment

Sistem harus dirancang agar aman terhadap concurrent request, duplicate request, payment callback berulang, dan kegagalan proses asynchronous.

---

# 2. Fixed Technology Stack

Stack berikut **WAJIB** digunakan.

## Backend

| Komponen | Teknologi |
|---|---|
| Language | Go 1.25+ |
| HTTP Framework | Gin |
| ORM | GORM |
| Database | PostgreSQL 17 |
| Cache | Redis 8 |
| Queue/Event | Redis Streams |
| Authentication | JWT |
| Password Hash | bcrypt |
| Validation | go-playground/validator |
| WebSocket | Gorilla WebSocket |
| Logging | Zap |
| Configuration | godotenv |
| Migration | golang-migrate |
| API Documentation | OpenAPI 3 / Swagger |
| Testing | Go testing |
| Integration Test | Testcontainers |
| Container | Docker |
| Reverse Proxy | Nginx |
| Version Control | Git |

## Architecture

**Modular Monolith + Clean Architecture**

Microservices **tidak diperlukan**.

---

# 3. Business Context

Perusahaan bernama **PT Digital Distribution** menjual produk elektronik melalui sistem internal dan aplikasi customer.

Perusahaan memiliki warehouse:

- Surabaya
- Sidoarjo
- Semarang
- Jakarta

Satu product dapat memiliki stok berbeda di setiap warehouse.

Contoh:

```text
ASUS ROG Gaming Laptop

Surabaya  : 20
Sidoarjo  : 15
Semarang  : 30
Jakarta   : 10
```

Customer harus memilih warehouse ketika melakukan order.

Sistem harus memastikan stok tidak pernah oversold meskipun banyak customer melakukan checkout pada waktu yang sama.

---

# 4. User Roles

Sistem memiliki empat role.

## 4.1 ADMIN

Permission:

- Manage users
- Manage products
- Manage categories
- Manage warehouses
- View inventory
- Adjust inventory
- View all orders
- View payments
- View shipments
- View audit logs

## 4.2 SALES

Permission:

- View products
- View categories
- Create order
- View customer orders
- View payment
- View shipment

## 4.3 WAREHOUSE

Permission:

- View products
- View inventory
- Process order
- Pack order
- Update shipment
- View warehouse orders

## 4.4 CUSTOMER

Permission:

- View active products
- View product detail
- Create own order
- View own orders
- View own payment
- View own shipment
- View notifications
- Cancel eligible order

---

# 5. High-Level Architecture

```text
                         CLIENT
                            |
                            v
                         NGINX
                            |
                            v
                    +---------------+
                    |   Go / Gin    |
                    |      API      |
                    +---------------+
                            |
              +-------------+-------------+
              |             |             |
              v             v             v
        PostgreSQL        Redis       WebSocket
              |             |
              |             v
              |       Redis Streams
              |             |
              |             v
              |      Background Workers
              |             |
              +-------------+
```

Internal request flow:

```text
HTTP Request
     |
     v
Middleware
     |
     v
Handler
     |
     v
Service
     |
     v
Repository
     |
     v
PostgreSQL
```

Rules:

- Handler tidak boleh berisi business logic kompleks.
- Service menjadi tempat business logic.
- Repository bertanggung jawab terhadap data access.
- Transaction dikontrol oleh service/use-case.
- Redis hanya digunakan sebagai cache/state/event infrastructure, bukan sebagai source of truth untuk inventory.
- PostgreSQL menjadi source of truth.

---

# 6. Project Structure

Gunakan struktur:

```text
order-management/
│
├── cmd/
│   └── server/
│       └── main.go
│
├── internal/
│   ├── auth/
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   ├── model.go
│   │   └── dto.go
│   │
│   ├── user/
│   ├── category/
│   ├── product/
│   ├── warehouse/
│   ├── inventory/
│   ├── order/
│   ├── payment/
│   ├── shipment/
│   ├── notification/
│   ├── audit/
│   │
│   ├── worker/
│   ├── websocket/
│   ├── middleware/
│   ├── router/
│   └── config/
│
├── pkg/
│   ├── database/
│   ├── redis/
│   ├── jwt/
│   ├── logger/
│   ├── response/
│   ├── validator/
│   └── utils/
│
├── migrations/
│
├── tests/
│
├── docs/
│   └── swagger.yaml
│
├── Dockerfile
├── docker-compose.yml
├── nginx.conf
├── Makefile
├── .env.example
├── .gitignore
├── go.mod
├── go.sum
└── README.md
```

---

# 7. Environment Configuration

File `.env.example`:

```env
APP_NAME=order-management
APP_ENV=development
APP_PORT=8080

DATABASE_HOST=postgres
DATABASE_PORT=5432
DATABASE_USER=postgres
DATABASE_PASSWORD=postgres
DATABASE_NAME=order_management
DATABASE_SSLMODE=disable

REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=

JWT_ACCESS_SECRET=change-this-access-secret
JWT_REFRESH_SECRET=change-this-refresh-secret

ACCESS_TOKEN_EXPIRE=15m
REFRESH_TOKEN_EXPIRE=168h

WORKER_COUNT=4

LOG_LEVEL=info
```

Secret production tidak boleh disimpan di repository.

---

# 8. Database Design

## 8.1 users

```sql
users
--------------------------------
id UUID PRIMARY KEY
name VARCHAR(100) NOT NULL
email VARCHAR(150) UNIQUE NOT NULL
password_hash TEXT NOT NULL
role VARCHAR(20) NOT NULL
status VARCHAR(20) NOT NULL
created_at TIMESTAMP NOT NULL
updated_at TIMESTAMP NOT NULL
```

Role:

```text
ADMIN
SALES
WAREHOUSE
CUSTOMER
```

Status:

```text
ACTIVE
INACTIVE
SUSPENDED
```

---

# 9. Categories

```sql
categories
--------------------------------
id UUID PRIMARY KEY
name VARCHAR(100) NOT NULL
description TEXT
created_at TIMESTAMP NOT NULL
updated_at TIMESTAMP NOT NULL
deleted_at TIMESTAMP NULL
```

Category name harus unique.

---

# 10. Products

```sql
products
--------------------------------
id UUID PRIMARY KEY
category_id UUID NOT NULL
sku VARCHAR(50) UNIQUE NOT NULL
name VARCHAR(150) NOT NULL
description TEXT
price NUMERIC(15,2) NOT NULL
cost_price NUMERIC(15,2) NOT NULL
weight NUMERIC(10,2) NOT NULL
status VARCHAR(20) NOT NULL
created_at TIMESTAMP NOT NULL
updated_at TIMESTAMP NOT NULL
deleted_at TIMESTAMP NULL
```

Status:

```text
ACTIVE
INACTIVE
DISCONTINUED
```

Business rules:

- SKU unique.
- Price > 0.
- Cost price > 0.
- Weight >= 0.
- Product yang sudah digunakan order tidak boleh hard-delete.
- Gunakan soft delete.

---

# 11. Warehouses

```sql
warehouses
--------------------------------
id UUID PRIMARY KEY
code VARCHAR(20) UNIQUE NOT NULL
name VARCHAR(100) NOT NULL
address TEXT NOT NULL
city VARCHAR(100) NOT NULL
status VARCHAR(20) NOT NULL
created_at TIMESTAMP NOT NULL
updated_at TIMESTAMP NOT NULL
```

Status:

```text
ACTIVE
INACTIVE
```

Warehouse inactive tidak dapat dipilih untuk order baru.

---

# 12. Inventory

```sql
inventories
--------------------------------
id UUID PRIMARY KEY
product_id UUID NOT NULL
warehouse_id UUID NOT NULL
quantity INTEGER NOT NULL
reserved_quantity INTEGER NOT NULL
created_at TIMESTAMP NOT NULL
updated_at TIMESTAMP NOT NULL
```

Constraint:

```text
UNIQUE(product_id, warehouse_id)
quantity >= 0
reserved_quantity >= 0
reserved_quantity <= quantity
```

Available stock:

```text
available_stock = quantity - reserved_quantity
```

Contoh:

```text
quantity          = 100
reserved_quantity = 30
available_stock   = 70
```

---

# 13. Inventory Transactions

Semua perubahan inventory harus mempunyai history.

```sql
inventory_transactions
--------------------------------
id UUID PRIMARY KEY
inventory_id UUID NOT NULL
type VARCHAR(30) NOT NULL
quantity INTEGER NOT NULL
reference_type VARCHAR(50)
reference_id UUID
description TEXT
created_at TIMESTAMP NOT NULL
```

Type:

```text
STOCK_IN
STOCK_OUT
RESERVE
RELEASE
ADJUSTMENT
```

Contoh:

```text
RESERVE
quantity: 5
reference_type: ORDER
reference_id: <order-id>
```

---

# 14. Orders

```sql
orders
--------------------------------
id UUID PRIMARY KEY
order_number VARCHAR(50) UNIQUE NOT NULL
customer_id UUID NOT NULL
warehouse_id UUID NOT NULL
status VARCHAR(30) NOT NULL
subtotal NUMERIC(15,2) NOT NULL
discount NUMERIC(15,2) NOT NULL
tax NUMERIC(15,2) NOT NULL
shipping_cost NUMERIC(15,2) NOT NULL
grand_total NUMERIC(15,2) NOT NULL
created_at TIMESTAMP NOT NULL
updated_at TIMESTAMP NOT NULL
```

---

# 15. Order Items

```sql
order_items
--------------------------------
id UUID PRIMARY KEY
order_id UUID NOT NULL
product_id UUID NOT NULL
quantity INTEGER NOT NULL
unit_price NUMERIC(15,2) NOT NULL
subtotal NUMERIC(15,2) NOT NULL
created_at TIMESTAMP NOT NULL
```

Penting:

`unit_price` harus disimpan di order item.

Jangan mengambil harga product saat menampilkan order lama karena harga product dapat berubah.

Contoh:

```text
Product sekarang:
Rp25.000.000

Harga ketika order:
Rp23.000.000
```

Order harus tetap menggunakan:

```text
Rp23.000.000
```

---

# 16. Order Status

Gunakan state machine.

```text
PENDING
   |
   v
WAITING_PAYMENT
   |
   +----------------+
   |                |
   v                v
PAID             EXPIRED
   |
   v
PROCESSING
   |
   v
PACKED
   |
   v
SHIPPED
   |
   v
COMPLETED
```

Cancellation dapat dilakukan pada status tertentu:

```text
PENDING           -> CANCELLED
WAITING_PAYMENT   -> CANCELLED
PAID              -> CANCELLED
```

Tidak boleh:

```text
COMPLETED -> PENDING
SHIPPED   -> PENDING
CANCELLED -> PAID
EXPIRED   -> SHIPPED
```

Semua transition harus divalidasi service.

---

# 17. Payments

```sql
payments
--------------------------------
id UUID PRIMARY KEY
order_id UUID NOT NULL
transaction_id VARCHAR(100) UNIQUE NOT NULL
payment_number VARCHAR(50) UNIQUE NOT NULL
amount NUMERIC(15,2) NOT NULL
status VARCHAR(20) NOT NULL
payment_method VARCHAR(30) NOT NULL
paid_at TIMESTAMP NULL
expired_at TIMESTAMP NOT NULL
created_at TIMESTAMP NOT NULL
updated_at TIMESTAMP NOT NULL
```

Payment status:

```text
PENDING
PAID
FAILED
EXPIRED
```

Payment method:

```text
BANK_TRANSFER
VIRTUAL_ACCOUNT
E_WALLET
```

Payment gateway hanya simulasi.

---

# 18. Shipments

```sql
shipments
--------------------------------
id UUID PRIMARY KEY
order_id UUID UNIQUE NOT NULL
courier VARCHAR(50) NOT NULL
tracking_number VARCHAR(100) UNIQUE NOT NULL
status VARCHAR(30) NOT NULL
shipped_at TIMESTAMP NULL
delivered_at TIMESTAMP NULL
created_at TIMESTAMP NOT NULL
updated_at TIMESTAMP NOT NULL
```

Shipment status:

```text
READY
PICKED_UP
IN_TRANSIT
DELIVERED
FAILED
```

---

# 19. Notifications

```sql
notifications
--------------------------------
id UUID PRIMARY KEY
user_id UUID NOT NULL
type VARCHAR(50) NOT NULL
title VARCHAR(150) NOT NULL
message TEXT NOT NULL
is_read BOOLEAN NOT NULL DEFAULT FALSE
created_at TIMESTAMP NOT NULL
```

---

# 20. Audit Logs

```sql
audit_logs
--------------------------------
id UUID PRIMARY KEY
user_id UUID NULL
action VARCHAR(100) NOT NULL
entity VARCHAR(100) NOT NULL
entity_id UUID NULL
old_data JSONB
new_data JSONB
ip_address VARCHAR(50)
user_agent TEXT
created_at TIMESTAMP NOT NULL
```

Audit log diperlukan untuk operasi sensitif:

- Login
- Create product
- Update product
- Inventory adjustment
- Create order
- Cancel order
- Payment callback
- Role change

---

# 21. Entity Relationship

```text
users
  |
  +---- orders
  |       |
  |       +---- order_items ---- products
  |       |
  |       +---- payments
  |       |
  |       +---- shipments
  |
  +---- notifications
  |
  +---- audit_logs

categories
  |
  +---- products
          |
          +---- inventories ---- warehouses
          |
          +---- order_items

inventories
  |
  +---- inventory_transactions
```

---

# 22. Authentication API

## Register

```http
POST /api/v1/auth/register
```

Request:

```json
{
  "name": "Bagus Batra",
  "email": "bagus@example.com",
  "password": "Password123!"
}
```

Rules:

- Email valid.
- Email unique.
- Password minimum 8 characters.
- Customer menjadi default role.

Response:

```json
{
  "success": true,
  "message": "Registration successful",
  "data": {
    "id": "uuid",
    "name": "Bagus Batra",
    "email": "bagus@example.com",
    "role": "CUSTOMER"
  }
}
```

---

# 23. Login

```http
POST /api/v1/auth/login
```

Request:

```json
{
  "email": "bagus@example.com",
  "password": "Password123!"
}
```

Response:

```json
{
  "success": true,
  "data": {
    "access_token": "jwt",
    "refresh_token": "jwt",
    "expires_in": 900
  }
}
```

Access token:

```text
15 minutes
```

Refresh token:

```text
7 days
```

---

# 24. Refresh Token

```http
POST /api/v1/auth/refresh
```

Request:

```json
{
  "refresh_token": "jwt"
}
```

System:

```text
Validate refresh token
        |
        v
Check token status
        |
        v
Generate access token
        |
        v
Return response
```

---

# 25. Logout

```http
POST /api/v1/auth/logout
```

Refresh token harus di-revoke.

Redis dapat digunakan untuk menyimpan refresh token state.

---

# 26. Product API

```http
GET    /api/v1/products
POST   /api/v1/products
GET    /api/v1/products/:id
PUT    /api/v1/products/:id
DELETE /api/v1/products/:id
```

List support:

```text
page
limit
search
category_id
status
sort
order
```

Contoh:

```http
GET /api/v1/products?page=1&limit=20&search=laptop&status=ACTIVE&sort=price&order=desc
```

---

# 27. Product Cache

Product detail harus menggunakan cache-aside.

```text
GET product
     |
     v
Redis
     |
   hit?
  /   \
yes    no
 |      |
return PostgreSQL
         |
         v
        Redis
         |
         v
       return
```

TTL disarankan:

```text
5–15 minutes
```

Ketika product diubah:

```text
UPDATE PostgreSQL
       |
       v
DELETE Redis Cache
```

---

# 28. Category API

```http
GET    /api/v1/categories
POST   /api/v1/categories
GET    /api/v1/categories/:id
PUT    /api/v1/categories/:id
DELETE /api/v1/categories/:id
```

Category yang masih memiliki product aktif tidak boleh dihapus secara sembarangan.

---

# 29. Warehouse API

```http
GET    /api/v1/warehouses
POST   /api/v1/warehouses
GET    /api/v1/warehouses/:id
PUT    /api/v1/warehouses/:id
DELETE /api/v1/warehouses/:id
```

Warehouse inactive tidak boleh menerima order baru.

---

# 30. Inventory API

```http
GET  /api/v1/inventory
GET  /api/v1/inventory/:id
POST /api/v1/inventory/stock-in
POST /api/v1/inventory/adjust
GET  /api/v1/inventory/:id/transactions
```

Query:

```text
product_id
warehouse_id
low_stock
page
limit
```

Low stock threshold:

```text
available_stock <= 5
```

---

# 31. Stock In

```http
POST /api/v1/inventory/stock-in
```

Request:

```json
{
  "product_id": "uuid",
  "warehouse_id": "uuid",
  "quantity": 100,
  "description": "Restock from supplier"
}
```

Flow:

```text
Validate
  |
  v
Begin transaction
  |
  v
Lock inventory
  |
  v
Increase quantity
  |
  v
Create inventory transaction
  |
  v
Commit
```

---

# 32. Inventory Adjustment

Hanya ADMIN.

```http
POST /api/v1/inventory/adjust
```

Request:

```json
{
  "product_id": "uuid",
  "warehouse_id": "uuid",
  "quantity": -2,
  "description": "Damaged item"
}
```

Rules:

- Tidak boleh membuat quantity negatif.
- Wajib membuat audit log.
- Wajib membuat inventory transaction.

---

# 33. Create Order API

```http
POST /api/v1/orders
```

Request:

```json
{
  "warehouse_id": "uuid",
  "items": [
    {
      "product_id": "uuid",
      "quantity": 2
    },
    {
      "product_id": "uuid",
      "quantity": 1
    }
  ]
}
```

---

# 34. Create Order Business Flow

```text
HTTP Request
     |
     v
JWT Authentication
     |
     v
Validate Request
     |
     v
Validate Warehouse
     |
     v
BEGIN TRANSACTION
     |
     +--> Lock Inventory
     |
     +--> Check Available Stock
     |
     +--> Get Product Price
     |
     +--> Calculate Subtotal
     |
     +--> Calculate Tax
     |
     +--> Calculate Shipping
     |
     +--> Create Order
     |
     +--> Create Order Items
     |
     +--> Reserve Inventory
     |
     +--> Create Inventory Transaction
     |
     v
COMMIT
     |
     v
Publish ORDER_CREATED
     |
     v
HTTP Response
```

Jika salah satu proses gagal:

```text
ROLLBACK
```

---

# 35. Pricing Rule

Subtotal:

```text
subtotal = sum(quantity * unit_price)
```

Discount:

```text
discount = business rule
```

Untuk versi awal, gunakan:

```text
discount = 0
```

Tax:

```text
tax = subtotal * 11%
```

Shipping:

```text
shipping_cost = fixed Rp20.000
```

Grand total:

```text
grand_total =
subtotal
- discount
+ tax
+ shipping_cost
```

Semua perhitungan harus menggunakan decimal-safe numeric handling.

Jangan menggunakan floating-point untuk menyimpan nilai uang di database.

---

# 36. Order Number

Format:

```text
ORD-YYYYMMDD-XXXXXX
```

Contoh:

```text
ORD-20260927-000001
```

Harus unique dan aman terhadap concurrent request.

---

# 37. Inventory Reservation

Misalnya:

```text
quantity = 10
reserved_quantity = 3
```

Available:

```text
10 - 3 = 7
```

Customer membeli 5:

```text
reserved_quantity = 8
available = 2
```

Jika customer membeli 3 lagi:

```text
reserved_quantity = 11
```

Request harus ditolak karena available sebelumnya hanya 2.

---

# 38. Concurrency Requirement

Ini adalah requirement terpenting.

Initial:

```text
Stock = 10
```

Terdapat 100 concurrent request.

Setiap request:

```text
quantity = 1
```

Expected:

```text
10 requests berhasil
90 requests gagal
```

Tidak boleh:

```text
stock < 0
```

Tidak boleh:

```text
success > 10
```

Tidak boleh ada duplicate reservation.

---

# 39. Database Locking

Gunakan PostgreSQL row-level lock.

Konsep:

```sql
BEGIN;

SELECT *
FROM inventories
WHERE product_id = $1
AND warehouse_id = $2
FOR UPDATE;

-- check stock

-- update reserved_quantity

COMMIT;
```

Lock hanya digunakan selama transaction dan dilepaskan setelah commit/rollback.

---

# 40. Payment Flow

Setelah order dibuat:

```text
PENDING
   |
   v
WAITING_PAYMENT
   |
   v
Payment PENDING
```

Payment endpoint:

```http
POST /api/v1/orders/:id/payment
```

Response:

```json
{
  "payment_id": "uuid",
  "transaction_id": "PAY-20260927-000001",
  "amount": 5600000,
  "status": "PENDING",
  "expired_at": "2026-09-27T10:30:00+07:00"
}
```

---

# 41. Payment Callback

```http
POST /api/v1/payments/callback
```

Request:

```json
{
  "transaction_id": "PAY-20260927-000001",
  "order_id": "uuid",
  "status": "PAID"
}
```

Flow:

```text
Receive Callback
      |
      v
Validate Transaction
      |
      v
BEGIN TRANSACTION
      |
      +--> Lock Payment
      |
      +--> Check Current Status
      |
      +--> Update Payment
      |
      +--> Update Order = PAID
      |
      +--> Publish PAYMENT_PAID
      |
      v
COMMIT
```

---

# 42. Idempotency

Callback:

```text
PAY-001
```

dapat diterima berkali-kali.

Contoh:

```text
Request 1 -> PAID
Request 2 -> PAID
Request 3 -> PAID
Request 4 -> PAID
```

Hasil akhir hanya satu efek bisnis.

Database harus memiliki:

```text
UNIQUE(transaction_id)
```

Sistem juga harus memeriksa status payment sebelum melakukan side effect.

---

# 43. Order Cancellation

```http
POST /api/v1/orders/:id/cancel
```

Jika status:

```text
PENDING
WAITING_PAYMENT
```

maka:

```text
Order -> CANCELLED
Reserved stock -> RELEASE
```

Jika status:

```text
SHIPPED
COMPLETED
```

request ditolak.

---

# 44. Payment Expiration

Payment expired setelah:

```text
30 minutes
```

Worker harus mencari payment/order yang sudah melewati `expired_at`.

Flow:

```text
Payment PENDING
      |
      v
expired_at reached
      |
      v
Payment EXPIRED
      |
      v
Order EXPIRED
      |
      v
Release Reserved Stock
```

Proses harus idempotent.

---

# 45. Redis Streams

Gunakan stream:

```text
order_events
payment_events
notification_events
```

Contoh event:

```json
{
  "event_id": "uuid",
  "event_type": "ORDER_CREATED",
  "aggregate_id": "order-uuid",
  "payload": {
    "order_id": "uuid",
    "customer_id": "uuid"
  },
  "created_at": "2026-09-27T10:00:00Z"
}
```

Event type:

```text
ORDER_CREATED
ORDER_CANCELLED
PAYMENT_PAID
PAYMENT_FAILED
ORDER_PROCESSING
ORDER_PACKED
ORDER_SHIPPED
ORDER_COMPLETED
```

---

# 46. Consumer Group

Gunakan Redis Stream Consumer Group.

Contoh:

```text
order_events
     |
     +---- order-worker-1
     +---- order-worker-2
     +---- order-worker-3
```

Worker harus melakukan acknowledgement setelah job berhasil.

Jika worker crash sebelum ACK, message dapat diproses kembali.

---

# 47. Retry Mechanism

Job gagal harus memiliki retry.

Contoh:

```text
Attempt 1
   |
  fail
   |
Attempt 2
   |
  fail
   |
Attempt 3
   |
  fail
   |
Dead Letter
```

Backoff:

```text
1 second
5 seconds
30 seconds
```

Maksimal:

```text
3 attempts
```

---

# 48. Worker Pool

Minimal 4 workers.

```go
for i := 0; i < 4; i++ {
    go worker(...)
}
```

Worker harus:

- menerima context
- menangani panic
- melakukan logging
- melakukan retry
- melakukan ACK setelah sukses
- berhenti secara graceful

---

# 49. Notification Worker

Ketika:

```text
PAYMENT_PAID
```

worker membuat notification:

```text
Title:
Payment Successful

Message:
Your payment for order ORD-20260927-000001 has been confirmed.
```

Ketika:

```text
ORDER_SHIPPED
```

notification:

```text
Order Shipped
Your order has been shipped.
```

---

# 50. WebSocket

Endpoint:

```text
GET /ws
```

Authentication menggunakan JWT.

Customer hanya boleh menerima event miliknya sendiri.

Contoh:

```json
{
  "event": "ORDER_STATUS_UPDATED",
  "data": {
    "order_id": "uuid",
    "status": "SHIPPED"
  }
}
```

WebSocket Manager:

```text
user_id
   |
   +---- connection 1
   +---- connection 2
```

Ketika order berubah:

```text
Order Service
      |
      v
Event
      |
      v
WebSocket Manager
      |
      v
User Connection
```

---

# 51. Shipment Flow

Setelah order:

```text
PAID
```

Warehouse dapat memproses:

```text
PAID
  |
  v
PROCESSING
  |
  v
PACKED
  |
  v
SHIPPED
```

Saat SHIPPED:

- tracking number dibuat
- shipment dibuat
- customer mendapatkan notification
- WebSocket event dikirim

---

# 52. Shipment API

```http
GET   /api/v1/shipments/:id
POST  /api/v1/orders/:id/pack
POST  /api/v1/orders/:id/ship
POST  /api/v1/shipments/:id/deliver
```

Role:

```text
WAREHOUSE
```

dapat melakukan proses warehouse.

---

# 53. Audit Logging

Untuk setiap operasi penting:

```text
user_id
action
entity
entity_id
old_data
new_data
ip_address
user_agent
timestamp
```

Contoh:

```json
{
  "action": "UPDATE_PRODUCT",
  "entity": "PRODUCT",
  "entity_id": "uuid",
  "old_data": {
    "price": 2000000
  },
  "new_data": {
    "price": 2500000
  }
}
```

---

# 54. Middleware

Minimal:

```text
Recovery
Request ID
Logger
CORS
JWT Authentication
RBAC
Rate Limiting
```

Flow:

```text
Request
  |
  v
Request ID
  |
  v
Logger
  |
  v
Recovery
  |
  v
Rate Limiter
  |
  v
JWT
  |
  v
RBAC
  |
  v
Handler
```

---

# 55. Request ID

Setiap request mendapatkan ID.

Header:

```text
X-Request-ID: req-abc123
```

Jika client tidak mengirim ID:

```text
Server generate ID
```

Request ID harus muncul dalam logs.

---

# 56. Logging

Gunakan Zap structured logging.

Contoh:

```json
{
  "level": "info",
  "request_id": "req-123",
  "method": "POST",
  "path": "/api/v1/orders",
  "user_id": "uuid",
  "status": 201,
  "duration_ms": 42
}
```

Error:

```json
{
  "level": "error",
  "request_id": "req-123",
  "service": "inventory",
  "order_id": "uuid",
  "error": "insufficient stock"
}
```

Jangan log:

- password
- JWT
- refresh token
- data sensitif

---

# 57. Rate Limiting

Gunakan Redis.

Login:

```text
5 requests / minute / IP
```

API umum:

```text
100 requests / minute / IP
```

Jika exceeded:

```http
429 Too Many Requests
```

---

# 58. Standard API Response

Success:

```json
{
  "success": true,
  "message": "Product retrieved successfully",
  "data": {}
}
```

List:

```json
{
  "success": true,
  "message": "Products retrieved successfully",
  "data": [],
  "meta": {
    "page": 1,
    "limit": 20,
    "total": 100,
    "total_pages": 5
  }
}
```

Error:

```json
{
  "success": false,
  "message": "Validation failed",
  "code": "VALIDATION_ERROR",
  "errors": [
    {
      "field": "email",
      "message": "invalid email"
    }
  ]
}
```

---

# 59. HTTP Status Code

Gunakan:

```text
200 OK
201 Created
204 No Content

400 Bad Request
401 Unauthorized
403 Forbidden
404 Not Found
409 Conflict
422 Unprocessable Entity
429 Too Many Requests
500 Internal Server Error
```

---

# 60. Error Code

Gunakan error code yang konsisten.

Contoh:

```text
AUTH_INVALID_CREDENTIALS
AUTH_TOKEN_EXPIRED
AUTH_UNAUTHORIZED
AUTH_FORBIDDEN

PRODUCT_NOT_FOUND
PRODUCT_SKU_EXISTS
PRODUCT_INACTIVE

WAREHOUSE_NOT_FOUND
WAREHOUSE_INACTIVE

INVENTORY_NOT_FOUND
INSUFFICIENT_STOCK
INVENTORY_NEGATIVE

ORDER_NOT_FOUND
ORDER_INVALID_STATUS
ORDER_ALREADY_CANCELLED

PAYMENT_NOT_FOUND
PAYMENT_ALREADY_PAID
PAYMENT_EXPIRED
PAYMENT_INVALID_CALLBACK

SHIPMENT_NOT_FOUND
SHIPMENT_INVALID_STATUS
```

---

# 61. Pagination

Default:

```text
page = 1
limit = 20
```

Maximum:

```text
limit = 100
```

Jika client:

```text
limit=1000
```

server harus membatasi menjadi 100 atau menolak request.

---

# 62. Sorting

Whitelist field.

Allowed:

```text
name
price
created_at
updated_at
```

Jangan langsung memasukkan query parameter ke SQL tanpa validasi.

---

# 63. Security Requirements

Wajib:

- bcrypt password
- JWT
- RBAC
- SQL parameterization
- input validation
- rate limiting
- CORS
- request size limit
- secure headers melalui Nginx
- no secrets in Git
- no password/token in logs

Dilarang:

```go
fmt.Sprintf("SELECT ... %s", input)
```

untuk query yang berasal dari user.

---

# 64. Health Check

Endpoint:

```http
GET /health
```

Response:

```json
{
  "status": "ok",
  "services": {
    "database": "ok",
    "redis": "ok"
  }
}
```

Readiness:

```http
GET /ready
```

Digunakan untuk memastikan dependency tersedia sebelum menerima traffic.

---

# 65. Graceful Shutdown

Saat menerima:

```text
SIGTERM
SIGINT
```

aplikasi harus:

```text
Stop accepting new requests
        |
        v
Wait active requests
        |
        v
Stop background workers
        |
        v
Close WebSocket connections
        |
        v
Close Redis
        |
        v
Close PostgreSQL
        |
        v
Exit
```

Gunakan:

```go
context.WithTimeout()
```

---

# 66. Docker

Services:

```text
nginx
api
postgres
redis
```

Contoh:

```text
docker compose up -d
```

harus menjalankan seluruh sistem.

Database migration harus dapat dijalankan secara repeatable.

---

# 67. Nginx

Nginx bertugas:

- reverse proxy
- request size limit
- security headers
- WebSocket proxy
- routing ke Go API

Flow:

```text
Client
  |
  v
Nginx :80
  |
  v
Go API :8080
```

---

# 68. API Versioning

Semua endpoint menggunakan:

```text
/api/v1
```

Contoh:

```text
/api/v1/auth/login
/api/v1/products
/api/v1/orders
```

---

# 69. API Endpoint Summary

## Auth

```text
POST /api/v1/auth/register
POST /api/v1/auth/login
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
```

## Users

```text
GET    /api/v1/users
GET    /api/v1/users/:id
POST   /api/v1/users
PUT    /api/v1/users/:id
PATCH  /api/v1/users/:id/status
```

## Categories

```text
GET    /api/v1/categories
POST   /api/v1/categories
GET    /api/v1/categories/:id
PUT    /api/v1/categories/:id
DELETE /api/v1/categories/:id
```

## Products

```text
GET    /api/v1/products
POST   /api/v1/products
GET    /api/v1/products/:id
PUT    /api/v1/products/:id
DELETE /api/v1/products/:id
```

## Warehouses

```text
GET    /api/v1/warehouses
POST   /api/v1/warehouses
GET    /api/v1/warehouses/:id
PUT    /api/v1/warehouses/:id
DELETE /api/v1/warehouses/:id
```

## Inventory

```text
GET  /api/v1/inventory
GET  /api/v1/inventory/:id
POST /api/v1/inventory/stock-in
POST /api/v1/inventory/adjust
GET  /api/v1/inventory/:id/transactions
```

## Orders

```text
GET  /api/v1/orders
POST /api/v1/orders
GET  /api/v1/orders/:id
POST /api/v1/orders/:id/cancel
POST /api/v1/orders/:id/pack
POST /api/v1/orders/:id/ship
```

## Payments

```text
POST /api/v1/orders/:id/payment
GET  /api/v1/payments/:id
POST /api/v1/payments/callback
```

## Shipments

```text
GET  /api/v1/shipments/:id
POST /api/v1/shipments/:id/deliver
```

## Notifications

```text
GET   /api/v1/notifications
PATCH /api/v1/notifications/:id/read
PATCH /api/v1/notifications/read-all
```

## Audit

```text
GET /api/v1/audit-logs
GET /api/v1/audit-logs/:id
```

## System

```text
GET /health
GET /ready
GET /swagger/index.html
GET /ws
```

---

# 70. Testing Strategy

## Unit Test

Minimal coverage:

```text
AuthService
ProductService
InventoryService
OrderService
PaymentService
ShipmentService
```

Business rules yang harus dites:

- invalid credentials
- duplicate email
- duplicate SKU
- insufficient stock
- invalid order transition
- payment expiration
- cancellation
- duplicate callback
- stock release

---

# 71. Integration Test

Test menggunakan real PostgreSQL dan Redis melalui Testcontainers.

Scenario:

```text
Create user
   |
Create product
   |
Create warehouse
   |
Create inventory
   |
Create order
   |
Reserve inventory
   |
Create payment
   |
Payment callback
   |
Order PAID
   |
Process order
   |
Pack
   |
Ship
   |
Complete
```

---

# 72. Concurrency Test

Scenario:

```text
Initial stock = 10

100 goroutines
each requests quantity = 1
```

Expected:

```text
Success = 10
Failed = 90
Final stock/reservation state consistent
```

Test harus memverifikasi:

```text
No negative stock
No duplicate reservation
No duplicate order
No race condition
```

Jalankan juga:

```bash
go test -race ./...
```

---

# 73. Idempotency Test

Payment callback dikirim:

```text
10 kali
```

Expected:

```text
Payment updated once
Order transitioned once
Business side effects executed once
```

---

# 74. Worker Test

Simulasikan:

```text
Worker receives job
Worker processes job
Worker ACK
```

Kemudian:

```text
Worker fails
Worker does not ACK
Message becomes available again
Retry occurs
```

Setelah 3 kegagalan:

```text
Dead Letter
```

---

# 75. Acceptance Test — Critical Scenario

## Scenario A — Successful Checkout

Initial:

```text
Product:
ASUS ROG

Warehouse:
Surabaya

Stock:
10
```

Customer membeli:

```text
2
```

Expected:

```text
Order = WAITING_PAYMENT
Reserved = 2
Available = 8
Payment = PENDING
```

---

# 76. Acceptance Test — Payment

Payment callback:

```text
PAID
```

Expected:

```text
Payment = PAID
Order = PAID
```

Inventory belum boleh salah hitung.

Reservation tetap konsisten sampai tahap stock-out sesuai business flow yang ditentukan implementasi.

---

# 77. Acceptance Test — Cancellation

Order:

```text
WAITING_PAYMENT
```

Customer cancel.

Expected:

```text
Order = CANCELLED
Reserved stock released
```

---

# 78. Acceptance Test — Expiration

Order:

```text
WAITING_PAYMENT
```

Setelah 30 menit:

```text
Order = EXPIRED
Payment = EXPIRED
Reserved stock released
```

---

# 79. Acceptance Test — Concurrency

Initial:

```text
Stock = 10
```

100 concurrent requests:

```text
quantity = 1
```

Expected:

```text
10 success
90 failure
```

Final invariant:

```text
quantity >= 0
reserved_quantity >= 0
reserved_quantity <= quantity
```

---

# 80. Acceptance Test — Duplicate Callback

Callback yang sama dikirim 10 kali.

Expected:

```text
One logical payment
One order state transition
No duplicate stock movement
No duplicate notification with the same event semantics
```

---

# 81. Acceptance Test — Authorization

CUSTOMER mencoba:

```http
POST /api/v1/inventory/adjust
```

Expected:

```http
403 Forbidden
```

WAREHOUSE mencoba:

```http
DELETE /api/v1/users/:id
```

Expected:

```http
403 Forbidden
```

ADMIN dapat melakukan operasi tersebut jika permission mengizinkan.

---

# 82. Production Quality Requirements

Implementasi harus memperhatikan:

- clean code
- separation of concerns
- dependency injection
- context propagation
- transaction boundary
- error wrapping
- structured logging
- graceful shutdown
- connection pooling
- database indexing
- cache invalidation
- idempotency
- retry
- concurrency safety
- race detection

---

# 83. Database Indexing

Minimal index:

```text
users.email
products.sku
products.category_id
products.status
inventories.product_id
inventories.warehouse_id
orders.order_number
orders.customer_id
orders.status
orders.created_at
payments.transaction_id
payments.order_id
shipments.order_id
notifications.user_id
audit_logs.user_id
audit_logs.entity
audit_logs.created_at
```

Composite index yang relevan:

```text
inventories(product_id, warehouse_id)
orders(customer_id, created_at)
orders(status, created_at)
```

---

# 84. Transaction Rules

Gunakan transaction untuk:

### Create Order

```text
Order
Order Items
Inventory Reservation
Inventory Transaction
```

### Payment Callback

```text
Payment
Order Status
Related Event/Outbox-like persistence if implemented
```

### Inventory Adjustment

```text
Inventory
Inventory Transaction
Audit Log
```

Jika salah satu gagal:

```text
ROLLBACK
```

---

# 85. Cache Rules

Redis tidak boleh menjadi sumber kebenaran untuk:

```text
Inventory quantity
Payment status
Order financial total
```

PostgreSQL tetap menjadi source of truth.

Redis digunakan untuk:

```text
Product cache
Session/token state
Rate limiting
Event streams
Temporary state
```

---

# 86. Money Handling

Database:

```sql
NUMERIC(15,2)
```

Go tidak boleh menggunakan `float64` untuk business calculation uang jika menyebabkan risiko precision.

Gunakan pendekatan decimal/integer minor unit yang konsisten.

Contoh menggunakan minor unit:

```text
Rp25.000.000
= 25000000
```

atau decimal library.

---

# 87. Context Handling

Semua operasi I/O harus menerima context.

Contoh pola:

```go
func (s *OrderService) CreateOrder(
    ctx context.Context,
    input CreateOrderInput,
) (*Order, error)
```

Context digunakan untuk:

- request cancellation
- timeout
- tracing/request metadata

---

# 88. Database Connection Pool

Konfigurasi harus memperhatikan:

```text
MaxOpenConns
MaxIdleConns
ConnMaxLifetime
ConnMaxIdleTime
```

Jangan membuat koneksi database baru setiap request.

---

# 89. Graceful Worker Shutdown

Worker harus menggunakan:

```text
context.Context
WaitGroup
```

Flow:

```text
SIGTERM
   |
   v
Cancel context
   |
   v
Stop receiving new jobs
   |
   v
Finish current jobs
   |
   v
WaitGroup.Wait()
   |
   v
Exit
```

---

# 90. Swagger

Semua endpoint harus terdokumentasi.

Dokumentasi minimal:

- endpoint
- method
- authentication
- request body
- query parameter
- response
- status code
- error response

Swagger dapat diakses:

```text
/swagger/index.html
```

---

# 91. Makefile

Minimal command:

```text
make run
make build
make test
make test-race
make migrate-up
make migrate-down
make swagger
make docker-up
make docker-down
make lint
```

---

# 92. Git Workflow

Gunakan branch:

```text
main
develop
feature/*
bugfix/*
```

Contoh:

```text
feature/authentication
feature/product
feature/inventory
feature/order
feature/payment
feature/worker
```

Commit harus jelas:

```text
feat: implement authentication
feat: add inventory reservation
fix: prevent negative inventory
test: add concurrent checkout test
docs: update API documentation
```

---

# 93. Definition of Done

Sebuah feature dianggap selesai jika:

- business logic selesai
- validation tersedia
- authorization tersedia
- error handling tersedia
- transaction benar jika diperlukan
- unit test tersedia
- integration test tersedia jika relevan
- logging tersedia
- Swagger diperbarui
- README diperbarui
- tidak ada race condition
- tidak ada secret hardcoded

---

# 94. Phase Implementation

## Phase 1 — Foundation

Implement:

- Go project
- Gin
- configuration
- PostgreSQL
- Redis
- Docker
- logger
- error handling
- response helper
- middleware
- health check

---

## Phase 2 — Authentication

Implement:

- register
- login
- bcrypt
- JWT
- refresh token
- logout
- RBAC
- rate limit

---

## Phase 3 — Master Data

Implement:

- users
- categories
- products
- warehouses

---

## Phase 4 — Inventory

Implement:

- inventory
- stock in
- adjustment
- inventory transaction
- locking
- reservation
- release

---

## Phase 5 — Order

Implement:

- create order
- order list
- order detail
- order state machine
- cancellation
- transaction

---

## Phase 6 — Payment

Implement:

- create payment
- callback
- idempotency
- expiration

---

## Phase 7 — Async Processing

Implement:

- Redis Streams
- consumer groups
- worker pool
- retry
- dead letter
- notification

---

## Phase 8 — Shipment

Implement:

- packing
- shipping
- tracking
- delivery

---

## Phase 9 — Realtime

Implement:

- WebSocket
- connection manager
- order status events
- notification events

---

## Phase 10 — Audit

Implement:

- audit logs
- sensitive action tracking
- admin audit endpoint

---

## Phase 11 — Testing

Implement:

- unit tests
- integration tests
- concurrency tests
- race tests
- idempotency tests
- worker tests

---

## Phase 12 — Production

Implement:

- Nginx
- Docker
- graceful shutdown
- health/readiness
- connection pooling
- structured logs
- documentation
- Makefile

---

# 95. Optional Extra Hard

Jika core requirement sudah selesai, tambahkan:

## Optimistic Locking

Tambahkan:

```text
version INTEGER
```

untuk entity tertentu.

## Dead Letter Stream

```text
order_events
       |
       v
worker
       |
    failure
       |
       v
dead_letter_events
```

## Exponential Backoff

```text
1s
2s
4s
8s
```

## Metrics

Tambahkan Prometheus:

```text
http_requests_total
http_request_duration_seconds
order_created_total
order_failed_total
inventory_reservation_total
payment_success_total
worker_job_total
worker_job_failed_total
```

## OpenTelemetry

Tracing:

```text
HTTP Request
    |
    v
Order Service
    |
    v
Inventory Repository
    |
    v
PostgreSQL
```

---

# 96. Final Technical Challenge

Sistem harus mampu menangani skenario berikut.

Initial:

```text
Product:
ASUS ROG

Warehouse:
Surabaya

Quantity:
10
```

Kemudian:

```text
100 customers
```

melakukan:

```text
POST /api/v1/orders
```

secara bersamaan.

Setiap customer membeli:

```text
1 unit
```

System harus menghasilkan:

```text
SUCCESS = 10
FAILED = 90
```

Tidak boleh:

```text
negative inventory
overselling
duplicate reservation
duplicate order
duplicate payment
```

Setelah salah satu order dibatalkan:

```text
Reserved stock
      |
      v
Released
      |
      v
Available stock bertambah
```

Customer lain kemudian dapat membeli kembali stock yang tersedia.

---

# 97. Expected Final Architecture

```text
                           CLIENT
                              |
                              v
                           NGINX
                              |
              +---------------+---------------+
              |                               |
              v                               v
         REST API                         WebSocket
              |
              v
         Middleware
              |
              v
          Handlers
              |
              v
           Services
              |
       +------+------+
       |             |
       v             v
 Repository       Redis
       |             |
       v             +----------+
 PostgreSQL                     |
                                 v
                         Redis Streams
                                 |
                  +--------------+--------------+
                  |              |              |
                  v              v              v
              Order Worker  Payment Worker  Notification
                                                 Worker
```

---

# 98. Final Deliverables

Developer harus menghasilkan:

```text
1. Source code Go
2. Database migrations
3. Dockerfile
4. docker-compose.yml
5. nginx.conf
6. .env.example
7. Swagger/OpenAPI
8. Unit tests
9. Integration tests
10. Concurrency tests
11. README.md
12. Makefile
13. Architecture documentation
14. Database documentation
15. API documentation
```

---

# 99. Minimum Completion Criteria

Project minimum dianggap berhasil jika:

- [ ] Application dapat dijalankan dengan Docker Compose.
- [ ] PostgreSQL terkoneksi.
- [ ] Redis terkoneksi.
- [ ] Register/Login berjalan.
- [ ] JWT berjalan.
- [ ] RBAC berjalan.
- [ ] Product CRUD berjalan.
- [ ] Warehouse berjalan.
- [ ] Inventory berjalan.
- [ ] Order berjalan.
- [ ] Transaction berjalan.
- [ ] Stock reservation berjalan.
- [ ] Concurrent checkout aman.
- [ ] Payment simulation berjalan.
- [ ] Idempotency berjalan.
- [ ] Order expiration berjalan.
- [ ] Redis Streams berjalan.
- [ ] Background worker berjalan.
- [ ] Notification berjalan.
- [ ] WebSocket berjalan.
- [ ] Shipment berjalan.
- [ ] Audit log berjalan.
- [ ] Rate limiting berjalan.
- [ ] Swagger tersedia.
- [ ] Unit test tersedia.
- [ ] Integration test tersedia.
- [ ] `go test -race ./...` berhasil.
- [ ] Graceful shutdown berjalan.
- [ ] README lengkap.

---

# 100. Prinsip Implementasi

Selama mengerjakan project, prioritaskan:

```text
Correctness
    >
Security
    >
Data Consistency
    >
Concurrency Safety
    >
Maintainability
    >
Performance
```

Jangan mengorbankan consistency database hanya untuk mengejar performa.

Untuk inventory, payment, dan order, **correctness lebih penting daripada sekadar cepat**.

---

# 101. Catatan Untuk Technical Test

Kandidat tidak diwajibkan menyelesaikan seluruh fitur bonus.

Prioritas:

### Must Have

```text
Authentication
Product
Warehouse
Inventory
Order
Transaction
Concurrency
Payment
Idempotency
Redis
Worker
Testing
Docker
```

### Should Have

```text
Notification
WebSocket
Audit Log
Rate Limiting
Swagger
Graceful Shutdown
```

### Nice To Have

```text
Prometheus
Grafana
OpenTelemetry
Circuit Breaker
Dead Letter Queue
Optimistic Locking
```

---

# 102. Target Kompetensi yang Diuji

Study case ini dirancang untuk menguji:

### Golang

- struct/interface
- error handling
- context
- goroutine
- channel
- worker pool
- mutex bila diperlukan
- graceful shutdown
- package design

### Backend

- REST API
- authentication
- authorization
- validation
- pagination
- transaction
- business logic
- API design

### Database

- relational modeling
- foreign key
- index
- transaction
- row locking
- isolation
- consistency

### Distributed/Async

- Redis
- caching
- Redis Streams
- consumer group
- retry
- idempotency
- event-driven processing

### Production Engineering

- Docker
- Nginx
- logging
- testing
- configuration
- health check
- observability
- deployment readiness

---

# 103. Final Objective

Tujuan akhir bukan hanya menghasilkan:

```text
CRUD Product
CRUD User
CRUD Order
```

Tetapi menghasilkan backend yang mampu menjawab pertanyaan:

> "Apa yang terjadi jika 100 customer membeli stock terakhir pada waktu yang sama?"

> "Apa yang terjadi jika payment gateway mengirim callback 10 kali?"

> "Apa yang terjadi jika worker mati setelah database berubah tetapi sebelum message di-ACK?"

> "Apa yang terjadi jika Redis mati?"

> "Apa yang terjadi jika PostgreSQL gagal?"

> "Bagaimana memastikan customer A tidak dapat melihat order customer B?"

> "Bagaimana memastikan inventory tidak pernah oversold?"

> "Bagaimana mengetahui siapa yang mengubah harga produk?"

Implementasi yang baik harus mempunyai jawaban teknis yang jelas untuk seluruh skenario tersebut.

---

## End of Specification
