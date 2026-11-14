package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/app/repository"
	"github.com/viesloxy/ngawimusical-backend/helper"
)

// EventUsecase menangani logika bisnis event dan tier.
type EventUsecase struct {
	txMgr      repository.TxManager
	eventRepo  repository.EventRepository
	tierRepo   repository.TicketTierRepository
	staffRepo  repository.EventStaffRepository
	auditRepo  repository.AuditLogRepository
	outboxRepo repository.OutboxRepository
}

// NewEventUsecase membuat instance baru EventUsecase.
func NewEventUsecase(
	txMgr repository.TxManager,
	eventRepo repository.EventRepository,
	tierRepo repository.TicketTierRepository,
	staffRepo repository.EventStaffRepository,
	auditRepo repository.AuditLogRepository,
	outboxRepo repository.OutboxRepository,
) *EventUsecase {
	return &EventUsecase{
		txMgr:      txMgr,
		eventRepo:  eventRepo,
		tierRepo:   tierRepo,
		staffRepo:  staffRepo,
		auditRepo:  auditRepo,
		outboxRepo: outboxRepo,
	}
}

// CreateEvent membuat event baru dalam status DRAFT.
func (uc *EventUsecase) CreateEvent(ctx context.Context, organizerID int64, req model.CreateEventRequest, requestID string) (*model.Event, error) {
	if err := helper.Validate(req); err != nil {
		return nil, err
	}

	saleOpenAt, err := time.Parse(time.RFC3339, req.SaleOpenAt)
	if err != nil {
		return nil, helper.NewAppError(422, "VALIDATION_FAILED", "Format sale_open_at harus RFC 3339.")
	}
	saleCloseAt, err := time.Parse(time.RFC3339, req.SaleCloseAt)
	if err != nil {
		return nil, helper.NewAppError(422, "VALIDATION_FAILED", "Format sale_close_at harus RFC 3339.")
	}
	startsAt, err := time.Parse(time.RFC3339, req.StartsAt)
	if err != nil {
		return nil, helper.NewAppError(422, "VALIDATION_FAILED", "Format starts_at harus RFC 3339.")
	}
	endsAt, err := time.Parse(time.RFC3339, req.EndsAt)
	if err != nil {
		return nil, helper.NewAppError(422, "VALIDATION_FAILED", "Format ends_at harus RFC 3339.")
	}

	if !model.ValidateTimeline(saleOpenAt, saleCloseAt, startsAt, endsAt) {
		return nil, helper.NewAppError(422, "VALIDATION_FAILED", "Urutan waktu harus sale_open_at < sale_close_at < starts_at < ends_at.")
	}

	code, err := generateEventCode()
	if err != nil {
		return nil, fmt.Errorf("generate event code: %w", err)
	}

	event := &model.Event{
		Code:              code,
		Title:             req.Title,
		Venue:             req.Venue,
		VenueCapacity:     req.VenueCapacity,
		Status:            model.EventStatusDraft,
		SaleControl:       model.SaleControlEnabled,
		QueueEpoch:        1,
		MaxTicketsPerUser: req.MaxTicketsPerUser,
		SaleOpenAt:        saleOpenAt.UTC(),
		SaleCloseAt:       saleCloseAt.UTC(),
		StartsAt:          startsAt.UTC(),
		EndsAt:            endsAt.UTC(),
		OrganizerID:       organizerID,
		Version:           1,
	}

	if err := uc.txMgr.WithTxOnce(ctx, func(ctx context.Context, tx repository.DBTX) error {
		if err := uc.eventRepo.Create(ctx, tx, event); err != nil {
			return fmt.Errorf("simpan event: %w", err)
		}

		audit := &model.AuditLog{
			ActorID:      &organizerID,
			Action:       "event.create",
			ResourceType: "event",
			ResourceID:   fmt.Sprintf("%d", event.ID),
			RequestID:    requestID,
		}
		return uc.auditRepo.Create(ctx, tx, audit)
	}); err != nil {
		return nil, err
	}

	return event, nil
}

