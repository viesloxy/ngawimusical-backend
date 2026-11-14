package model

import "time"

// Status booking.
const (
	BookingStatusHeld           = "HELD"
	BookingStatusPaymentPending = "PAYMENT_PENDING"
	BookingStatusConfirmed      = "CONFIRMED"
	BookingStatusCancelled      = "CANCELLED"
	BookingStatusExpired        = "EXPIRED"
	BookingStatusRefundPending  = "REFUND_PENDING"
	BookingStatusRefunded       = "REFUNDED"
)

// Booking adalah pesanan tiket yang dibuat oleh attendee.
type Booking struct {
	ID               int64      `json:"id"`
	BookingCode      string     `json:"booking_code"`
	UserID           int64      `json:"user_id"`
	EventID          int64      `json:"event_id"`
	TierID           int64      `json:"tier_id"`
	Quantity         int        `json:"quantity"`
	Status           string     `json:"status"`
	UnitPriceAmount  int64      `json:"unit_price_amount"`
	UnitFeeAmount    int64      `json:"unit_fee_amount"`
	TotalAmount      int64      `json:"total_amount"`
	Currency         string     `json:"currency"`
	ExpiresAt        time.Time  `json:"expires_at"`
	ReleaseAt        time.Time  `json:"release_at"`
	FirstConfirmedAt *time.Time `json:"first_confirmed_at,omitempty"`
	SwapCount        int        `json:"swap_count"`
	Version          int        `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// IsActive memeriksa apakah booking masih memegang stok.
func (b Booking) IsActive() bool {
	return b.Status == BookingStatusHeld || b.Status == BookingStatusPaymentPending
}

// IsTerminal memeriksa apakah booking sudah selesai.
func (b Booking) IsTerminal() bool {
	switch b.Status {
	case BookingStatusCancelled, BookingStatusExpired, BookingStatusRefunded:
		return true
	default:
		return false
	}
}

// CanCancel memeriksa apakah booking bisa dibatalkan attendee (sebelum payment attempt).
func (b Booking) CanCancel() bool {
	return b.Status == BookingStatusHeld
}

// CanSwap memeriksa apakah booking bisa ditukar tier-nya.
func (b Booking) CanSwap(maxSwap int) bool {
	return b.Status == BookingStatusHeld && b.SwapCount < maxSwap
}

// CanCreatePayment memeriksa apakah booking bisa membuat payment attempt.
func (b Booking) CanCreatePayment(now time.Time) bool {
	return b.Status == BookingStatusHeld && now.Before(b.ExpiresAt)
}

// CanConfirm memeriksa apakah booking bisa dikonfirmasi oleh settlement.
func (b Booking) CanConfirm() bool {
	return b.Status == BookingStatusPaymentPending
}

// CalculateTotal menghitung total dari harga per unit, biaya, dan quantity.
func CalculateTotal(unitPrice, unitFee int64, quantity int) int64 {
	return int64(quantity) * (unitPrice + unitFee)
}

// UserEventLimit adalah batas tiket per user per event.
type UserEventLimit struct {
	EventID        int64     `json:"event_id"`
	UserID         int64     `json:"user_id"`
	Held           int       `json:"held"`
	PurchasedTotal int       `json:"purchased_total"`
	FrozenMax      int       `json:"frozen_max"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// RemainingCapacity menghitung sisa kapasitas pembelian user.
func (u UserEventLimit) RemainingCapacity() int {
	remaining := u.FrozenMax - u.Held - u.PurchasedTotal
	if remaining < 0 {
		return 0
	}
	return remaining
}

// BookingChange mencatat riwayat perubahan booking (swap).
type BookingChange struct {
	ID             int64     `json:"id"`
	OperationID    string    `json:"operation_id"`
	BookingID      int64     `json:"booking_id"`
	ActorID        int64     `json:"actor_id"`
	OldTierID      int64     `json:"old_tier_id"`
	NewTierID      int64     `json:"new_tier_id"`
	OldTotalAmount int64     `json:"old_total_amount"`
	NewTotalAmount int64     `json:"new_total_amount"`
	CreatedAt      time.Time `json:"created_at"`
}

// AdmissionRedemption mencatat penggunaan admission pass.
type AdmissionRedemption struct {
	JTI        string    `json:"jti"`
	BookingID  int64     `json:"booking_id"`
	EventID    int64     `json:"event_id"`
	UserID     int64     `json:"user_id"`
	QueueEpoch int       `json:"queue_epoch"`
	RedeemedAt time.Time `json:"redeemed_at"`
}

// IdempotencyRecord menyimpan hasil operasi yang sudah committed.
type IdempotencyRecord struct {
	UserID         int64      `json:"user_id"`
	OperationScope string     `json:"operation_scope"`
	IdempotencyKey string     `json:"idempotency_key"`
	RequestHash    string     `json:"request_hash"`
	ResponseStatus *int       `json:"response_status,omitempty"`
	ResponseBody   []byte     `json:"response_body,omitempty"`
	ResourceID     *string    `json:"resource_id,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	RetentionUntil time.Time  `json:"retention_until"`
	CreatedAt      time.Time  `json:"created_at"`
}

// IsCompleted memeriksa apakah operasi sudah selesai dan hasilnya bisa di-replay.
func (r IdempotencyRecord) IsCompleted() bool {
	return r.CompletedAt != nil && r.ResponseStatus != nil
}

// CreateBookingRequest adalah DTO untuk membuat booking baru.
type CreateBookingRequest struct {
	EventID  string `json:"event_id" validate:"required"`
	TierID   string `json:"tier_id" validate:"required"`
	Quantity int    `json:"quantity" validate:"required,gte=1,lte=4"`
}

// SwapTierRequest adalah DTO untuk menukar tier booking.
type SwapTierRequest struct {
	NewTierID string `json:"new_tier_id" validate:"required"`
}

// BookingResponse adalah DTO respons booking.
type BookingResponse struct {
	ID              int64     `json:"id"`
	BookingCode     string    `json:"booking_code"`
	EventID         int64     `json:"event_id"`
	TierID          int64     `json:"tier_id"`
	Quantity        int       `json:"quantity"`
	Status          string    `json:"status"`
	UnitPriceAmount int64     `json:"unit_price_amount"`
	UnitFeeAmount   int64     `json:"unit_fee_amount"`
	TotalAmount     int64     `json:"total_amount"`
	Currency        string    `json:"currency"`
	ExpiresAt       time.Time `json:"expires_at"`
	ReleaseAt       time.Time `json:"release_at"`
	SwapCount       int       `json:"swap_count"`
	CreatedAt       time.Time `json:"created_at"`
}

// ToBookingResponse mengonversi entity Booking ke DTO respons.
func ToBookingResponse(b Booking) BookingResponse {
	return BookingResponse{
		ID:              b.ID,
		BookingCode:     b.BookingCode,
		EventID:         b.EventID,
		TierID:          b.TierID,
		Quantity:        b.Quantity,
		Status:          b.Status,
		UnitPriceAmount: b.UnitPriceAmount,
		UnitFeeAmount:   b.UnitFeeAmount,
		TotalAmount:     b.TotalAmount,
		Currency:        b.Currency,
		ExpiresAt:       b.ExpiresAt,
		ReleaseAt:       b.ReleaseAt,
		SwapCount:       b.SwapCount,
		CreatedAt:       b.CreatedAt,
	}
}
