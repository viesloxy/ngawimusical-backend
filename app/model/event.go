package model

import "time"

// Status event.
const (
	EventStatusDraft     = "DRAFT"
	EventStatusPublished = "PUBLISHED"
	EventStatusCancelled = "CANCELLED"
	EventStatusCompleted = "COMPLETED"
	EventStatusArchived  = "ARCHIVED"
)

// Sale control.
const (
	SaleControlEnabled = "ENABLED"
	SaleControlPaused  = "PAUSED"
	SaleControlClosed  = "CLOSED"
)

// Staff role pada penugasan event.
const (
	StaffRoleGate      = "GATE_STAFF"
	StaffRoleOrganizer = "ORGANIZER_STAFF"
)

// Staff assignment status.
const (
	StaffStatusActive  = "ACTIVE"
	StaffStatusRevoked = "REVOKED"
)

// Event adalah entitas konser yang dikelola oleh organizer.
type Event struct {
	ID                int64     `json:"id"`
	Code              string    `json:"code"`
	Title             string    `json:"title"`
	Venue             string    `json:"venue"`
	VenueCapacity     int       `json:"venue_capacity"`
	Status            string    `json:"status"`
	SaleControl       string    `json:"sale_control"`
	QueueEpoch        int       `json:"queue_epoch"`
	MaxTicketsPerUser int       `json:"max_tickets_per_user"`
	SaleOpenAt        time.Time `json:"sale_open_at"`
	SaleCloseAt       time.Time `json:"sale_close_at"`
	StartsAt          time.Time `json:"starts_at"`
	EndsAt            time.Time `json:"ends_at"`
	OrganizerID       int64     `json:"organizer_id"`
	Version           int       `json:"version"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// TicketTier adalah kategori tiket dengan kuota dan harga.
type TicketTier struct {
	ID           int64     `json:"id"`
	EventID      int64     `json:"event_id"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	PriceAmount  int64     `json:"price_amount"`
	FeeAmount    int64     `json:"fee_amount"`
	Quota        int       `json:"quota"`
	HeldCount    int       `json:"held_count"`
	SoldCount    int       `json:"sold_count"`
	RetiredCount int       `json:"retired_count"`
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AvailableCount menghitung stok yang tersedia dari sisi server.
func (t TicketTier) AvailableCount() int {
	available := t.Quota - t.HeldCount - t.SoldCount - t.RetiredCount
	if available < 0 {
		return 0
	}
	return available
}

// IsSoldOut memeriksa apakah stok habis (termasuk yang sementara di-hold).
func (t TicketTier) IsSoldOut() bool {
	return t.AvailableCount() == 0
}

// EventStaff adalah penugasan petugas pada event.
type EventStaff struct {
	EventID   int64     `json:"event_id"`
	UserID    int64     `json:"user_id"`
	StaffRole string    `json:"staff_role"`
	GateID    string    `json:"gate_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// CanTransitionTo memeriksa apakah transisi status event diizinkan.
func (e Event) CanTransitionTo(target string) bool {
	switch e.Status {
	case EventStatusDraft:
		return target == EventStatusPublished || target == EventStatusArchived
	case EventStatusPublished:
		return target == EventStatusCancelled || target == EventStatusCompleted
	default:
		return false
	}
}

// CanTransitionSaleTo memeriksa apakah transisi sale control diizinkan.
func (e Event) CanTransitionSaleTo(target string) bool {
	switch e.SaleControl {
	case SaleControlEnabled:
		return target == SaleControlPaused || target == SaleControlClosed
	case SaleControlPaused:
		return target == SaleControlEnabled || target == SaleControlClosed
	default:
		return false
	}
}

// IsPublished memeriksa apakah event sudah dipublikasikan.
func (e Event) IsPublished() bool {
	return e.Status == EventStatusPublished
}

// IsSaleActive memeriksa apakah event menerima pemesanan baru.
func (e Event) IsSaleActive(now time.Time) bool {
	return e.Status == EventStatusPublished &&
		e.SaleControl == SaleControlEnabled &&
		!now.Before(e.SaleOpenAt) &&
		now.Before(e.SaleCloseAt)
}

// ValidateTimeline memastikan urutan waktu event benar.
func ValidateTimeline(saleOpen, saleClose, starts, ends time.Time) bool {
	return saleOpen.Before(saleClose) &&
		saleClose.Before(starts) &&
		starts.Before(ends)
}

// CreateEventRequest adalah DTO untuk membuat event baru.
type CreateEventRequest struct {
	Title             string `json:"title" validate:"required,min=3,max=150"`
	Venue             string `json:"venue" validate:"required,min=3,max=150"`
	VenueCapacity     int    `json:"venue_capacity" validate:"required,gte=1"`
	MaxTicketsPerUser int    `json:"max_tickets_per_user" validate:"required,gte=1,lte=10"`
	SaleOpenAt        string `json:"sale_open_at" validate:"required"`
	SaleCloseAt       string `json:"sale_close_at" validate:"required"`
	StartsAt          string `json:"starts_at" validate:"required"`
	EndsAt            string `json:"ends_at" validate:"required"`
}

// UpdateEventRequest adalah DTO untuk memperbarui event draft.
type UpdateEventRequest struct {
	Title         *string `json:"title,omitempty" validate:"omitempty,min=3,max=150"`
	Venue         *string `json:"venue,omitempty" validate:"omitempty,min=3,max=150"`
	VenueCapacity *int    `json:"venue_capacity,omitempty" validate:"omitempty,gte=1"`
	SaleOpenAt    *string `json:"sale_open_at,omitempty"`
	SaleCloseAt   *string `json:"sale_close_at,omitempty"`
	StartsAt      *string `json:"starts_at,omitempty"`
	EndsAt        *string `json:"ends_at,omitempty"`
	Version       int     `json:"version" validate:"required,gte=1"`
}

// CreateTierRequest adalah DTO untuk membuat tier baru.
type CreateTierRequest struct {
	Code        string `json:"code" validate:"required,min=1,max=40"`
	Name        string `json:"name" validate:"required,min=1,max=100"`
	PriceAmount int64  `json:"price_amount" validate:"required,gte=1"`
	FeeAmount   int64  `json:"fee_amount" validate:"gte=0"`
	Quota       int    `json:"quota" validate:"required,gte=0"`
}

// UpdateTierRequest adalah DTO untuk memperbarui tier.
type UpdateTierRequest struct {
	Name        *string `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	PriceAmount *int64  `json:"price_amount,omitempty" validate:"omitempty,gte=1"`
	FeeAmount   *int64  `json:"fee_amount,omitempty" validate:"omitempty,gte=0"`
	Quota       *int    `json:"quota,omitempty" validate:"omitempty,gte=0"`
	Version     int     `json:"version" validate:"required,gte=1"`
}

// EventListResponse adalah DTO respons daftar event.
type EventListResponse struct {
	ID            int64     `json:"id"`
	Code          string    `json:"code"`
	Title         string    `json:"title"`
	Venue         string    `json:"venue"`
	VenueCapacity int       `json:"venue_capacity"`
	Status        string    `json:"status"`
	SaleOpenAt    time.Time `json:"sale_open_at"`
	SaleCloseAt   time.Time `json:"sale_close_at"`
	StartsAt      time.Time `json:"starts_at"`
	EndsAt        time.Time `json:"ends_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// TierResponse adalah DTO respons tier.
type TierResponse struct {
	ID             int64     `json:"id"`
	EventID        int64     `json:"event_id"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	PriceAmount    int64     `json:"price_amount"`
	FeeAmount      int64     `json:"fee_amount"`
	Quota          int       `json:"quota"`
	AvailableCount int       `json:"available_count"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

// TierAvailabilityStatus mengembalikan label ketersediaan untuk konsumen.
func TierAvailabilityStatus(available int) string {
	if available == 0 {
		return "SOLD_OUT"
	}
	return "AVAILABLE"
}
