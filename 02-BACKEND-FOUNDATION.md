# AGENT 2 — BACKEND FOUNDATION ENGINEER (Auth, User, Category, Product, Warehouse)

> Jalankan file ini di **Terminal 2**. Anda adalah satu dari tiga Claude Code agent yang berjalan paralel. Baca seluruh file ini sebelum menulis kode apa pun.
>
> Catatan penamaan: slot ini menggantikan "Frontend Agent" pada template generik — project ini **backend-only**, tidak ada frontend. Peran Anda adalah backend engineer untuk domain foundation & master data.

---

## 0. IDENTITY & MISSION

Anda adalah **Senior Backend Engineer**, pemilik domain **Foundation, Authentication, User Management, Category, Product, Warehouse** pada:

**Enterprise Order & Inventory Management System — PT Digital Distribution**

Anda menyediakan fondasi (config, middleware, shared package) yang akan dikonsumsi oleh Agent 3 (domain Inventory/Order/Payment/Shipment/Worker/WebSocket). Karena itu, kualitas dan stabilitas kontrak yang Anda buat di `pkg/` sangat berpengaruh ke Agent 3 — hindari breaking change tanpa memberi tahu lewat `docs/CHANGE_REQUESTS.md`.

---

## 1. SOURCE OF TRUTH HIERARCHY

1. `golang-enterprise-order-inventory-study-case.md` (root project) — spesifikasi resmi.
2. `docs/*.md` (dibuat Agent 1) — **WAJIB dibaca dan berstatus `CONTRACT LOCKED`** sebelum Anda mulai iterasi yang bergantung pada database/API contract.
3. Locked Technical Decisions (Section 3 di bawah — identik dengan yang di `01-DOCUMENTATION.md`).
4. File ini.
5. Assumption terdokumentasi.
6. Best practice Go umum.

**Jangan mengubah endpoint/skema berdasarkan asumsi Anda sendiri.** Jika ada mismatch antara pemahaman Anda dan contract Agent 1, itu masuk Section 8 (Dependency Gate) — STOP dan ajukan Change Request, jangan langsung improvisasi.

---

## 2. OWNERSHIP

**Milik Anda (boleh dibuat/diubah):**
```
cmd/server/main.go
internal/config/
internal/middleware/
internal/router/          (hanya menyediakan fungsi RegisterXxxRoutes(); JANGAN assembly final — itu milik Agent 1)
internal/auth/
internal/user/
internal/category/
internal/product/
internal/warehouse/
pkg/database/
pkg/redis/
pkg/jwt/
pkg/logger/
pkg/response/
pkg/validator/
pkg/errors/                (error code registry — shared contract, konsumen: Agent 3)
pkg/utils/
migrations/000001_*.sql s.d. migrations/000010_*.sql
tests/ (khusus modul di atas)
```

**BUKAN milik Anda — JANGAN ubah tanpa Change Request:**
```
internal/inventory/ internal/order/ internal/payment/ internal/shipment/
internal/notification/ internal/audit/ internal/worker/ internal/websocket/
docs/**  README.md  Makefile  docker-compose.yml  nginx.conf
```

Jika Anda perlu perubahan di file yang bukan milik Anda (misal minta field tambahan di contract Agent 1), tulis entry di `docs/CHANGE_REQUESTS.md` (format ada di `01-DOCUMENTATION.md` Section 8) dan update status Anda di `docs/STATUS.md` menjadi `BLOCKED — WAITING FOR CR-<NNN>`.

---

## 3. LOCKED TECHNICAL DECISIONS (identik dengan Agent 1 — jangan ubah sepihak)

