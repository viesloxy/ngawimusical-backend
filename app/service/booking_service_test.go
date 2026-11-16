package service_test

import (
	"context"
	"fmt"
	"github.com/viesloxy/ngawimusical-backend/app/usecase"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/app/repository"
	"github.com/viesloxy/ngawimusical-backend/app/service"
	"github.com/viesloxy/ngawimusical-backend/helper"
	"github.com/viesloxy/ngawimusical-backend/middleware"
)

type fakeBookingRepo struct {
	bookings map[int64]*model.Booking
	nextID   int64
}

func newFakeBookingRepo() *fakeBookingRepo {
	return &fakeBookingRepo{
		bookings: make(map[int64]*model.Booking),
		nextID:   1,
	}
}

func (f *fakeBookingRepo) Create(ctx context.Context, tx repository.DBTX, b *model.Booking) error {
	// cek active hold constraint
	for _, existing := range f.bookings {
		if existing.UserID == b.UserID && existing.EventID == b.EventID && existing.IsActive() {
			return repository.ErrActiveHoldExists
		}
	}
	b.ID = f.nextID
	f.nextID++
	f.bookings[b.ID] = b
	return nil
}

func (f *fakeBookingRepo) GetByID(ctx context.Context, tx repository.DBTX, id int64) (*model.Booking, error) {
	b, ok := f.bookings[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return b, nil
}

func (f *fakeBookingRepo) GetByIDForUpdate(ctx context.Context, tx repository.DBTX, id int64) (*model.Booking, error) {
	return f.GetByID(ctx, tx, id)
}

func (f *fakeBookingRepo) GetByBookingCode(ctx context.Context, tx repository.DBTX, code string) (*model.Booking, error) {
	for _, b := range f.bookings {
		if b.BookingCode == code {
			return b, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (f *fakeBookingRepo) ListByUserID(ctx context.Context, tx repository.DBTX, userID int64, cursor *helper.Cursor, limit int) ([]model.Booking, error) {
	var list []model.Booking
	for _, b := range f.bookings {
		if b.UserID == userID {
			list = append(list, *b)
		}
	}
	return list, nil
}

func (f *fakeBookingRepo) ListByEventID(ctx context.Context, tx repository.DBTX, eventID int64, cursor *helper.Cursor, limit int) ([]model.Booking, error) {
	var list []model.Booking
	for _, b := range f.bookings {
		if b.EventID == eventID {
			list = append(list, *b)
		}
	}
	return list, nil
}

func (f *fakeBookingRepo) Update(ctx context.Context, tx repository.DBTX, booking *model.Booking) error {
	f.bookings[booking.ID] = booking
	return nil
}

func (f *fakeBookingRepo) UpdateStatus(ctx context.Context, tx repository.DBTX, id int64, status string, version int) error {
	b, ok := f.bookings[id]
	if !ok {
		return repository.ErrNotFound
	}
	b.Status = status
	b.Version++
	return nil
}

func (f *fakeBookingRepo) FindActiveByUserAndEvent(ctx context.Context, tx repository.DBTX, userID int64, eventID int64) (*model.Booking, error) {
	for _, b := range f.bookings {
		if b.UserID == userID && b.EventID == eventID && b.IsActive() {
			return b, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (f *fakeBookingRepo) ListDueRelease(ctx context.Context, tx repository.DBTX, now time.Time, limit int) ([]model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListActiveByEventID(ctx context.Context, tx repository.DBTX, eventID int64) ([]model.Booking, error) {
	return nil, nil
}
func (f *fakeBookingRepo) ListConfirmedByEventID(ctx context.Context, tx repository.DBTX, eventID int64) ([]model.Booking, error) {
	return nil, nil
}

type fakeUserLimitRepo struct {
	limits map[string]*model.UserEventLimit
}

func newFakeUserLimitRepo() *fakeUserLimitRepo {
	return &fakeUserLimitRepo{limits: make(map[string]*model.UserEventLimit)}
}

func (f *fakeUserLimitRepo) key(eventID, userID int64) string {
	return fmt.Sprintf("%d:%d", eventID, userID)
}

func (f *fakeUserLimitRepo) UpsertAndLock(ctx context.Context, tx repository.DBTX, eventID int64, userID int64, frozenMax int) (*model.UserEventLimit, error) {
	k := f.key(eventID, userID)
	lim, ok := f.limits[k]
	if !ok {
		lim = &model.UserEventLimit{
			EventID:        eventID,
			UserID:         userID,
			Held:           0,
			PurchasedTotal: 0,
			FrozenMax:      frozenMax,
			UpdatedAt:      time.Now().UTC(),
		}
		f.limits[k] = lim
	}
	return lim, nil
}

func (f *fakeUserLimitRepo) AddHeld(ctx context.Context, tx repository.DBTX, eventID int64, userID int64, delta int) error {
	k := f.key(eventID, userID)
	lim := f.limits[k]
	if lim.Held+lim.PurchasedTotal+delta > lim.FrozenMax {
		return repository.ErrUserLimitExceeded
	}
	lim.Held += delta
	return nil
}

func (f *fakeUserLimitRepo) SubtractHeld(ctx context.Context, tx repository.DBTX, eventID int64, userID int64, delta int) error {
	k := f.key(eventID, userID)
	lim, ok := f.limits[k]
	if ok && lim != nil {
		lim.Held -= delta
	}
	return nil
}

func (f *fakeUserLimitRepo) AddPurchasedTotal(ctx context.Context, tx repository.DBTX, eventID int64, userID int64, delta int) error {
	k := f.key(eventID, userID)
	lim, ok := f.limits[k]
	if ok && lim != nil {
		lim.Held -= delta
		lim.PurchasedTotal += delta
	}
	return nil
}

func (f *fakeUserLimitRepo) GetByEventAndUser(ctx context.Context, tx repository.DBTX, eventID int64, userID int64) (*model.UserEventLimit, error) {
	k := f.key(eventID, userID)
	lim, ok := f.limits[k]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return lim, nil
}

type fakeIdempotencyRepo struct {
	records map[string]*model.IdempotencyRecord
}

func newFakeIdempotencyRepo() *fakeIdempotencyRepo {
	return &fakeIdempotencyRepo{records: make(map[string]*model.IdempotencyRecord)}
}

func (f *fakeIdempotencyRepo) key(userID int64, scope, k string) string {
	return fmt.Sprintf("%d:%s:%s", userID, scope, k)
}

func (f *fakeIdempotencyRepo) InsertOrGet(ctx context.Context, tx repository.DBTX, record *model.IdempotencyRecord) (*model.IdempotencyRecord, bool, error) {
	k := f.key(record.UserID, record.OperationScope, record.IdempotencyKey)
	existing, ok := f.records[k]
	if ok {
		if existing.RequestHash != record.RequestHash {
			return nil, false, repository.ErrIdempotencyKeyReused
		}
		return existing, false, nil
	}
	f.records[k] = record
	return record, true, nil
}

func (f *fakeIdempotencyRepo) Complete(ctx context.Context, tx repository.DBTX, userID int64, scope string, key string, responseStatus int, responseBody []byte, resourceID string) error {
	k := f.key(userID, scope, key)
	rec := f.records[k]
	now := time.Now().UTC()
	rec.ResponseStatus = &responseStatus
	rec.ResponseBody = responseBody
	rec.ResourceID = &resourceID
	rec.CompletedAt = &now
	return nil
}

type fakeInventoryRepo struct{}

func (f *fakeInventoryRepo) Create(ctx context.Context, tx repository.DBTX, m *model.InventoryMovement) error {
	return nil
}

func setupBookingTestApp() (*fiber.App, *fakeEventRepo, *fakeTierRepo, *fakeBookingRepo) {
	app := fiber.New(fiber.Config{
		ErrorHandler: helper.ErrorHandler,
	})

	eventRepo := newFakeEventRepo()
	tierRepo := newFakeTierRepo()
	bookingRepo := newFakeBookingRepo()
	userLimitRepo := newFakeUserLimitRepo()
	idempotencyRepo := newFakeIdempotencyRepo()
	inventoryRepo := &fakeInventoryRepo{}
	outboxRepo := &fakeOutboxRepo{}
	auditRepo := &fakeAuditRepo{}
	txMgr := &noopTxMgr{}

	uc := usecase.NewBookingUsecase(
		txMgr, eventRepo, tierRepo, bookingRepo, userLimitRepo, idempotencyRepo,
		inventoryRepo, outboxRepo, auditRepo, 10*time.Minute, 2*time.Minute,
	)
	svc := service.NewBookingService(uc, []byte("test-secret-at-least-32-bytes-long!"))

	// Middleware inject test actor
	app.Use(func(c *fiber.Ctx) error {
		middleware.SetActor(c, middleware.Actor{
			UserID: 1,
			Role:   "attendee",
		})
		return c.Next()
	})

	api := app.Group("/api/v1")
	svc.Register(api)

	return app, eventRepo, tierRepo, bookingRepo
}

func TestBookingService_CreateHold_Success(t *testing.T) {
	app, eventRepo, tierRepo, _ := setupBookingTestApp()

	now := time.Now().UTC()
	eventRepo.events[1] = &model.Event{
		ID:                1,
		Code:              "evt_1",
		Status:            model.EventStatusPublished,
		SaleControl:       model.SaleControlEnabled,
		MaxTicketsPerUser: 4,
		SaleOpenAt:        now.Add(-1 * time.Hour),
		SaleCloseAt:       now.Add(2 * time.Hour),
	}

	tierRepo.tiers[1] = &model.TicketTier{
		ID:          1,
		EventID:     1,
		Code:        "VIP",
		PriceAmount: 100000,
		FeeAmount:   5000,
		Quota:       10,
		HeldCount:   0,
	}

	payload := `{"event_id":"1","tier_id":"1","quantity":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "random-idempotency-key-12345")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d; want 201 Created", resp.StatusCode)
	}

	// Replay request dengan key dan payload sama
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", strings.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "random-idempotency-key-12345")

	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if resp2.StatusCode != http.StatusCreated {
		t.Errorf("replay status = %d; want 201 Created", resp2.StatusCode)
	}
}

func TestBookingService_CreateHold_SoldOut(t *testing.T) {
	app, eventRepo, tierRepo, _ := setupBookingTestApp()

	now := time.Now().UTC()
	eventRepo.events[1] = &model.Event{
		ID:                1,
		Status:            model.EventStatusPublished,
		SaleControl:       model.SaleControlEnabled,
		MaxTicketsPerUser: 4,
		SaleOpenAt:        now.Add(-1 * time.Hour),
		SaleCloseAt:       now.Add(2 * time.Hour),
	}

	tierRepo.tiers[1] = &model.TicketTier{
		ID:        1,
		EventID:   1,
		Quota:     1,
		HeldCount: 1, // stok habis
	}

	payload := `{"event_id":"1","tier_id":"1","quantity":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "random-idempotency-key-soldout")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d; want 409 Conflict (SOLD_OUT)", resp.StatusCode)
	}
}

func TestBookingService_CancelHold(t *testing.T) {
	app, _, tierRepo, bookingRepo := setupBookingTestApp()

	tierRepo.tiers[1] = &model.TicketTier{
		ID:        1,
		HeldCount: 2,
	}

	bookingRepo.bookings[1] = &model.Booking{
		ID:          1,
		BookingCode: "bkg_test",
		UserID:      1,
		EventID:     1,
		TierID:      1,
		Quantity:    2,
		Status:      model.BookingStatusHeld,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookings/1/cancel", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d; want 200 OK", resp.StatusCode)
	}

	if bookingRepo.bookings[1].Status != model.BookingStatusCancelled {
		t.Errorf("status = %s; want CANCELLED", bookingRepo.bookings[1].Status)
	}

	if tierRepo.tiers[1].HeldCount != 0 {
		t.Errorf("held_count = %d; want 0", tierRepo.tiers[1].HeldCount)
	}
}
