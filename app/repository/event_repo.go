package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/helper"
)

// EventRepository menyediakan akses data untuk entitas event.
type EventRepository interface {
	Create(ctx context.Context, tx DBTX, event *model.Event) error
	GetByID(ctx context.Context, tx DBTX, id int64) (*model.Event, error)
	GetByCode(ctx context.Context, tx DBTX, code string) (*model.Event, error)
	GetByIDForShare(ctx context.Context, tx DBTX, id int64) (*model.Event, error)
	GetByIDForUpdate(ctx context.Context, tx DBTX, id int64) (*model.Event, error)
	ListPublished(ctx context.Context, tx DBTX, cursor *helper.Cursor, limit int) ([]model.Event, error)
	Update(ctx context.Context, tx DBTX, event *model.Event) error
	UpdateStatus(ctx context.Context, tx DBTX, id int64, status string, version int) error
	UpdateSaleControl(ctx context.Context, tx DBTX, id int64, saleControl string, version int) error
}

// TicketTierRepository menyediakan akses data untuk entitas tier.
type TicketTierRepository interface {
	Create(ctx context.Context, tx DBTX, tier *model.TicketTier) error
	GetByID(ctx context.Context, tx DBTX, id int64) (*model.TicketTier, error)
	GetByIDForUpdate(ctx context.Context, tx DBTX, id int64) (*model.TicketTier, error)
	ListByEventID(ctx context.Context, tx DBTX, eventID int64) ([]model.TicketTier, error)
	Update(ctx context.Context, tx DBTX, tier *model.TicketTier) error
	HoldStock(ctx context.Context, tx DBTX, tierID int64, eventID int64, quantity int) (*model.TicketTier, error)
	ReleaseHeldStock(ctx context.Context, tx DBTX, tierID int64, quantity int) error
	ConfirmSale(ctx context.Context, tx DBTX, tierID int64, quantity int) error
	RetireRefund(ctx context.Context, tx DBTX, tierID int64, quantity int) error
	SumQuotaByEventID(ctx context.Context, tx DBTX, eventID int64) (int, error)
}

// EventStaffRepository menyediakan akses data untuk penugasan petugas.
type EventStaffRepository interface {
	Create(ctx context.Context, tx DBTX, staff *model.EventStaff) error
	GetByEventAndUser(ctx context.Context, tx DBTX, eventID int64, userID int64) (*model.EventStaff, error)
	ListByEventID(ctx context.Context, tx DBTX, eventID int64) ([]model.EventStaff, error)
	IsStaffAssigned(ctx context.Context, tx DBTX, eventID int64, userID int64, staffRole string) (bool, error)
}

// AuditLogRepository menyediakan akses data untuk audit log.
type AuditLogRepository interface {
	Create(ctx context.Context, tx DBTX, log *model.AuditLog) error
}

// OutboxRepository menyediakan akses data untuk outbox.
type OutboxRepository interface {
	Create(ctx context.Context, tx DBTX, outbox *model.Outbox) error
	ClaimBatch(ctx context.Context, tx DBTX, leaseOwner string, leaseUntil time.Time, limit int) ([]model.Outbox, error)
	MarkPublished(ctx context.Context, tx DBTX, id string) error
}

// InventoryMovementRepository menyediakan akses data untuk ledger perubahan stok.
type InventoryMovementRepository interface {
	Create(ctx context.Context, tx DBTX, movement *model.InventoryMovement) error
}

// PostgresEventRepository implementasi PostgreSQL untuk EventRepository.
type PostgresEventRepository struct {
	db DBTX
}

func NewPostgresEventRepository(db DBTX) *PostgresEventRepository {
	return &PostgresEventRepository{db: db}
}

