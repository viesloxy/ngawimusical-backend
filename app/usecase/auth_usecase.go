package usecase

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/app/repository"
	"github.com/viesloxy/ngawimusical-backend/helper"
)

// AuthUsecaseConfig berisi parameter yang diberikan pemanggil; usecase tidak membaca environment.
type AuthUsecaseConfig struct {
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	BcryptCost int
}

// AuthUsecase berisi aturan register, login, rotasi refresh token, dan logout.
// Usecase tidak menerima *fiber.Ctx dan tidak menulis SQL.
type AuthUsecase struct {
	users     repository.UserRepository
	tx        repository.TxManager
	tokens    *helper.TokenIssuer
	cfg       AuthUsecaseConfig
	dummyHash string
}

// NewAuthUsecase menyiapkan usecase. Dummy hash dibuat sekali agar login ke akun
// yang tidak ada tetap memakan waktu bcrypt yang sama.
func NewAuthUsecase(users repository.UserRepository, tx repository.TxManager, tokens *helper.TokenIssuer, cfg AuthUsecaseConfig) (*AuthUsecase, error) {
	dummy, err := helper.HashPassword(uuid.NewString(), cfg.BcryptCost)
	if err != nil {
		return nil, err
	}
	return &AuthUsecase{users: users, tx: tx, tokens: tokens, cfg: cfg, dummyHash: dummy}, nil
}

// errInvalidCredentials dipakai untuk semua kegagalan login agar tidak membocorkan keberadaan akun.
var errInvalidCredentials = helper.NewAppError(http.StatusUnauthorized, "UNAUTHENTICATED", "Email atau password salah.")

// ErrSessionInvalid dikembalikan untuk refresh token yang tidak bisa dipakai.
var ErrSessionInvalid = helper.NewAppError(http.StatusUnauthorized, "UNAUTHENTICATED", "Sesi tidak valid, silakan login ulang.")

// Register membuat akun attendee baru. Tidak membuka sesi: login tetap lewat Login.
func (a *AuthUsecase) Register(ctx context.Context, req model.RegisterRequest) error {
	hash, err := helper.HashPassword(req.Password, a.cfg.BcryptCost)
	if err != nil {
		return helper.NewValidationError(map[string]string{"password": "Maksimal 72 byte."})
	}

	return a.tx.WithTx(ctx, func(ctx context.Context, db repository.DBTX) error {
		_, err := a.users.CreateUser(ctx, db, model.User{
			Username:     strings.TrimSpace(req.Username),
			Email:        strings.ToLower(strings.TrimSpace(req.Email)),
			PasswordHash: hash,
			RoleName:     model.RoleAttendee,
			Status:       model.AccountActive,
		})
		if errors.Is(err, repository.ErrConflict) {
			return helper.Conflict("CONFLICT", "Akun tidak dapat didaftarkan dengan data tersebut.")
		}
		return err
	})
}

// Login memeriksa kredensial dan status akun, lalu membuka sesi baru.
func (a *AuthUsecase) Login(ctx context.Context, req model.LoginRequest) (model.TokenResponse, error) {
	var session model.TokenResponse
	err := a.tx.WithTx(ctx, func(ctx context.Context, db repository.DBTX) error {
		user, err := a.users.FindByEmail(ctx, db, req.Email)
		if errors.Is(err, repository.ErrNotFound) {
			// Samakan waktu dengan jalur akun ada.
			helper.ComparePassword(a.dummyHash, req.Password)
			return errInvalidCredentials
		}
		if err != nil {
			return err
		}

		if !helper.ComparePassword(user.PasswordHash, req.Password) {
			return errInvalidCredentials
		}
		if model.LoginBlockReason(user.Status) != nil {
			return errInvalidCredentials
		}

		session, err = a.issueSession(ctx, db, user, uuid.NewString(), nil)
		return err
	})
	return session, err
}

// issueSession membuat sesi baru dan menyimpan refresh token-nya.
func (a *AuthUsecase) issueSession(ctx context.Context, db repository.DBTX, user model.User, family string, parentID *int64) (model.TokenResponse, error) {
	session, next, err := a.newSession(user, family, parentID)
	if err != nil {
		return model.TokenResponse{}, err
	}
	if _, err := a.users.CreateRefreshToken(ctx, db, next); err != nil {
		return model.TokenResponse{}, err
	}
	return session, nil
}