// GetEvent mengambil detail event berdasarkan ID.
func (uc *EventUsecase) GetEvent(ctx context.Context, eventID int64) (*model.Event, error) {
	event, err := uc.eventRepo.GetByID(ctx, nil, eventID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, helper.NotFound()
		}
		return nil, fmt.Errorf("ambil event: %w", err)
	}
	return event, nil
}

// ListPublishedEvents mengambil daftar event yang dipublikasikan.
func (uc *EventUsecase) ListPublishedEvents(ctx context.Context, cursor *helper.Cursor, limit int) ([]model.Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	events, err := uc.eventRepo.ListPublished(ctx, nil, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("daftar event: %w", err)
	}
	return events, nil
}

// UpdateEvent memperbarui event draft dengan optimistic version check.
func (uc *EventUsecase) UpdateEvent(ctx context.Context, eventID int64, organizerID int64, req model.UpdateEventRequest, requestID string) (*model.Event, error) {
	if err := helper.Validate(req); err != nil {
		return nil, err
	}

	var updated *model.Event

	if err := uc.txMgr.WithTx(ctx, func(ctx context.Context, tx repository.DBTX) error {
		event, err := uc.eventRepo.GetByIDForUpdate(ctx, tx, eventID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return helper.NotFound()
			}
			return fmt.Errorf("ambil event: %w", err)
		}

		if event.OrganizerID != organizerID {
			return helper.NotFound()
		}

		if event.Status != model.EventStatusDraft {
			return helper.Conflict("EVENT_CONFIGURATION_FROZEN", "Event yang sudah dipublikasikan tidak bisa diubah.")
		}

		if event.Version != req.Version {
			return helper.Conflict("VERSION_CONFLICT", "Data sudah berubah, muat ulang dan coba lagi.")
		}

		if req.Title != nil {
			event.Title = *req.Title
		}
		if req.Venue != nil {
			event.Venue = *req.Venue
		}
		if req.VenueCapacity != nil {
			event.VenueCapacity = *req.VenueCapacity
		}

		saleOpenAt := event.SaleOpenAt
		saleCloseAt := event.SaleCloseAt
		startsAt := event.StartsAt
		endsAt := event.EndsAt

		if req.SaleOpenAt != nil {
			t, err := time.Parse(time.RFC3339, *req.SaleOpenAt)
			if err != nil {
				return helper.NewAppError(422, "VALIDATION_FAILED", "Format sale_open_at harus RFC 3339.")
			}
			saleOpenAt = t.UTC()
			event.SaleOpenAt = saleOpenAt
		}
		if req.SaleCloseAt != nil {
			t, err := time.Parse(time.RFC3339, *req.SaleCloseAt)
			if err != nil {
				return helper.NewAppError(422, "VALIDATION_FAILED", "Format sale_close_at harus RFC 3339.")
			}
			saleCloseAt = t.UTC()
			event.SaleCloseAt = saleCloseAt
		}
		if req.StartsAt != nil {
			t, err := time.Parse(time.RFC3339, *req.StartsAt)
			if err != nil {
				return helper.NewAppError(422, "VALIDATION_FAILED", "Format starts_at harus RFC 3339.")
			}
			startsAt = t.UTC()
			event.StartsAt = startsAt
		}
		if req.EndsAt != nil {
			t, err := time.Parse(time.RFC3339, *req.EndsAt)
			if err != nil {
				return helper.NewAppError(422, "VALIDATION_FAILED", "Format ends_at harus RFC 3339.")
			}
			endsAt = t.UTC()
			event.EndsAt = endsAt
		}

		if !model.ValidateTimeline(saleOpenAt, saleCloseAt, startsAt, endsAt) {
			return helper.NewAppError(422, "VALIDATION_FAILED", "Urutan waktu harus sale_open_at < sale_close_at < starts_at < ends_at.")
		}

		event.Version++
		if err := uc.eventRepo.Update(ctx, tx, event); err != nil {
			if errors.Is(err, repository.ErrVersionConflict) {
				return helper.Conflict("VERSION_CONFLICT", "Data sudah berubah, muat ulang dan coba lagi.")
			}
			return fmt.Errorf("simpan event: %w", err)
		}

		audit := &model.AuditLog{
			ActorID:      &organizerID,
			Action:       "event.update",
			ResourceType: "event",
			ResourceID:   fmt.Sprintf("%d", event.ID),
			RequestID:    requestID,
		}
		if err := uc.auditRepo.Create(ctx, tx, audit); err != nil {
			return fmt.Errorf("tulis audit: %w", err)
		}

		updated = event
		return nil
	}); err != nil {
		return nil, err
	}

	return updated, nil
}

