// Package testfake berisi implementasi dalam memori untuk pengujian usecase dan service tanpa database.
package testfake

import (
	"context"
	"sync"
	"time"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/app/repository"
)

// UserRepo adalah repository dalam memori untuk menguji aturan usecase tanpa database.
type UserRepo struct {
	mu     sync.Mutex
	nextID int64
	Users  map[int64]model.User
	tokens map[int64]model.RefreshToken
	ids    map[int64]model.IdentityActionToken
}

func NewUserRepo() *UserRepo {
	return &UserRepo{
		Users:  map[int64]model.User{},
		tokens: map[int64]model.RefreshToken{},
		ids:    map[int64]model.IdentityActionToken{},
	}
}

func (f *UserRepo) id() int64 {
	f.nextID++
	return f.nextID
}

func (f *UserRepo) CreateUser(_ context.Context, _ repository.DBTX, u model.User) (model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.Users {
		if existing.Email == u.Email {
			return model.User{}, repository.ErrConflict
		}
	}
	u.ID = f.id()
	u.AuthVersion = 1
	f.Users[u.ID] = u
	return u, nil
}

func (f *UserRepo) FindByEmail(_ context.Context, _ repository.DBTX, email string) (model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.Users {
		if u.Email == email {
			return u, nil
		}
	}
	return model.User{}, repository.ErrNotFound
}

func (f *UserRepo) FindByID(_ context.Context, _ repository.DBTX, id int64) (model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.Users[id]
	if !ok {
		return model.User{}, repository.ErrNotFound
	}
	return u, nil
}

func (f *UserRepo) UpdateProfile(context.Context, repository.DBTX, model.User) error { return nil }

// UpdatePassword meniru repository: hash diganti, auth_version naik, dan seluruh refresh token dicabut.
func (f *UserRepo) UpdatePassword(_ context.Context, _ repository.DBTX, userID int64, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.Users[userID]
	if !ok {
		return repository.ErrNotFound
	}
	u.PasswordHash = hash
	u.AuthVersion++
	f.Users[userID] = u
	now := time.Now()
	for id, t := range f.ids {
		if t.UserID == userID && t.ConsumedAt == nil {
			t.ConsumedAt = &now
			f.ids[id] = t
		}
	}
	for id, t := range f.tokens {
		if t.UserID == userID && t.RevokedAt == nil {
			t.RevokedAt = &now
			f.tokens[id] = t
		}
	}
	return nil
}

func (f *UserRepo) MarkEmailVerified(_ context.Context, _ repository.DBTX, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.Users[userID]
	if !ok {
		return repository.ErrNotFound
	}
	if u.EmailVerifiedAt == nil {
		now := time.Now()
		u.EmailVerifiedAt = &now
		f.Users[userID] = u
	}
	return nil
}

func (f *UserRepo) CreateRefreshToken(_ context.Context, _ repository.DBTX, t model.RefreshToken) (model.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t.ID = f.id()
	f.tokens[t.ID] = t
	return t, nil
}

func (f *UserRepo) FindRefreshTokenByHash(_ context.Context, _ repository.DBTX, hash string) (model.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tokens {
		if t.TokenHash == hash {
			return t, nil
		}
	}
	return model.RefreshToken{}, repository.ErrNotFound
}

// RotateRefreshToken meniru aturan repository: token lama harus belum dipakai dan belum dicabut.
func (f *UserRepo) RotateRefreshToken(_ context.Context, _ repository.DBTX, oldID int64, next model.RefreshToken) (model.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	old, ok := f.tokens[oldID]
	if !ok || old.UsedAt != nil || old.RevokedAt != nil {
		return model.RefreshToken{}, repository.ErrConflict
	}
	now := time.Now()
	old.UsedAt = &now
	f.tokens[oldID] = old
	next.ID = f.id()
	f.tokens[next.ID] = next
	return next, nil
}

func (f *UserRepo) RevokeRefreshFamily(_ context.Context, _ repository.DBTX, familyID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for id, t := range f.tokens {
		if t.FamilyID == familyID && t.RevokedAt == nil {
			t.RevokedAt = &now
			f.tokens[id] = t
		}
	}
	return nil
}

func (f *UserRepo) CreateIdentityActionToken(_ context.Context, _ repository.DBTX, t model.IdentityActionToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t.ID = f.id()
	f.ids[t.ID] = t
	return nil
}

// ConsumeIdentityActionToken meniru aturan repository: hanya token cocok, belum terpakai, dan belum kedaluwarsa.
func (f *UserRepo) ConsumeIdentityActionToken(_ context.Context, _ repository.DBTX, hash, purpose string, now time.Time) (model.IdentityActionToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, t := range f.ids {
		if t.TokenHash == hash && t.Purpose == purpose && t.ConsumedAt == nil && now.Before(t.ExpiresAt) {
			t.ConsumedAt = &now
			f.ids[id] = t
			return t, nil
		}
	}
	return model.IdentityActionToken{}, repository.ErrConflict
}

func (f *UserRepo) ListPermissions(context.Context, repository.DBTX, int64) ([]string, error) {
	return nil, nil
}

func (f *UserRepo) ListActiveEventStaff(context.Context, repository.DBTX, int64) ([]model.EventStaff, error) {
	return nil, nil
}

// Tx menjalankan fungsi tanpa transaksi nyata; cukup untuk menguji alur usecase.
type Tx struct{}

func (Tx) WithTx(ctx context.Context, fn repository.TxFunc) error     { return fn(ctx, nil) }
func (Tx) WithTxOnce(ctx context.Context, fn repository.TxFunc) error { return fn(ctx, nil) }