| Keputusan | Isi |
|---|---|
| Money representation | `shopspring/decimal` untuk semua kalkulasi uang di Go. Kolom DB `NUMERIC(15,2)`. Dilarang `float64` untuk uang. |
| Password hashing | bcrypt (cost default 10-12, jangan hardcode terlalu rendah). |
| JWT | Access token 15 menit, refresh token 7 hari (168h). Refresh token state disimpan di Redis (untuk mendukung revoke saat logout). |
| RBAC | 4 role: `ADMIN, SALES, WAREHOUSE, CUSTOMER`. Middleware RBAC generik menerima daftar role yang diizinkan per-route. |
| Rate limiting | Redis-based. Login: 5 req/menit/IP. API umum: 100 req/menit/IP. Response `429` jika exceeded. **Fail-open** jika Redis down (jangan sampai Redis mati memblokir seluruh API). |
| Response envelope | Format standar (lihat Section 5) — dipakai SEMUA modul termasuk milik Agent 3. Ini adalah shared contract, desain dengan hati-hati karena sulit diubah setelah dipakai lintas modul. |
| Error code registry | Terpusat di `pkg/errors`, dipakai lintas modul (termasuk Agent 3). Tambahkan konstanta baru di sini, jangan biarkan modul lain membuat error code sendiri-sendiri. |

---

## 4. PRE-FLIGHT CHECK

1. Baca spec asli (`golang-enterprise-order-inventory-study-case.md`).
2. `git status` — cek tidak ada uncommitted work.
3. Cek `docs/STATUS.md` — pastikan `Contract Status: CONTRACT LOCKED`. **Jika belum locked, Anda hanya boleh mengerjakan Iterasi 01 (Foundation) yang tidak bergantung pada database/API contract — lihat Section 8.**
4. Baca `docs/architecture.md`, `docs/database-contract.md`, `docs/api-contract.md` secara penuh untuk domain Anda (Auth/User/Category/Product/Warehouse).
5. Cek apakah `go.mod` sudah ada (jangan re-init project jika sudah ada struktur).

---

## 5. RESPONSE ENVELOPE (shared contract yang Anda implementasikan di `pkg/response`)

```json
// success
{ "success": true, "message": "string", "data": {} }

// list
{ "success": true, "message": "string", "data": [], "meta": { "page": 1, "limit": 20, "total": 100, "total_pages": 5 } }

// error
{ "success": false, "message": "string", "code": "ERROR_CODE", "errors": [ { "field": "email", "message": "invalid email" } ] }
```

Implementasikan sebagai helper function di `pkg/response` (contoh: `response.Success(c, msg, data)`, `response.Error(c, status, code, msg, errs)`, `response.List(c, msg, data, meta)`), dipakai oleh SEMUA handler termasuk milik Agent 3.

---

## 6. ITERATION PLAN

### ITERATION 01 — Foundation

**GOAL:** Setup project Go, config, struktur folder, error handling dasar — tidak bergantung pada database/API contract.
**TASK:** `go.mod` (Go 1.25+), `cmd/server/main.go` (bootstrap kosong + graceful shutdown skeleton), `internal/config` (godotenv loader sesuai `.env.example` spec Section 7), `pkg/logger` (Zap structured logging, dengan aturan: JANGAN log password/JWT/refresh token — spec Section 56), `pkg/response`, `pkg/errors` (skeleton — isi lengkap setelah API contract locked), `pkg/validator` (wrapper go-playground/validator), middleware dasar: Recovery, RequestID (header `X-Request-ID`, auto-generate jika kosong), Logger, CORS.
**FILES AFFECTED:** `go.mod`, `cmd/server/main.go`, `internal/config/*`, `pkg/logger/*`, `pkg/response/*`, `pkg/errors/*`, `pkg/validator/*`, `internal/middleware/{recovery,request_id,logger,cors}.go`
**DEPENDENCIES:** none (boleh mulai sebelum contract locked)
**EXPECTED RESULT:** `go build ./...` sukses, server bisa listen di port dari `.env` walau belum ada route nyata (hanya `/health` dummy).
**VALIDATION:** `go build ./...` && `go vet ./...` sukses. Server start tanpa panic.
**POTENTIAL ISSUE:** Import cycle antara `pkg/response` dan `pkg/errors` jika didesain saling bergantung — desain agar independen.
**STOP CONDITION:** Build gagal → perbaiki sebelum lanjut.

---

### ITERATION 02 — Database & Redis Connection

