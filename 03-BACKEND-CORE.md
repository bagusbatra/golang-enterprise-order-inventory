# AGENT 3 — BACKEND CORE ENGINEER (Inventory, Order, Payment, Shipment, Worker, WebSocket, Notification, Audit)

> Jalankan file ini di **Terminal 3**. Anda adalah satu dari tiga Claude Code agent yang berjalan paralel. Baca seluruh file ini sebelum menulis kode apa pun.
>
> Catatan penamaan: slot ini menggantikan "Backend Agent" generik pada template — karena project ini backend-only, domain backend dipecah dua. Anda memegang bagian **transactional core & async processing** — bagian paling kritikal dari seluruh technical test ini (concurrency safety, idempotency).

---

## 0. IDENTITY & MISSION

Anda adalah **Senior Backend Engineer**, pemilik domain **Inventory, Order, Payment, Shipment, Notification, Audit, Worker, WebSocket** pada:

**Enterprise Order & Inventory Management System — PT Digital Distribution**

Ini adalah domain yang menjawab pertanyaan inti spec (Section 103):

> "Apa yang terjadi jika 100 customer membeli stock terakhir pada waktu yang sama?" — jawaban Anda ada di kode Anda, bukan cuma dokumentasi.
> "Apa yang terjadi jika payment gateway mengirim callback 10 kali?" — jawaban Anda.
> "Apa yang terjadi jika worker mati setelah database berubah tapi sebelum message di-ACK?" — jawaban Anda.

**Correctness > Security > Data Consistency > Concurrency Safety > Maintainability > Performance** (spec Section 100). Jangan pernah mengorbankan konsistensi demi performa di domain ini.

---

## 1. SOURCE OF TRUTH HIERARCHY

1. `golang-enterprise-order-inventory-study-case.md` — spesifikasi resmi.
2. `docs/*.md` (Agent 1) — **WAJIB `CONTRACT LOCKED`** sebelum Anda membuat migration/endpoint domain Anda.
3. Locked Technical Decisions (Section 3 di bawah).
4. File ini.
5. Assumption terdokumentasi.
6. Best practice Go/concurrency umum.

Anda **mengonsumsi** (tidak mengubah) shared package dari Agent 2: `pkg/response`, `pkg/jwt`, `pkg/errors`, `pkg/logger`, `pkg/validator`, `internal/middleware`. Jika perlu penambahan (misal error code baru), tulis Change Request ke `docs/CHANGE_REQUESTS.md` yang ditujukan ke Agent 2 (bukan Agent 1, karena file itu milik Agent 2) — tapi tetap catat di file yang sama agar terpusat.

---

## 2. OWNERSHIP

**Milik Anda:**
```
internal/inventory/
internal/order/
internal/payment/
internal/shipment/
internal/notification/
internal/audit/
internal/worker/
internal/websocket/
migrations/000011_*.sql s.d. migrations/000030_*.sql
tests/ (khusus modul di atas, termasuk integration & concurrency test)
```

**BUKAN milik Anda — JANGAN ubah tanpa Change Request:**
```
internal/auth/ internal/user/ internal/category/ internal/product/ internal/warehouse/
pkg/**  cmd/server/main.go (kecuali menambah worker startup — koordinasikan)
docs/**  README.md  Makefile  docker-compose.yml  nginx.conf
```

`cmd/server/main.go` perlu memanggil worker startup Anda (`worker.StartAll(ctx, ...)`) — ini titik integrasi dengan file milik Agent 2. **Jangan edit `main.go` langsung**; sediakan fungsi exported yang jelas (`worker.Bootstrap(ctx context.Context, deps Dependencies) error`) dan catat signature-nya di `docs/CHANGE_REQUESTS.md` sebagai notifikasi ke Agent 1/2 untuk di-wire di iterasi finalisasi.

---

## 3. LOCKED TECHNICAL DECISIONS (jangan ubah sepihak)

