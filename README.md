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

## Menjalankan

Prasyarat:

- Go 1.26 atau lebih baru
- Docker dan Docker Compose

```bash
git clone https://github.com/viesloxy/ngawimusical-backend.git
cd ngawimusical-backend
cp .env.example .env
docker compose up -d
go mod tidy
go run . -role=all
```

## Pengujian

```bash
go vet ./...
go build ./...
go test ./...
```

## Status

Tahap awal pengembangan. Struktur backend dan fitur inti sedang dikerjakan.

## Tim

| Nama | GitHub | Fokus |
| --- | --- | --- |
| Muhammad Ikhsanudin Arsalan | [@ZaganJade](https://github.com/ZaganJade) | Autentikasi dan keamanan API |
| Vito Aditya | [@viesloxy](https://github.com/viesloxy) | Infrastruktur dan integrasi |
| Muhammad Rizki Ibrahim | [@rzkibrahim](https://github.com/rzkibrahim) | Reservasi dan pembayaran |