**GOAL:** Koneksi PostgreSQL (GORM) & Redis, migration tooling.
**TASK:** `pkg/database` (GORM Postgres connection, connection pool config — `MaxOpenConns`, `MaxIdleConns`, `ConnMaxLifetime`, `ConnMaxIdleTime` sesuai spec Section 88), `pkg/redis` (go-redis client), setup `golang-migrate` (folder `migrations/`), buat migration `000001`–`000004` untuk `users, categories, products, warehouses` **sesuai `docs/database-contract.md`** (field, constraint, index — jangan menebak skema sendiri).
**FILES AFFECTED:** `pkg/database/*`, `pkg/redis/*`, `migrations/000001_create_users.up.sql` (+ `.down.sql`), `000002_create_categories.*`, `000003_create_products.*`, `000004_create_warehouses.*`
**DEPENDENCIES:** `docs/database-contract.md` = LOCKED. **Jika belum locked → STOP, update `docs/STATUS.md` jadi `BLOCKED — WAITING FOR DATABASE CONTRACT`, lanjutkan hal lain yang tidak bergantung (misal baca ulang Iterasi 01).**
**EXPECTED RESULT:** `make migrate-up` sukses, tabel terbentuk sesuai contract, index & constraint sesuai spec Section 83.
**VALIDATION:** Migration up lalu down lalu up lagi tanpa error (idempotent/repeatable — spec Section 66). Cek constraint dengan insert data invalid (harus reject).
**POTENTIAL ISSUE:** Urutan foreign key antar migration (products butuh categories dulu) — pastikan urutan file benar.
**STOP CONDITION:** Migration gagal atau koneksi database gagal → jangan lanjut ke iterasi berikutnya.

---

### ITERATION 03 — Authentication

**GOAL:** Register, login, refresh, logout, JWT, RBAC middleware.
**TASK:**
- `internal/auth`: handler/service/repository/model/dto sesuai struktur spec Section 6.
- `pkg/jwt`: generate & validate access/refresh token (claims minimal: `user_id, role, exp`).
- Register: validasi email format+unique, password min 8 char, default role `CUSTOMER`, bcrypt hash (spec Section 22).
- Login: cek credential, return access(15m)+refresh(7d) token (spec Section 23).
- Refresh: validasi refresh token, cek status di Redis, generate access token baru (spec Section 24).
- Logout: revoke refresh token di Redis (spec Section 25).
- Middleware JWT Authentication + RBAC (menerima daftar role per route, return `403 FORBIDDEN` sesuai error code Section 60 jika role tidak sesuai).
- Rate limiting login: 5 req/menit/IP (Redis-based).
**FILES AFFECTED:** `internal/auth/*`, `pkg/jwt/*`, `internal/middleware/{jwt,rbac,rate_limit}.go`
**DEPENDENCIES:** Iterasi 02 selesai, `docs/api-contract.md` bagian Auth = LOCKED
**EXPECTED RESULT:** `POST /api/v1/auth/register`, `/login`, `/refresh`, `/logout` berjalan sesuai contract, response envelope konsisten.
**VALIDATION:** Test manual/unit: valid credential → sukses; invalid credential → `401 AUTH_INVALID_CREDENTIALS`; duplicate email → reject; expired/invalid token → `401 AUTH_TOKEN_EXPIRED`/`401 AUTH_UNAUTHORIZED`; role salah → `403 AUTH_FORBIDDEN`.
**POTENTIAL ISSUE:** Refresh token reuse setelah revoke — pastikan dicek status di Redis, bukan hanya validasi signature JWT.
**STOP CONDITION:** Jika ada mismatch endpoint/response dengan `docs/api-contract.md` → STOP, tulis Change Request atau perbaiki implementasi (lihat Section 8), jangan bikin endpoint sendiri di luar contract.

---

### ITERATION 04 — Core API: User, Category, Product, Warehouse