| Keputusan | Isi |
|---|---|
| **Stock-out timing** | Physical `quantity` dikurangi **saat order menjadi PAID**, dalam transaksi payment callback yang sama: `payment→PAID`, `order→PAID`, `quantity -= reserved_quantity`, `reserved_quantity -= reserved_quantity` (per item order tsb), buat `inventory_transactions` type `STOCK_OUT` dengan `reference_type=ORDER, reference_id=order_id`. Setelah PAID, order tersebut tidak punya reserved_quantity aktif lagi. |
| **Order number generation** | Tabel `order_number_sequences(sequence_date DATE PRIMARY KEY, last_number INT NOT NULL DEFAULT 0)`. Di transaksi create-order: `SELECT last_number FROM order_number_sequences WHERE sequence_date = $today FOR UPDATE` (insert row jika belum ada, misal via `ON CONFLICT DO NOTHING` lalu re-select FOR UPDATE, atau upsert), increment, format `ORD-YYYYMMDD-XXXXXX` (6 digit zero-padded). |
| **Money** | `shopspring/decimal` untuk subtotal/tax/shipping/grand_total. Tax = subtotal × 11% (`decimal.NewFromFloat(0.11)` atau lebih aman pakai `decimal.NewFromInt(11).Div(decimal.NewFromInt(100))`). Shipping fixed Rp20.000. Discount = 0 (fixed, tidak perlu rule engine). |
| **Payment method** | Field opsional di `POST /orders/:id/payment`, default `BANK_TRANSFER`. |
| **Courier** | Field opsional di `POST /orders/:id/ship`, default `"Simulated Courier"`. |
| **customer_id** | Selalu dari JWT context (`user_id` claim), bukan dari body. |
| **WebSocket auth** | JWT via query param `GET /ws?token=<jwt>` — validasi manual pakai `pkg/jwt` (bukan middleware Gin biasa karena ini upgrade connection). |
| **Redis** | Bukan source of truth untuk inventory/payment/order — PostgreSQL adalah satu-satunya source of truth. Redis hanya untuk Streams (event), cache, rate-limit. |

---

## 4. PRE-FLIGHT CHECK

1. Baca spec asli secara utuh, fokus Section 12-13, 16, 22 (Order Number), 30-49 (Inventory-Payment-Worker), 96-97.
2. `git status`.
3. Cek `docs/STATUS.md` → `Contract Status: CONTRACT LOCKED`. Jika belum, cek Section 8 (Dependency Gate) — bagian foundation Worker/Redis-Streams skeleton boleh dimulai, tapi migration/endpoint tunggu.
4. Cek `docs/STATUS.md` bagian Agent 2 — apakah `pkg/response`, `pkg/jwt`, `pkg/errors`, middleware JWT/RBAC sudah `COMPLETED` minimal Iterasi 01 & 03 mereka? Anda butuh ini untuk Iterasi 03 Anda (Order API perlu JWT+RBAC).
5. Baca `docs/database-contract.md`, `docs/api-contract.md`, `docs/architecture.md` untuk domain Anda.

---

## 5. THE CRITICAL REQUIREMENT — BACA DUA KALI

Spec Section 38, 79, 96 (kata "Ini adalah requirement terpenting"):

```
Initial stock = 10
100 concurrent request, masing-masing quantity = 1
Expected: EXACTLY 10 success, 90 failed
Tidak boleh: stock < 0, success > 10, duplicate reservation
```

Mekanisme wajib: **PostgreSQL row-level lock** (`SELECT ... FOR UPDATE`) di dalam transaksi create-order, bukan mutex Go di memory (karena aplikasi bisa multi-instance, dan source of truth harus di DB — spec Section 39). Test ini WAJIB lolos sebelum Anda boleh melaporkan domain Order/Inventory `COMPLETED`.

---

## 6. ITERATION PLAN

### ITERATION 01 — Foundation (Worker Skeleton, Redis Streams Client)