func (r *PostgresEventRepository) executor(tx DBTX) DBTX {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *PostgresEventRepository) Create(ctx context.Context, tx DBTX, event *model.Event) error {
	query := `
		INSERT INTO events (code, title, venue, venue_capacity, status, sale_control, queue_epoch, max_tickets_per_user,
		                    sale_open_at, sale_close_at, starts_at, ends_at, organizer_id, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, clock_timestamp(), clock_timestamp())
		RETURNING id, created_at, updated_at`

	err := r.executor(tx).QueryRow(ctx, query,
		event.Code, event.Title, event.Venue, event.VenueCapacity, event.Status, event.SaleControl,
		event.QueueEpoch, event.MaxTicketsPerUser, event.SaleOpenAt, event.SaleCloseAt,
		event.StartsAt, event.EndsAt, event.OrganizerID, event.Version,
	).Scan(&event.ID, &event.CreatedAt, &event.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrConflict
		}
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (r *PostgresEventRepository) GetByID(ctx context.Context, tx DBTX, id int64) (*model.Event, error) {
	query := `
		SELECT id, code, title, venue, venue_capacity, status, sale_control, queue_epoch, max_tickets_per_user,
		       sale_open_at, sale_close_at, starts_at, ends_at, organizer_id, version, created_at, updated_at
		FROM events WHERE id = $1`

	var event model.Event
	err := r.executor(tx).QueryRow(ctx, query, id).Scan(
		&event.ID, &event.Code, &event.Title, &event.Venue, &event.VenueCapacity, &event.Status,
		&event.SaleControl, &event.QueueEpoch, &event.MaxTicketsPerUser, &event.SaleOpenAt,
		&event.SaleCloseAt, &event.StartsAt, &event.EndsAt, &event.OrganizerID, &event.Version,
		&event.CreatedAt, &event.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get event by id: %w", err)
	}
	return &event, nil
}

func (r *PostgresEventRepository) GetByCode(ctx context.Context, tx DBTX, code string) (*model.Event, error) {
	query := `
		SELECT id, code, title, venue, venue_capacity, status, sale_control, queue_epoch, max_tickets_per_user,
		       sale_open_at, sale_close_at, starts_at, ends_at, organizer_id, version, created_at, updated_at
		FROM events WHERE code = $1`

	var event model.Event
	err := r.executor(tx).QueryRow(ctx, query, code).Scan(
		&event.ID, &event.Code, &event.Title, &event.Venue, &event.VenueCapacity, &event.Status,
		&event.SaleControl, &event.QueueEpoch, &event.MaxTicketsPerUser, &event.SaleOpenAt,
		&event.SaleCloseAt, &event.StartsAt, &event.EndsAt, &event.OrganizerID, &event.Version,
		&event.CreatedAt, &event.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get event by code: %w", err)
	}
	return &event, nil
}

func (r *PostgresEventRepository) GetByIDForShare(ctx context.Context, tx DBTX, id int64) (*model.Event, error) {
	query := `
		SELECT id, code, title, venue, venue_capacity, status, sale_control, queue_epoch, max_tickets_per_user,
		       sale_open_at, sale_close_at, starts_at, ends_at, organizer_id, version, created_at, updated_at
		FROM events WHERE id = $1 FOR SHARE`

	var event model.Event
	err := r.executor(tx).QueryRow(ctx, query, id).Scan(
		&event.ID, &event.Code, &event.Title, &event.Venue, &event.VenueCapacity, &event.Status,
		&event.SaleControl, &event.QueueEpoch, &event.MaxTicketsPerUser, &event.SaleOpenAt,
		&event.SaleCloseAt, &event.StartsAt, &event.EndsAt, &event.OrganizerID, &event.Version,
		&event.CreatedAt, &event.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get event for share: %w", err)
	}
	return &event, nil
}

func (r *PostgresEventRepository) GetByIDForUpdate(ctx context.Context, tx DBTX, id int64) (*model.Event, error) {
	query := `
		SELECT id, code, title, venue, venue_capacity, status, sale_control, queue_epoch, max_tickets_per_user,
		       sale_open_at, sale_close_at, starts_at, ends_at, organizer_id, version, created_at, updated_at
		FROM events WHERE id = $1 FOR UPDATE`

	var event model.Event
	err := r.executor(tx).QueryRow(ctx, query, id).Scan(
		&event.ID, &event.Code, &event.Title, &event.Venue, &event.VenueCapacity, &event.Status,
		&event.SaleControl, &event.QueueEpoch, &event.MaxTicketsPerUser, &event.SaleOpenAt,
		&event.SaleCloseAt, &event.StartsAt, &event.EndsAt, &event.OrganizerID, &event.Version,
		&event.CreatedAt, &event.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get event for update: %w", err)
	}
	return &event, nil
}

func (r *PostgresEventRepository) ListPublished(ctx context.Context, tx DBTX, cursor *helper.Cursor, limit int) ([]model.Event, error) {
	var rows pgx.Rows
	var err error

	if cursor == nil {
		query := `
			SELECT id, code, title, venue, venue_capacity, status, sale_control, queue_epoch, max_tickets_per_user,
			       sale_open_at, sale_close_at, starts_at, ends_at, organizer_id, version, created_at, updated_at
			FROM events
			WHERE status = 'PUBLISHED'
			ORDER BY created_at DESC, id DESC
			LIMIT $1`
		rows, err = r.executor(tx).Query(ctx, query, limit)
	} else {
		query := `
			SELECT id, code, title, venue, venue_capacity, status, sale_control, queue_epoch, max_tickets_per_user,
			       sale_open_at, sale_close_at, starts_at, ends_at, organizer_id, version, created_at, updated_at
			FROM events
			WHERE status = 'PUBLISHED'
			  AND (created_at, id) < ($1, $2)
			ORDER BY created_at DESC, id DESC
			LIMIT $3`
		rows, err = r.executor(tx).Query(ctx, query, cursor.CreatedAt, cursor.ID, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("list published events: %w", err)
	}
	defer rows.Close()

	var events []model.Event
	for rows.Next() {
		var event model.Event
		if err := rows.Scan(
			&event.ID, &event.Code, &event.Title, &event.Venue, &event.VenueCapacity, &event.Status,
			&event.SaleControl, &event.QueueEpoch, &event.MaxTicketsPerUser, &event.SaleOpenAt,
			&event.SaleCloseAt, &event.StartsAt, &event.EndsAt, &event.OrganizerID, &event.Version,
			&event.CreatedAt, &event.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan event row: %w", err)
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

func (r *PostgresEventRepository) Update(ctx context.Context, tx DBTX, event *model.Event) error {
	query := `
		UPDATE events
		SET title = $1, venue = $2, venue_capacity = $3, status = $4, sale_control = $5,
		    queue_epoch = $6, max_tickets_per_user = $7, sale_open_at = $8, sale_close_at = $9,
		    starts_at = $10, ends_at = $11, version = version + 1, updated_at = clock_timestamp()
		WHERE id = $12 AND version = $13`

	cmd, err := r.executor(tx).Exec(ctx, query,
		event.Title, event.Venue, event.VenueCapacity, event.Status, event.SaleControl,
		event.QueueEpoch, event.MaxTicketsPerUser, event.SaleOpenAt, event.SaleCloseAt,
		event.StartsAt, event.EndsAt, event.ID, event.Version-1,
	)
	if err != nil {
		return fmt.Errorf("update event: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (r *PostgresEventRepository) UpdateStatus(ctx context.Context, tx DBTX, id int64, status string, version int) error {
	query := `
		UPDATE events
		SET status = $1, version = version + 1, updated_at = clock_timestamp()
		WHERE id = $2 AND version = $3`

	cmd, err := r.executor(tx).Exec(ctx, query, status, id, version)
	if err != nil {
		return fmt.Errorf("update event status: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (r *PostgresEventRepository) UpdateSaleControl(ctx context.Context, tx DBTX, id int64, saleControl string, version int) error {
	query := `
		UPDATE events
		SET sale_control = $1, version = version + 1, updated_at = clock_timestamp()
		WHERE id = $2 AND version = $3`

	cmd, err := r.executor(tx).Exec(ctx, query, saleControl, id, version)
	if err != nil {
		return fmt.Errorf("update event sale_control: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

// PostgresTicketTierRepository implementasi PostgreSQL untuk TicketTierRepository.
type PostgresTicketTierRepository struct {
	db DBTX
}

func NewPostgresTicketTierRepository(db DBTX) *PostgresTicketTierRepository {
	return &PostgresTicketTierRepository{db: db}
}

func (r *PostgresTicketTierRepository) executor(tx DBTX) DBTX {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *PostgresTicketTierRepository) Create(ctx context.Context, tx DBTX, tier *model.TicketTier) error {
	query := `
		INSERT INTO ticket_tiers (event_id, code, name, price_amount, fee_amount, quota, held_count, sold_count, retired_count, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 0, 0, 0, 1, clock_timestamp(), clock_timestamp())
		RETURNING id, created_at, updated_at`

	err := r.executor(tx).QueryRow(ctx, query,
		tier.EventID, tier.Code, tier.Name, tier.PriceAmount, tier.FeeAmount, tier.Quota,
	).Scan(&tier.ID, &tier.CreatedAt, &tier.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrConflict
		}
		return fmt.Errorf("insert tier: %w", err)
	}
	return nil
}

func (r *PostgresTicketTierRepository) GetByID(ctx context.Context, tx DBTX, id int64) (*model.TicketTier, error) {
	query := `
		SELECT id, event_id, code, name, price_amount, fee_amount, quota, held_count, sold_count, retired_count, version, created_at, updated_at
		FROM ticket_tiers WHERE id = $1`

	var tier model.TicketTier
	err := r.executor(tx).QueryRow(ctx, query, id).Scan(
		&tier.ID, &tier.EventID, &tier.Code, &tier.Name, &tier.PriceAmount, &tier.FeeAmount,
		&tier.Quota, &tier.HeldCount, &tier.SoldCount, &tier.RetiredCount, &tier.Version,
		&tier.CreatedAt, &tier.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get tier by id: %w", err)
	}
	return &tier, nil
}

func (r *PostgresTicketTierRepository) GetByIDForUpdate(ctx context.Context, tx DBTX, id int64) (*model.TicketTier, error) {
	query := `
		SELECT id, event_id, code, name, price_amount, fee_amount, quota, held_count, sold_count, retired_count, version, created_at, updated_at
		FROM ticket_tiers WHERE id = $1 FOR UPDATE`

	var tier model.TicketTier
	err := r.executor(tx).QueryRow(ctx, query, id).Scan(
		&tier.ID, &tier.EventID, &tier.Code, &tier.Name, &tier.PriceAmount, &tier.FeeAmount,
		&tier.Quota, &tier.HeldCount, &tier.SoldCount, &tier.RetiredCount, &tier.Version,
		&tier.CreatedAt, &tier.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get tier for update: %w", err)
	}
	return &tier, nil
}

func (r *PostgresTicketTierRepository) ListByEventID(ctx context.Context, tx DBTX, eventID int64) ([]model.TicketTier, error) {
	query := `
		SELECT id, event_id, code, name, price_amount, fee_amount, quota, held_count, sold_count, retired_count, version, created_at, updated_at
		FROM ticket_tiers WHERE event_id = $1 ORDER BY price_amount ASC, id ASC`

	rows, err := r.executor(tx).Query(ctx, query, eventID)
	if err != nil {
		return nil, fmt.Errorf("list tiers: %w", err)
	}
	defer rows.Close()

	var tiers []model.TicketTier
	for rows.Next() {
		var tier model.TicketTier
		if err := rows.Scan(
			&tier.ID, &tier.EventID, &tier.Code, &tier.Name, &tier.PriceAmount, &tier.FeeAmount,
			&tier.Quota, &tier.HeldCount, &tier.SoldCount, &tier.RetiredCount, &tier.Version,
			&tier.CreatedAt, &tier.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan tier: %w", err)
		}
		tiers = append(tiers, tier)
	}
	return tiers, rows.Err()
}

func (r *PostgresTicketTierRepository) Update(ctx context.Context, tx DBTX, tier *model.TicketTier) error {
	query := `
		UPDATE ticket_tiers
		SET name = $1, price_amount = $2, fee_amount = $3, quota = $4, version = version + 1, updated_at = clock_timestamp()
		WHERE id = $5 AND version = $6`

	cmd, err := r.executor(tx).Exec(ctx, query,
		tier.Name, tier.PriceAmount, tier.FeeAmount, tier.Quota, tier.ID, tier.Version-1,
	)
	if err != nil {
		return fmt.Errorf("update tier: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (r *PostgresTicketTierRepository) HoldStock(ctx context.Context, tx DBTX, tierID int64, eventID int64, quantity int) (*model.TicketTier, error) {
	query := `
		UPDATE ticket_tiers
		SET held_count = held_count + $1,
		    version = version + 1,
		    updated_at = clock_timestamp()
		WHERE id = $2 AND event_id = $3
		  AND quota - held_count - sold_count - retired_count >= $1
		RETURNING id, event_id, code, name, price_amount, fee_amount, quota, held_count, sold_count, retired_count, version, created_at, updated_at`

	var tier model.TicketTier
	err := r.executor(tx).QueryRow(ctx, query, quantity, tierID, eventID).Scan(
		&tier.ID, &tier.EventID, &tier.Code, &tier.Name, &tier.PriceAmount, &tier.FeeAmount,
		&tier.Quota, &tier.HeldCount, &tier.SoldCount, &tier.RetiredCount, &tier.Version,
		&tier.CreatedAt, &tier.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSoldOut
		}
		return nil, fmt.Errorf("hold stock: %w", err)
	}
	return &tier, nil
}

func (r *PostgresTicketTierRepository) ReleaseHeldStock(ctx context.Context, tx DBTX, tierID int64, quantity int) error {
	query := `
		UPDATE ticket_tiers
		SET held_count = held_count - $1,
		    version = version + 1,
		    updated_at = clock_timestamp()
		WHERE id = $2 AND held_count >= $1`

	cmd, err := r.executor(tx).Exec(ctx, query, quantity, tierID)
	if err != nil {
		return fmt.Errorf("release held stock: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("gagal melepaskan stok held pada tier %d", tierID)
	}
	return nil
}

func (r *PostgresTicketTierRepository) ConfirmSale(ctx context.Context, tx DBTX, tierID int64, quantity int) error {
	query := `
		UPDATE ticket_tiers
		SET held_count = held_count - $1,
		    sold_count = sold_count + $1,
		    version = version + 1,
		    updated_at = clock_timestamp()
		WHERE id = $2 AND held_count >= $1`

	cmd, err := r.executor(tx).Exec(ctx, query, quantity, tierID)
	if err != nil {
		return fmt.Errorf("confirm sale tier: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("gagal konfirmasi sale pada tier %d", tierID)
	}
	return nil
}

func (r *PostgresTicketTierRepository) RetireRefund(ctx context.Context, tx DBTX, tierID int64, quantity int) error {
	query := `
		UPDATE ticket_tiers
		SET sold_count = sold_count - $1,
		    retired_count = retired_count + $1,
		    version = version + 1,
		    updated_at = clock_timestamp()
		WHERE id = $2 AND sold_count >= $1`

	cmd, err := r.executor(tx).Exec(ctx, query, quantity, tierID)
	if err != nil {
		return fmt.Errorf("retire refund tier: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("gagal retire refund pada tier %d", tierID)
	}
	return nil
}

func (r *PostgresTicketTierRepository) SumQuotaByEventID(ctx context.Context, tx DBTX, eventID int64) (int, error) {
	query := `SELECT COALESCE(SUM(quota), 0) FROM ticket_tiers WHERE event_id = $1`
	var total int
	err := r.executor(tx).QueryRow(ctx, query, eventID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("sum quota: %w", err)
	}
	return total, nil
}

// PostgresAuditLogRepository implementasi PostgreSQL untuk AuditLogRepository.
type PostgresAuditLogRepository struct {
	db DBTX
}

func NewPostgresAuditLogRepository(db DBTX) *PostgresAuditLogRepository {
	return &PostgresAuditLogRepository{db: db}
}

func (r *PostgresAuditLogRepository) Create(ctx context.Context, tx DBTX, log *model.AuditLog) error {
	exec := tx
	if exec == nil {
		exec = r.db
	}
	query := `
		INSERT INTO audit_logs (actor_id, action, resource_type, resource_id, request_id, changes, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, clock_timestamp())
		RETURNING id, created_at`

	return exec.QueryRow(ctx, query,
		log.ActorID, log.Action, log.ResourceType, log.ResourceID, log.RequestID, log.Changes, log.Reason,
	).Scan(&log.ID, &log.CreatedAt)
}

// PostgresOutboxRepository implementasi PostgreSQL untuk OutboxRepository.
type PostgresOutboxRepository struct {
	db DBTX
}

func NewPostgresOutboxRepository(db DBTX) *PostgresOutboxRepository {
	return &PostgresOutboxRepository{db: db}
}

func (r *PostgresOutboxRepository) Create(ctx context.Context, tx DBTX, outbox *model.Outbox) error {
	exec := tx
	if exec == nil {
		exec = r.db
	}
	query := `
		INSERT INTO outbox (id, aggregate_id, aggregate_version, event_type, schema_version, payload, created_at, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5, $6, clock_timestamp(), clock_timestamp())
		RETURNING created_at`

	return exec.QueryRow(ctx, query,
		outbox.ID, outbox.AggregateID, outbox.AggregateVersion, outbox.EventType, outbox.SchemaVersion, outbox.Payload,
	).Scan(&outbox.CreatedAt)
}

func (r *PostgresOutboxRepository) ClaimBatch(ctx context.Context, tx DBTX, leaseOwner string, leaseUntil time.Time, limit int) ([]model.Outbox, error) {
	exec := tx
	if exec == nil {
		exec = r.db
	}
	query := `
		UPDATE outbox
		SET lease_owner = $1, lease_until = $2
		WHERE id IN (
			SELECT id FROM outbox
			WHERE published_at IS NULL AND next_attempt_at <= clock_timestamp() AND (lease_until IS NULL OR lease_until < clock_timestamp())
			ORDER BY created_at ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, aggregate_id, aggregate_version, event_type, schema_version, payload, lease_owner, lease_until, next_attempt_at, published_at, created_at`

	rows, err := exec.Query(ctx, query, leaseOwner, leaseUntil, limit)
	if err != nil {
		return nil, fmt.Errorf("claim outbox: %w", err)
	}
	defer rows.Close()

	var list []model.Outbox
	for rows.Next() {
		var o model.Outbox
		if err := rows.Scan(
			&o.ID, &o.AggregateID, &o.AggregateVersion, &o.EventType, &o.SchemaVersion,
			&o.Payload, &o.LeaseOwner, &o.LeaseUntil, &o.NextAttemptAt, &o.PublishedAt, &o.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan outbox: %w", err)
		}
		list = append(list, o)
	}
	return list, rows.Err()
}

func (r *PostgresOutboxRepository) MarkPublished(ctx context.Context, tx DBTX, id string) error {
	exec := tx
	if exec == nil {
		exec = r.db
	}
	query := `UPDATE outbox SET published_at = clock_timestamp() WHERE id = $1`
	_, err := exec.Exec(ctx, query, id)
	return err
}

// PostgresInventoryMovementRepository implementasi PostgreSQL untuk InventoryMovementRepository.
type PostgresInventoryMovementRepository struct {
	db DBTX
}

func NewPostgresInventoryMovementRepository(db DBTX) *PostgresInventoryMovementRepository {
	return &PostgresInventoryMovementRepository{db: db}
}

func (r *PostgresInventoryMovementRepository) Create(ctx context.Context, tx DBTX, movement *model.InventoryMovement) error {
	exec := tx
	if exec == nil {
		exec = r.db
	}
	query := `
		INSERT INTO inventory_movements (operation_id, tier_id, booking_id, movement_type, delta_held, delta_sold, delta_retired, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, clock_timestamp())
		RETURNING id, created_at`

	return exec.QueryRow(ctx, query,
		movement.OperationID, movement.TierID, movement.BookingID, movement.MovementType,
		movement.DeltaHeld, movement.DeltaSold, movement.DeltaRetired, movement.Reason,
	).Scan(&movement.ID, &movement.CreatedAt)
}
