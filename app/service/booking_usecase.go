package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/app/repository"
	"github.com/viesloxy/ngawimusical-backend/helper"
)

// BookingUsecase menangani logika bisnis reservasi hold tiket dan pembatalan.
type BookingUsecase struct {
	txMgr           repository.TxManager
	eventRepo       repository.EventRepository
	tierRepo        repository.TicketTierRepository
	bookingRepo     repository.BookingRepository
	userLimitRepo   repository.UserEventLimitRepository
	idempotencyRepo repository.IdempotencyRepository
	inventoryRepo   repository.InventoryMovementRepository
	outboxRepo      repository.OutboxRepository
	auditRepo       repository.AuditLogRepository
	holdTTL         time.Duration
	releaseGrace    time.Duration
}

// NewBookingUsecase membuat instance baru BookingUsecase.
func NewBookingUsecase(
	txMgr repository.TxManager,
	eventRepo repository.EventRepository,
	tierRepo repository.TicketTierRepository,
	bookingRepo repository.BookingRepository,
	userLimitRepo repository.UserEventLimitRepository,
	idempotencyRepo repository.IdempotencyRepository,
	inventoryRepo repository.InventoryMovementRepository,
	outboxRepo repository.OutboxRepository,
	auditRepo repository.AuditLogRepository,
	holdTTL time.Duration,
	releaseGrace time.Duration,
) *BookingUsecase {
	if holdTTL <= 0 {
		holdTTL = 10 * time.Minute
	}
	if releaseGrace <= 0 {
		releaseGrace = 2 * time.Minute
	}
	return &BookingUsecase{
		txMgr:           txMgr,
		eventRepo:       eventRepo,
		tierRepo:        tierRepo,
		bookingRepo:     bookingRepo,
		userLimitRepo:   userLimitRepo,
		idempotencyRepo: idempotencyRepo,
		inventoryRepo:   inventoryRepo,
		outboxRepo:      outboxRepo,
		auditRepo:       auditRepo,
		holdTTL:         holdTTL,
		releaseGrace:    releaseGrace,
	}
}

// CreateHoldRequestInternal parameter usecase untuk create hold.
type CreateHoldRequestInternal struct {
	UserID         int64
	EventID        int64
	TierID         int64
	Quantity       int
	IdempotencyKey string
	RequestID      string
	PayloadBytes   []byte
}

