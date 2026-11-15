package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/viesloxy/ngawimusical-backend/app/model"
)

// PostgresEventStaffRepository adalah implementasi EventStaffRepository untuk PostgreSQL.
type PostgresEventStaffRepository struct {
	db DBTX
}

func NewPostgresEventStaffRepository(db DBTX) *PostgresEventStaffRepository {
	return &PostgresEventStaffRepository{db: db}
}

func (r *PostgresEventStaffRepository) executor(tx DBTX) DBTX {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *PostgresEventStaffRepository) Create(ctx context.Context, tx DBTX, staff *model.EventStaff) error {
	query := `
		INSERT INTO event_staff (event_id, user_id, staff_role, gate_id, status, created_at)
		VALUES ($1, $2, $3, $4, $5, clock_timestamp())
		RETURNING created_at`

	return r.executor(tx).QueryRow(ctx, query,
		staff.EventID, staff.UserID, staff.StaffRole, staff.GateID, staff.Status,
	).Scan(&staff.CreatedAt)
}

func (r *PostgresEventStaffRepository) GetByEventAndUser(ctx context.Context, tx DBTX, eventID int64, userID int64) (*model.EventStaff, error) {
	query := `
		SELECT event_id, user_id, staff_role, gate_id, status, created_at
		FROM event_staff
		WHERE event_id = $1 AND user_id = $2`

	var staff model.EventStaff
	err := r.executor(tx).QueryRow(ctx, query, eventID, userID).Scan(
		&staff.EventID, &staff.UserID, &staff.StaffRole, &staff.GateID, &staff.Status, &staff.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ambil event staff: %w", err)
	}

	return &staff, nil
}

func (r *PostgresEventStaffRepository) ListByEventID(ctx context.Context, tx DBTX, eventID int64) ([]model.EventStaff, error) {
	query := `
		SELECT event_id, user_id, staff_role, gate_id, status, created_at
		FROM event_staff
		WHERE event_id = $1
		ORDER BY created_at ASC`

	rows, err := r.executor(tx).Query(ctx, query, eventID)
	if err != nil {
		return nil, fmt.Errorf("daftar event staff: %w", err)
	}
	defer rows.Close()

	var staffList []model.EventStaff
	for rows.Next() {
		var staff model.EventStaff
		if err := rows.Scan(
			&staff.EventID, &staff.UserID, &staff.StaffRole, &staff.GateID, &staff.Status, &staff.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan event staff: %w", err)
		}
		staffList = append(staffList, staff)
	}

	return staffList, rows.Err()
}

func (r *PostgresEventStaffRepository) IsStaffAssigned(ctx context.Context, tx DBTX, eventID int64, userID int64, staffRole string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM event_staff
			WHERE event_id = $1 AND user_id = $2 AND staff_role = $3 AND status = 'ACTIVE'
		)`

	var assigned bool
	if err := r.executor(tx).QueryRow(ctx, query, eventID, userID, staffRole).Scan(&assigned); err != nil {
		return false, fmt.Errorf("periksa event staff: %w", err)
	}

	return assigned, nil
}
