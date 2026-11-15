package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/viesloxy/ngawimusical-backend/app/model"
)

// UserRepository berisi akses data user. Setiap method menerima DBTX
// sehingga bisa dijalankan di pool biasa maupun di dalam transaksi TxManager.
type UserRepository interface {
	CreateUser(ctx context.Context, db DBTX, user model.User) (model.User, error)
	FindByEmail(ctx context.Context, db DBTX, email string) (model.User, error)
	FindByID(ctx context.Context, db DBTX, id int64) (model.User, error)
	UpdateProfile(ctx context.Context, db DBTX, user model.User) error
	UpdatePassword(ctx context.Context, db DBTX, userID int64, passwordHash string) error
	MarkEmailVerified(ctx context.Context, db DBTX, userID int64) error

	CreateRefreshToken(ctx context.Context, db DBTX, token model.RefreshToken) (model.RefreshToken, error)
	FindRefreshTokenByHash(ctx context.Context, db DBTX, tokenHash string) (model.RefreshToken, error)
	RotateRefreshToken(ctx context.Context, db DBTX, oldID int64, next model.RefreshToken) (model.RefreshToken, error)
	RevokeRefreshFamily(ctx context.Context, db DBTX, familyID string) error

	CreateIdentityActionToken(ctx context.Context, db DBTX, token model.IdentityActionToken) error
	ConsumeIdentityActionToken(ctx context.Context, db DBTX, tokenHash, purpose string, now time.Time) (model.IdentityActionToken, error)

	ListPermissions(ctx context.Context, db DBTX, userID int64) ([]string, error)
	ListActiveEventStaff(ctx context.Context, db DBTX, userID int64) ([]model.EventStaff, error)
}

type userPostgres struct{}

// NewUserRepository membuat implementasi UserRepository untuk PostgreSQL.
func NewUserRepository() UserRepository {
	return userPostgres{}
}

const userSelect = `
	SELECT u.id, u.username, u.email, u.password_hash, u.role_id, r.name, u.status,
	       u.email_verified_at, u.auth_version, u.created_at, u.updated_at
	FROM users u
	JOIN roles r ON r.id = u.role_id`

