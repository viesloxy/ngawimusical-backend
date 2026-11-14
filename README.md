# NgawiMusical Backend

REST API untuk penjualan tiket konser NgawiMusical. Proyek ini dibangun dengan Go dan Clean Architecture sebagai tugas kelompok mata kuliah Pemrograman Backend Lanjut.

## Fitur

- Autentikasi dan otorisasi berbasis role
- Katalog event dan kategori tiket
- Waiting room dan pembatasan antrean
- Reservasi stok sementara
- Pembayaran dan penerbitan e-ticket
- Check-in tiket
- Laporan penjualan

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

Migrasi database dijalankan otomatis saat aplikasi start. Proses `worker` dan `scheduler` saat ini masih berupa loop placeholder sampai modulnya diimplementasikan.

## Endpoint

| Method | Path | Keterangan |
| --- | --- | --- |
| GET | `/health/live` | Liveness probe |
| GET | `/health/ready` | Readiness probe |
| GET | `/api/v1/health/live` | Alias liveness |
| GET | `/api/v1/health/ready` | Alias readiness |

## Pengujian

```bash
go vet ./...
go build ./...
go test ./...
```

## Status

Fondasi backend sudah tersedia: konfigurasi, koneksi PostgreSQL, Redis, MongoDB, migrasi awal, helper response, health check, dan Docker Compose. Modul autentikasi dan transaksi tiket sedang dikerjakan.

## Tim

| Nama | GitHub | Fokus |
| --- | --- | --- |
| Muhammad Ikhsanudin Arsalan | [@ZaganJade](https://github.com/ZaganJade) | Autentikasi dan keamanan API |
| Vito Aditya | [@viesloxy](https://github.com/viesloxy) | Infrastruktur dan integrasi |
| Muhammad Rizki Ibrahim | [@rzkibrahim](https://github.com/rzkibrahim) | Reservasi dan pembayaran |
