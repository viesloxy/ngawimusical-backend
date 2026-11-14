package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/helper"
)

// BookingRepository menyediakan akses data untuk entitas booking.
type BookingRepository interface {
	Create(ctx context.Context, tx DBTX, booking *model.Booking) error
	GetByID(ctx context.Context, tx DBTX, id int64) (*model.Booking, error)
	GetByIDForUpdate(ctx context.Context, tx DBTX, id int64) (*model.Booking, error)
	GetByBookingCode(ctx context.Context, tx DBTX, code string) (*model.Booking, error)
	ListByUserID(ctx context.Context, tx DBTX, userID int64, cursor *helper.Cursor, limit int) ([]model.Booking, error)
	ListByEventID(ctx context.Context, tx DBTX, eventID int64, cursor *helper.Cursor, limit int) ([]model.Booking, error)
	Update(ctx context.Context, tx DBTX, booking *model.Booking) error
	UpdateStatus(ctx context.Context, tx DBTX, id int64, status string, version int) error
	FindActiveByUserAndEvent(ctx context.Context, tx DBTX, userID int64, eventID int64) (*model.Booking, error)
	ListDueRelease(ctx context.Context, tx DBTX, now time.Time, limit int) ([]model.Booking, error)
	ListActiveByEventID(ctx context.Context, tx DBTX, eventID int64) ([]model.Booking, error)
	ListConfirmedByEventID(ctx context.Context, tx DBTX, eventID int64) ([]model.Booking, error)
}

// UserEventLimitRepository menyediakan akses data untuk batas tiket per user per event.
type UserEventLimitRepository interface {
	UpsertAndLock(ctx context.Context, tx DBTX, eventID int64, userID int64, frozenMax int) (*model.UserEventLimit, error)
	AddHeld(ctx context.Context, tx DBTX, eventID int64, userID int64, delta int) error
	SubtractHeld(ctx context.Context, tx DBTX, eventID int64, userID int64, delta int) error
	AddPurchasedTotal(ctx context.Context, tx DBTX, eventID int64, userID int64, delta int) error
	GetByEventAndUser(ctx context.Context, tx DBTX, eventID int64, userID int64) (*model.UserEventLimit, error)
}

// IdempotencyRepository menyediakan akses data untuk idempotency record.
type IdempotencyRepository interface {
	InsertOrGet(ctx context.Context, tx DBTX, record *model.IdempotencyRecord) (*model.IdempotencyRecord, bool, error)
	Complete(ctx context.Context, tx DBTX, userID int64, scope string, key string,
		responseStatus int, responseBody []byte, resourceID string) error
}

// BookingChangeRepository menyediakan akses data untuk riwayat perubahan booking.
type BookingChangeRepository interface {
	Create(ctx context.Context, tx DBTX, change *model.BookingChange) error
}

// AdmissionRedemptionRepository menyediakan akses data untuk penggunaan admission pass.
type AdmissionRedemptionRepository interface {
	Create(ctx context.Context, tx DBTX, redemption *model.AdmissionRedemption) error
}

// PaymentAttemptRepository menyediakan akses data untuk payment attempt.
type PaymentAttemptRepository interface {
	Create(ctx context.Context, tx DBTX, attempt *model.PaymentAttempt) error
	GetByID(ctx context.Context, tx DBTX, id int64) (*model.PaymentAttempt, error)
	GetByBookingID(ctx context.Context, tx DBTX, bookingID int64) (*model.PaymentAttempt, error)
	GetByMerchantOrderID(ctx context.Context, tx DBTX, orderID string) (*model.PaymentAttempt, error)
	Update(ctx context.Context, tx DBTX, attempt *model.PaymentAttempt) error
	ListPendingReconciliation(ctx context.Context, tx DBTX, limit int) ([]model.PaymentAttempt, error)
}

// PaymentInboxRepository menyediakan akses data untuk payment webhook inbox.
type PaymentInboxRepository interface {
	Upsert(ctx context.Context, tx DBTX, inbox *model.PaymentInbox) (bool, error)
	ClaimBatch(ctx context.Context, tx DBTX, leaseUntil time.Time, limit int) ([]model.PaymentInbox, error)
	MarkDone(ctx context.Context, tx DBTX, id int64) error
	MarkFailed(ctx context.Context, tx DBTX, id int64, err string, retryAt time.Time) error
}

