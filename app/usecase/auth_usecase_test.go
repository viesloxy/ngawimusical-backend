package usecase_test

import (
	"context"
	"errors"
	"github.com/viesloxy/ngawimusical-backend/internal/testfake"
	"net/http"
	"testing"
	"time"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/app/usecase"
	"github.com/viesloxy/ngawimusical-backend/helper"
)

func newTestUsecase(t *testing.T, repo *testfake.UserRepo) *usecase.AuthUsecase {
	t.Helper()
	secret, err := helper.NewRefreshToken() // secret acak per test, bukan nilai tetap
	if err != nil {
		t.Fatalf("buat secret acak: %v", err)
	}
	issuer := helper.NewTokenIssuer(secret, "ngawimusical", "ngawimusical-api", 15*time.Minute)
	uc, err := usecase.NewAuthUsecase(repo, testfake.Tx{}, issuer, usecase.AuthUsecaseConfig{
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

func TestRegisterNormalizesEmailWithoutSession(t *testing.T) {
	repo := testfake.NewUserRepo()
	uc := newTestUsecase(t, repo)

	err := uc.Register(context.Background(), model.RegisterRequest{
		Username: "arsalan", Email: "  Arsalan@Example.COM ", Password: "rahasia123",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := repo.FindByEmail(context.Background(), nil, "arsalan@example.com"); err != nil {
		t.Fatalf("email harus tersimpan dalam huruf kecil: %v", err)
	}
}

func TestRegisterDuplicateEmailReturnsSpecificConflict(t *testing.T) {
	uc := newTestUsecase(t, testfake.NewUserRepo())
	req := model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"}

	if err := uc.Register(context.Background(), req); err != nil {
		t.Fatalf("register pertama: %v", err)
	}
	err := uc.Register(context.Background(), req)
	requireAppError(t, err, http.StatusConflict, "EMAIL_ALREADY_REGISTERED")
}

func TestRegisterDuplicateUsernameReturnsSpecificConflict(t *testing.T) {
	uc := newTestUsecase(t, testfake.NewUserRepo())
	if err := uc.Register(context.Background(), model.RegisterRequest{Username: "arsalan", Email: "a.com", Password: "rahasia123"}); err != nil {
		t.Fatalf("register pertama: %v", err)
	}
	err := uc.Register(context.Background(), model.RegisterRequest{Username: "arsalan", Email: "b.com", Password: "rahasia123"})
	requireAppError(t, err, http.StatusConflict, "USERNAME_TAKEN")
}

func TestLoginFailuresUseSameGenericMessage(t *testing.T) {
	repo := testfake.NewUserRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	if err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"}); err != nil {
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
	repo := testfake.NewUserRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	if err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	for id, u := range repo.Users {
		u.Status = model.AccountSuspended
		repo.Users[id] = u
	}

	_, err := uc.Login(ctx, model.LoginRequest{Email: "a@example.com", Password: "rahasia123"})
	requireAppError(t, err, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestReusedRefreshTokenRevokesWholeFamily(t *testing.T) {
	repo := testfake.NewUserRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	first, err := registerAndLogin(t, uc, ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"})
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
	uc := newTestUsecase(t, testfake.NewUserRepo())
	ctx := context.Background()

	session, err := registerAndLogin(t, uc, ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"})
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
	repo := testfake.NewUserRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	userID := repo.Users[1].ID

	raw, err := uc.RequestEmailVerification(ctx, userID)
	if err != nil || raw == "" {
		t.Fatalf("minta verifikasi harus memberi token: %v", err)
	}
	if err := uc.ConfirmEmail(ctx, raw); err != nil {
		t.Fatalf("konfirmasi pertama: %v", err)
	}
	if repo.Users[userID].EmailVerifiedAt == nil {
		t.Fatal("email harus bertanda terverifikasi")
	}

	requireAppError(t, uc.ConfirmEmail(ctx, raw), http.StatusBadRequest, "TOKEN_INVALID")
}

func TestEmailVerificationSkippedWhenAlreadyVerified(t *testing.T) {
	repo := testfake.NewUserRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	if err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	userID := repo.Users[1].ID
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
	repo := testfake.NewUserRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	old, err := registerAndLogin(t, uc, ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"})
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
	repo := testfake.NewUserRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	if err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	raw, _ := uc.RequestPasswordReset(ctx, "a@example.com")
	if err := uc.ResetPassword(ctx, raw, "passwordbaru1"); err != nil {
		t.Fatalf("reset pertama: %v", err)
	}
	requireAppError(t, uc.ResetPassword(ctx, raw, "passwordketiga1"), http.StatusBadRequest, "TOKEN_INVALID")
}

func TestPasswordResetForUnknownEmailReturnsNothing(t *testing.T) {
	uc := newTestUsecase(t, testfake.NewUserRepo())

	raw, err := uc.RequestPasswordReset(context.Background(), "tidak@ada.com")
	if err != nil || raw != "" {
		t.Fatalf("email tidak terdaftar harus diam-diam tanpa token, dapat %q, %v", raw, err)
	}
}

func TestResetPasswordRejectsPasswordLongerThan72Bytes(t *testing.T) {
	repo := testfake.NewUserRepo()
	uc := newTestUsecase(t, repo)
	ctx := context.Background()

	if err := uc.Register(ctx, model.RegisterRequest{Username: "arsalan", Email: "a@example.com", Password: "rahasia123"}); err != nil {
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

// registerAndLogin mendaftarkan akun lalu login, untuk tes yang butuh sesi.
func registerAndLogin(t *testing.T, uc *usecase.AuthUsecase, ctx context.Context, req model.RegisterRequest) (model.TokenResponse, error) {
	t.Helper()
	if err := uc.Register(ctx, req); err != nil {
		t.Fatalf("register: %v", err)
	}
	return uc.Login(ctx, model.LoginRequest{Email: req.Email, Password: req.Password})
}
