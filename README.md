# NgawiMusical Backend

REST API untuk penjualan tiket konser NgawiMusical. Proyek ini dibangun dengan Go dan Clean Architecture sebagai tugas kelompok mata kuliah Pemrograman Backend Lanjut.

## Fitur

Sudah tersedia:

- Registrasi, login, refresh token, dan logout.
- Otorisasi berbasis role dan ownership.
- Katalog event published beserta tier.
- Pembuatan draft event, tier, publish, pause, resume, dan close.
- Reservasi hold tiket dengan batas stok dan batas per akun.
- Pembatalan hold sebelum pembayaran.
- Idempotency untuk create hold.
- Health check dan Docker Compose.

Rencana berikutnya:

- Waiting room dan admission pass.
- Pembayaran dan penerbitan e-ticket.
- Check-in tiket.
- Refund dan laporan penjualan.
- Modul ulasan MongoDB.

## Teknologi

- Go 1.26
- Fiber v2
- PostgreSQL
- Redis
- MongoDB
- Docker Compose

## Struktur proyek

```text
app/
  model/        entity, value object, dan DTO
  repository/   kontrak dan implementasi akses data
  service/      usecase dan handler HTTP
config/         konfigurasi aplikasi
database/       koneksi database dan migrasi
helper/         utilitas umum
middleware/     middleware HTTP
route/          registrasi endpoint
worker/         pekerjaan asinkron
```

## Proses

Aplikasi dapat dijalankan sebagai beberapa proses:

| Role | Fungsi |
| --- | --- |
| `api` | HTTP API |
| `worker` | Pekerjaan pembayaran, refund, dan notifikasi |
| `scheduler` | Pemindai expiry dan rekonsiliasi |
| `all` | Semua proses dalam satu binary |

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

Menjalankan API secara lokal dan dependency lewat Docker:

```bash
docker compose up -d postgres redis-admission redis-queue mongo
go mod tidy
go run . -role=api
```

Migrasi database dijalankan otomatis saat aplikasi start dan dilindungi advisory lock agar aman ketika beberapa proses start bersamaan. Port host Compose dapat diubah lewat `API_PORT`, misalnya `API_PORT=18080 docker compose up -d`. Proses `worker` dan `scheduler` saat ini masih berupa loop placeholder sampai modul pembayaran dan expiry diimplementasikan.

## Endpoint

| Method | Path | Keterangan |
| --- | --- | --- |
| GET | `/health/live` | Liveness probe |
| GET | `/health/ready` | Readiness probe |
| POST | `/api/v1/auth/register` | Registrasi akun |
| POST | `/api/v1/auth/login` | Login |
| POST | `/api/v1/auth/refresh` | Rotasi refresh token |
| POST | `/api/v1/auth/logout` | Logout |
| GET | `/api/v1/auth/me` | Profil sendiri |
| GET | `/api/v1/events` | Daftar event published |
| GET | `/api/v1/events/:id` | Detail event |
| GET | `/api/v1/events/:id/tiers` | Daftar tier |
| POST | `/api/v1/events` | Buat draft event |
| PATCH | `/api/v1/events/:id` | Ubah draft event |
| POST | `/api/v1/events/:id/tiers` | Tambah tier |
| PATCH | `/api/v1/tiers/:id` | Ubah tier |
| POST | `/api/v1/events/:id/publish` | Publikasikan event |
| POST | `/api/v1/events/:id/pause` | Jeda penjualan |
| POST | `/api/v1/events/:id/resume` | Lanjutkan penjualan |
| POST | `/api/v1/events/:id/close` | Tutup penjualan |
| POST | `/api/v1/events/:id/cancel` | Batalkan event |
| POST | `/api/v1/events/:id/complete` | Selesaikan event |
| POST | `/api/v1/bookings` | Buat hold tiket |
| GET | `/api/v1/bookings` | Daftar booking sendiri |
| GET | `/api/v1/bookings/:id` | Detail booking |
| POST | `/api/v1/bookings/:id/cancel` | Batalkan hold |

Endpoint create booking membutuhkan header `Idempotency-Key` sepanjang 16 sampai 128 karakter.

## Pengujian

```bash
go vet ./...
go build ./...
go test ./...
```

## Status

Modul autentikasi, katalog event, dan reservasi hold sudah berjalan dan diuji melalui Docker Compose. Pembayaran, e-ticket, check-in, refund, laporan, dan ulasan belum diimplementasikan.

## Tim

| Nama | GitHub | Fokus |
| --- | --- | --- |
| Muhammad Ikhsanudin Arsalan | [@ZaganJade](https://github.com/ZaganJade) | Autentikasi dan keamanan API |
| Vito Aditya | [@viesloxy](https://github.com/viesloxy) | Infrastruktur dan integrasi |
| Muhammad Rizki Ibrahim | [@rzkibrahim](https://github.com/rzkibrahim) | Reservasi dan pembayaran |