**GOAL:** Setup skeleton yang tidak bergantung pada database contract — worker pool framework, Redis Streams client wrapper.
**TASK:** `internal/worker` skeleton: `Pool` struct (context, WaitGroup, worker count dari `.env` `WORKER_COUNT`), graceful shutdown pattern (spec Section 89: SIGTERM → cancel context → stop receiving job → finish current job → `WaitGroup.Wait()` → exit). `internal/websocket` skeleton: connection manager struct (`map[userID][]*Connection` + mutex, karena ini in-memory state per instance — boleh pakai `sync.Mutex`/`sync.Map` di sini, ini BUKAN inventory data). Redis Streams helper (produce/consume wrapper di atas `pkg/redis`).
**FILES AFFECTED:** `internal/worker/pool.go`, `internal/websocket/manager.go`
**DEPENDENCIES:** `pkg/redis`, `pkg/logger` dari Agent 2 (minimal Iterasi 01 mereka selesai)
**EXPECTED RESULT:** Worker pool bisa start/stop tanpa panic, menerima context cancellation.
**VALIDATION:** Test start pool, kirim SIGTERM (simulasi via context cancel), pastikan graceful (tidak ada goroutine leak — cek dengan `go test -race`).
**STOP CONDITION:** `pkg/redis`/`pkg/logger` belum tersedia dari Agent 2 → tunggu, kerjakan desain/interface dulu.

---

### ITERATION 02 — Database: Inventory Domain

**GOAL:** Migration + model + repository untuk inventory, sesuai `docs/database-contract.md`.
**TASK:** Migration `000011_create_inventories`, `000012_create_inventory_transactions`, `000013_create_order_number_sequences` (lihat Locked Decision Section 3). Model GORM. Repository dengan method `LockForUpdate(ctx, productID, warehouseID) (*Inventory, error)` yang eksplisit menggunakan `SELECT ... FOR UPDATE` (via GORM `Clauses(clause.Locking{Strength: "UPDATE"})` atau raw SQL — pilih yang lebih predictable, raw SQL diperbolehkan untuk locking-critical path karena bukan "query dari input user" yang dilarang Section 63, ini adalah query internal dengan parameter aman).
**FILES AFFECTED:** `migrations/000011_*`, `000012_*`, `000013_*`, `internal/inventory/{model,repository}.go`
**DEPENDENCIES:** `docs/database-contract.md` = LOCKED
**EXPECTED RESULT:** `make migrate-up` sukses, constraint `quantity>=0, reserved_quantity>=0, reserved_quantity<=quantity, UNIQUE(product_id, warehouse_id)` terpasang di level database (bukan hanya di aplikasi — defense in depth).
**VALIDATION:** Insert data yang melanggar constraint → database reject (bukan aplikasi yang mencegah).
**STOP CONDITION:** Database contract belum locked → STOP, update status.

---

### ITERATION 03 — Inventory Service: Stock-In, Adjustment, Reservation Core

**GOAL:** Implementasi service inventory termasuk mekanisme locking yang akan dipakai ulang oleh Order Service.
**TASK:**
- `POST /inventory/stock-in`: transaksi (validate → begin tx → lock inventory row → increase quantity → create inventory_transaction type `STOCK_IN` → commit). Spec Section 31.
- `POST /inventory/adjust`: ADMIN only, tidak boleh membuat quantity negatif, wajib audit log (panggil interface `AuditLogger` dari domain Anda sendiri — Audit juga milik Anda, jadi langsung implement, tidak perlu interface cross-agent di sini) + inventory_transaction type `ADJUSTMENT`. Spec Section 32.
- `GET /inventory`, `GET /inventory/:id`, `GET /inventory/:id/transactions` dengan query `product_id, warehouse_id, low_stock (available<=5), page, limit`.
- **Fungsi inti yang akan dipakai Order Service:** `ReserveStock(ctx, tx, productID, warehouseID, qty) error` — HARUS dipanggil di dalam transaksi yang sama dengan lock yang sudah diambil (jangan buka transaksi baru di sini, terima `*gorm.DB` tx sebagai parameter agar caller mengontrol boundary transaksi — spec Section 5: "Transaction dikontrol oleh service/use-case").
- **`ReleaseStock(ctx, tx, productID, warehouseID, qty) error`** — untuk cancellation & expiration.
**FILES AFFECTED:** `internal/inventory/{handler,service,dto}.go`
**DEPENDENCIES:** Iterasi 02, `docs/api-contract.md` bagian Inventory = LOCKED
**EXPECTED RESULT:** Stock-in & adjustment berjalan sesuai contoh spec Section 37 (quantity=10, reserved=3, available=7; beli 5 → reserved=8 available=2; beli 3 lagi → **ditolak** karena available sebelumnya hanya 2).
**VALIDATION:** Reproduksi contoh Section 37 sebagai test case eksplisit.
**POTENTIAL ISSUE:** Jangan lakukan check-then-act tanpa lock (`available = qty - reserved; if available >= requested` HARUS terjadi setelah row terkunci `FOR UPDATE`, dalam transaksi yang belum commit) — ini sumber race condition paling umum.
**STOP CONDITION:** Jika desain Anda tidak bisa menjamin atomicity check+reserve → JANGAN lanjut ke Order, perbaiki dulu di sini karena Order Service akan bergantung pada fungsi ini.

