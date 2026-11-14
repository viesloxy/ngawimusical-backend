package model_test

import (
	"testing"

	"github.com/viesloxy/ngawimusical-backend/app/model"
)

func TestCalculateTotal(t *testing.T) {
	unitPrice := int64(100000)
	unitFee := int64(5000)
	quantity := 2

	expected := int64(210000)
	got := model.CalculateTotal(unitPrice, unitFee, quantity)
	if got != expected {
		t.Errorf("CalculateTotal() = %d; want %d", got, expected)
	}
}

func TestBooking_Lifecycle(t *testing.T) {
	b := model.Booking{
		Status:    model.BookingStatusHeld,
		SwapCount: 0,
	}

	if !b.IsActive() {
		t.Errorf("diharapkan booking HELD berstatus aktif")
	}

	if !b.CanCancel() {
		t.Errorf("diharapkan booking HELD bisa dibatalkan")
	}

	if !b.CanSwap(2) {
		t.Errorf("diharapkan booking HELD dengan swap_count 0 bisa swap")
	}

	// Setelah swap 2 kali
	b.SwapCount = 2
	if b.CanSwap(2) {
		t.Errorf("diharapkan booking dengan swap_count 2 tidak bisa swap lagi")
	}

	// Terminal status
	b.Status = model.BookingStatusCancelled
	if !b.IsTerminal() {
		t.Errorf("diharapkan CANCELLED adalah status terminal")
	}
	if b.CanCancel() {
		t.Errorf("diharapkan CANCELLED tidak bisa dibatalkan lagi")
	}

	b.Status = model.BookingStatusExpired
	if !b.IsTerminal() {
		t.Errorf("diharapkan EXPIRED adalah status terminal")
	}
	if b.CanCancel() {
		t.Errorf("diharapkan EXPIRED tidak bisa dibatalkan")
	}
}

func TestUserEventLimit_RemainingCapacity(t *testing.T) {
	limit := model.UserEventLimit{
		FrozenMax:      4,
		Held:           1,
		PurchasedTotal: 2,
	}

	if got := limit.RemainingCapacity(); got != 1 {
		t.Errorf("RemainingCapacity() = %d; want 1", got)
	}

	limit.Held = 2
	if got := limit.RemainingCapacity(); got != 0 {
		t.Errorf("RemainingCapacity() = %d; want 0", got)
	}
}
