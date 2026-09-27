# Enterprise Order & Inventory Management System

Backend REST API + WebSocket untuk **PT Digital Distribution** — perusahaan distribusi elektronik dengan 4 gudang (Surabaya, Sidoarjo, Semarang, Jakarta). Dibangun sebagai technical test backend engineer (Go), dengan penekanan utama pada **concurrency safety** (mencegah overselling stok) dan **idempotency** (aman terhadap duplicate payment callback).

> Status pengerjaan, contract lock, dan pembagian kerja lintas-agent ada di [`docs/STATUS.md`](docs/STATUS.md).

---

## Features

- Authentication (register/login/refresh/logout) + RBAC 4 role (ADMIN, SALES, WAREHOUSE, CUSTOMER)
- Product, Category, Warehouse management dengan soft delete & cache-aside
- Multi-warehouse inventory dengan reservation, adjustment, dan transaction history lengkap
- Order lifecycle penuh (state machine) dengan **concurrency-safe stock reservation** (row-level lock PostgreSQL)
- Payment simulation dengan **idempotent callback** dan expiration worker otomatis
- Shipment flow (pack → ship → deliver) dengan tracking number
- Notification in-app + realtime update via WebSocket
- Event-driven background processing via Redis Streams (consumer group, retry, dead letter)
- Audit logging untuk operasi sensitif
- Rate limiting, structured logging, graceful shutdown

Detail lengkap requirement & acceptance criteria: [`docs/requirements.md`](docs/requirements.md).

---

## Tech Stack

| Layer | Teknologi |
|---|---|
| Language | Go 1.25+ |
| HTTP Framework | Gin |
| ORM | GORM |
| Database | PostgreSQL 17 |
| Cache / Queue | Redis 8 (cache-aside, rate-limit, Redis Streams) |
| Auth | JWT (access 15m / refresh 7d) + bcrypt |
| WebSocket | Gorilla WebSocket |
| Logging | Zap (structured) |
| Migration | golang-migrate |
| API Docs | OpenAPI 3 / Swagger |
| Testing | Go testing + Testcontainers |
| Deployment | Docker Compose + Nginx |

Arsitektur: **Modular Monolith + Clean Architecture** — detail di [`docs/architecture.md`](docs/architecture.md).

---

## Installation

### Prasyarat
- Go 1.25+
- Docker & Docker Compose
- `golang-migrate` CLI (untuk migration manual di luar Docker)

### Setup

```bash
git clone <repo-url>
cd order-management
cp .env.example .env
# sesuaikan secret JWT_ACCESS_SECRET / JWT_REFRESH_SECRET sebelum production
```

### Menjalankan dengan Docker Compose (direkomendasikan)

```bash
make docker-up
# menjalankan nginx + api + postgres + redis
```

Migration dijalankan otomatis saat container `api` start, atau manual:

```bash
make migrate-up
```

### Menjalankan lokal (tanpa Docker, untuk development)

```bash
# pastikan PostgreSQL & Redis lokal berjalan, sesuaikan .env
make migrate-up
make run
```

Server berjalan di `http://localhost:8080` (langsung) atau `http://localhost` (via Nginx jika pakai Docker Compose).

---

## Environment Variables

Lihat [`.env.example`](.env.example) untuk daftar lengkap. Jangan commit file `.env` — sudah masuk `.gitignore`.

---

## Testing

```bash
make test          # unit test
make test-race     # seluruh test dengan race detector — WAJIB bersih
```

Integration, concurrency, idempotency, dan streams test menggunakan Testcontainers (butuh Docker aktif saat `go test`):

```bash
go test ./tests/concurrency/... -v   # 100 concurrent request vs stock=10
go test ./tests/idempotency/... -v   # 10x payment callback konkuren
go test ./tests/integration/... -v   # end-to-end + acceptance test Section 75-81
go test ./tests/streams/... -v       # retry/backoff/dead-letter/crash-recovery
```

Skenario kritikal yang sudah PASS (termasuk dengan `-race`, lihat `docs/STATUS.md` untuk detail run):
- **Concurrency:** stock=10, 100 concurrent request → tepat 10 sukses, 90 gagal, tidak pernah stock negatif.
- **Idempotency:** payment callback yang sama dikirim 10x SECARA KONKUREN → efek bisnis hanya terjadi sekali.

> **Catatan Windows:** `go test -race` butuh cgo + C compiler. Jika belum ada, install MinGW-w64 (`winget install BrechtSanders.WinLibs.POSIX.UCRT`) lalu set `CGO_ENABLED=1`.

---

## API Documentation

Kontrak API lengkap (request/response/error per endpoint, role permission matrix) didokumentasikan di [`docs/api-contract.md`](docs/api-contract.md).

> **Known gap:** Swagger UI (`/swagger/index.html`) **belum diimplementasikan** — bukan blocker untuk fungsionalitas backend, tapi menyisakan satu item "Should Have" yang belum selesai. Lihat `docs/FINAL_REPORT.md`.

---

## Database

Skema lengkap & ERD: [`docs/database-contract.md`](docs/database-contract.md).

---

## Flowchart Bisnis Kritikal

Authentication flow, create-order flow, payment callback + idempotency flow, expiration flow, shipment flow: [`docs/flowcharts.md`](docs/flowcharts.md).

---

## Deployment

`docker compose up -d --build` menjalankan seluruh stack (Nginx reverse proxy di port 80, API di internal port 8080, PostgreSQL, Redis). Nginx menangani request size limit, security header dasar, dan proxy WebSocket (`/ws`).

---

## Project Structure

```text
cmd/server/           entrypoint + route assembly (lihat catatan di bawah)
internal/             domain modules: auth, user, category, product, warehouse,
                       inventory, order, payment, shipment, notification, audit,
                       worker, websocket, middleware, config
pkg/                   shared package: database, redis, jwt, logger, response,
                       validator, errors, audit, utils
migrations/            SQL migration (golang-migrate), 000001-000019
tests/                 integration, concurrency, idempotency, streams (Testcontainers)
docs/                  requirements, architecture, database contract, api contract,
                       flowchart, development status, final report
```

> Deviasi kecil dari struktur di spesifikasi awal: tidak ada package `internal/router/` terpisah — assembly seluruh route dilakukan langsung di `cmd/server/main.go` (fungsinya sama, tiap domain tetap menyediakan `RegisterRoutes()` sendiri).

---

## Development Process

Project ini dikembangkan dengan 3 agent paralel (Documentation, Backend Foundation, Backend Core) mengikuti contract-first workflow. Lihat `01-DOCUMENTATION.md`, `02-BACKEND-FOUNDATION.md`, `03-BACKEND-CORE.md` di root repo untuk detail proses, dan `docs/STATUS.md` untuk status terkini.