---

### ITERATION 04 — Order Service: Create Order, State Machine, Pricing

**GOAL:** Implementasi order creation end-to-end sesuai flow spec Section 34, dengan concurrency safety sebagai prioritas nomor satu.
**TASK:**
- `POST /orders`: JWT auth → validate request → validate warehouse ACTIVE (panggil `warehouse.IsActive` dari Agent 2 — pastikan sudah tersedia, jika belum, STOP dan cek status Agent 2) → **BEGIN TRANSACTION** → untuk setiap item: `inventory.LockForUpdate` (urutkan lock berdasarkan `product_id` ASC untuk mencegah deadlock antar-request dengan item order berbeda-beda urutan) → cek available stock → ambil harga produk (snapshot ke `unit_price`, **jangan** ambil harga saat display order lama — spec Section 15) → hitung subtotal/tax/shipping/grand_total (`shopspring/decimal`) → generate order_number (lock counter table, lihat Section 3) → create `orders` + `order_items` → `inventory.ReserveStock` → create `inventory_transactions` type `RESERVE` → **COMMIT** → publish event `ORDER_CREATED` ke Redis Streams (async, setelah commit, jangan sebelum commit — jika publish gagal, order tetap valid, event bisa di-retry/reconcile terpisah, jangan rollback order karena event gagal publish).
- Order status awal: **`WAITING_PAYMENT`** langsung (state `PENDING` di diagram spec Section 16 dianggap transient/logical, tidak perlu dipersist sebagai row status terpisah sebelum WAITING_PAYMENT — order dibuat langsung dengan status `WAITING_PAYMENT` karena pada saat itu semua validasi sudah lolos dan reservation sudah terjadi).
- State machine: implementasikan sebagai fungsi `CanTransition(from, to OrderStatus) bool` dengan whitelist transisi eksplisit dari spec Section 16, dipanggil di SETIAP perubahan status di seluruh modul Anda (Order, Payment, Shipment) — jangan ada `order.Status = "PAID"` langsung tanpa lewat fungsi ini.
- `GET /orders`, `GET /orders/:id` dengan ownership check (CUSTOMER hanya lihat order sendiri — cek `customer_id == jwt.user_id` kecuali role ADMIN/SALES/WAREHOUSE).
- `POST /orders/:id/cancel`: hanya dari `PENDING/WAITING_PAYMENT/PAID` (spec Section 43) → transaksi: order→CANCELLED, `inventory.ReleaseStock`, inventory_transaction type `RELEASE`.
**FILES AFFECTED:** `internal/order/{handler,service,repository,model,dto,state_machine}.go`
**DEPENDENCIES:** Iterasi 03, Agent 2 `warehouse.IsActive` & `product` price lookup tersedia, `docs/api-contract.md` bagian Order = LOCKED
**EXPECTED RESULT:** Reproduksi tepat skenario Section 75 (Acceptance Test — Successful Checkout): stock 10, beli 2 → Order=WAITING_PAYMENT, Reserved=2, Available=8, Payment=PENDING (payment dibuat di Iterasi 05, untuk sekarang cukup order+reservation).
**VALIDATION:** Unit test state machine (semua transisi ilegal dari spec Section 16 ditolak). Integration test manual: create order dengan stock cukup → sukses; stock tidak cukup → `409 INSUFFICIENT_STOCK`.
**STOP CONDITION:** Jika lock ordering antar-item tidak konsisten (risiko deadlock) → perbaiki sebelum lanjut ke concurrency test.

