package service_test

import (
	"context"
	"encoding/json"
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

// Fake repository implementations for EventService tests
type fakeEventRepo struct {
	events map[int64]*model.Event
	nextID int64
}

func newFakeEventRepo() *fakeEventRepo {
	return &fakeEventRepo{
		events: make(map[int64]*model.Event),
		nextID: 1,
	}
}

func (f *fakeEventRepo) Create(ctx context.Context, tx repository.DBTX, e *model.Event) error {
	e.ID = f.nextID
	f.nextID++
	f.events[e.ID] = e
	return nil
}

func (f *fakeEventRepo) GetByID(ctx context.Context, tx repository.DBTX, id int64) (*model.Event, error) {
	e, ok := f.events[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return e, nil
}

func (f *fakeEventRepo) GetByCode(ctx context.Context, tx repository.DBTX, code string) (*model.Event, error) {
	for _, e := range f.events {
		if e.Code == code {
			return e, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (f *fakeEventRepo) GetByIDForShare(ctx context.Context, tx repository.DBTX, id int64) (*model.Event, error) {
	return f.GetByID(ctx, tx, id)
}

func (f *fakeEventRepo) GetByIDForUpdate(ctx context.Context, tx repository.DBTX, id int64) (*model.Event, error) {
	return f.GetByID(ctx, tx, id)
}

func (f *fakeEventRepo) ListPublished(ctx context.Context, tx repository.DBTX, cursor *helper.Cursor, limit int) ([]model.Event, error) {
	var list []model.Event
	for _, e := range f.events {
		if e.Status == model.EventStatusPublished {
			list = append(list, *e)
		}
	}
	return list, nil
}

func (f *fakeEventRepo) Update(ctx context.Context, tx repository.DBTX, event *model.Event) error {
	f.events[event.ID] = event
	return nil
}

func (f *fakeEventRepo) UpdateStatus(ctx context.Context, tx repository.DBTX, id int64, status string, version int) error {
	e, ok := f.events[id]
	if !ok {
		return repository.ErrNotFound
	}
	e.Status = status
	e.Version++
	return nil
}

func (f *fakeEventRepo) UpdateSaleControl(ctx context.Context, tx repository.DBTX, id int64, saleControl string, version int) error {
	e, ok := f.events[id]
	if !ok {
		return repository.ErrNotFound
	}
	e.SaleControl = saleControl
	e.Version++
	return nil
}

type fakeTierRepo struct {
	tiers  map[int64]*model.TicketTier
	nextID int64
}

func newFakeTierRepo() *fakeTierRepo {
	return &fakeTierRepo{
		tiers:  make(map[int64]*model.TicketTier),
		nextID: 1,
	}
}

func (f *fakeTierRepo) Create(ctx context.Context, tx repository.DBTX, t *model.TicketTier) error {
	t.ID = f.nextID
	f.nextID++
	f.tiers[t.ID] = t
	return nil
}

func (f *fakeTierRepo) GetByID(ctx context.Context, tx repository.DBTX, id int64) (*model.TicketTier, error) {
	t, ok := f.tiers[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return t, nil
}

func (f *fakeTierRepo) GetByIDForUpdate(ctx context.Context, tx repository.DBTX, id int64) (*model.TicketTier, error) {
	return f.GetByID(ctx, tx, id)
}

func (f *fakeTierRepo) ListByEventID(ctx context.Context, tx repository.DBTX, eventID int64) ([]model.TicketTier, error) {
	var list []model.TicketTier
	for _, t := range f.tiers {
		if t.EventID == eventID {
			list = append(list, *t)
		}
	}
	return list, nil
}

func (f *fakeTierRepo) Update(ctx context.Context, tx repository.DBTX, tier *model.TicketTier) error {
	f.tiers[tier.ID] = tier
	return nil
}

func (f *fakeTierRepo) HoldStock(ctx context.Context, tx repository.DBTX, tierID int64, eventID int64, quantity int) (*model.TicketTier, error) {
	t, ok := f.tiers[tierID]
	if !ok {
		return nil, repository.ErrNotFound
	}
	if t.AvailableCount() < quantity {
		return nil, repository.ErrSoldOut
	}
	t.HeldCount += quantity
	return t, nil
}

func (f *fakeTierRepo) ReleaseHeldStock(ctx context.Context, tx repository.DBTX, tierID int64, quantity int) error {
	t, ok := f.tiers[tierID]
	if !ok {
		return repository.ErrNotFound
	}
	t.HeldCount -= quantity
	return nil
}

func (f *fakeTierRepo) ConfirmSale(ctx context.Context, tx repository.DBTX, tierID int64, quantity int) error {
	t, ok := f.tiers[tierID]
	if !ok {
		return repository.ErrNotFound
	}
	t.HeldCount -= quantity
	t.SoldCount += quantity
	return nil
}

func (f *fakeTierRepo) RetireRefund(ctx context.Context, tx repository.DBTX, tierID int64, quantity int) error {
	t, ok := f.tiers[tierID]
	if !ok {
		return repository.ErrNotFound
	}
	t.SoldCount -= quantity
	t.RetiredCount += quantity
	return nil
}

func (f *fakeTierRepo) SumQuotaByEventID(ctx context.Context, tx repository.DBTX, eventID int64) (int, error) {
	total := 0
	for _, t := range f.tiers {
		if t.EventID == eventID {
			total += t.Quota
		}
	}
	return total, nil
}

type fakeAuditRepo struct{}

func (f *fakeAuditRepo) Create(ctx context.Context, tx repository.DBTX, log *model.AuditLog) error {
	return nil
}

type fakeOutboxRepo struct{}

func (f *fakeOutboxRepo) Create(ctx context.Context, tx repository.DBTX, o *model.Outbox) error {
	return nil
}
func (f *fakeOutboxRepo) ClaimBatch(ctx context.Context, tx repository.DBTX, leaseOwner string, leaseUntil time.Time, limit int) ([]model.Outbox, error) {
	return nil, nil
}
func (f *fakeOutboxRepo) MarkPublished(ctx context.Context, tx repository.DBTX, id string) error {
	return nil
}

type fakeStaffRepo struct{}

func (f *fakeStaffRepo) Create(ctx context.Context, tx repository.DBTX, s *model.EventStaff) error {
	return nil
}
func (f *fakeStaffRepo) GetByEventAndUser(ctx context.Context, tx repository.DBTX, eventID, userID int64) (*model.EventStaff, error) {
	return nil, repository.ErrNotFound
}
func (f *fakeStaffRepo) ListByEventID(ctx context.Context, tx repository.DBTX, eventID int64) ([]model.EventStaff, error) {
	return nil, nil
}
func (f *fakeStaffRepo) IsStaffAssigned(ctx context.Context, tx repository.DBTX, eventID, userID int64, staffRole string) (bool, error) {
	return false, nil
}

type noopTxMgr struct{}

func (m *noopTxMgr) WithTx(ctx context.Context, fn repository.TxFunc) error {
	return fn(ctx, nil)
}
func (m *noopTxMgr) WithTxOnce(ctx context.Context, fn repository.TxFunc) error {
	return fn(ctx, nil)
}

func setupEventTestApp() (*fiber.App, *fakeEventRepo, *fakeTierRepo) {
	app := fiber.New(fiber.Config{
		ErrorHandler: helper.ErrorHandler,
	})

	eventRepo := newFakeEventRepo()
	tierRepo := newFakeTierRepo()
	auditRepo := &fakeAuditRepo{}
	outboxRepo := &fakeOutboxRepo{}
	staffRepo := &fakeStaffRepo{}
	txMgr := &noopTxMgr{}

	uc := service.NewEventUsecase(txMgr, eventRepo, tierRepo, staffRepo, auditRepo, outboxRepo)
	svc := service.NewEventService(uc, []byte("test-secret-at-least-32-bytes-long!"))

	api := app.Group("/api/v1")
	svc.Register(api)

	return app, eventRepo, tierRepo
}

func TestEventService_ListPublished(t *testing.T) {
	app, eventRepo, _ := setupEventTestApp()

	// Tambahkan 1 event published dan 1 draft
	eventRepo.events[1] = &model.Event{
		ID:            1,
		Code:          "evt_published",
		Title:         "Konser Rock Ngawi",
		Status:        model.EventStatusPublished,
		SaleControl:   model.SaleControlEnabled,
		VenueCapacity: 1000,
		CreatedAt:     time.Now().UTC(),
	}
	eventRepo.events[2] = &model.Event{
		ID:            2,
		Code:          "evt_draft",
		Title:         "Konser Jazz Solo",
		Status:        model.EventStatusDraft,
		SaleControl:   model.SaleControlEnabled,
		VenueCapacity: 500,
		CreatedAt:     time.Now().UTC(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d; want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Data struct {
			Items []model.EventListResponse `json:"items"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)

	if len(body.Data.Items) != 1 {
		t.Errorf("got %d published events; want 1", len(body.Data.Items))
	}
	if len(body.Data.Items) > 0 && body.Data.Items[0].Code != "evt_published" {
		t.Errorf("got event code %s; want evt_published", body.Data.Items[0].Code)
	}
}

func TestEventService_CreateEvent_Auth(t *testing.T) {
	app, _, _ := setupEventTestApp()

	// Tanpa middleware Actor -> Unauthenticated (401)
	payload := `{"title":"Konser Test","venue":"Stadion Ngawi","venue_capacity":1000,"max_tickets_per_user":4,"sale_open_at":"2026-12-01T00:00:00Z","sale_close_at":"2026-12-01T12:00:00Z","starts_at":"2026-12-01T18:00:00Z","ends_at":"2026-12-01T22:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401 Unauthenticated", resp.StatusCode)
	}
}

func TestEventService_CreateEvent_WithActor(t *testing.T) {
	app := fiber.New(fiber.Config{
		ErrorHandler: helper.ErrorHandler,
	})

	eventRepo := newFakeEventRepo()
	tierRepo := newFakeTierRepo()
	uc := service.NewEventUsecase(&noopTxMgr{}, eventRepo, tierRepo, &fakeStaffRepo{}, &fakeAuditRepo{}, &fakeOutboxRepo{})
	svc := service.NewEventService(uc, []byte("test-secret-at-least-32-bytes-long!"))

	// Inject middleware actor palsu
	app.Use(func(c *fiber.Ctx) error {
		middleware.SetActor(c, middleware.Actor{
			UserID: 10,
			Role:   "organizer",
		})
		return c.Next()
	})

	api := app.Group("/api/v1")
	svc.Register(api)

	payload := `{"title":"Konser Test","venue":"Stadion Ngawi","venue_capacity":1000,"max_tickets_per_user":4,"sale_open_at":"2026-12-01T00:00:00Z","sale_close_at":"2026-12-01T12:00:00Z","starts_at":"2026-12-01T18:00:00Z","ends_at":"2026-12-01T22:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d; want 201 Created", resp.StatusCode)
	}
}

// newEventAppAs membuat app event dengan aktor palsu. Aktor nil berarti pengunjung anonim.
func newEventAppAs(actor *middleware.Actor, events ...*model.Event) (*fiber.App, *fakeEventRepo) {
	app := fiber.New(fiber.Config{ErrorHandler: helper.ErrorHandler})

	eventRepo := newFakeEventRepo()
	for _, e := range events {
		eventRepo.events[e.ID] = e
	}
	uc := service.NewEventUsecase(&noopTxMgr{}, eventRepo, newFakeTierRepo(), &fakeStaffRepo{}, &fakeAuditRepo{}, &fakeOutboxRepo{})
	svc := service.NewEventService(uc, []byte("test-secret-at-least-32-bytes-long!"))

	if actor != nil {
		app.Use(func(c *fiber.Ctx) error {
			middleware.SetActor(c, *actor)
			return c.Next()
		})
	}
	svc.Register(app.Group("/api/v1"))
	return app, eventRepo
}

func TestCancelEventByOtherOrganizerReturns404(t *testing.T) {
	event := &model.Event{ID: 1, OrganizerID: 10, Status: model.EventStatusPublished, Version: 1}
	app, _ := newEventAppAs(&middleware.Actor{UserID: 99, Role: model.RoleOrganizer}, event)

	resp, _ := postJSON(t, app, "/api/v1/events/1/cancel", `{"reason":"alasan uji"}`, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("organizer lain harus 404, dapat %d", resp.StatusCode)
	}
}

func TestCancelEventByAttendeeReturns403(t *testing.T) {
	event := &model.Event{ID: 1, OrganizerID: 10, Status: model.EventStatusPublished, Version: 1}
	app, _ := newEventAppAs(&middleware.Actor{UserID: 99, Role: model.RoleAttendee}, event)

	resp, _ := postJSON(t, app, "/api/v1/events/1/cancel", `{"reason":"alasan uji"}`, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("attendee harus 403, dapat %d", resp.StatusCode)
	}
}

func TestListTiersOfDraftEventHiddenFromPublic(t *testing.T) {
	event := &model.Event{ID: 1, OrganizerID: 10, Status: model.EventStatusDraft, Version: 1}
	app, _ := newEventAppAs(nil, event)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/1/tiers", nil)
	resp, _ := doRequest(t, app, req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("tier event draft harus 404 untuk publik, dapat %d", resp.StatusCode)
	}
}

func TestListTiersOfDraftEventVisibleToOwner(t *testing.T) {
	event := &model.Event{ID: 1, OrganizerID: 10, Status: model.EventStatusDraft, Version: 1}
	app, _ := newEventAppAs(&middleware.Actor{UserID: 10, Role: model.RoleOrganizer}, event)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/1/tiers", nil)
	resp, _ := doRequest(t, app, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pemilik harus bisa melihat tier draft, dapat %d", resp.StatusCode)
	}
}

func TestListTiersOfPublishedEventVisibleToPublic(t *testing.T) {
	event := &model.Event{ID: 1, OrganizerID: 10, Status: model.EventStatusPublished, Version: 1}
	app, _ := newEventAppAs(nil, event)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/1/tiers", nil)
	resp, _ := doRequest(t, app, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("event published harus terbuka untuk publik, dapat %d", resp.StatusCode)
	}
}