// CreateHold melakukan reservasi tiket sementara (status HELD).
// Menerapkan protokol lock global dan conditional update atomik.
func (uc *BookingUsecase) CreateHold(ctx context.Context, req CreateHoldRequestInternal) (*model.BookingResponse, error) {
	if req.Quantity < 1 || req.Quantity > 4 {
		return nil, helper.NewValidationError(map[string]string{"quantity": "Quantity harus antara 1 dan 4."})
	}
	if req.IdempotencyKey == "" {
		return nil, helper.NewValidationError(map[string]string{"idempotency_key": "Idempotency-Key wajib disertakan."})
	}

	requestHash := repository.HashPayload(req.PayloadBytes)
	retentionUntil := time.Now().UTC().Add(30 * 24 * time.Hour)

	var result *model.BookingResponse

	err := uc.txMgr.WithTx(ctx, func(ctx context.Context, tx repository.DBTX) error {
		// 1. Event Guard (FOR SHARE)
		event, err := uc.eventRepo.GetByIDForShare(ctx, tx, req.EventID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return helper.NotFound()
			}
			return fmt.Errorf("ambil event guard: %w", err)
		}

		now := time.Now().UTC()
		if !event.IsSaleActive(now) {
			return helper.Conflict("CONFLICT", "Penjualan tiket untuk event ini sedang tidak aktif.")
		}

		// 2. Idempotency Record (INSERT ON CONFLICT / SELECT FOR UPDATE)
		idemRecord := &model.IdempotencyRecord{
			UserID:         req.UserID,
			OperationScope: fmt.Sprintf("booking:create:%d", req.EventID),
			IdempotencyKey: req.IdempotencyKey,
			RequestHash:    requestHash,
			RetentionUntil: retentionUntil,
		}

		existingIdem, isNew, err := uc.idempotencyRepo.InsertOrGet(ctx, tx, idemRecord)
		if err != nil {
			if errors.Is(err, repository.ErrIdempotencyKeyReused) {
				return helper.Conflict("IDEMPOTENCY_KEY_REUSED", "Idempotency-Key sudah dipakai dengan request berbeda.")
			}
			if errors.Is(err, repository.ErrOperationInProgress) {
				return helper.Conflict("OPERATION_IN_PROGRESS", "Operasi dengan Idempotency-Key ini sedang berlangsung.")
			}
			return fmt.Errorf("idempotency check: %w", err)
		}

		// Replay jika sudah selesai
		if !isNew && existingIdem.IsCompleted() {
			var resp model.BookingResponse
			if err := json.Unmarshal(existingIdem.ResponseBody, &resp); err != nil {
				return fmt.Errorf("unmarshal replayed response: %w", err)
			}
			result = &resp
			return nil
		}

		// 3. User Event Limit (Baris stabil FOR UPDATE)
		userLimit, err := uc.userLimitRepo.UpsertAndLock(ctx, tx, req.EventID, req.UserID, event.MaxTicketsPerUser)
		if err != nil {
			return fmt.Errorf("lock user event limit: %w", err)
		}

		if userLimit.RemainingCapacity() < req.Quantity {
			return helper.Conflict("USER_LIMIT_EXCEEDED", "Jumlah tiket melebihi batas akun untuk konser ini.")
		}

		// Cek apakah user sudah punya hold aktif (1 hold aktif per user per event)
		activeBooking, err := uc.bookingRepo.FindActiveByUserAndEvent(ctx, tx, req.UserID, req.EventID)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("cek active booking: %w", err)
		}
		if activeBooking != nil {
			return helper.Conflict("ACTIVE_HOLD_EXISTS", "Anda masih memiliki reservasi aktif yang belum diselesaikan.")
		}

		// 4. Booking Record (Persiapkan ID dan Kode Booking)
		bookingCode, err := generateBookingCode()
		if err != nil {
			return fmt.Errorf("generate booking code: %w", err)
		}

		// 5. Tier Stock Conditional Update (Atomik: quota - held - sold - retired >= quantity)
		tier, err := uc.tierRepo.HoldStock(ctx, tx, req.TierID, req.EventID, req.Quantity)
		if err != nil {
			if errors.Is(err, repository.ErrSoldOut) {
				return helper.Conflict("SOLD_OUT", "Tiket untuk kategori ini sudah habis.")
			}
			return fmt.Errorf("hold tier stock: %w", err)
		}

		// Tambahkan held pada user limit
		if err := uc.userLimitRepo.AddHeld(ctx, tx, req.EventID, req.UserID, req.Quantity); err != nil {
			if errors.Is(err, repository.ErrUserLimitExceeded) {
				return helper.Conflict("USER_LIMIT_EXCEEDED", "Jumlah tiket melebihi batas akun untuk konser ini.")
			}
			return fmt.Errorf("update user limit held: %w", err)
		}

		// 6. Simpan Booking HELD
		expiresAt := now.Add(uc.holdTTL)
		releaseAt := expiresAt.Add(uc.releaseGrace)
		totalAmount := model.CalculateTotal(tier.PriceAmount, tier.FeeAmount, req.Quantity)

		booking := &model.Booking{
			BookingCode:     bookingCode,
			UserID:          req.UserID,
			EventID:         req.EventID,
			TierID:          req.TierID,
			Quantity:        req.Quantity,
			Status:          model.BookingStatusHeld,
			UnitPriceAmount: tier.PriceAmount,
			UnitFeeAmount:   tier.FeeAmount,
			TotalAmount:     totalAmount,
			Currency:        "IDR",
			ExpiresAt:       expiresAt,
			ReleaseAt:       releaseAt,
			SwapCount:       0,
			Version:         1,
		}

		if err := uc.bookingRepo.Create(ctx, tx, booking); err != nil {
			if errors.Is(err, repository.ErrActiveHoldExists) {
				return helper.Conflict("ACTIVE_HOLD_EXISTS", "Anda masih memiliki reservasi aktif yang belum diselesaikan.")
			}
			return fmt.Errorf("create booking: %w", err)
		}

		// 7. Inventory Movement Ledger
		movement := &model.InventoryMovement{
			OperationID:  req.IdempotencyKey,
			TierID:       req.TierID,
			BookingID:    booking.ID,
			MovementType: model.MovementHold,
			DeltaHeld:    req.Quantity,
			DeltaSold:    0,
			DeltaRetired: 0,
			Reason:       fmt.Sprintf("Hold booking %s", booking.BookingCode),
		}
		if err := uc.inventoryRepo.Create(ctx, tx, movement); err != nil {
			return fmt.Errorf("catat ledger: %w", err)
		}

		// 8. Outbox Event (BookingHeld)
		payloadMap := map[string]any{
			"booking_id":   booking.ID,
			"booking_code": booking.BookingCode,
			"user_id":      booking.UserID,
			"event_id":     booking.EventID,
			"tier_id":      booking.TierID,
			"quantity":     booking.Quantity,
			"total_amount": booking.TotalAmount,
			"expires_at":   booking.ExpiresAt,
			"release_at":   booking.ReleaseAt,
		}
		payloadJSON, _ := json.Marshal(payloadMap)

		outbox := &model.Outbox{
			ID:               uuid.NewString(),
			AggregateID:      fmt.Sprintf("booking:%d", booking.ID),
			AggregateVersion: booking.Version,
			EventType:        model.OutboxEventBookingHeld,
			SchemaVersion:    1,
			Payload:          payloadJSON,
		}
		if err := uc.outboxRepo.Create(ctx, tx, outbox); err != nil {
			return fmt.Errorf("catat outbox: %w", err)
		}

		// 9. Audit Log
		audit := &model.AuditLog{
			ActorID:      &req.UserID,
			Action:       "booking.hold",
			ResourceType: "booking",
			ResourceID:   fmt.Sprintf("%d", booking.ID),
			RequestID:    req.RequestID,
		}
		if err := uc.auditRepo.Create(ctx, tx, audit); err != nil {
			return fmt.Errorf("catat audit: %w", err)
		}

		// 10. Complete Idempotency Record
		respDTO := model.ToBookingResponse(*booking)
		respBytes, _ := json.Marshal(respDTO)

		if err := uc.idempotencyRepo.Complete(ctx, tx, req.UserID, idemRecord.OperationScope, req.IdempotencyKey,
			201, respBytes, fmt.Sprintf("%d", booking.ID)); err != nil {
			return fmt.Errorf("complete idempotency: %w", err)
		}

		result = &respDTO
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// CancelHold membatalkan booking HELD sebelum payment attempt dibuat.
func (uc *BookingUsecase) CancelHold(ctx context.Context, bookingID int64, userID int64, requestID string) error {
	return uc.txMgr.WithTx(ctx, func(ctx context.Context, tx repository.DBTX) error {
		// 1. Kunci booking FOR UPDATE
		booking, err := uc.bookingRepo.GetByIDForUpdate(ctx, tx, bookingID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return helper.NotFound()
			}
			return fmt.Errorf("ambil booking: %w", err)
		}

		// Pastikan ownership
		if booking.UserID != userID {
			return helper.NotFound()
		}

		// Hanya booking HELD yang bisa dibatalkan attendee
		if !booking.CanCancel() {
			return helper.Conflict("CONFLICT", "Hanya booking HELD yang bisa dibatalkan.")
		}

		// 2. Kunci dan lepaskan stok tier
		if err := uc.tierRepo.ReleaseHeldStock(ctx, tx, booking.TierID, booking.Quantity); err != nil {
			return fmt.Errorf("lepas stok tier: %w", err)
		}

		// 3. Kurangi held pada user limit
		if err := uc.userLimitRepo.SubtractHeld(ctx, tx, booking.EventID, booking.UserID, booking.Quantity); err != nil {
			return fmt.Errorf("kurangi user limit held: %w", err)
		}

		// 4. Ubah status booking ke CANCELLED
		booking.Status = model.BookingStatusCancelled
		booking.Version++
		if err := uc.bookingRepo.Update(ctx, tx, booking); err != nil {
			return fmt.Errorf("update status booking: %w", err)
		}

		// 5. Catat Inventory Movement
		movement := &model.InventoryMovement{
			OperationID:  fmt.Sprintf("cancel:%d:%s", booking.ID, requestID),
			TierID:       booking.TierID,
			BookingID:    booking.ID,
			MovementType: model.MovementReleaseCancel,
			DeltaHeld:    -booking.Quantity,
			DeltaSold:    0,
			DeltaRetired: 0,
			Reason:       fmt.Sprintf("Cancel booking %s oleh attendee", booking.BookingCode),
		}
		if err := uc.inventoryRepo.Create(ctx, tx, movement); err != nil {
			return fmt.Errorf("catat ledger: %w", err)
		}

		// 6. Outbox Event (BookingCancelled)
		payloadMap := map[string]any{
			"booking_id":   booking.ID,
			"booking_code": booking.BookingCode,
			"user_id":      booking.UserID,
			"event_id":     booking.EventID,
		}
		payloadJSON, _ := json.Marshal(payloadMap)

		outbox := &model.Outbox{
			ID:               uuid.NewString(),
			AggregateID:      fmt.Sprintf("booking:%d", booking.ID),
			AggregateVersion: booking.Version,
			EventType:        model.OutboxEventBookingCancelled,
			SchemaVersion:    1,
			Payload:          payloadJSON,
		}
		if err := uc.outboxRepo.Create(ctx, tx, outbox); err != nil {
			return fmt.Errorf("catat outbox: %w", err)
		}

		// 7. Audit Log
		audit := &model.AuditLog{
			ActorID:      &userID,
			Action:       "booking.cancel",
			ResourceType: "booking",
			ResourceID:   fmt.Sprintf("%d", booking.ID),
			RequestID:    requestID,
		}
		return uc.auditRepo.Create(ctx, tx, audit)
	})
}

// GetBooking mengambil detail booking dengan verifikasi kepemilikan.
func (uc *BookingUsecase) GetBooking(ctx context.Context, bookingID int64, userID int64, role string) (*model.BookingResponse, error) {
	booking, err := uc.bookingRepo.GetByID(ctx, nil, bookingID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, helper.NotFound()
		}
		return nil, fmt.Errorf("get booking: %w", err)
	}

	if booking.UserID != userID && role != "admin" {
		return nil, helper.NotFound()
	}

	resp := model.ToBookingResponse(*booking)
	return &resp, nil
}

// ListUserBookings mengambil daftar booking milik user.
func (uc *BookingUsecase) ListUserBookings(ctx context.Context, userID int64, cursor *helper.Cursor, limit int) ([]model.BookingResponse, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	bookings, err := uc.bookingRepo.ListByUserID(ctx, nil, userID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list bookings: %w", err)
	}

	responses := make([]model.BookingResponse, len(bookings))
	for i, b := range bookings {
		responses[i] = model.ToBookingResponse(b)
	}
	return responses, nil
}

func generateBookingCode() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "bkg_" + hex.EncodeToString(bytes), nil
}