---

### ITERATION 05 — Concurrency Test (WAJIB LOLOS SEBELUM LANJUT)

**GOAL:** Membuktikan requirement terpenting spec (Section 38, 79, 96) terpenuhi.
**TASK:** Test Go: `stock=10`, spawn 100 goroutine, masing-masing `POST /orders` (via service call langsung atau httptest) dengan `quantity=1` pada produk/warehouse yang sama, secara bersamaan (`sync.WaitGroup`, semua goroutine start setelah barrier/channel signal agar benar-benar konkuren).
**FILES AFFECTED:** `internal/order/concurrency_test.go` atau `tests/concurrency/`
**DEPENDENCIES:** Iterasi 04, Testcontainers (real PostgreSQL, bukan mock — locking behavior tidak valid dites dengan mock/sqlite)
**EXPECTED RESULT:** **Tepat** 10 sukses, 90 gagal (`INSUFFICIENT_STOCK`), final `quantity=10` (belum PAID jadi belum stock-out), `reserved_quantity=10`, `available=0`. Tidak ada goroutine yang sukses melebihi 10.
**VALIDATION:** Jalankan test berkali-kali (minimal 5x) untuk pastikan tidak flaky. Jalankan dengan `go test -race`.
**POTENTIAL ISSUE:** Jika hasil tidak konsisten (kadang 11 sukses, kadang 9) → ada race condition di check-then-act, kembali ke Iterasi 03/04, JANGAN lanjut sebelum ini benar-benar deterministik.
**STOP CONDITION:** Test gagal atau flaky → **HARD STOP**, ini requirement terpenting di seluruh project, tidak boleh dilewati atau "diperbaiki nanti".

---

### ITERATION 06 — Payment: Create, Idempotent Callback, Expiration Worker

**GOAL:** Payment simulation dengan idempotency sebagai fokus utama (spec Section 40-44, 73, 80).
**TASK:**
- Migration `000016_create_payments` — `UNIQUE(transaction_id)` **di level database**, bukan hanya cek aplikasi.
- `POST /orders/:id/payment`: create payment PENDING, `expired_at = now + 30 menit`, generate `payment_number`/`transaction_id`.
- `POST /payments/callback`: **transaksi**: lock payment row (`FOR UPDATE`) → cek status CURRENT payment SEBELUM melakukan side effect apa pun (spec Section 42: "Sistem juga harus memeriksa status payment sebelum melakukan side effect") → jika sudah `PAID`, return sukses tanpa efek ganda (idempotent no-op, bukan error) → jika masih `PENDING`: update payment→PAID, order→PAID (via state machine Section 04), **stock-out** (lihat Locked Decision: `quantity -= reserved`, `reserved -= reserved`, inventory_transaction `STOCK_OUT`), publish `PAYMENT_PAID` → commit.
- Payment Expiration Worker (background, jalan periodik misal setiap 1 menit): query payment `status=PENDING AND expired_at < now()`, untuk masing-masing: transaksi → payment→EXPIRED, order→EXPIRED, `inventory.ReleaseStock`, inventory_transaction `RELEASE` → commit. **Idempotent**: gunakan `WHERE status = 'PENDING'` di UPDATE (bukan cuma di SELECT) supaya jika worker run 2x bersamaan, hanya satu yang berhasil update (row lock/optimistic check).
**FILES AFFECTED:** `migrations/000016_*`, `internal/payment/*`, `internal/worker/payment_expiration.go`
**DEPENDENCIES:** Iterasi 04, `docs/api-contract.md` bagian Payment = LOCKED
**EXPECTED RESULT:** Reproduksi Section 76 (payment callback → Order=PAID) dan Section 78 (expiration → Order=EXPIRED, stock released).
**VALIDATION:** **Idempotency test** (Section 73, 80): kirim callback yang sama 10x → payment updated sekali, order transisi sekali, tidak ada duplicate stock movement/notification. Jalankan sungguhan dengan goroutine paralel mengirim callback bersamaan, bukan cuma sequential loop.
**STOP CONDITION:** Callback ganda menyebabkan efek bisnis ganda (misal stock terpotong 2x) → **HARD STOP**, ini requirement kritikal kedua setelah concurrency test.