// CreateTier membuat tier baru untuk event draft.
func (uc *EventUsecase) CreateTier(ctx context.Context, eventID int64, organizerID int64, req model.CreateTierRequest, requestID string) (*model.TicketTier, error) {
	if err := helper.Validate(req); err != nil {
		return nil, err
	}

	var tier *model.TicketTier

	if err := uc.txMgr.WithTxOnce(ctx, func(ctx context.Context, tx repository.DBTX) error {
		event, err := uc.eventRepo.GetByIDForUpdate(ctx, tx, eventID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return helper.NotFound()
			}
			return fmt.Errorf("ambil event: %w", err)
		}

		if event.OrganizerID != organizerID {
			return helper.NotFound()
		}

		if event.Status != model.EventStatusDraft {
			return helper.Conflict("EVENT_CONFIGURATION_FROZEN", "Tier hanya bisa ditambah pada event draft.")
		}

		totalQuota, err := uc.tierRepo.SumQuotaByEventID(ctx, tx, eventID)
		if err != nil {
			return fmt.Errorf("hitung kuota: %w", err)
		}
		if totalQuota+req.Quota > event.VenueCapacity {
			return helper.NewAppError(422, "VALIDATION_FAILED", "Total kuota tier melebihi kapasitas venue.")
		}

		tier = &model.TicketTier{
			EventID:     eventID,
			Code:        req.Code,
			Name:        req.Name,
			PriceAmount: req.PriceAmount,
			FeeAmount:   req.FeeAmount,
			Quota:       req.Quota,
			Version:     1,
		}

		if err := uc.tierRepo.Create(ctx, tx, tier); err != nil {
			if errors.Is(err, repository.ErrConflict) {
				return helper.Conflict("CONFLICT", "Kode tier sudah dipakai pada event ini.")
			}
			return fmt.Errorf("simpan tier: %w", err)
		}

		audit := &model.AuditLog{
			ActorID:      &organizerID,
			Action:       "tier.create",
			ResourceType: "ticket_tier",
			ResourceID:   fmt.Sprintf("%d", tier.ID),
			RequestID:    requestID,
		}
		return uc.auditRepo.Create(ctx, tx, audit)
	}); err != nil {
		return nil, err
	}

	return tier, nil
}