// newSession membuat access token JWT dan refresh token baru. Refresh token belum disimpan.
func (a *AuthUsecase) newSession(user model.User, family string, parentID *int64) (model.TokenResponse, model.RefreshToken, error) {
	now := time.Now().UTC()

	access, err := a.tokens.SignAccess(user.ID, user.RoleName, user.AuthVersion, now)
	if err != nil {
		return model.TokenResponse{}, model.RefreshToken{}, err
	}
	raw, err := helper.NewRefreshToken()
	if err != nil {
		return model.TokenResponse{}, model.RefreshToken{}, err
	}

	session := model.TokenResponse{
		AccessToken:  access,
		RefreshToken: raw,
		ExpiresIn:    int64(a.cfg.AccessTTL.Seconds()),
		ExpiresAt:    time.Now().Add(a.cfg.AccessTTL).UTC().Format(time.RFC3339),
	}
	next := model.RefreshToken{
		UserID:    user.ID,
		TokenHash: helper.HashRefreshToken(raw),
		FamilyID:  family,
		ParentID:  parentID,
		ExpiresAt: now.Add(a.cfg.RefreshTTL),
	}
	return session, next, nil
}

// Refresh menukar refresh token lama dengan sesi baru dalam satu transaksi.
// Jika token yang sudah terpakai dikirim lagi, seluruh family dicabut.
func (a *AuthUsecase) Refresh(ctx context.Context, raw string) (model.TokenResponse, error) {
	var session model.TokenResponse
	var family string
	reused := false

	err := a.tx.WithTx(ctx, func(ctx context.Context, db repository.DBTX) error {
		current, err := a.users.FindRefreshTokenByHash(ctx, db, helper.HashRefreshToken(raw))
		if errors.Is(err, repository.ErrNotFound) {
			return ErrSessionInvalid
		}
		if err != nil {
			return err
		}
		family = current.FamilyID

		if current.UsedAt != nil {
			reused = true
			return ErrSessionInvalid
		}
		if !current.IsUsable(time.Now().UTC()) {
			return ErrSessionInvalid
		}

		user, err := a.users.FindByID(ctx, db, current.UserID)
		if errors.Is(err, repository.ErrNotFound) {
			return ErrSessionInvalid
		}
		if err != nil {
			return err
		}
		if model.LoginBlockReason(user.Status) != nil {
			return ErrSessionInvalid
		}

		pair, next, err := a.newSession(user, current.FamilyID, &current.ID)
		if err != nil {
			return err
		}
		if _, err := a.users.RotateRefreshToken(ctx, db, current.ID, next); err != nil {
			if errors.Is(err, repository.ErrConflict) {
				// Dua permintaan paralel memakai token yang sama: yang kalah dianggap pencurian.
				reused = true
				return ErrSessionInvalid
			}
			return err
		}
		session = pair
		return nil
	})

	if reused {
		// Transaksi di atas sudah di-rollback, jadi pencabutan dilakukan terpisah.
		// Context dilepas dari pembatalan request agar klien yang menutup koneksi tidak
		// membatalkan pencabutan, tetapi tetap dibatasi waktu.
		revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if revokeErr := a.tx.WithTx(revokeCtx, func(ctx context.Context, db repository.DBTX) error {
			return a.users.RevokeRefreshFamily(ctx, db, family)
		}); revokeErr != nil {
			return model.TokenResponse{}, revokeErr
		}
		return model.TokenResponse{}, ErrSessionInvalid
	}
	return session, err
}

// Masa berlaku token aksi. Tetap di kode agar konfigurasi wiring tidak perlu berubah.
const (
	verifyEmailTTL   = 24 * time.Hour
	resetPasswordTTL = 30 * time.Minute
)

// errTokenInvalid dikembalikan untuk token aksi yang tidak ada, sudah dipakai, atau kedaluwarsa.
var errTokenInvalid = helper.NewAppError(http.StatusBadRequest, "TOKEN_INVALID", "Token tidak valid atau sudah kedaluwarsa.")

// RequestEmailVerification membuat token verifikasi email untuk user yang sedang login.
// Mengembalikan string kosong bila email sudah terverifikasi. Pengiriman email belum ada.
func (a *AuthUsecase) RequestEmailVerification(ctx context.Context, userID int64) (string, error) {
	var raw string
	err := a.tx.WithTx(ctx, func(ctx context.Context, db repository.DBTX) error {
		user, err := a.users.FindByID(ctx, db, userID)
		if errors.Is(err, repository.ErrNotFound) {
			return ErrSessionInvalid
		}
		if err != nil {
			return err
		}
		if user.EmailVerifiedAt != nil {
			return nil
		}
		raw, err = a.issueActionToken(ctx, db, user.ID, model.PurposeVerifyEmail, verifyEmailTTL)
		return err
	})
	return raw, err
}