---

### ITERATION 07 — Async Processing: Redis Streams, Consumer Group, Retry, Dead Letter

**GOAL:** Event-driven worker sesuai spec Section 45-49.
**TASK:** Publish event ke stream `order_events`/`payment_events`/`notification_events` dengan payload sesuai format spec Section 45. Consumer group (`XGROUP CREATE`, `XREADGROUP`) dengan minimal 4 worker (`WORKER_COUNT` dari env — spec Section 48). Worker: terima context, handle panic (recover + log, jangan crash seluruh pool), retry dengan backoff 1s/5s/30s, maksimal 3 attempt (spec Section 47), setelah gagal 3x → tulis ke "dead letter" (bisa berupa stream terpisah `<name>_dead_letter` atau tabel — pilih salah satu, dokumentasikan). ACK (`XACK`) hanya setelah job SUKSES — jika worker crash sebelum ACK, message harus bisa diproses ulang oleh worker lain (verifikasi dengan test: simulasikan crash, restart consumer, pastikan pending message di-claim ulang via `XPENDING`/`XCLAIM`).
**FILES AFFECTED:** `internal/worker/{order_worker,payment_worker,notification_worker}.go`
**DEPENDENCIES:** Iterasi 01, 04, 06
**EXPECTED RESULT:** Worker pool jalan, konsumsi event, retry saat gagal, ACK saat sukses.
**VALIDATION (Worker Test, spec Section 74):** Simulasikan job gagal (handler return error sengaja) → tidak ACK → message tersedia lagi → retry terjadi → setelah 3 gagal → dead letter.
**STOP CONDITION:** Message hilang tanpa diproses (bukan di-retry, bukan di dead letter) → bug kritikal, perbaiki sebelum lanjut.

---

### ITERATION 08 — Notification Worker

**GOAL:** Notification dibuat otomatis dari event (spec Section 49).
**TASK:** Migration `000018_create_notifications`. Consumer untuk `PAYMENT_PAID` → buat notification "Payment Successful". Consumer untuk `ORDER_SHIPPED` → "Order Shipped". `GET /notifications`, `PATCH /notifications/:id/read`, `PATCH /notifications/read-all` — ownership check (user hanya lihat notifikasi sendiri).
**FILES AFFECTED:** `migrations/000018_*`, `internal/notification/*`
**DEPENDENCIES:** Iterasi 07
**EXPECTED RESULT:** Notification muncul otomatis setelah payment sukses/order shipped, tanpa duplikat meski event diproses lebih dari sekali (idempotency check: cek apakah notification untuk `reference_id + event_type` yang sama sudah ada sebelum insert, atau terima duplikat notification record tapi pastikan **efek bisnis lain** tidak duplikat — dokumentasikan pilihan Anda).
**VALIDATION:** Reproduksi Section 80 — "No duplicate notification with the same event semantics".
**STOP CONDITION:** -

---

### ITERATION 09 — Shipment Flow

**GOAL:** PAID→PROCESSING→PACKED→SHIPPED→DELIVERED (spec Section 51-52).
**TASK:** Migration `000017_create_shipments`. `POST /orders/:id/pack` (role WAREHOUSE, order PAID→PROCESSING lalu →PACKED — atau langsung PACKED sesuai state machine, ikuti diagram Section 16 persis), `POST /orders/:id/ship` (WAREHOUSE, generate tracking_number, create shipment record, order→SHIPPED, buat notification, publish WS event & `ORDER_SHIPPED`), `POST /shipments/:id/deliver` (shipment→DELIVERED, order→COMPLETED), `GET /shipments/:id`.
**FILES AFFECTED:** `migrations/000017_*`, `internal/shipment/*`
**DEPENDENCIES:** Iterasi 04, 08
**EXPECTED RESULT:** Full lifecycle order dari create sampai COMPLETED bisa direproduksi (spec Section 71 integration scenario).
**VALIDATION:** Role selain WAREHOUSE mencoba pack/ship → `403 FORBIDDEN` (spec Section 81).
**STOP CONDITION:** -

---

### ITERATION 10 — WebSocket Realtime

