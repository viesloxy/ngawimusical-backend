package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/app/repository"
)

// createTestBookingFixture membuat user, event terbit, dan satu tier untuk uji booking.
// Test dilewati bila database tidak tersedia (lihat openTestPool).
func createTestBookingFixture(t *testing.T, pool *pgxpool.Pool) (model.User, model.Event, model.TicketTier) {
	t.Helper()
	ctx := context.Background()
	users := repository.NewUserRepository()
	organizer := createTestUser(t, users, pool)

	now := time.Now().UTC()
	event := model.Event{
		Code:              "UJI-" + uuid.NewString()[:12],
		Title:             "Konser uji",
		Venue:             "Venue uji",
		VenueCapacity:     100,
		Status:            model.EventStatusPublished,
		SaleControl:       "ENABLED",
		QueueEpoch:        1,
		MaxTicketsPerUser: 4,
		SaleOpenAt:        now.Add(-time.Hour),
		SaleCloseAt:       now.Add(24 * time.Hour),
		StartsAt:          now.Add(48 * time.Hour),
		EndsAt:            now.Add(50 * time.Hour),
		OrganizerID:       organizer.ID,
		Version:           1,
	}
	events := repository.NewPostgresEventRepository(pool)
	if err := events.Create(ctx, pool, &event); err != nil {
		t.Fatalf("buat event uji: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM events WHERE id = $1`, event.ID)
	})

	tier := model.TicketTier{
		EventID:     event.ID,
		Code:        "REG",
		Name:        "Regular",
		PriceAmount: 100000,
		FeeAmount:   5000,
		Quota:       50,
		Version:     1,
	}
	if err := repository.NewPostgresTicketTierRepository(pool).Create(ctx, pool, &tier); err != nil {
		t.Fatalf("buat tier uji: %v", err)
	}
	return organizer, event, tier
}

func TestBookingHoldIsFoundAsActiveForSameUserAndEvent(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	user, event, tier := createTestBookingFixture(t, pool)

	now := time.Now().UTC()
	booking := model.Booking{
		BookingCode:     "BK-" + uuid.NewString()[:12],
		UserID:          user.ID,
		EventID:         event.ID,
		TierID:          tier.ID,
		Quantity:        2,
		Status:          model.BookingStatusHeld,
		UnitPriceAmount: tier.PriceAmount,
		UnitFeeAmount:   tier.FeeAmount,
		TotalAmount:     2 * (tier.PriceAmount + tier.FeeAmount),
		Currency:        "IDR",
		ExpiresAt:       now.Add(15 * time.Minute),
		ReleaseAt:       now.Add(30 * time.Minute),
		Version:         1,
	}
	bookings := repository.NewPostgresBookingRepository(pool)
	if err := bookings.Create(ctx, pool, &booking); err != nil {
		t.Fatalf("buat hold: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bookings WHERE id = $1`, booking.ID)
	})

	active, err := bookings.FindActiveByUserAndEvent(ctx, pool, user.ID, event.ID)
	if err != nil {
		t.Fatalf("cari hold aktif: %v", err)
	}
	if active.BookingCode != booking.BookingCode {
		t.Fatalf("hold aktif harus sama dengan yang dibuat, dapat %s", active.BookingCode)
	}
}

func TestBookingStatusUpdateRejectsStaleVersion(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	user, event, tier := createTestBookingFixture(t, pool)

	now := time.Now().UTC()
	booking := model.Booking{
		BookingCode:     "BK-" + uuid.NewString()[:12],
		UserID:          user.ID,
		EventID:         event.ID,
		TierID:          tier.ID,
		Quantity:        1,
		Status:          model.BookingStatusHeld,
		UnitPriceAmount: tier.PriceAmount,
		UnitFeeAmount:   tier.FeeAmount,
		TotalAmount:     tier.PriceAmount + tier.FeeAmount,
		Currency:        "IDR",
		ExpiresAt:       now.Add(15 * time.Minute),
		ReleaseAt:       now.Add(30 * time.Minute),
		Version:         1,
	}
	bookings := repository.NewPostgresBookingRepository(pool)
	if err := bookings.Create(ctx, pool, &booking); err != nil {
		t.Fatalf("buat hold: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM bookings WHERE id = $1`, booking.ID)
	})

	err := bookings.UpdateStatus(ctx, pool, booking.ID, model.BookingStatusCancelled, 99)
	if !errors.Is(err, repository.ErrVersionConflict) {
		t.Fatalf("versi basi harus ditolak dengan ErrVersionConflict, dapat %v", err)
	}
	if err := bookings.UpdateStatus(ctx, pool, booking.ID, model.BookingStatusCancelled, 1); err != nil {
		t.Fatalf("versi benar harus berhasil: %v", err)
	}
}
