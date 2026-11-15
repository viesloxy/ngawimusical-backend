package model

import "time"

// Status payment attempt.
const (
	PaymentStatusCreatePending = "CREATE_PENDING"
	PaymentStatusPending       = "PENDING"
	PaymentStatusUnknown       = "UNKNOWN"
	PaymentStatusSucceeded     = "SUCCEEDED"
	PaymentStatusFailed        = "FAILED"
	PaymentStatusRefundPending = "REFUND_PENDING"
	PaymentStatusRefunded      = "REFUNDED"
)

// Payment inbox processing state.
const (
	InboxStatePending    = "PENDING"
	InboxStateProcessing = "PROCESSING"
	InboxStateDone       = "DONE"
	InboxStateFailed     = "FAILED"
)

// Refund reason.
const (
	RefundReasonEventCancelled     = "EVENT_CANCELLED"
	RefundReasonUnallocatedPayment = "UNALLOCATED_PAYMENT"
)

// Refund status.
const (
	RefundStatusPending   = "PENDING"
	RefundStatusUnknown   = "UNKNOWN"
	RefundStatusSucceeded = "SUCCEEDED"
	RefundStatusFailed    = "FAILED"
)

// PaymentAttempt adalah percobaan pembayaran untuk satu booking.
type PaymentAttempt struct {
	ID                    int64      `json:"id"`
	BookingID             int64      `json:"booking_id"`
	MerchantOrderID       string     `json:"merchant_order_id"`
	Provider              string     `json:"provider"`
	ProviderTransactionID *string    `json:"provider_transaction_id,omitempty"`
	Amount                int64      `json:"amount"`
	Currency              string     `json:"currency"`
	Status                string     `json:"status"`
	ProviderPaidAt        *time.Time `json:"provider_paid_at,omitempty"`
	ObservedAt            *time.Time `json:"observed_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// IsFinal memeriksa apakah status payment sudah final.
func (p PaymentAttempt) IsFinal() bool {
	switch p.Status {
	case PaymentStatusSucceeded, PaymentStatusFailed, PaymentStatusRefunded:
		return true
	default:
		return false
	}
}

// NeedsReconciliation memeriksa apakah payment perlu direkonsiliasi.
func (p PaymentAttempt) NeedsReconciliation() bool {
	switch p.Status {
	case PaymentStatusCreatePending, PaymentStatusPending, PaymentStatusUnknown:
		return true
	default:
		return false
	}
}

// PaymentInbox menyimpan webhook dari provider pembayaran.
type PaymentInbox struct {
	ID              int64      `json:"id"`
	DedupeKey       string     `json:"dedupe_key"`
	Provider        string     `json:"provider"`
	PayloadHash     string     `json:"payload_hash"`
	Payload         []byte     `json:"payload"`
	ProcessingState string     `json:"processing_state"`
	LeaseUntil      *time.Time `json:"lease_until,omitempty"`
	RetryAt         time.Time  `json:"retry_at"`
	LastError       *string    `json:"last_error,omitempty"`
	ReceivedAt      time.Time  `json:"received_at"`
}

// Refund adalah catatan pengembalian dana.
type Refund struct {
	ID               int64     `json:"id"`
	CaseKey          string    `json:"case_key"`
	PaymentAttemptID int64     `json:"payment_attempt_id"`
	BookingID        int64     `json:"booking_id"`
	Reason           string    `json:"reason"`
	Amount           int64     `json:"amount"`
	Status           string    `json:"status"`
	ProviderRefundID *string   `json:"provider_refund_id,omitempty"`
	Attempts         int       `json:"attempts"`
	LastError        *string   `json:"last_error,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// CreatePaymentRequest adalah DTO untuk membuat payment attempt.
type CreatePaymentRequest struct{}

// PaymentResponse adalah DTO respons payment.
type PaymentResponse struct {
	ID              int64     `json:"id"`
	BookingID       int64     `json:"booking_id"`
	MerchantOrderID string    `json:"merchant_order_id"`
	Provider        string    `json:"provider"`
	Amount          int64     `json:"amount"`
	Currency        string    `json:"currency"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// RefundResponse adalah DTO respons refund.
type RefundResponse struct {
	ID        int64     `json:"id"`
	CaseKey   string    `json:"case_key"`
	BookingID int64     `json:"booking_id"`
	Reason    string    `json:"reason"`
	Amount    int64     `json:"amount"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// PaymentProvider adalah port untuk adapter pembayaran.
type PaymentProvider interface {
	CreatePayment(merchantOrderID string, amount int64, currency string, expiresAt time.Time) (providerTxID string, err error)
	GetPaymentStatus(merchantOrderID string) (status string, paidAt *time.Time, err error)
	CancelOrExpire(merchantOrderID string) error
	Refund(refundCaseKey string, merchantOrderID string, amount int64) (providerRefundID string, err error)
}

// NotificationProvider adalah port untuk adapter notifikasi.
type NotificationProvider interface {
	SendEmail(idempotencyKey string, to string, subject string, body string) (providerMessageID string, err error)
}