// UpdateTier memperbarui tier event draft.
func (uc *EventUsecase) UpdateTier(ctx context.Context, tierID int64, organizerID int64, req model.UpdateTierRequest, requestID string) (*model.TicketTier, error) {
	if err := helper.Validate(req); err != nil {
		return nil, err
	}

	var updated *model.TicketTier

	if err := uc.txMgr.WithTx(ctx, func(ctx context.Context, tx repository.DBTX) error {
		tier, err := uc.tierRepo.GetByIDForUpdate(ctx, tx, tierID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return helper.NotFound()
			}
			return fmt.Errorf("ambil tier: %w", err)
		}

		event, err := uc.eventRepo.GetByIDForUpdate(ctx, tx, tier.EventID)
		if err != nil {
			return fmt.Errorf("ambil event: %w", err)
		}

		if event.OrganizerID != organizerID {
			return helper.NotFound()
		}

		if event.Status != model.EventStatusDraft {
			return helper.Conflict("EVENT_CONFIGURATION_FROZEN", "Tier hanya bisa diubah pada event draft.")
		}

		if tier.Version != req.Version {
			return helper.Conflict("VERSION_CONFLICT", "Data sudah berubah, muat ulang dan coba lagi.")
		}

		if req.Name != nil {
			tier.Name = *req.Name
		}
		if req.PriceAmount != nil {
			tier.PriceAmount = *req.PriceAmount
		}
		if req.FeeAmount != nil {
			tier.FeeAmount = *req.FeeAmount
		}
		if req.Quota != nil {
			oldQuota := tier.Quota
			newQuota := *req.Quota

			totalQuota, err := uc.tierRepo.SumQuotaByEventID(ctx, tx, tier.EventID)
			if err != nil {
				return fmt.Errorf("hitung kuota: %w", err)
			}
			if totalQuota-oldQuota+newQuota > event.VenueCapacity {
				return helper.NewAppError(422, "VALIDATION_FAILED", "Total kuota tier melebihi kapasitas venue.")
			}
			tier.Quota = newQuota
		}

		tier.Version++
		if err := uc.tierRepo.Update(ctx, tx, tier); err != nil {
			if errors.Is(err, repository.ErrVersionConflict) {
				return helper.Conflict("VERSION_CONFLICT", "Data sudah berubah, muat ulang dan coba lagi.")
			}
			return fmt.Errorf("simpan tier: %w", err)
		}

		audit := &model.AuditLog{
			ActorID:      &organizerID,
			Action:       "tier.update",
			ResourceType: "ticket_tier",
			ResourceID:   fmt.Sprintf("%d", tier.ID),
			RequestID:    requestID,
		}
		if err := uc.auditRepo.Create(ctx, tx, audit); err != nil {
			return fmt.Errorf("tulis audit: %w", err)
		}

		updated = tier
		return nil
	}); err != nil {
		return nil, err
	}

	return updated, nil
}

// ListTiers mengambil daftar tier untuk satu event.
func (uc *EventUsecase) ListTiers(ctx context.Context, eventID int64) ([]model.TicketTier, error) {
	tiers, err := uc.tierRepo.ListByEventID(ctx, nil, eventID)
	if err != nil {
		return nil, fmt.Errorf("daftar tier: %w", err)
	}
	return tiers, nil
}

// PublishEvent mempublikasikan event dari status DRAFT.
func (uc *EventUsecase) PublishEvent(ctx context.Context, eventID int64, organizerID int64, requestID string) error {
	return uc.txMgr.WithTxOnce(ctx, func(ctx context.Context, tx repository.DBTX) error {
		event, err := uc.eventRepo.GetByIDForUpdate(ctx, tx, eventID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return helper.NotFound()
			}
			return fmt.Errorf("ambil event: %w", err)
		}

		if event.OrganizerID != organizerID {
			return helper.NotFound()
		}

		if !event.CanTransitionTo(model.EventStatusPublished) {
			return helper.Conflict("CONFLICT", "Event tidak bisa dipublikasikan dari status "+event.Status+".")
		}

		tiers, err := uc.tierRepo.ListByEventID(ctx, tx, eventID)
		if err != nil {
			return fmt.Errorf("daftar tier: %w", err)
		}

		if len(tiers) == 0 {
			return helper.NewAppError(422, "VALIDATION_FAILED", "Event harus memiliki minimal satu tier.")
		}

		totalQuota := 0
		for _, t := range tiers {
			if t.PriceAmount <= 0 {
				return helper.NewAppError(422, "VALIDATION_FAILED", "Setiap tier harus memiliki harga lebih dari 0.")
			}
			totalQuota += t.Quota
		}

		if totalQuota > event.VenueCapacity {
			return helper.NewAppError(422, "VALIDATION_FAILED", "Total kuota tier melebihi kapasitas venue.")
		}

		event.Status = model.EventStatusPublished
		event.Version++
		if err := uc.eventRepo.Update(ctx, tx, event); err != nil {
			return fmt.Errorf("simpan event: %w", err)
		}

		audit := &model.AuditLog{
			ActorID:      &organizerID,
			Action:       "event.publish",
			ResourceType: "event",
			ResourceID:   fmt.Sprintf("%d", event.ID),
			RequestID:    requestID,
		}
		return uc.auditRepo.Create(ctx, tx, audit)
	})
}

