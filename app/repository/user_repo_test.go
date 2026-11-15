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
	"github.com/viesloxy/ngawimusical-backend/config"
	"github.com/viesloxy/ngawimusical-backend/database"
	"github.com/viesloxy/ngawimusical-backend/helper"
)

// openTestPool memakai PostgreSQL dari konfigurasi environment.
// Test dilewati bila database tidak dapat dihubungi.
func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Skipf("konfigurasi tidak tersedia: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPostgres(ctx, cfg.DB)
	if err != nil {
		t.Skipf("database test tidak tersedia: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("jalankan migrasi: %v", err)
	}
	return pool
}

func createTestUser(t *testing.T, repo repository.UserRepository, pool *pgxpool.Pool) model.User {
	t.Helper()

	id := uuid.NewString()
	user, err := repo.CreateUser(context.Background(), pool, model.User{
		Username:     "uji_" + id[:8],
		Email:        id + "@example.test",
		PasswordHash: "hash-uji",
		RoleName:     model.RoleAttendee,
		Status:       model.AccountActive,
	})
	if err != nil {
		t.Fatalf("buat user uji: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	})
	return user
}

func TestCreateUserRejectsDuplicateEmail(t *testing.T) {
	pool := openTestPool(t)
	repo := repository.NewUserRepository()
	user := createTestUser(t, repo, pool)

	_, err := repo.CreateUser(context.Background(), pool, model.User{
		Username:     "lain_" + uuid.NewString()[:8],
		Email:        user.Email,
		PasswordHash: "hash-uji",
		RoleName:     model.RoleAttendee,
		Status:       model.AccountActive,
	})
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("email duplikat harus ErrConflict, dapat: %v", err)
	}
}

func TestRefreshTokenReuseIsRejectedAndFamilyRevoked(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	repo := repository.NewUserRepository()
	user := createTestUser(t, repo, pool)

	family := uuid.NewString()
	expires := time.Now().Add(time.Hour)
	first, err := repo.CreateRefreshToken(ctx, pool, model.RefreshToken{
		UserID: user.ID, TokenHash: uuid.NewString() + uuid.NewString()[:8], FamilyID: family, ExpiresAt: expires,
	})
	if err != nil {
		t.Fatalf("buat refresh token awal: %v", err)
	}

	second, err := repo.RotateRefreshToken(ctx, pool, first.ID, model.RefreshToken{
		UserID: user.ID, TokenHash: uuid.NewString() + uuid.NewString()[:8], FamilyID: family, ParentID: &first.ID, ExpiresAt: expires,
	})
	if err != nil {
		t.Fatalf("rotasi pertama harus berhasil: %v", err)
	}

	if _, err := repo.RotateRefreshToken(ctx, pool, first.ID, model.RefreshToken{
		UserID: user.ID, TokenHash: uuid.NewString() + uuid.NewString()[:8], FamilyID: family, ExpiresAt: expires,
	}); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("token yang sudah dipakai harus ErrConflict, dapat: %v", err)
	}

	if err := repo.RevokeRefreshFamily(ctx, pool, family); err != nil {
		t.Fatalf("cabut family: %v", err)
	}
	revoked, err := repo.FindRefreshTokenByHash(ctx, pool, second.TokenHash)
	if err != nil {
		t.Fatalf("cari token baru: %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatal("token dalam family yang sama harus ikut dicabut")
	}
}

func TestIdentityActionTokenIsSingleUse(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	repo := repository.NewUserRepository()
	user := createTestUser(t, repo, pool)

	hash := uuid.NewString() + uuid.NewString()[:8]
	if err := repo.CreateIdentityActionToken(ctx, pool, model.IdentityActionToken{
		UserID: user.ID, TokenHash: hash, Purpose: model.PurposeVerifyEmail, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("buat token aksi: %v", err)
	}

	now := time.Now()
	if _, err := repo.ConsumeIdentityActionToken(ctx, pool, hash, model.PurposeVerifyEmail, now); err != nil {
		t.Fatalf("pemakaian pertama harus berhasil: %v", err)
	}
	if _, err := repo.ConsumeIdentityActionToken(ctx, pool, hash, model.PurposeVerifyEmail, now); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("pemakaian kedua harus ErrConflict, dapat: %v", err)
	}
}

// Mengganti password harus menaikkan auth_version dan mencabut seluruh refresh token user.
func TestUpdatePasswordRevokesRefreshTokens(t *testing.T) {
	pool := openTestPool(t)
	repo := repository.NewUserRepository()
	user := createTestUser(t, repo, pool)
	ctx := context.Background()

	token, err := repo.CreateRefreshToken(ctx, pool, model.RefreshToken{
		UserID:    user.ID,
		TokenHash: helper.HashRefreshToken(uuid.NewString()),
		FamilyID:  uuid.NewString(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("buat refresh token: %v", err)
	}

	if err := repo.UpdatePassword(ctx, pool, user.ID, "hash-baru"); err != nil {
		t.Fatalf("update password: %v", err)
	}

	updated, err := repo.FindByID(ctx, pool, user.ID)
	if err != nil {
		t.Fatalf("baca user: %v", err)
	}
	if updated.AuthVersion != user.AuthVersion+1 {
		t.Fatalf("auth_version harus naik, dapat %d", updated.AuthVersion)
	}

	revoked, err := repo.FindRefreshTokenByHash(ctx, pool, token.TokenHash)
	if err != nil {
		t.Fatalf("baca refresh token: %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatal("refresh token harus dicabut setelah password diganti")
	}
}

// Mengubah email harus mencabut refresh token lama lewat pencabutan seluruh token user.
func TestEmailChangeRevokesRefreshTokens(t *testing.T) {
	pool := openTestPool(t)
	repo := repository.NewUserRepository()
	user := createTestUser(t, repo, pool)
	ctx := context.Background()

	token, err := repo.CreateRefreshToken(ctx, pool, model.RefreshToken{
		UserID:    user.ID,
		TokenHash: helper.HashRefreshToken(uuid.NewString()),
		FamilyID:  uuid.NewString(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("buat refresh token: %v", err)
	}

	user.Email = uuid.NewString() + "@example.test"
	if err := repo.UpdateProfile(ctx, pool, user); err != nil {
		t.Fatalf("update profil: %v", err)
	}

	revoked, err := repo.FindRefreshTokenByHash(ctx, pool, token.TokenHash)
	if err != nil {
		t.Fatalf("baca refresh token: %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Fatal("refresh token harus dicabut setelah email berubah")
	}
}