// ConfirmEmail menandai email terverifikasi bila token valid, lalu menandai token itu terpakai.
func (a *AuthUsecase) ConfirmEmail(ctx context.Context, raw string) error {
	return a.tx.WithTx(ctx, func(ctx context.Context, db repository.DBTX) error {
		token, err := a.consumeActionToken(ctx, db, raw, model.PurposeVerifyEmail)
		if err != nil {
			return err
		}
		return a.users.MarkEmailVerified(ctx, db, token.UserID)
	})
}

// RequestPasswordReset membuat token reset password bila email terdaftar dan akun aktif.
// Untuk email yang tidak ada atau akun yang diblokir, tidak ada token dan tidak ada error,
// sehingga respons tidak membocorkan keberadaan akun.
func (a *AuthUsecase) RequestPasswordReset(ctx context.Context, email string) (string, error) {
	var raw string
	err := a.tx.WithTx(ctx, func(ctx context.Context, db repository.DBTX) error {
		user, err := a.users.FindByEmail(ctx, db, email)
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if model.LoginBlockReason(user.Status) != nil {
			return nil
		}
		raw, err = a.issueActionToken(ctx, db, user.ID, model.PurposeResetPassword, resetPasswordTTL)
		return err
	})
	return raw, err
}

// ResetPassword memakai token reset untuk mengganti password. Seluruh sesi user ikut dicabut.
func (a *AuthUsecase) ResetPassword(ctx context.Context, raw, newPassword string) error {
	hash, err := helper.HashPassword(newPassword, a.cfg.BcryptCost)
	if err != nil {
		return helper.NewValidationError(map[string]string{"new_password": "Maksimal 72 byte."})
	}

	return a.tx.WithTx(ctx, func(ctx context.Context, db repository.DBTX) error {
		token, err := a.consumeActionToken(ctx, db, raw, model.PurposeResetPassword)
		if err != nil {
			return err
		}
		user, err := a.users.FindByID(ctx, db, token.UserID)
		if errors.Is(err, repository.ErrNotFound) {
			return errTokenInvalid
		}
		if err != nil {
			return err
		}
		if model.LoginBlockReason(user.Status) != nil {
			return errTokenInvalid
		}
		return a.users.UpdatePassword(ctx, db, user.ID, hash)
	})
}

// issueActionToken menyimpan hash token aksi dan mengembalikan token mentahnya sekali saja.
func (a *AuthUsecase) issueActionToken(ctx context.Context, db repository.DBTX, userID int64, purpose string, ttl time.Duration) (string, error) {
	raw, err := helper.NewRefreshToken()
	if err != nil {
		return "", err
	}
	err = a.users.CreateIdentityActionToken(ctx, db, model.IdentityActionToken{
		UserID:    userID,
		TokenHash: helper.HashRefreshToken(raw),
		Purpose:   purpose,
		ExpiresAt: time.Now().UTC().Add(ttl),
	})
	return raw, err
}

// consumeActionToken menandai token terpakai secara atomik. Token yang tidak cocok ditolak generik.
func (a *AuthUsecase) consumeActionToken(ctx context.Context, db repository.DBTX, raw, purpose string) (model.IdentityActionToken, error) {
	token, err := a.users.ConsumeIdentityActionToken(ctx, db, helper.HashRefreshToken(raw), purpose, time.Now().UTC())
	if errors.Is(err, repository.ErrConflict) || errors.Is(err, repository.ErrNotFound) {
		return model.IdentityActionToken{}, errTokenInvalid
	}
	return token, err
}

// Logout mencabut seluruh family dari refresh token yang diberikan.
// Token yang tidak dikenal tetap dianggap berhasil agar tidak membocorkan informasi.
func (a *AuthUsecase) Logout(ctx context.Context, raw string) error {
	return a.tx.WithTx(ctx, func(ctx context.Context, db repository.DBTX) error {
		current, err := a.users.FindRefreshTokenByHash(ctx, db, helper.HashRefreshToken(raw))
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return a.users.RevokeRefreshFamily(ctx, db, current.FamilyID)
	})
}