// PauseSale menghentikan penjualan sementara.
func (uc *EventUsecase) PauseSale(ctx context.Context, eventID int64, organizerID int64, requestID string) error {
	return uc.transitionSaleControl(ctx, eventID, organizerID, model.SaleControlPaused, "event.pause_sale", requestID)
}

// ResumeSale melanjutkan penjualan.
func (uc *EventUsecase) ResumeSale(ctx context.Context, eventID int64, organizerID int64, requestID string) error {
	return uc.transitionSaleControl(ctx, eventID, organizerID, model.SaleControlEnabled, "event.resume_sale", requestID)
}

// CloseSale menutup penjualan secara permanen.
func (uc *EventUsecase) CloseSale(ctx context.Context, eventID int64, organizerID int64, requestID string) error {
	return uc.transitionSaleControl(ctx, eventID, organizerID, model.SaleControlClosed, "event.close_sale", requestID)
}

func (uc *EventUsecase) transitionSaleControl(ctx context.Context, eventID int64, organizerID int64, target string, action string, requestID string) error {
	return uc.txMgr.WithTxOnce(ctx, func(ctx context.Context, tx repository.DBTX) error {
		event, err := uc.eventRepo.GetByIDForUpdate(ctx, tx, eventID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return helper.NotFound()
			}
			return fmt.Errorf("ambil event: %w", err)
		}

		if event.OrganizerID != organizerID {
			return helper.NotFound()
		}

		if !event.IsPublished() {
			return helper.Conflict("CONFLICT", "Sale control hanya bisa diubah pada event yang sudah dipublikasikan.")
		}

		if !event.CanTransitionSaleTo(target) {
			return helper.Conflict("CONFLICT", fmt.Sprintf("Transisi sale control dari %s ke %s tidak diizinkan.", event.SaleControl, target))
		}

		event.SaleControl = target
		event.Version++
		if err := uc.eventRepo.Update(ctx, tx, event); err != nil {
			return fmt.Errorf("simpan event: %w", err)
		}

		audit := &model.AuditLog{
			ActorID:      &organizerID,
			Action:       action,
			ResourceType: "event",
			ResourceID:   fmt.Sprintf("%d", event.ID),
			RequestID:    requestID,
		}
		return uc.auditRepo.Create(ctx, tx, audit)
	})
}

// CancelEvent membatalkan event dan memulai proses pembatalan booking.
func (uc *EventUsecase) CancelEvent(ctx context.Context, eventID int64, actorID int64, reason string, requestID string) error {
	return uc.txMgr.WithTxOnce(ctx, func(ctx context.Context, tx repository.DBTX) error {
		event, err := uc.eventRepo.GetByIDForUpdate(ctx, tx, eventID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return helper.NotFound()
			}
			return fmt.Errorf("ambil event: %w", err)
		}

		if !event.CanTransitionTo(model.EventStatusCancelled) {
			return helper.Conflict("CONFLICT", "Event tidak bisa dibatalkan dari status "+event.Status+".")
		}

		event.Status = model.EventStatusCancelled
		event.SaleControl = model.SaleControlClosed
		event.Version++
		if err := uc.eventRepo.Update(ctx, tx, event); err != nil {
			return fmt.Errorf("simpan event: %w", err)
		}

		payload, _ := json.Marshal(map[string]any{
			"event_id": event.ID,
			"reason":   reason,
		})
		outbox := &model.Outbox{
			ID:               uuid.NewString(),
			AggregateID:      fmt.Sprintf("event:%d", event.ID),
			AggregateVersion: event.Version,
			EventType:        model.OutboxEventEventCancelled,
			SchemaVersion:    1,
			Payload:          payload,
		}
		if err := uc.outboxRepo.Create(ctx, tx, outbox); err != nil {
			return fmt.Errorf("tulis outbox: %w", err)
		}

		audit := &model.AuditLog{
			ActorID:      &actorID,
			Action:       "event.cancel",
			ResourceType: "event",
			ResourceID:   fmt.Sprintf("%d", event.ID),
			RequestID:    requestID,
			Reason:       &reason,
		}
		return uc.auditRepo.Create(ctx, tx, audit)
	})
}

