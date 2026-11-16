# NgawiMusical Backend

REST API untuk penjualan tiket konser NgawiMusical. Proyek ini dibangun dengan Go dan Clean Architecture sebagai tugas kelompok mata kuliah Pemrograman Backend Lanjut.

## Deskripsi Sistem

NgawiMusical Backend melayani katalog konser, pengelolaan event, reservasi hold tiket, pembatalan hold, autentikasi, dan health check. PostgreSQL menjadi sumber kebenaran untuk stok, booking, idempotency, ledger, dan outbox. Redis dipakai untuk admission dan antrean. MongoDB disiapkan untuk modul ulasan.

Fokus utama sistem adalah mencegah stok negatif, membatasi tiket per akun, membuat retry aman melalui idempotency, dan menjaga seluruh perubahan stok dalam satu transaksi.

## Tim dan Peran

| Anggota | NIM | GitHub | Peran | Branch |
| --- | --- | --- | --- | --- |
| Muhammad Ikhsanudin Arsalan | 434241083 | [@ZaganJade](https://github.com/ZaganJade) | Autentikasi, otorisasi, keamanan API | `feat/auth-arsalan` |
| Vito Aditya | 434241084 | [@viesloxy](https://github.com/viesloxy) | Git Master, infrastruktur, integrasi | `feat/infra-vito` |
| Muhammad Rizki Ibrahim | 434241085 | [@rzkibrahim](https://github.com/rzkibrahim) | Event, booking, konkurensi, worker | `feat/domain-rizki` |

## Role pada Sistem

| Role | Hak akses utama |
| --- | --- |
| Pengunjung | Melihat katalog event published dan daftar tier |
| Attendee | Mendaftar, login, melihat katalog, membuat hold, dan membatalkan hold |
| Organizer | Mengelola event miliknya, menambah tier, publish, pause, resume, close, dan cancel |
| Gate staff | Disiapkan untuk validasi tiket pada tahap check-in |
| Admin | Disiapkan untuk pengelolaan role, exception, audit, dan operasi |

## Fitur

| No | Fitur | Role | Status |
| --- | --- | --- | --- |
| 1 | Registrasi dan login | Semua | Selesai |
| 2 | Refresh token dan logout | Attendee, organizer, admin | Selesai |
| 3 | Verifikasi email dan reset password | Semua | Selesai di kode, pengiriman email belum |
| 4 | Katalog event published | Publik | Selesai |
| 5 | Pengelolaan draft event dan tier | Organizer, admin | Selesai |
| 6 | Publish, pause, resume, close, cancel | Organizer, admin | Selesai |
| 7 | Reservasi hold tiket | Attendee | Selesai |
| 8 | Pembatalan hold | Attendee | Selesai |
| 9 | Idempotency create hold | Attendee | Selesai |
| 10 | Health check dan Docker Compose | Infrastruktur | Selesai |
| 11 | Pembayaran, e-ticket, check-in | Attendee, gate staff | Belum |
| 12 | Refund, laporan, ulasan | Admin, organizer | Belum |

## Teknologi

- Go 1.26
- Fiber v2
- PostgreSQL
- Redis
- MongoDB
- Docker Compose

## Arsitektur

```text
Client
  -> Fiber API
  -> service HTTP
  -> usecase
  -> repository
  -> PostgreSQL

Redis admission dan queue, MongoDB ulasan, worker, serta scheduler
dijalankan sebagai komponen pendukung.
```

Aturan dependensi:

- `app/model` tidak mengimpor package internal proyek.
- `app/repository` tidak mengimpor Fiber.
- `app/usecase` tidak mengimpor Fiber maupun `app/service`, dan tidak menulis SQL.
- `app/service` menangani HTTP dan memanggil `app/usecase`.
- Semua error dipetakan ke envelope response yang sama.
- Tidak ada folder `controller/` atau `handler/`.

## Struktur Repository

```text
app/
  model/        entity, value object, DTO, aturan transisi
  repository/   kontrak dan implementasi akses data
  usecase/      logika bisnis dan transaksi
  service/      handler HTTP
config/         konfigurasi aplikasi
database/       koneksi database dan migrasi
helper/         error, response, validator, cursor, crypto
middleware/     request id, actor, auth, rate limit
route/          registrasi endpoint
worker/         worker dan scheduler
```

## Rancangan Database

| Kelompok | Tabel | Fungsi |
| --- | --- | --- |
| Autentikasi | `users`, `roles`, `role_permissions`, `refresh_tokens`, `identity_action_tokens` | Akun, role, sesi, dan token aksi |
| Event | `events`, `event_staff`, `ticket_tiers`, `user_event_limits` | Katalog, tier, staf, dan batas tiket per akun |
| Booking | `bookings`, `booking_changes`, `admission_redemptions`, `idempotency_records`, `inventory_movements` | Hold, riwayat, idempotency, dan ledger stok |
| Pembayaran | `payment_attempts`, `payment_inbox`, `refunds` | Disiapkan untuk pembayaran dan refund |
| Tiket | `tickets`, `check_ins` | Disiapkan untuk e-ticket dan check-in |
| Operasional | `outbox`, `consumer_receipts`, `notification_deliveries`, `audit_logs`, `operational_exceptions` | Pekerjaan asinkron, audit, dan exception |

Invariant utama:

- `held_count + sold_count + retired_count <= quota`.
- `held + purchased_total <= frozen_max`.
- Satu akun hanya boleh punya satu hold aktif per event.
- Create hold memakai conditional update agar stok tidak negatif.
- Uang memakai integer rupiah `int64`, waktu memakai UTC.

## Endpoint

| Method | Path | Akses |
| --- | --- | --- |
| POST | `/api/v1/auth/register` | Publik |
| POST | `/api/v1/auth/login` | Publik |
| POST | `/api/v1/auth/refresh` | Refresh token |
| POST | `/api/v1/auth/logout` | Pemilik sesi |
| GET | `/api/v1/auth/me` | Terautentikasi |
| POST | `/api/v1/auth/verify-email/request` | Terautentikasi |
| POST | `/api/v1/auth/verify-email` | Token |
| POST | `/api/v1/auth/password/forgot` | Publik |
| POST | `/api/v1/auth/password/reset` | Token |
| GET | `/api/v1/events` | Publik |
| GET | `/api/v1/events/:id` | Publik atau pemilik |
| GET | `/api/v1/events/:id/tiers` | Publik atau pemilik |
| POST | `/api/v1/events` | Organizer, admin |
| PATCH | `/api/v1/events/:id` | Organizer pemilik, admin |
| POST | `/api/v1/events/:id/tiers` | Organizer pemilik, admin |
| PATCH | `/api/v1/tiers/:id` | Organizer pemilik, admin |
| POST | `/api/v1/events/:id/publish` | Organizer pemilik, admin |
| POST | `/api/v1/events/:id/pause` | Organizer pemilik, admin |
| POST | `/api/v1/events/:id/resume` | Organizer pemilik, admin |
| POST | `/api/v1/events/:id/close` | Organizer pemilik, admin |
| POST | `/api/v1/events/:id/cancel` | Organizer pemilik, admin |
| POST | `/api/v1/events/:id/complete` | Organizer pemilik, admin |
| POST | `/api/v1/bookings` | Attendee, Idempotency-Key |
| GET | `/api/v1/bookings` | Pemilik |
| GET | `/api/v1/bookings/:id` | Pemilik |
| POST | `/api/v1/bookings/:id/cancel` | Pemilik |
| GET | `/health/live` | Infrastruktur |
| GET | `/health/ready` | Infrastruktur |

Response sukses memakai bentuk `{"data": ..., "request_id": "req_..."}`. Response error memakai bentuk `{"error": {"code": ..., "message": ..., "fields": ..., "retryable": ...}, "request_id": "req_..."}`.

## Menjalankan

Prasyarat:

- Go 1.26 atau lebih baru
- Docker dan Docker Compose

Menjalankan seluruh layanan dengan Docker:

```bash
git clone https://github.com/viesloxy/ngawimusical-backend.git
cd ngawimusical-backend
cp .env.example .env
docker compose up -d --build
```

Jika port 8080 dipakai proses lain, atur port host:

```bash
API_PORT=18080 docker compose up -d --build
```

Menjalankan API lokal dengan dependency Docker:

```bash
docker compose up -d postgres redis-admission redis-queue mongo
go mod tidy
go run . -role=api
```

Migrasi berjalan otomatis saat aplikasi start dan dilindungi advisory lock agar aman ketika API, worker, dan scheduler start bersamaan.

## Pengujian

```bash
gofmt -l .
go vet ./...
go build ./...
go test ./... -count=1
```

Test integrasi repository membutuhkan PostgreSQL. Jika dijalankan dari host, arahkan `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, dan `DB_NAME` ke database uji. Jika port host bentrok, jalankan test di network Docker.

## Alur Git

- `main` hanya diubah melalui pull request.
- Setiap anggota bekerja pada branch sendiri.
- Commit dibuat kecil dengan pesan bahasa Indonesia.
- Pull request memerlukan review code owner.
- Merge memakai merge commit, bukan squash.
- Force push dan penghapusan branch diblokir.

Pull request yang sudah merged:

| PR | Branch | Isi |
| --- | --- | --- |
| #1 | `feat/domain-rizki` | Katalog event dan reservasi hold |
| #2 | `feat/auth-arsalan` | Autentikasi dan otorisasi |
| #3 | `feat/infra-vito` | Integrasi route, migrasi, Docker, README |
| #4 | `feat/infra-vito` | CODEOWNERS dan review wajib |
| #5 | `feat/auth-arsalan` | Verifikasi email, reset password, perbaikan event, test integrasi |

## Status dan Batasan

Sudah berjalan:

- Autentikasi, rotasi refresh token, dan logout.
- Katalog event, tier, publish, dan pengelolaan sale control.
- Reservasi hold, pembatalan hold, dan idempotency.
- Health check dan Docker Compose.

Belum dikerjakan:

- Pembayaran, webhook, refund, e-ticket, check-in, dan ulasan.
- Waiting room dan admission pass.
- Pengiriman email verifikasi dan reset password.
- Worker dan scheduler masih berupa loop placeholder.
- Uji konkurensi penuh dan load test.

## Pranala

- Repository: https://github.com/viesloxy/ngawimusical-backend
- Pull request: https://github.com/viesloxy/ngawimusical-backend/pulls
- Issues: https://github.com/viesloxy/ngawimusical-backend/issues