**GOAL:** `GET /ws` dengan JWT auth, event per-user (spec Section 50).
**TASK:** Upgrade connection, validasi JWT dari query param (Locked Decision), register ke connection manager (Iterasi 01) dengan key `user_id`. Saat Order Service/Shipment Service mengubah status, kirim event `ORDER_STATUS_UPDATED` ke SEMUA koneksi milik `user_id` terkait (customer tidak boleh menerima event customer lain — spec Section 81 "Bagaimana memastikan customer A tidak dapat melihat order customer B").
**FILES AFFECTED:** `internal/websocket/*`, hook di `internal/order/service.go` & `internal/shipment/service.go` (panggil `websocket.Manager.Notify(userID, event)` setelah commit — bukan di dalam transaksi DB)
**DEPENDENCIES:** Iterasi 01, 04, 09
**EXPECTED RESULT:** Client A yang connect dengan token miliknya hanya menerima event order miliknya.
**VALIDATION:** Test dengan 2 client (2 JWT berbeda), pastikan tidak ada cross-talk.
**STOP CONDITION:** -

---

### ITERATION 11 — Audit Log

**GOAL:** Audit trail untuk operasi sensitif (spec Section 20, 53).
**TASK:** Migration `000019_create_audit_logs`. `AuditLogger` interface + implementasi (`Log(ctx, userID, action, entity, entityID, oldData, newData, ip, userAgent)`), dipanggil dari: login, create/update product (Agent 2 — sediakan hook/interface agar Agent 2 bisa memanggil tanpa import langsung ke internal Anda; gunakan interface yang didefinisikan di `pkg` atau dependency injection saat wiring di `main.go`), inventory adjustment, create/cancel order, payment callback, role change (Agent 2). `GET /audit-logs`, `GET /audit-logs/:id` (ADMIN only).
**FILES AFFECTED:** `migrations/000019_*`, `internal/audit/*`
**DEPENDENCIES:** Iterasi 02
**PENTING — Cross-agent wiring:** Karena `AuditLogger` dipanggil dari domain Agent 2 (product, user) DAN domain Anda (inventory, order, payment), definisikan interface-nya di lokasi netral (`pkg/audit` — usulkan ke Agent 2 via Change Request, atau taruh sementara di `internal/audit` dan export interface, lalu Agent 2 inject dependency ini saat wiring di `main.go` — koordinasikan lewat `docs/CHANGE_REQUESTS.md` sebagai notifikasi, bukan permintaan approval, karena ini teknis implementasi bukan perubahan contract).
**VALIDATION:** Setiap operasi sensitif di spec Section 20 menghasilkan 1 row audit log dengan `old_data`/`new_data` JSONB terisi benar.
**STOP CONDITION:** -

---

### ITERATION 12 — Integration & Idempotency Test (Full Scenario)

**GOAL:** Reproduksi skenario end-to-end spec Section 71 & seluruh Acceptance Test Section 75-81 menggunakan Testcontainers (real Postgres+Redis).
**TASK:** Test suite: create user→product→warehouse→inventory→order→reserve→payment→callback→PAID→process→pack→ship→deliver→COMPLETED. Plus test cancellation (Section 77), expiration (Section 78), duplicate callback (Section 80), authorization (Section 81).
**FILES AFFECTED:** `tests/integration/*`
**DEPENDENCIES:** Semua iterasi sebelumnya
**EXPECTED RESULT:** Seluruh acceptance test spec Section 75-81 lolos.
**VALIDATION:** `go test ./tests/integration/... -race`
**STOP CONDITION:** Satu pun acceptance test gagal → jangan declare `COMPLETED`.

---

### ITERATION 13 — Backend QA

**GOAL:** Quality gate final.
**TASK:** `go build ./...`, `go vet ./...`, `go test ./... -race` (SELURUH project, bukan cuma modul Anda — spec Section 72 eksplisit minta ini), lint, review manual: tidak ada mutex Go dipakai sebagai pengganti DB lock untuk data yang perlu konsisten across-instance (mutex hanya boleh untuk in-memory state seperti WebSocket connection manager), tidak ada secret hardcoded.
**FILES AFFECTED:** -
**DEPENDENCIES:** Iterasi 12
**EXPECTED RESULT:** Semua check hijau, termasu race detector bersih.
**VALIDATION:** Checklist Final Quality Gate (spec Section 99) — item concurrency/idempotency/worker/websocket/shipment/audit.
**STOP CONDITION:** `go test -race` menunjukkan race condition apa pun → **HARD STOP**, ini tidak boleh di-skip untuk technical test ini.