// CompleteEvent menandai event selesai setelah waktu ends_at.
func (uc *EventUsecase) CompleteEvent(ctx context.Context, eventID int64, organizerID int64, requestID string) error {
	return uc.txMgr.WithTxOnce(ctx, func(ctx context.Context, tx repository.DBTX) error {
		event, err := uc.eventRepo.GetByIDForUpdate(ctx, tx, eventID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return helper.NotFound()
			}
			return fmt.Errorf("ambil event: %w", err)
		}

		if event.OrganizerID != organizerID {
			return helper.NotFound()
		}

		if !event.CanTransitionTo(model.EventStatusCompleted) {
			return helper.Conflict("CONFLICT", "Event tidak bisa ditandai selesai dari status "+event.Status+".")
		}

		now := time.Now().UTC()
		if now.Before(event.EndsAt) {
			return helper.Conflict("CONFLICT", "Event belum selesai. Tunggu sampai ends_at terlewati.")
		}

		event.Status = model.EventStatusCompleted
		event.SaleControl = model.SaleControlClosed
		event.Version++
		if err := uc.eventRepo.Update(ctx, tx, event); err != nil {
			return fmt.Errorf("simpan event: %w", err)
		}

		audit := &model.AuditLog{
			ActorID:      &organizerID,
			Action:       "event.complete",
			ResourceType: "event",
			ResourceID:   fmt.Sprintf("%d", event.ID),
			RequestID:    requestID,
		}
		return uc.auditRepo.Create(ctx, tx, audit)
	})
}

// GetEventForOwner mengambil detail event yang dimiliki organizer.
func (uc *EventUsecase) GetEventForOwner(ctx context.Context, eventID int64, organizerID int64) (*model.Event, error) {
	event, err := uc.eventRepo.GetByID(ctx, nil, eventID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, helper.NotFound()
		}
		return nil, fmt.Errorf("ambil event: %w", err)
	}
	if event.OrganizerID != organizerID {
		return nil, helper.NotFound()
	}
	return event, nil
}

func generateEventCode() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "evt_" + hex.EncodeToString(bytes), nil
}

// ToEventListResponse mengonversi entity Event ke DTO daftar.
func ToEventListResponse(e model.Event) model.EventListResponse {
	return model.EventListResponse{
		ID:            e.ID,
		Code:          e.Code,
		Title:         e.Title,
		Venue:         e.Venue,
		VenueCapacity: e.VenueCapacity,
		Status:        e.Status,
		SaleOpenAt:    e.SaleOpenAt,
		SaleCloseAt:   e.SaleCloseAt,
		StartsAt:      e.StartsAt,
		EndsAt:        e.EndsAt,
		CreatedAt:     e.CreatedAt,
	}
}

// ToTierResponse mengonversi entity TicketTier ke DTO respons.
func ToTierResponse(t model.TicketTier) model.TierResponse {
	available := t.AvailableCount()
	return model.TierResponse{
		ID:             t.ID,
		EventID:        t.EventID,
		Code:           t.Code,
		Name:           t.Name,
		PriceAmount:    t.PriceAmount,
		FeeAmount:      t.FeeAmount,
		Quota:          t.Quota,
		AvailableCount: available,
		Status:         model.TierAvailabilityStatus(available),
		CreatedAt:      t.CreatedAt,
	}
}

// suppress unused import
var _ = slog.Info