// RefundRepository menyediakan akses data untuk refund.
type RefundRepository interface {
	Create(ctx context.Context, tx DBTX, refund *model.Refund) error
	GetByID(ctx context.Context, tx DBTX, id int64) (*model.Refund, error)
	GetByCaseKey(ctx context.Context, tx DBTX, caseKey string) (*model.Refund, error)
	Update(ctx context.Context, tx DBTX, refund *model.Refund) error
	ListPending(ctx context.Context, tx DBTX, limit int) ([]model.Refund, error)
}

// TicketRepository menyediakan akses data untuk tiket.
type TicketRepository interface {
	CreateBatch(ctx context.Context, tx DBTX, tickets []model.Ticket) error
	GetByTokenHash(ctx context.Context, tx DBTX, tokenHash string) (*model.Ticket, error)
	GetByIDForUpdate(ctx context.Context, tx DBTX, id int64) (*model.Ticket, error)
	ListByBookingID(ctx context.Context, tx DBTX, bookingID int64) ([]model.Ticket, error)
	UpdateStatus(ctx context.Context, tx DBTX, id int64, status string) error
	CheckIn(ctx context.Context, tx DBTX, id int64) (bool, error)
	VoidByBookingID(ctx context.Context, tx DBTX, bookingID int64) (int, error)
}

// CheckInRepository menyediakan akses data untuk check-in.
type CheckInRepository interface {
	Create(ctx context.Context, tx DBTX, checkIn *model.CheckIn) error
	GetByTicketID(ctx context.Context, tx DBTX, ticketID int64) (*model.CheckIn, error)
}

// OperationalExceptionRepository menyediakan akses data untuk kasus operasional.
type OperationalExceptionRepository interface {
	Create(ctx context.Context, tx DBTX, exception *model.OperationalException) error
	GetByCaseKey(ctx context.Context, tx DBTX, caseKey string) (*model.OperationalException, error)
	Update(ctx context.Context, tx DBTX, exception *model.OperationalException) error
}

// ==========================================
// Postgres Implementations
// ==========================================

// PostgresBookingRepository implementasi PostgreSQL untuk BookingRepository.
type PostgresBookingRepository struct {
	db DBTX
}

func NewPostgresBookingRepository(db DBTX) *PostgresBookingRepository {
	return &PostgresBookingRepository{db: db}
}

func (r *PostgresBookingRepository) executor(tx DBTX) DBTX {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *PostgresBookingRepository) Create(ctx context.Context, tx DBTX, b *model.Booking) error {
	query := `
		INSERT INTO bookings (booking_code, user_id, event_id, tier_id, quantity, status,
		                      unit_price_amount, unit_fee_amount, total_amount, currency,
		                      expires_at, release_at, swap_count, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, clock_timestamp(), clock_timestamp())
		RETURNING id, created_at, updated_at`

	err := r.executor(tx).QueryRow(ctx, query,
		b.BookingCode, b.UserID, b.EventID, b.TierID, b.Quantity, b.Status,
		b.UnitPriceAmount, b.UnitFeeAmount, b.TotalAmount, b.Currency,
		b.ExpiresAt, b.ReleaseAt, b.SwapCount, b.Version,
	).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				if pgErr.ConstraintName == "bookings_one_active_per_user_event" {
					return ErrActiveHoldExists
				}
				return ErrConflict
			}
		}
		return fmt.Errorf("insert booking: %w", err)
	}
	return nil
}

func (r *PostgresBookingRepository) GetByID(ctx context.Context, tx DBTX, id int64) (*model.Booking, error) {
	query := `
		SELECT id, booking_code, user_id, event_id, tier_id, quantity, status,
		       unit_price_amount, unit_fee_amount, total_amount, currency,
		       expires_at, release_at, first_confirmed_at, swap_count, version, created_at, updated_at
		FROM bookings WHERE id = $1`

	var b model.Booking
	err := r.executor(tx).QueryRow(ctx, query, id).Scan(
		&b.ID, &b.BookingCode, &b.UserID, &b.EventID, &b.TierID, &b.Quantity, &b.Status,
		&b.UnitPriceAmount, &b.UnitFeeAmount, &b.TotalAmount, &b.Currency,
		&b.ExpiresAt, &b.ReleaseAt, &b.FirstConfirmedAt, &b.SwapCount, &b.Version,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get booking by id: %w", err)
	}
	return &b, nil
}

