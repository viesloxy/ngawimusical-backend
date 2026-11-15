package model

import (
	"errors"
	"time"
)

// Role akun yang sesuai dengan tabel roles.
const (
	RoleAdmin     = "admin"
	RoleOrganizer = "organizer"
	RoleGateStaff = "gate_staff"
	RoleAttendee  = "attendee"
)

// Status akun yang sesuai dengan kolom users.status.
const (
	AccountActive    = "ACTIVE"
	AccountSuspended = "SUSPENDED"
	AccountDeleted   = "DELETED"
)

// Jenis token aksi yang sesuai dengan kolom identity_action_tokens.purpose.
const (
	PurposeVerifyEmail   = "VERIFY_EMAIL"
	PurposeResetPassword = "RESET_PASSWORD"
)

// Peran staf pada event yang sesuai dengan event_staff.staff_role.
const (
	StaffRoleGateStaff      = "GATE_STAFF"
	StaffRoleOrganizerStaff = "ORGANIZER_STAFF"
)

// Status penugasan staf yang sesuai dengan event_staff.status.
const (
	StaffActive  = "ACTIVE"
	StaffRevoked = "REVOKED"
)

var (
	ErrAccountSuspended = errors.New("akun ditangguhkan")
	ErrAccountDeleted   = errors.New("akun sudah dihapus")
	ErrAccountInactive  = errors.New("akun belum aktif")
)

// User adalah akun pengguna.
type User struct {
	ID              int64
	Username        string
	Email           string
	PasswordHash    string
	RoleID          int
	RoleName        string
	Status          string
	EmailVerifiedAt *time.Time
	AuthVersion     int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Role adalah peran yang dimiliki user.
type Role struct {
	ID        int
	Name      string
	CreatedAt time.Time
}

// RefreshToken adalah token sesi yang disimpan sebagai hash.
type RefreshToken struct {
	ID        int64
	UserID    int64
	TokenHash string
	FamilyID  string
	ParentID  *int64
	ExpiresAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// IdentityActionToken adalah token sekali pakai untuk verifikasi email dan reset password.
type IdentityActionToken struct {
	ID         int64
	UserID     int64
	TokenHash  string
	Purpose    string
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// EventStaff adalah penugasan staf pada sebuah event.
type EventStaff struct {
	EventID   int64
	UserID    int64
	StaffRole string
	GateID    string
	Status    string
	CreatedAt time.Time
}

// LoginBlockReason mengembalikan alasan akun tidak boleh login.
// Nil berarti akun aktif dan boleh login.
func LoginBlockReason(status string) error {
	switch status {
	case AccountActive:
		return nil
	case AccountSuspended:
		return ErrAccountSuspended
	case AccountDeleted:
		return ErrAccountDeleted
	default:
		return ErrAccountInactive
	}
}

// IsUsable menandakan token aksi belum dipakai dan belum kedaluwarsa.
func (t IdentityActionToken) IsUsable(now time.Time) bool {
	return t.ConsumedAt == nil && now.Before(t.ExpiresAt)
}

// IsUsable menandakan refresh token belum dirotasi, belum dicabut, dan belum kedaluwarsa.
func (t RefreshToken) IsUsable(now time.Time) bool {
	return t.UsedAt == nil && t.RevokedAt == nil && now.Before(t.ExpiresAt)
}
