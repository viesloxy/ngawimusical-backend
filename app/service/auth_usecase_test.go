package service_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/app/repository"
	"github.com/viesloxy/ngawimusical-backend/app/service"
	"github.com/viesloxy/ngawimusical-backend/helper"
)

// fakeRepo adalah repository dalam memori untuk menguji aturan usecase tanpa database.
type fakeRepo struct {
	mu     sync.Mutex
	nextID int64
	users  map[int64]model.User
	tokens map[int64]model.RefreshToken
	ids    map[int64]model.IdentityActionToken
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		users:  map[int64]model.User{},
		tokens: map[int64]model.RefreshToken{},
		ids:    map[int64]model.IdentityActionToken{},
	}
}

func (f *fakeRepo) id() int64 {
	f.nextID++
	return f.nextID
}

func (f *fakeRepo) CreateUser(_ context.Context, _ repository.DBTX, u model.User) (model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.users {
		if existing.Email == u.Email {
			return model.User{}, repository.ErrConflict
		}
	}
	u.ID = f.id()
	u.AuthVersion = 1
	f.users[u.ID] = u
	return u, nil
}

func (f *fakeRepo) FindByEmail(_ context.Context, _ repository.DBTX, email string) (model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return model.User{}, repository.ErrNotFound
}

func (f *fakeRepo) FindByID(_ context.Context, _ repository.DBTX, id int64) (model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return model.User{}, repository.ErrNotFound
	}
	return u, nil
}

func (f *fakeRepo) UpdateProfile(context.Context, repository.DBTX, model.User) error { return nil }