func (r *PostgresBookingRepository) GetByIDForUpdate(ctx context.Context, tx DBTX, id int64) (*model.Booking, error) {
	query := `
		SELECT id, booking_code, user_id, event_id, tier_id, quantity, status,
		       unit_price_amount, unit_fee_amount, total_amount, currency,
		       expires_at, release_at, first_confirmed_at, swap_count, version, created_at, updated_at
		FROM bookings WHERE id = $1 FOR UPDATE`

	var b model.Booking
	err := r.executor(tx).QueryRow(ctx, query, id).Scan(
		&b.ID, &b.BookingCode, &b.UserID, &b.EventID, &b.TierID, &b.Quantity, &b.Status,
		&b.UnitPriceAmount, &b.UnitFeeAmount, &b.TotalAmount, &b.Currency,
		&b.ExpiresAt, &b.ReleaseAt, &b.FirstConfirmedAt, &b.SwapCount, &b.Version,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get booking for update: %w", err)
	}
	return &b, nil
}

func (r *PostgresBookingRepository) GetByBookingCode(ctx context.Context, tx DBTX, code string) (*model.Booking, error) {
	query := `
		SELECT id, booking_code, user_id, event_id, tier_id, quantity, status,
		       unit_price_amount, unit_fee_amount, total_amount, currency,
		       expires_at, release_at, first_confirmed_at, swap_count, version, created_at, updated_at
		FROM bookings WHERE booking_code = $1`

	var b model.Booking
	err := r.executor(tx).QueryRow(ctx, query, code).Scan(
		&b.ID, &b.BookingCode, &b.UserID, &b.EventID, &b.TierID, &b.Quantity, &b.Status,
		&b.UnitPriceAmount, &b.UnitFeeAmount, &b.TotalAmount, &b.Currency,
		&b.ExpiresAt, &b.ReleaseAt, &b.FirstConfirmedAt, &b.SwapCount, &b.Version,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get booking by code: %w", err)
	}
	return &b, nil
}