func (userPostgres) CreateUser(ctx context.Context, db DBTX, user model.User) (model.User, error) {
	err := db.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO users (username, email, password_hash, role_id, status)
			VALUES ($1, $2, $3, (SELECT id FROM roles WHERE name = $4), $5)
			RETURNING id, username, email, password_hash, role_id, status, email_verified_at, auth_version, created_at, updated_at
		)
		SELECT i.id, i.username, i.email, i.password_hash, i.role_id, r.name, i.status,
		       i.email_verified_at, i.auth_version, i.created_at, i.updated_at
		FROM inserted i
		JOIN roles r ON r.id = i.role_id`,
		user.Username, normalizeEmail(user.Email), user.PasswordHash, user.RoleName, user.Status,
	).Scan(scanUser(&user)...)
	return user, mapDBError(err)
}

func (userPostgres) FindByEmail(ctx context.Context, db DBTX, email string) (model.User, error) {
	var user model.User
	err := db.QueryRow(ctx, userSelect+` WHERE u.email = $1`, normalizeEmail(email)).Scan(scanUser(&user)...)
	return user, mapDBError(err)
}

func (userPostgres) FindByID(ctx context.Context, db DBTX, id int64) (model.User, error) {
	var user model.User
	err := db.QueryRow(ctx, userSelect+` WHERE u.id = $1`, id).Scan(scanUser(&user)...)
	return user, mapDBError(err)
}

func (userPostgres) UpdateProfile(ctx context.Context, db DBTX, user model.User) error {
	var current string
	if err := db.QueryRow(ctx, `SELECT email FROM users WHERE id = $1 FOR UPDATE`, user.ID).Scan(&current); err != nil {
		return mapDBError(err)
	}
	email := normalizeEmail(user.Email)

	// Email yang berubah harus diverifikasi ulang dan mencabut sesi lama lewat auth_version.
	tag, err := db.Exec(ctx, `
		UPDATE users
		SET username = $2,
		    email = $3::varchar,
		    email_verified_at = CASE WHEN email IS DISTINCT FROM $3::varchar THEN NULL ELSE email_verified_at END,
		    auth_version = CASE WHEN email IS DISTINCT FROM $3::varchar THEN auth_version + 1 ELSE auth_version END,
		    updated_at = clock_timestamp()
		WHERE id = $1`,
		user.ID, user.Username, email,
	)
	if err != nil {
		return mapDBError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if current != email {
		return credentialChanged(ctx, db, user.ID)
	}
	return nil
}

// UpdatePassword mengganti hash password, menaikkan auth_version agar access token lama
// tidak lagi lolos, dan mencabut seluruh refresh token user. Pemanggil wajib memakai transaksi.
func (userPostgres) UpdatePassword(ctx context.Context, db DBTX, userID int64, passwordHash string) error {
	tag, err := db.Exec(ctx, `
		UPDATE users
		SET password_hash = $2,
		    auth_version = auth_version + 1,
		    updated_at = clock_timestamp()
		WHERE id = $1`,
		userID, passwordHash,
	)
	if err != nil {
		return mapDBError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return credentialChanged(ctx, db, userID)
}

// MarkEmailVerified menandai email user sudah diverifikasi. Nilai yang sudah ada tidak ditimpa.
func (userPostgres) MarkEmailVerified(ctx context.Context, db DBTX, userID int64) error {
	tag, err := db.Exec(ctx, `
		UPDATE users
		SET email_verified_at = COALESCE(email_verified_at, clock_timestamp()),
		    updated_at = clock_timestamp()
		WHERE id = $1`,
		userID,
	)
	if err != nil {
		return mapDBError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// credentialChanged dipanggil setiap kali kredensial berubah: sesi lama dan token aksi yang masih
// terbuka ikut dibatalkan dalam transaksi yang sama.
func credentialChanged(ctx context.Context, db DBTX, userID int64) error {
	if err := revokeAllRefreshTokens(ctx, db, userID); err != nil {
		return err
	}
	_, err := db.Exec(ctx, `
		UPDATE identity_action_tokens
		SET consumed_at = clock_timestamp()
		WHERE user_id = $1 AND consumed_at IS NULL`,
		userID,
	)
	return mapDBError(err)
}

func revokeAllRefreshTokens(ctx context.Context, db DBTX, userID int64) error {
	_, err := db.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = clock_timestamp()
		WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	)
	return mapDBError(err)
}

func (userPostgres) CreateRefreshToken(ctx context.Context, db DBTX, token model.RefreshToken) (model.RefreshToken, error) {
	return insertRefreshToken(ctx, db, token)
}

func (userPostgres) FindRefreshTokenByHash(ctx context.Context, db DBTX, tokenHash string) (model.RefreshToken, error) {
	var token model.RefreshToken
	err := db.QueryRow(ctx, `
		SELECT id, user_id, token_hash, family_id::text, parent_id, expires_at, used_at, revoked_at, created_at
		FROM refresh_tokens
		WHERE token_hash = $1`,
		tokenHash,
	).Scan(&token.ID, &token.UserID, &token.TokenHash, &token.FamilyID, &token.ParentID,
		&token.ExpiresAt, &token.UsedAt, &token.RevokedAt, &token.CreatedAt)
	return token, mapDBError(err)
}

// RotateRefreshToken menandai token lama sebagai terpakai lalu menyimpan penggantinya.
// Pemanggil wajib menjalankannya dalam satu transaksi. Jika token lama sudah terpakai
// atau dicabut, mengembalikan ErrConflict dan tidak ada token baru yang tersimpan.
func (userPostgres) RotateRefreshToken(ctx context.Context, db DBTX, oldID int64, next model.RefreshToken) (model.RefreshToken, error) {
	tag, err := db.Exec(ctx, `
		UPDATE refresh_tokens
		SET used_at = clock_timestamp()
		WHERE id = $1 AND user_id = $2 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > clock_timestamp()`,
		oldID, next.UserID,
	)
	if err != nil {
		return model.RefreshToken{}, mapDBError(err)
	}
	if tag.RowsAffected() == 0 {
		return model.RefreshToken{}, ErrConflict
	}
	return insertRefreshToken(ctx, db, next)
}

func (userPostgres) RevokeRefreshFamily(ctx context.Context, db DBTX, familyID string) error {
	_, err := db.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = clock_timestamp()
		WHERE family_id = $1 AND revoked_at IS NULL`,
		familyID,
	)
	return mapDBError(err)
}

