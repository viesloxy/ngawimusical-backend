package model_test

import (
	"testing"
	"time"

	"github.com/viesloxy/ngawimusical-backend/app/model"
)

func TestEvent_TimelineValidation(t *testing.T) {
	now := time.Now().UTC()
	open := now.Add(1 * time.Hour)
	close := now.Add(2 * time.Hour)
	start := now.Add(3 * time.Hour)
	end := now.Add(4 * time.Hour)

	if !model.ValidateTimeline(open, close, start, end) {
		t.Errorf("diharapkan timeline valid, tetapi menghasilkan false")
	}

	// Kasus salah: close setelah start
	if model.ValidateTimeline(open, start.Add(1*time.Hour), start, end) {
		t.Errorf("diharapkan timeline tidak valid jika close > start")
	}

	// Kasus salah: open setelah close
	if model.ValidateTimeline(close, open, start, end) {
		t.Errorf("diharapkan timeline tidak valid jika open > close")
	}

	// Kasus salah: start setelah end
	if model.ValidateTimeline(open, close, end, start) {
		t.Errorf("diharapkan timeline tidak valid jika start > end")
	}
}

func TestEvent_Transition(t *testing.T) {
	evt := model.Event{
		Status:      model.EventStatusDraft,
		SaleControl: model.SaleControlEnabled,
	}

	// Draft -> Published diperbolehkan
	if !evt.CanTransitionTo(model.EventStatusPublished) {
		t.Errorf("diharapkan Draft -> Published diizinkan")
	}

	// Draft -> Archived diperbolehkan
	if !evt.CanTransitionTo(model.EventStatusArchived) {
		t.Errorf("diharapkan Draft -> Archived diizinkan")
	}

	// Draft -> Cancelled tidak diperbolehkan
	if evt.CanTransitionTo(model.EventStatusCancelled) {
		t.Errorf("diharapkan Draft -> Cancelled ditolak")
	}

	// Published -> Cancelled diperbolehkan
	evt.Status = model.EventStatusPublished
	if !evt.CanTransitionTo(model.EventStatusCancelled) {
		t.Errorf("diharapkan Published -> Cancelled diizinkan")
	}

	// Published -> Completed diperbolehkan
	if !evt.CanTransitionTo(model.EventStatusCompleted) {
		t.Errorf("diharapkan Published -> Completed diizinkan")
	}

	// Cancelled adalah terminal
	evt.Status = model.EventStatusCancelled
	if evt.CanTransitionTo(model.EventStatusCompleted) {
		t.Errorf("diharapkan Cancelled tidak bisa transisi ke state lain")
	}
}

func TestEvent_SaleControlTransition(t *testing.T) {
	evt := model.Event{
		Status:      model.EventStatusPublished,
		SaleControl: model.SaleControlEnabled,
	}

	// Enabled -> Paused
	if !evt.CanTransitionSaleTo(model.SaleControlPaused) {
		t.Errorf("diharapkan Enabled -> Paused diizinkan")
	}

	// Enabled -> Closed
	if !evt.CanTransitionSaleTo(model.SaleControlClosed) {
		t.Errorf("diharapkan Enabled -> Closed diizinkan")
	}

	// Paused -> Enabled
	evt.SaleControl = model.SaleControlPaused
	if !evt.CanTransitionSaleTo(model.SaleControlEnabled) {
		t.Errorf("diharapkan Paused -> Enabled diizinkan")
	}

	// Closed adalah terminal
	evt.SaleControl = model.SaleControlClosed
	if evt.CanTransitionSaleTo(model.SaleControlEnabled) {
		t.Errorf("diharapkan Closed tidak bisa transisi lagi")
	}
}

func TestTicketTier_AvailableCount(t *testing.T) {
	tier := model.TicketTier{
		Quota:        100,
		HeldCount:    20,
		SoldCount:    30,
		RetiredCount: 10,
	}

	expected := 40
	if got := tier.AvailableCount(); got != expected {
		t.Errorf("AvailableCount() = %d; want %d", got, expected)
	}

	if tier.IsSoldOut() {
		t.Errorf("IsSoldOut() diharapkan false saat stok 40")
	}

	tier.HeldCount = 60
	if got := tier.AvailableCount(); got != 0 {
		t.Errorf("AvailableCount() = %d; want 0 saat kuota habis", got)
	}

	if !tier.IsSoldOut() {
		t.Errorf("IsSoldOut() diharapkan true saat sisa 0")
	}
}
