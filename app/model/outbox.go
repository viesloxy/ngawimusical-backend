package model

import "time"

// Ticket status.
const (
	TicketStatusValid     = "VALID"
	TicketStatusCheckedIn = "CHECKED_IN"
	TicketStatusVoid      = "VOID"
)

// Inventory movement type.
const (
	MovementHold          = "HOLD"
	MovementReleaseCancel = "RELEASE_CANCEL"
	MovementReleaseExpire = "RELEASE_EXPIRE"
	MovementSwapOut       = "SWAP_OUT"
	MovementSwapIn        = "SWAP_IN"
	MovementConfirmSale   = "CONFIRM_SALE"
	MovementRetireRefund  = "RETIRE_REFUND"
)

// Outbox event type.
const (
	OutboxEventBookingHeld        = "BookingHeld"
	OutboxEventBookingTierChanged = "BookingTierChanged"
	OutboxEventBookingExpired     = "BookingExpired"
	OutboxEventBookingCancelled   = "BookingCancelled"
	OutboxEventPaymentCreateReq   = "PaymentCreateRequested"
	OutboxEventBookingConfirmed   = "BookingConfirmed"
	OutboxEventEventCancelled     = "EventCancelled"
	OutboxEventRefundRequested    = "RefundRequested"
	OutboxEventBookingRefunded    = "BookingRefunded"
)

// Notification delivery state.
const (
	DeliveryStatePending         = "PENDING"
	DeliveryStateSent            = "SENT"
	DeliveryStateUnknown         = "UNKNOWN"
	DeliveryStateFailedPermanent = "FAILED_PERMANENT"
)

// Operational exception state.
const (
	ExceptionStateOpen          = "OPEN"
	ExceptionStateInvestigating = "INVESTIGATING"
	ExceptionStateResolved      = "RESOLVED"
)

// Ticket adalah unit e-ticket yang diterbitkan setelah booking dikonfirmasi.
type Ticket struct {
	ID              int64      `json:"id"`
	BookingID       int64      `json:"booking_id"`
	Ordinal         int        `json:"ordinal"`
	EventID         int64      `json:"event_id"`
	TierID          int64      `json:"tier_id"`
	TokenHash       string     `json:"-"`
	TokenCiphertext string     `json:"-"`
	Status          string     `json:"status"`
	CheckedInAt     *time.Time `json:"checked_in_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// IsValid memeriksa apakah tiket masih bisa digunakan.
func (t Ticket) IsValid() bool {
	return t.Status == TicketStatusValid
}

// CheckIn adalah catatan pemindaian tiket oleh petugas.
type CheckIn struct {
	ID          int64     `json:"id"`
	TicketID    int64     `json:"ticket_id"`
	StaffID     int64     `json:"staff_id"`
	GateID      string    `json:"gate_id"`
	OperationID string    `json:"operation_id"`
	ScannedAt   time.Time `json:"scanned_at"`
}

// InventoryMovement adalah audit perubahan stok pada tier.
type InventoryMovement struct {
	ID           int64     `json:"id"`
	OperationID  string    `json:"operation_id"`
	TierID       int64     `json:"tier_id"`
	BookingID    int64     `json:"booking_id"`
	MovementType string    `json:"movement_type"`
	DeltaHeld    int       `json:"delta_held"`
	DeltaSold    int       `json:"delta_sold"`
	DeltaRetired int       `json:"delta_retired"`
	Reason       string    `json:"reason"`
	CreatedAt    time.Time `json:"created_at"`
}

// Outbox menyimpan event domain yang belum dikirim ke consumer.
type Outbox struct {
	ID               string     `json:"id"`
	AggregateID      string     `json:"aggregate_id"`
	AggregateVersion int        `json:"aggregate_version"`
	EventType        string     `json:"event_type"`
	SchemaVersion    int        `json:"schema_version"`
	Payload          []byte     `json:"payload"`
	LeaseOwner       *string    `json:"lease_owner,omitempty"`
	LeaseUntil       *time.Time `json:"lease_until,omitempty"`
	NextAttemptAt    time.Time  `json:"next_attempt_at"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// ConsumerReceipt menandai event yang sudah diproses oleh consumer tertentu.
type ConsumerReceipt struct {
	Consumer    string    `json:"consumer"`
	EventID     string    `json:"event_id"`
	ProcessedAt time.Time `json:"processed_at"`
}

// NotificationDelivery menyimpan status pengiriman notifikasi.
type NotificationDelivery struct {
	ID                int64      `json:"id"`
	EventID           string     `json:"event_id"`
	UserID            int64      `json:"user_id"`
	Channel           string     `json:"channel"`
	Recipient         string     `json:"recipient"`
	Subject           string     `json:"subject"`
	Body              string     `json:"body"`
	State             string     `json:"state"`
	ProviderKey       string     `json:"provider_key"`
	ProviderMessageID *string    `json:"provider_message_id,omitempty"`
	LeaseUntil        *time.Time `json:"lease_until,omitempty"`
	Attempts          int        `json:"attempts"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// AuditLog menyimpan catatan audit perubahan resource.
type AuditLog struct {
	ID           int64     `json:"id"`
	ActorID      *int64    `json:"actor_id,omitempty"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	RequestID    string    `json:"request_id"`
	Changes      []byte    `json:"changes,omitempty"`
	Reason       *string   `json:"reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// OperationalException mencatat kasus operasional yang memerlukan penanganan manual.
type OperationalException struct {
	ID           int64      `json:"id"`
	CaseKey      string     `json:"case_key"`
	Category     string     `json:"category"`
	ResourceID   string     `json:"resource_id"`
	Owner        string     `json:"owner"`
	State        string     `json:"state"`
	Evidence     []byte     `json:"evidence"`
	NextActionAt time.Time  `json:"next_action_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// TicketResponse adalah DTO respons tiket untuk pemilik.
type TicketResponse struct {
	ID          int64      `json:"id"`
	BookingCode string     `json:"booking_code"`
	Ordinal     int        `json:"ordinal"`
	EventID     int64      `json:"event_id"`
	TierID      int64      `json:"tier_id"`
	Status      string     `json:"status"`
	QRToken     string     `json:"qr_token,omitempty"`
	CheckedInAt *time.Time `json:"checked_in_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// CheckInRequest adalah DTO untuk memindai tiket.
type CheckInRequest struct {
	Token string `json:"token" validate:"required"`
}

// CheckInResponse adalah DTO respons pemindaian tiket.
type CheckInResponse struct {
	TicketID  int64     `json:"ticket_id"`
	Status    string    `json:"status"`
	GateID    string    `json:"gate_id"`
	ScannedAt time.Time `json:"scanned_at"`
}