func insertRefreshToken(ctx context.Context, db DBTX, token model.RefreshToken) (model.RefreshToken, error) {
	err := db.QueryRow(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, family_id, parent_id, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`,
		token.UserID, token.TokenHash, token.FamilyID, token.ParentID, token.ExpiresAt,
	).Scan(&token.ID, &token.CreatedAt)
	return token, mapDBError(err)
}

func (userPostgres) CreateIdentityActionToken(ctx context.Context, db DBTX, token model.IdentityActionToken) error {
	err := db.QueryRow(ctx, `
		INSERT INTO identity_action_tokens (user_id, token_hash, purpose, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		token.UserID, token.TokenHash, token.Purpose, token.ExpiresAt,
	).Scan(&token.ID, &token.CreatedAt)
	return mapDBError(err)
}

// ConsumeIdentityActionToken menandai token aksi sebagai terpakai secara atomik.
// Token yang tidak ada, sudah terpakai, atau kedaluwarsa mengembalikan ErrConflict.
func (userPostgres) ConsumeIdentityActionToken(ctx context.Context, db DBTX, tokenHash, purpose string, now time.Time) (model.IdentityActionToken, error) {
	var token model.IdentityActionToken
	err := db.QueryRow(ctx, `
		UPDATE identity_action_tokens
		SET consumed_at = $3
		WHERE token_hash = $1 AND purpose = $2 AND consumed_at IS NULL AND expires_at > $3
		RETURNING id, user_id, token_hash, purpose, expires_at, consumed_at, created_at`,
		tokenHash, purpose, now,
	).Scan(&token.ID, &token.UserID, &token.TokenHash, &token.Purpose,
		&token.ExpiresAt, &token.ConsumedAt, &token.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.IdentityActionToken{}, ErrConflict
	}
	return token, mapDBError(err)
}

// ListPermissions membaca permission dari role user. Organizer hanya menerima
// scope milik sendiri (akhiran :own); pengecekan ownership tetap dilakukan service.
func (userPostgres) ListPermissions(ctx context.Context, db DBTX, userID int64) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT rp.permission
		FROM users u
		JOIN roles r ON r.id = u.role_id
		JOIN role_permissions rp ON rp.role_id = u.role_id
		WHERE u.id = $1 AND (r.name <> 'organizer' OR rp.permission LIKE '%:own')
		ORDER BY rp.permission`,
		userID,
	)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	var permissions []string
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, mapDBError(err)
		}
		permissions = append(permissions, permission)
	}
	return permissions, mapDBError(rows.Err())
}

func (userPostgres) ListActiveEventStaff(ctx context.Context, db DBTX, userID int64) ([]model.EventStaff, error) {
	rows, err := db.Query(ctx, `
		SELECT event_id, user_id, staff_role, gate_id, status, created_at
		FROM event_staff
		WHERE user_id = $1 AND status = $2
		ORDER BY event_id`,
		userID, model.StaffActive,
	)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	var assignments []model.EventStaff
	for rows.Next() {
		var staff model.EventStaff
		if err := rows.Scan(&staff.EventID, &staff.UserID, &staff.StaffRole, &staff.GateID, &staff.Status, &staff.CreatedAt); err != nil {
			return nil, mapDBError(err)
		}
		assignments = append(assignments, staff)
	}
	return assignments, mapDBError(rows.Err())
}

// normalizeEmail menyamakan email agar uniqueness tidak bergantung pada huruf besar/kecil.
// Lapisan usecase juga melakukannya; ini penjaga terakhir sebelum query.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func scanUser(user *model.User) []any {
	return []any{
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.RoleID, &user.RoleName,
		&user.Status, &user.EmailVerifiedAt, &user.AuthVersion, &user.CreatedAt, &user.UpdatedAt,
	}
}

// mapDBError menerjemahkan error driver ke sentinel repository.
// Kode 23505 (unique violation) menjadi ErrConflict dan baris kosong menjadi ErrNotFound.
func mapDBError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}