---

## 7. DEPENDENCY GATE

- **Jangan** membuat migration domain Anda sebelum `docs/database-contract.md` = LOCKED.
- **Jangan** implementasi endpoint sebelum `docs/api-contract.md` bagian terkait = LOCKED.
- **Jangan** mulai Order/Payment (Iterasi 04+) sebelum Agent 2 minimal selesai Iterasi 03 mereka (JWT+RBAC+middleware) — cek `docs/STATUS.md`.
- Worker/WebSocket skeleton (Iterasi 01) boleh dimulai lebih awal karena tidak bergantung pada contract/Agent 2.

---

## 8. MISMATCH / CHANGE REQUEST PROCEDURE

Sama seperti Agent 2 (lihat `02-BACKEND-FOUNDATION.md` Section 8) — tulis di `docs/CHANGE_REQUESTS.md`, jangan ubah contract langsung, tunggu resolusi dari Agent 1.

Khusus untuk kebutuhan lintas Agent 2 (misal butuh fungsi baru di `pkg/errors` atau field tambahan di JWT claims), tulis Change Request yang sama, tandai `Backend Foundation impact` dengan jelas — Agent 1 akan menjadi mediator jika perlu keputusan arsitektur, atau Anda bisa langsung koordinasi via `docs/STATUS.md` untuk hal teknis kecil yang tidak mengubah contract resmi.

---

## 9. STOP CONDITIONS (khusus domain Anda — tidak boleh dikompromikan)

- **Concurrency test (Iterasi 05) gagal/flaky** — hard stop, ini requirement terpenting seluruh project.
- **Idempotency test (Iterasi 06) gagal** — hard stop, callback ganda menyebabkan efek bisnis ganda.
- `go test -race` menunjukkan race condition di mana pun.
- Worker kehilangan message tanpa retry/dead letter.
- Migration gagal, koneksi database/redis gagal.
- File ownership conflict.

Selain itu: build gagal, test kritikal gagal, existing feature rusak, API contract mismatch yang tidak terselesaikan.

**Jangan menutupi race condition dengan menambah `time.Sleep` — itu bukan fix, itu menyembunyikan bug.**

---

## 10. CLEAN CODE & COMMENT STANDARD

Business logic di service, bukan handler. Transaction boundary jelas (satu fungsi = satu transaksi, jangan nested transaction yang membingungkan). Komentar **Bahasa Indonesia** untuk logic penting:

```go
// Lock diambil dengan urutan product_id ASC untuk mencegah deadlock
// ketika dua request memesan kombinasi produk yang berbeda urutan.
```

```go
// Idempotency dijamin dengan memeriksa status payment SEBELUM
// melakukan efek bisnis apa pun. Jika status sudah PAID,
// callback dianggap berhasil tanpa mengulang efek samping.
```

Jangan komentar di setiap baris.

---

## 11. STATUS REPORTING

Update `docs/STATUS.md`:
```markdown
## Agent 3 — Backend Core
Status: IN PROGRESS / BLOCKED / COMPLETED
Current Iteration: 0X
Last Update: <timestamp>
Concurrency Test: PASS / FAIL / NOT YET RUN
Idempotency Test: PASS / FAIL / NOT YET RUN
Blocking Reason (jika BLOCKED): -
```

---

## 12. FINAL BEHAVIOR CHECK

Apakah requirement ini benar-benar diminta? Apakah saya owner file ini? Apakah dependency (contract, Agent 2 foundation) saya sudah siap? Apakah race condition benar-benar sudah tidak ada (bukan "kelihatannya tidak ada")? Apakah saya bisa menjelaskan ke interviewer persis apa yang terjadi jika 100 orang checkout stok terakhir bersamaan, dan apa yang terjadi jika payment gateway kirim callback 10 kali?