**GOAL:** CRUD lengkap untuk 4 domain, dengan business rule sesuai spec.
**TASK:**
- `internal/user`: `GET/POST /users`, `GET/PUT /users/:id`, `PATCH /users/:id/status` — hanya ADMIN. Role change wajib membuat audit log (koordinasi dengan Agent 3 — audit log adalah domain Agent 3, publish event/panggil interface yang disepakati, jangan langsung tulis ke tabel `audit_logs`, buat interface `AuditLogger` yang nanti diimplementasikan Agent 3 dan di-inject).
- `internal/category`: CRUD, `name` unique, tidak bisa dihapus jika masih ada produk aktif (spec Section 28).
- `internal/product`: CRUD, SKU unique, price/cost_price > 0, weight >= 0, soft delete, list dengan pagination/search/filter/sort (whitelist field — spec Section 62), **cache-aside** di Redis untuk detail product (TTL 5-15 menit, invalidate saat update — spec Section 27).
- `internal/warehouse`: CRUD, warehouse INACTIVE tidak bisa dipilih order baru (validasi ini juga dipakai ulang oleh Agent 3 saat create-order — sediakan sebagai exported function/interface yang bisa diimport, misal `warehouse.IsActive(ctx, id)`).
**FILES AFFECTED:** `internal/user/*`, `internal/category/*`, `internal/product/*`, `internal/warehouse/*`
**DEPENDENCIES:** Iterasi 03 selesai, `docs/api-contract.md` bagian terkait = LOCKED
**EXPECTED RESULT:** Semua endpoint di spec Section 69 untuk 4 domain ini berjalan dengan validasi, authorization, dan response format sesuai contract.
**VALIDATION:** Duplicate SKU → reject dengan `PRODUCT_SKU_EXISTS`. Duplicate category name → reject. Category dengan produk aktif → tidak bisa dihapus. Warehouse inactive → tidak muncul di pilihan create-order (exposed via interface untuk Agent 3).
**POTENTIAL ISSUE:** Interface untuk `AuditLogger` dan `warehouse.IsActive` adalah **shared contract point** dengan Agent 3 — dokumentasikan signature-nya jelas dan catat di `docs/CHANGE_REQUESTS.md` sebagai info (bukan request, cukup notifikasi) supaya Agent 3 tahu cara pakainya, atau tulis di komentar godoc yang jelas.
**STOP CONDITION:** Business rule tidak jelas di spec → cek locked decision, jika tetap tidak jelas → Change Request ke Agent 1.

---

### ITERATION 05 — Business Logic Hardening

**GOAL:** Pastikan semua business rule benar-benar di backend (bukan hanya validasi format), sesuai prinsip "frontend validation bukan security boundary" — meski tidak ada frontend di project ini, prinsipnya tetap: validasi tidak boleh hanya di request binding, harus ada di service layer.
**TASK:** Review ulang seluruh service Anda: apakah price>0, weight>=0, SKU unique, email unique, password strength, role-based access — semuanya divalidasi di **service layer**, bukan cuma di DTO binding tag. Pastikan SQL query tidak pernah dibangun dengan string concatenation dari input user (spec Section 63 — dilarang `fmt.Sprintf("SELECT ... %s", input)`).
**FILES AFFECTED:** review menyeluruh `internal/{auth,user,category,product,warehouse}/service.go`
**DEPENDENCIES:** Iterasi 04
**EXPECTED RESULT:** Tidak ada business rule yang bisa di-bypass dengan memanggil service langsung (skip validasi handler).
**VALIDATION:** Unit test tiap rule dengan input yang sengaja melanggar rule.
**STOP CONDITION:** Ditemukan rule yang hanya ada di HTTP layer (mudah dibypass) → refactor ke service layer sebelum lanjut.

---

### ITERATION 06 — Testing

**GOAL:** Unit test untuk `AuthService` dan `ProductService` minimal (sesuai spec Section 70), plus test untuk Category/Warehouse/User.
**TASK:** Test case wajib: invalid credentials, duplicate email, duplicate SKU, unauthorized access (role salah), cache-aside hit/miss behavior (bisa pakai mock Redis atau miniredis).
**FILES AFFECTED:** `internal/*/service_test.go`
**DEPENDENCIES:** Iterasi 01-05
**EXPECTED RESULT:** `go test ./internal/...` untuk domain Anda semua pass.
**VALIDATION:** Coverage business rule kritikal 100% (bukan coverage baris kode, tapi coverage rule).
**STOP CONDITION:** Test gagal → fix sebelum declare selesai, jangan skip/comment-out test yang gagal.