func (r *PostgresBookingRepository) ListByUserID(ctx context.Context, tx DBTX, userID int64, cursor *helper.Cursor, limit int) ([]model.Booking, error) {
	var rows pgx.Rows
	var err error

	if cursor == nil {
		query := `
			SELECT id, booking_code, user_id, event_id, tier_id, quantity, status,
			       unit_price_amount, unit_fee_amount, total_amount, currency,
			       expires_at, release_at, first_confirmed_at, swap_count, version, created_at, updated_at
			FROM bookings
			WHERE user_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2`
		rows, err = r.executor(tx).Query(ctx, query, userID, limit)
	} else {
		query := `
			SELECT id, booking_code, user_id, event_id, tier_id, quantity, status,
			       unit_price_amount, unit_fee_amount, total_amount, currency,
			       expires_at, release_at, first_confirmed_at, swap_count, version, created_at, updated_at
			FROM bookings
			WHERE user_id = $1 AND (created_at, id) < ($2, $3)
			ORDER BY created_at DESC, id DESC
			LIMIT $4`
		rows, err = r.executor(tx).Query(ctx, query, userID, cursor.CreatedAt, cursor.ID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list bookings by user: %w", err)
	}
	defer rows.Close()

	var list []model.Booking
	for rows.Next() {
		var b model.Booking
		if err := rows.Scan(
			&b.ID, &b.BookingCode, &b.UserID, &b.EventID, &b.TierID, &b.Quantity, &b.Status,
			&b.UnitPriceAmount, &b.UnitFeeAmount, &b.TotalAmount, &b.Currency,
			&b.ExpiresAt, &b.ReleaseAt, &b.FirstConfirmedAt, &b.SwapCount, &b.Version,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan booking row: %w", err)
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (r *PostgresBookingRepository) ListByEventID(ctx context.Context, tx DBTX, eventID int64, cursor *helper.Cursor, limit int) ([]model.Booking, error) {
	var rows pgx.Rows
	var err error

	if cursor == nil {
		query := `
			SELECT id, booking_code, user_id, event_id, tier_id, quantity, status,
			       unit_price_amount, unit_fee_amount, total_amount, currency,
			       expires_at, release_at, first_confirmed_at, swap_count, version, created_at, updated_at
			FROM bookings
			WHERE event_id = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2`
		rows, err = r.executor(tx).Query(ctx, query, eventID, limit)
	} else {
		query := `
			SELECT id, booking_code, user_id, event_id, tier_id, quantity, status,
			       unit_price_amount, unit_fee_amount, total_amount, currency,
			       expires_at, release_at, first_confirmed_at, swap_count, version, created_at, updated_at
			FROM bookings
			WHERE event_id = $1 AND (created_at, id) < ($2, $3)
			ORDER BY created_at DESC, id DESC
			LIMIT $4`
		rows, err = r.executor(tx).Query(ctx, query, eventID, cursor.CreatedAt, cursor.ID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list bookings by event: %w", err)
	}
	defer rows.Close()

	var list []model.Booking
	for rows.Next() {
		var b model.Booking
		if err := rows.Scan(
			&b.ID, &b.BookingCode, &b.UserID, &b.EventID, &b.TierID, &b.Quantity, &b.Status,
			&b.UnitPriceAmount, &b.UnitFeeAmount, &b.TotalAmount, &b.Currency,
			&b.ExpiresAt, &b.ReleaseAt, &b.FirstConfirmedAt, &b.SwapCount, &b.Version,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan booking row: %w", err)
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (r *PostgresBookingRepository) Update(ctx context.Context, tx DBTX, b *model.Booking) error {
	query := `
		UPDATE bookings
		SET tier_id = $1, unit_price_amount = $2, unit_fee_amount = $3, total_amount = $4,
		    status = $5, expires_at = $6, release_at = $7, first_confirmed_at = $8,
		    swap_count = $9, version = version + 1, updated_at = clock_timestamp()
		WHERE id = $10 AND version = $11`

	cmd, err := r.executor(tx).Exec(ctx, query,
		b.TierID, b.UnitPriceAmount, b.UnitFeeAmount, b.TotalAmount,
		b.Status, b.ExpiresAt, b.ReleaseAt, b.FirstConfirmedAt,
		b.SwapCount, b.ID, b.Version-1,
	)
	if err != nil {
		return fmt.Errorf("update booking: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (r *PostgresBookingRepository) UpdateStatus(ctx context.Context, tx DBTX, id int64, status string, version int) error {
	query := `
		UPDATE bookings
		SET status = $1, version = version + 1, updated_at = clock_timestamp()
		WHERE id = $2 AND version = $3`

	cmd, err := r.executor(tx).Exec(ctx, query, status, id, version)
	if err != nil {
		return fmt.Errorf("update booking status: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (r *PostgresBookingRepository) FindActiveByUserAndEvent(ctx context.Context, tx DBTX, userID int64, eventID int64) (*model.Booking, error) {
	query := `
		SELECT id, booking_code, user_id, event_id, tier_id, quantity, status,
		       unit_price_amount, unit_fee_amount, total_amount, currency,
		       expires_at, release_at, first_confirmed_at, swap_count, version, created_at, updated_at
		FROM bookings
		WHERE user_id = $1 AND event_id = $2 AND status IN ('HELD', 'PAYMENT_PENDING')
		LIMIT 1`

	var b model.Booking
	err := r.executor(tx).QueryRow(ctx, query, userID, eventID).Scan(
		&b.ID, &b.BookingCode, &b.UserID, &b.EventID, &b.TierID, &b.Quantity, &b.Status,
		&b.UnitPriceAmount, &b.UnitFeeAmount, &b.TotalAmount, &b.Currency,
		&b.ExpiresAt, &b.ReleaseAt, &b.FirstConfirmedAt, &b.SwapCount, &b.Version,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find active booking: %w", err)
	}
	return &b, nil
}

func (r *PostgresBookingRepository) ListDueRelease(ctx context.Context, tx DBTX, now time.Time, limit int) ([]model.Booking, error) {
	query := `
		SELECT id, booking_code, user_id, event_id, tier_id, quantity, status,
		       unit_price_amount, unit_fee_amount, total_amount, currency,
		       expires_at, release_at, first_confirmed_at, swap_count, version, created_at, updated_at
		FROM bookings
		WHERE status IN ('HELD', 'PAYMENT_PENDING') AND release_at <= $1
		ORDER BY release_at ASC, id ASC
		LIMIT $2`

	rows, err := r.executor(tx).Query(ctx, query, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list due release: %w", err)
	}
	defer rows.Close()

	var list []model.Booking
	for rows.Next() {
		var b model.Booking
		if err := rows.Scan(
			&b.ID, &b.BookingCode, &b.UserID, &b.EventID, &b.TierID, &b.Quantity, &b.Status,
			&b.UnitPriceAmount, &b.UnitFeeAmount, &b.TotalAmount, &b.Currency,
			&b.ExpiresAt, &b.ReleaseAt, &b.FirstConfirmedAt, &b.SwapCount, &b.Version,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan due booking: %w", err)
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (r *PostgresBookingRepository) ListActiveByEventID(ctx context.Context, tx DBTX, eventID int64) ([]model.Booking, error) {
	query := `
		SELECT id, booking_code, user_id, event_id, tier_id, quantity, status,
		       unit_price_amount, unit_fee_amount, total_amount, currency,
		       expires_at, release_at, first_confirmed_at, swap_count, version, created_at, updated_at
		FROM bookings
		WHERE event_id = $1 AND status IN ('HELD', 'PAYMENT_PENDING')`

	rows, err := r.executor(tx).Query(ctx, query, eventID)
	if err != nil {
		return nil, fmt.Errorf("list active bookings by event: %w", err)
	}
	defer rows.Close()

	var list []model.Booking
	for rows.Next() {
		var b model.Booking
		if err := rows.Scan(
			&b.ID, &b.BookingCode, &b.UserID, &b.EventID, &b.TierID, &b.Quantity, &b.Status,
			&b.UnitPriceAmount, &b.UnitFeeAmount, &b.TotalAmount, &b.Currency,
			&b.ExpiresAt, &b.ReleaseAt, &b.FirstConfirmedAt, &b.SwapCount, &b.Version,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan active booking: %w", err)
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (r *PostgresBookingRepository) ListConfirmedByEventID(ctx context.Context, tx DBTX, eventID int64) ([]model.Booking, error) {
	query := `
		SELECT id, booking_code, user_id, event_id, tier_id, quantity, status,
		       unit_price_amount, unit_fee_amount, total_amount, currency,
		       expires_at, release_at, first_confirmed_at, swap_count, version, created_at, updated_at
		FROM bookings
		WHERE event_id = $1 AND status IN ('CONFIRMED', 'REFUND_PENDING')`

	rows, err := r.executor(tx).Query(ctx, query, eventID)
	if err != nil {
		return nil, fmt.Errorf("list confirmed bookings by event: %w", err)
	}
	defer rows.Close()

	var list []model.Booking
	for rows.Next() {
		var b model.Booking
		if err := rows.Scan(
			&b.ID, &b.BookingCode, &b.UserID, &b.EventID, &b.TierID, &b.Quantity, &b.Status,
			&b.UnitPriceAmount, &b.UnitFeeAmount, &b.TotalAmount, &b.Currency,
			&b.ExpiresAt, &b.ReleaseAt, &b.FirstConfirmedAt, &b.SwapCount, &b.Version,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan confirmed booking: %w", err)
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

// PostgresUserEventLimitRepository implementasi PostgreSQL untuk UserEventLimitRepository.
type PostgresUserEventLimitRepository struct {
	db DBTX
}

func NewPostgresUserEventLimitRepository(db DBTX) *PostgresUserEventLimitRepository {
	return &PostgresUserEventLimitRepository{db: db}
}

func (r *PostgresUserEventLimitRepository) executor(tx DBTX) DBTX {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *PostgresUserEventLimitRepository) UpsertAndLock(ctx context.Context, tx DBTX, eventID int64, userID int64, frozenMax int) (*model.UserEventLimit, error) {
	exec := r.executor(tx)
	// 1. INSERT ON CONFLICT DO NOTHING
	insertQ := `
		INSERT INTO user_event_limits (event_id, user_id, held, purchased_total, frozen_max, updated_at)
		VALUES ($1, $2, 0, 0, $3, clock_timestamp())
		ON CONFLICT (event_id, user_id) DO NOTHING`
	if _, err := exec.Exec(ctx, insertQ, eventID, userID, frozenMax); err != nil {
		return nil, fmt.Errorf("insert user limit: %w", err)
	}

	// 2. SELECT ... FOR UPDATE
	selectQ := `
		SELECT event_id, user_id, held, purchased_total, frozen_max, updated_at
		FROM user_event_limits
		WHERE event_id = $1 AND user_id = $2
		FOR UPDATE`

	var lim model.UserEventLimit
	err := exec.QueryRow(ctx, selectQ, eventID, userID).Scan(
		&lim.EventID, &lim.UserID, &lim.Held, &lim.PurchasedTotal, &lim.FrozenMax, &lim.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("select for update user limit: %w", err)
	}
	return &lim, nil
}

func (r *PostgresUserEventLimitRepository) AddHeld(ctx context.Context, tx DBTX, eventID int64, userID int64, delta int) error {
	query := `
		UPDATE user_event_limits
		SET held = held + $1, updated_at = clock_timestamp()
		WHERE event_id = $2 AND user_id = $3
		  AND held + purchased_total + $1 <= frozen_max`

	cmd, err := r.executor(tx).Exec(ctx, query, delta, eventID, userID)
	if err != nil {
		return fmt.Errorf("add held user limit: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrUserLimitExceeded
	}
	return nil
}

func (r *PostgresUserEventLimitRepository) SubtractHeld(ctx context.Context, tx DBTX, eventID int64, userID int64, delta int) error {
	query := `
		UPDATE user_event_limits
		SET held = held - $1, updated_at = clock_timestamp()
		WHERE event_id = $2 AND user_id = $3 AND held >= $1`

	cmd, err := r.executor(tx).Exec(ctx, query, delta, eventID, userID)
	if err != nil {
		return fmt.Errorf("subtract held user limit: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("gagal mengurangi held user limit")
	}
	return nil
}

func (r *PostgresUserEventLimitRepository) AddPurchasedTotal(ctx context.Context, tx DBTX, eventID int64, userID int64, delta int) error {
	query := `
		UPDATE user_event_limits
		SET held = held - $1,
		    purchased_total = purchased_total + $1,
		    updated_at = clock_timestamp()
		WHERE event_id = $2 AND user_id = $3 AND held >= $1`

	cmd, err := r.executor(tx).Exec(ctx, query, delta, eventID, userID)
	if err != nil {
		return fmt.Errorf("add purchased total: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("gagal memindahkan held ke purchased_total")
	}
	return nil
}

func (r *PostgresUserEventLimitRepository) GetByEventAndUser(ctx context.Context, tx DBTX, eventID int64, userID int64) (*model.UserEventLimit, error) {
	query := `
		SELECT event_id, user_id, held, purchased_total, frozen_max, updated_at
		FROM user_event_limits WHERE event_id = $1 AND user_id = $2`

	var lim model.UserEventLimit
	err := r.executor(tx).QueryRow(ctx, query, eventID, userID).Scan(
		&lim.EventID, &lim.UserID, &lim.Held, &lim.PurchasedTotal, &lim.FrozenMax, &lim.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user limit: %w", err)
	}
	return &lim, nil
}

// PostgresIdempotencyRepository implementasi PostgreSQL untuk IdempotencyRepository.
type PostgresIdempotencyRepository struct {
	db DBTX
}

func NewPostgresIdempotencyRepository(db DBTX) *PostgresIdempotencyRepository {
	return &PostgresIdempotencyRepository{db: db}
}

func (r *PostgresIdempotencyRepository) executor(tx DBTX) DBTX {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *PostgresIdempotencyRepository) InsertOrGet(ctx context.Context, tx DBTX, rec *model.IdempotencyRecord) (*model.IdempotencyRecord, bool, error) {
	exec := r.executor(tx)

	// 1. INSERT ... ON CONFLICT DO NOTHING
	insertQ := `
		INSERT INTO idempotency_records (user_id, operation_scope, idempotency_key, request_hash, retention_until, created_at)
		VALUES ($1, $2, $3, $4, $5, clock_timestamp())
		ON CONFLICT (user_id, operation_scope, idempotency_key) DO NOTHING`

	cmd, err := exec.Exec(ctx, insertQ, rec.UserID, rec.OperationScope, rec.IdempotencyKey, rec.RequestHash, rec.RetentionUntil)
	if err != nil {
		return nil, false, fmt.Errorf("insert idempotency: %w", err)
	}

	inserted := cmd.RowsAffected() > 0

	// 2. SELECT ... FOR UPDATE
	selectQ := `
		SELECT user_id, operation_scope, idempotency_key, request_hash,
		       response_status, response_body, resource_id, completed_at, retention_until, created_at
		FROM idempotency_records
		WHERE user_id = $1 AND operation_scope = $2 AND idempotency_key = $3
		FOR UPDATE`

	var existing model.IdempotencyRecord
	err = exec.QueryRow(ctx, selectQ, rec.UserID, rec.OperationScope, rec.IdempotencyKey).Scan(
		&existing.UserID, &existing.OperationScope, &existing.IdempotencyKey, &existing.RequestHash,
		&existing.ResponseStatus, &existing.ResponseBody, &existing.ResourceID,
		&existing.CompletedAt, &existing.RetentionUntil, &existing.CreatedAt,
	)
	if err != nil {
		return nil, false, fmt.Errorf("select idempotency for update: %w", err)
	}

	if !inserted {
		if existing.RequestHash != rec.RequestHash {
			return nil, false, ErrIdempotencyKeyReused
		}
		if !existing.IsCompleted() {
			return nil, false, ErrOperationInProgress
		}
	}

	return &existing, inserted, nil
}

func (r *PostgresIdempotencyRepository) Complete(ctx context.Context, tx DBTX, userID int64, scope string, key string,
	responseStatus int, responseBody []byte, resourceID string) error {
	query := `
		UPDATE idempotency_records
		SET response_status = $1, response_body = $2, resource_id = $3, completed_at = clock_timestamp()
		WHERE user_id = $4 AND operation_scope = $5 AND idempotency_key = $6`

	_, err := r.executor(tx).Exec(ctx, query, responseStatus, responseBody, resourceID, userID, scope, key)
	return err
}

// PostgresBookingChangeRepository implementasi PostgreSQL untuk BookingChangeRepository.
type PostgresBookingChangeRepository struct {
	db DBTX
}

func NewPostgresBookingChangeRepository(db DBTX) *PostgresBookingChangeRepository {
	return &PostgresBookingChangeRepository{db: db}
}

func (r *PostgresBookingChangeRepository) Create(ctx context.Context, tx DBTX, change *model.BookingChange) error {
	exec := tx
	if exec == nil {
		exec = r.db
	}
	query := `
		INSERT INTO booking_changes (operation_id, booking_id, actor_id, old_tier_id, new_tier_id, old_total_amount, new_total_amount, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, clock_timestamp())
		RETURNING id, created_at`

	return exec.QueryRow(ctx, query,
		change.OperationID, change.BookingID, change.ActorID, change.OldTierID,
		change.NewTierID, change.OldTotalAmount, change.NewTotalAmount,
	).Scan(&change.ID, &change.CreatedAt)
}

// PostgresAdmissionRedemptionRepository implementasi PostgreSQL untuk AdmissionRedemptionRepository.
type PostgresAdmissionRedemptionRepository struct {
	db DBTX
}

func NewPostgresAdmissionRedemptionRepository(db DBTX) *PostgresAdmissionRedemptionRepository {
	return &PostgresAdmissionRedemptionRepository{db: db}
}

func (r *PostgresAdmissionRedemptionRepository) Create(ctx context.Context, tx DBTX, redemption *model.AdmissionRedemption) error {
	exec := tx
	if exec == nil {
		exec = r.db
	}
	query := `
		INSERT INTO admission_redemptions (jti, booking_id, event_id, user_id, queue_epoch, redeemed_at)
		VALUES ($1, $2, $3, $4, $5, clock_timestamp())
		RETURNING redeemed_at`

	return exec.QueryRow(ctx, query,
		redemption.JTI, redemption.BookingID, redemption.EventID, redemption.UserID, redemption.QueueEpoch,
	).Scan(&redemption.RedeemedAt)
}

// HashPayload helper untuk menghitung SHA-256 hash dari data request kanonis.
func HashPayload(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
