package repository

import "errors"

// Error sentinel dipakai repository dan usecase agar pemetaan ke response HTTP
// tidak bergantung pada kode PostgreSQL atau tipe driver.
var (
	ErrNotFound             = errors.New("resource tidak ditemukan")
	ErrConflict             = errors.New("konflik data")
	ErrSoldOut              = errors.New("stok tiket habis")
	ErrUserLimitExceeded    = errors.New("batas tiket pengguna terlampaui")
	ErrActiveHoldExists     = errors.New("masih ada hold aktif")
	ErrBookingExpired       = errors.New("booking sudah kedaluwarsa")
	ErrVersionConflict      = errors.New("versi data tidak cocok")
	ErrIdempotencyKeyReused = errors.New("idempotency key dipakai dengan payload berbeda")
	ErrOperationInProgress  = errors.New("operasi dengan key yang sama sedang berjalan")
)