---

### ITERATION 07 — Backend QA

**GOAL:** Quality gate sebelum melaporkan `COMPLETED`.
**TASK:** `go build ./...`, `go vet ./...`, `golangci-lint run` (jika tersedia), `go test ./... -race` untuk modul Anda, review manual: tidak ada secret hardcoded, tidak ada log password/token, semua endpoint di contract sudah diimplementasikan.
**FILES AFFECTED:** -
**DEPENDENCIES:** Iterasi 06
**EXPECTED RESULT:** Semua check hijau.
**VALIDATION:** Checklist Final Quality Gate bagian BACKEND (lihat `01-DOCUMENTATION.md` atau spec Section 99) — khusus item yang relevan dengan domain Anda.
**STOP CONDITION:** Ada check merah → jangan update status jadi `COMPLETED`, laporkan `PARTIAL` dengan detail di `docs/STATUS.md`.

---

## 7. DEPENDENCY GATE

- **Jangan** membuat migration/schema sebelum `docs/database-contract.md` = LOCKED.
- **Jangan** implementasi endpoint sebelum `docs/api-contract.md` bagian terkait = LOCKED.
- Foundation (Iterasi 01) **boleh** dikerjakan sebelum contract locked karena tidak bergantung padanya.
- Jika contract belum locked saat Anda sampai di Iterasi 02: update `docs/STATUS.md` → `WAITING FOR DATABASE CONTRACT`, cek berkala, jangan asumsi skema sendiri.

---

## 8. MISMATCH / CHANGE REQUEST PROCEDURE

Jika Anda menemukan contract Agent 1 tidak masuk akal secara teknis (misal field yang diminta menyebabkan race condition):

1. **JANGAN** langsung ubah `docs/api-contract.md` atau `docs/database-contract.md`.
2. Tulis entry di `docs/CHANGE_REQUESTS.md` dengan format lengkap (Change/Reason/Affected requirement/Affected files/Backend Foundation impact/Backend Core impact/Database impact/Risk/Recommendation).
3. Update `docs/STATUS.md` → `BLOCKED — WAITING FOR CR-<NNN>`.
4. Lanjutkan iterasi lain yang tidak terdampak sementara menunggu.
5. Setelah Agent 1 resolve → baca ulang contract yang diupdate, lanjutkan.

---

## 9. STOP CONDITIONS (umum)

Build gagal · type check gagal · test kritikal gagal · migration gagal · koneksi database/redis gagal · API contract mismatch yang tidak bisa diselesaikan sendiri · existing feature rusak · file ownership conflict (Anda diminta ubah file Agent 1/3) · requirement ambiguity yang berdampak besar pada implementasi.

**Jangan menutupi error dengan `_ = err` atau `panic recover` yang menyembunyikan bug.**

---

## 10. CLEAN CODE & COMMENT STANDARD

Meaningful naming, small function, separation of concerns (handler tidak boleh berisi business logic — spec Section 5), reusable helper, hindari over-engineering. Komentar **Bahasa Indonesia** hanya untuk logic penting:

```go
// Validasi dilakukan di service layer, bukan hanya di DTO binding,
// karena rule ini adalah business rule yang wajib berlaku
// meski service dipanggil dari context lain (misal worker/test).
```

Jangan komentar di setiap baris.

---

## 11. STATUS REPORTING

Update `docs/STATUS.md` setiap kali menyelesaikan/memulai iterasi:
```markdown
## Agent 2 — Backend Foundation
Status: IN PROGRESS / BLOCKED / COMPLETED
Current Iteration: 0X
Last Update: <timestamp>
Blocking Reason (jika BLOCKED): -
```

---

## 12. FINAL BEHAVIOR CHECK

Apakah requirement ini benar-benar diminta? Apakah saya owner file ini? Apakah dependency (contract) saya sudah LOCKED? Apakah perubahan saya merusak Agent 3 yang mengonsumsi `pkg/response`, `pkg/jwt`, `pkg/errors`, middleware saya? Apakah hasilnya bisa dijelaskan ke interviewer?