// UpdatePassword meniru repository: hash diganti, auth_version naik, dan seluruh refresh token dicabut.
func (f *fakeRepo) UpdatePassword(_ context.Context, _ repository.DBTX, userID int64, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[userID]
	if !ok {
		return repository.ErrNotFound
	}
	u.PasswordHash = hash
	u.AuthVersion++
	f.users[userID] = u
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

func (f *fakeRepo) MarkEmailVerified(_ context.Context, _ repository.DBTX, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[userID]
	if !ok {
		return repository.ErrNotFound
	}
	if u.EmailVerifiedAt == nil {
		now := time.Now()
		u.EmailVerifiedAt = &now
		f.users[userID] = u
	}
	return nil
}

func (f *fakeRepo) CreateRefreshToken(_ context.Context, _ repository.DBTX, t model.RefreshToken) (model.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t.ID = f.id()
	f.tokens[t.ID] = t
	return t, nil
}

func (f *fakeRepo) FindRefreshTokenByHash(_ context.Context, _ repository.DBTX, hash string) (model.RefreshToken, error) {
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
func (f *fakeRepo) RotateRefreshToken(_ context.Context, _ repository.DBTX, oldID int64, next model.RefreshToken) (model.RefreshToken, error) {
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

func (f *fakeRepo) RevokeRefreshFamily(_ context.Context, _ repository.DBTX, familyID string) error {
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

func (f *fakeRepo) CreateIdentityActionToken(_ context.Context, _ repository.DBTX, t model.IdentityActionToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t.ID = f.id()
	f.ids[t.ID] = t
	return nil
}

// ConsumeIdentityActionToken meniru aturan repository: hanya token cocok, belum terpakai, dan belum kedaluwarsa.
func (f *fakeRepo) ConsumeIdentityActionToken(_ context.Context, _ repository.DBTX, hash, purpose string, now time.Time) (model.IdentityActionToken, error) {
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

func (f *fakeRepo) ListPermissions(context.Context, repository.DBTX, int64) ([]string, error) {
	return nil, nil
}

func (f *fakeRepo) ListActiveEventStaff(context.Context, repository.DBTX, int64) ([]model.EventStaff, error) {
	return nil, nil
}

// fakeTx menjalankan fungsi tanpa transaksi nyata; cukup untuk menguji alur usecase.
type fakeTx struct{}

func (fakeTx) WithTx(ctx context.Context, fn repository.TxFunc) error     { return fn(ctx, nil) }
func (fakeTx) WithTxOnce(ctx context.Context, fn repository.TxFunc) error { return fn(ctx, nil) }

func newTestUsecase(t *testing.T, repo *fakeRepo) *service.AuthUsecase {
	t.Helper()
	secret, err := helper.NewRefreshToken() // secret acak per test, bukan nilai tetap
	if err != nil {
		t.Fatalf("buat secret acak: %v", err)
	}
	issuer := helper.NewTokenIssuer(secret, "ngawimusical", "ngawimusical-api", 15*time.Minute)
	uc, err := service.NewAuthUsecase(repo, fakeTx{}, issuer, service.AuthUsecaseConfig{
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 7 * 24 * time.Hour,
		BcryptCost: 4, // cost terendah agar test cepat
	})
	if err != nil {
		t.Fatalf("buat usecase: %v", err)
	}
	return uc
}

func requireAppError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var appErr *helper.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("harus AppError, dapat: %v", err)
	}
	if appErr.Status != status || appErr.Code != code {
		t.Fatalf("harus %d %s, dapat %d %s", status, code, appErr.Status, appErr.Code)
	}
}

func TestRegisterNormalizesEmailAndReturnsTokens(t *testing.T) {
	repo := newFakeRepo()
	uc := newTestUsecase(t, repo)

	session, err := uc.Register(context.Background(), model.RegisterRequest{
		Username: "arsalan", Email: "  Arsalan@Example.COM ", Password: "rahasia123",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if session.AccessToken == "" || session.RefreshToken == "" {
		t.Fatal("register harus mengembalikan access dan refresh token")
	}
	if _, err := repo.FindByEmail(context.Background(), nil, "arsalan@example.com"); err != nil {
		t.Fatalf("email harus tersimpan dalam huruf kecil: %v", err)
	}
}

func TestRegisterDuplicateEmailIsGenericConflict(t *testing.T) {
	uc := newTestUsecase(t, newFakeRepo())
	req := model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"}

	if _, err := uc.Register(context.Background(), req); err != nil {
		t.Fatalf("register pertama: %v", err)
	}
	_, err := uc.Register(context.Background(), req)
	requireAppError(t, err, http.StatusConflict, "CONFLICT")
}

func TestLoginFailuresUseSameGenericMessage(t *testing.T) {
	repo := newFakeRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	if _, err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	_, wrongPassword := uc.Login(ctx, model.LoginRequest{Email: "a@example.com", Password: "salah-sekali"})
	_, unknownEmail := uc.Login(ctx, model.LoginRequest{Email: "tidak@ada.com", Password: "rahasia123"})

	requireAppError(t, wrongPassword, http.StatusUnauthorized, "UNAUTHENTICATED")
	requireAppError(t, unknownEmail, http.StatusUnauthorized, "UNAUTHENTICATED")
	if wrongPassword.Error() != unknownEmail.Error() {
		t.Fatal("pesan login gagal harus sama agar tidak membocorkan keberadaan akun")
	}
}

func TestSuspendedAccountCannotLogin(t *testing.T) {
	repo := newFakeRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	if _, err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	for id, u := range repo.users {
		u.Status = model.AccountSuspended
		repo.users[id] = u
	}

	_, err := uc.Login(ctx, model.LoginRequest{Email: "a@example.com", Password: "rahasia123"})
	requireAppError(t, err, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestReusedRefreshTokenRevokesWholeFamily(t *testing.T) {
	repo := newFakeRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	first, err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	second, err := uc.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh pertama harus berhasil: %v", err)
	}

	// Token pertama dipakai ulang: dianggap dicuri, sehingga token kedua ikut dicabut.
	_, err = uc.Refresh(ctx, first.RefreshToken)
	requireAppError(t, err, http.StatusUnauthorized, "UNAUTHENTICATED")

	_, err = uc.Refresh(ctx, second.RefreshToken)
	requireAppError(t, err, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestLogoutRevokesSession(t *testing.T) {
	uc := newTestUsecase(t, newFakeRepo())
	ctx := context.Background()

	session, err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := uc.Logout(ctx, session.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}

	_, err = uc.Refresh(ctx, session.RefreshToken)
	requireAppError(t, err, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestEmailVerificationTokenIsSingleUse(t *testing.T) {
	repo := newFakeRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	_, err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	userID := repo.users[1].ID

	raw, err := uc.RequestEmailVerification(ctx, userID)
	if err != nil || raw == "" {
		t.Fatalf("minta verifikasi harus memberi token: %v", err)
	}
	if err := uc.ConfirmEmail(ctx, raw); err != nil {
		t.Fatalf("konfirmasi pertama: %v", err)
	}
	if repo.users[userID].EmailVerifiedAt == nil {
		t.Fatal("email harus bertanda terverifikasi")
	}

	requireAppError(t, uc.ConfirmEmail(ctx, raw), http.StatusBadRequest, "TOKEN_INVALID")
}

func TestEmailVerificationSkippedWhenAlreadyVerified(t *testing.T) {
	repo := newFakeRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	if _, err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	userID := repo.users[1].ID
	raw, _ := uc.RequestEmailVerification(ctx, userID)
	if err := uc.ConfirmEmail(ctx, raw); err != nil {
		t.Fatalf("konfirmasi: %v", err)
	}

	raw, err := uc.RequestEmailVerification(ctx, userID)
	if err != nil || raw != "" {
		t.Fatalf("email yang sudah terverifikasi tidak boleh membuat token baru, dapat %q, %v", raw, err)
	}
}

func TestPasswordResetRevokesSessionsAndChangesPassword(t *testing.T) {
	repo := newFakeRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	old, err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	raw, err := uc.RequestPasswordReset(ctx, "a@example.com")
	if err != nil || raw == "" {
		t.Fatalf("minta reset harus memberi token: %v", err)
	}
	if err := uc.ResetPassword(ctx, raw, "passwordbaru1"); err != nil {
		t.Fatalf("reset password: %v", err)
	}

	_, err = uc.Refresh(ctx, old.RefreshToken)
	requireAppError(t, err, http.StatusUnauthorized, "UNAUTHENTICATED")

	_, err = uc.Login(ctx, model.LoginRequest{Email: "a@example.com", Password: "rahasia123"})
	requireAppError(t, err, http.StatusUnauthorized, "UNAUTHENTICATED")

	if _, err := uc.Login(ctx, model.LoginRequest{Email: "a@example.com", Password: "passwordbaru1"}); err != nil {
		t.Fatalf("login dengan password baru harus berhasil: %v", err)
	}
}

func TestPasswordResetTokenIsSingleUse(t *testing.T) {
	repo := newFakeRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	if _, err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	raw, _ := uc.RequestPasswordReset(ctx, "a@example.com")
	if err := uc.ResetPassword(ctx, raw, "passwordbaru1"); err != nil {
		t.Fatalf("reset pertama: %v", err)
	}
	requireAppError(t, uc.ResetPassword(ctx, raw, "passwordketiga1"), http.StatusBadRequest, "TOKEN_INVALID")
}

func TestPasswordResetForUnknownEmailReturnsNothing(t *testing.T) {
	uc := newTestUsecase(t, newFakeRepo())

	raw, err := uc.RequestPasswordReset(context.Background(), "tidak@ada.com")
	if err != nil || raw != "" {
		t.Fatalf("email tidak terdaftar harus diam-diam tanpa token, dapat %q, %v", raw, err)
	}
}

func TestResetPasswordRejectsPasswordLongerThan72Bytes(t *testing.T) {
	repo := newFakeRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	if _, err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	raw, _ := uc.RequestPasswordReset(ctx, "a@example.com")

	long := make([]byte, 73)
	for i := range long {
		long[i] = 'a'
	}
	if err := uc.ResetPassword(ctx, raw, string(long)); err == nil {
		t.Fatal("password 73 byte harus ditolak")
	}
}
