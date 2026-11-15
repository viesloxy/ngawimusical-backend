package service

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/app/model"
	"github.com/viesloxy/ngawimusical-backend/app/repository"
	"github.com/viesloxy/ngawimusical-backend/helper"
	"github.com/viesloxy/ngawimusical-backend/middleware"
)

// AuthService adalah lapisan HTTP untuk endpoint auth. Logika berada di AuthUsecase.
type AuthService struct {
	usecase *AuthUsecase
	users   repository.UserRepository
	tx      repository.TxManager
	tokens  *helper.TokenIssuer
	limiter middleware.Limiter
}

func NewAuthService(usecase *AuthUsecase, users repository.UserRepository, tx repository.TxManager, tokens *helper.TokenIssuer, limiter middleware.Limiter) *AuthService {
	return &AuthService{usecase: usecase, users: users, tx: tx, tokens: tokens, limiter: limiter}
}

// Register memasang seluruh route auth di bawah router. Dipanggil sekali dari wiring utama.
func (s *AuthService) Register(router fiber.Router) {
	auth := router.Group("/auth")
	auth.Post("/register", s.handleRegister)
	auth.Post("/login", s.limitLogin, s.handleLogin)
	auth.Post("/refresh", s.handleRefresh)
	auth.Post("/logout", s.handleLogout)
	auth.Get("/me", middleware.RequireAuth(s.verifyAccess), s.handleMe)
	auth.Post("/verify-email/request", middleware.RequireAuth(s.verifyAccess), s.handleRequestVerification)
	auth.Post("/verify-email", s.limitByIP("verify"), s.handleConfirmEmail)
	auth.Post("/password/forgot", s.limitByIP("forgot"), s.handleForgotPassword)
	auth.Post("/password/reset", s.limitByIP("reset"), s.handleResetPassword)
}

// Respons 204 untuk alur token aksi tidak membocorkan apakah akun atau token ada.
// Token mentah belum dikirim ke user karena layanan email belum ada; usecase hanya mengembalikannya ke pemanggil.
func (s *AuthService) handleRequestVerification(c *fiber.Ctx) error {
	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}
	if _, err := s.usecase.RequestEmailVerification(c.UserContext(), actor.UserID); err != nil {
		return err
	}
	return helper.NoContent(c)
}

func (s *AuthService) handleConfirmEmail(c *fiber.Ctx) error {
	var req model.VerifyEmailRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.NewAppError(http.StatusBadRequest, "MALFORMED_JSON", "Format request tidak valid.")
	}
	if err := helper.Validate(req); err != nil {
		return err
	}
	if err := s.usecase.ConfirmEmail(c.UserContext(), req.Token); err != nil {
		return err
	}
	return helper.NoContent(c)
}

func (s *AuthService) handleForgotPassword(c *fiber.Ctx) error {
	var req model.ForgotPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.NewAppError(http.StatusBadRequest, "MALFORMED_JSON", "Format request tidak valid.")
	}
	if err := helper.Validate(req); err != nil {
		return err
	}
	normalized := strings.ToLower(strings.TrimSpace(req.Email))
	// Batas ganda: per pasangan email|IP dan per email, agar penyerang dari banyak IP tetap terbatas.
	if err := s.enforceLimit(c, "forgot:pair:"+normalized+"|"+c.IP()); err != nil {
		return err
	}
	if err := s.enforceLimit(c, "forgot:email:"+normalized); err != nil {
		return err
	}
	if _, err := s.usecase.RequestPasswordReset(c.UserContext(), req.Email); err != nil {
		return err
	}
	return helper.NoContent(c)
}

func (s *AuthService) handleResetPassword(c *fiber.Ctx) error {
	var req model.ResetPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.NewAppError(http.StatusBadRequest, "MALFORMED_JSON", "Format request tidak valid.")
	}
	if err := helper.Validate(req); err != nil {
		return err
	}
	if err := s.usecase.ResetPassword(c.UserContext(), req.Token, req.NewPassword); err != nil {
		return err
	}
	return helper.NoContent(c)
}

func (s *AuthService) handleRegister(c *fiber.Ctx) error {
	var req model.RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.NewAppError(http.StatusBadRequest, "MALFORMED_JSON", "Format request tidak valid.")
	}
	if err := helper.Validate(req); err != nil {
		return err
	}

	session, err := s.usecase.Register(c.UserContext(), req)
	if err != nil {
		return err
	}
	return helper.Created(c, session)
}

func (s *AuthService) handleLogin(c *fiber.Ctx) error {
	var req model.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.NewAppError(http.StatusBadRequest, "MALFORMED_JSON", "Format request tidak valid.")
	}
	if err := helper.Validate(req); err != nil {
		return err
	}

	// Batas per pasangan (email, IP) membatasi tebakan dari satu sumber terhadap satu akun.
	// Kunci tidak dibagi dengan pengguna lain, sehingga tebakan orang luar tidak bisa
	// mengunci login pemilik akun dari IP-nya sendiri.
	// ponytail: botnet dengan banyak IP masih bisa menebak lebih cepat; tambahkan backoff atau CAPTCHA bila perlu.
	if err := s.enforceLimit(c, "login:pair:"+strings.ToLower(strings.TrimSpace(req.Email))+"|"+c.IP()); err != nil {
		return err
	}

	session, err := s.usecase.Login(c.UserContext(), req)
	if err != nil {
		return err
	}
	return helper.OK(c, session)
}

func (s *AuthService) handleRefresh(c *fiber.Ctx) error {
	var req model.RefreshRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.NewAppError(http.StatusBadRequest, "MALFORMED_JSON", "Format request tidak valid.")
	}
	if err := helper.Validate(req); err != nil {
		return err
	}

	session, err := s.usecase.Refresh(c.UserContext(), req.RefreshToken)
	if err != nil {
		return err
	}
	return helper.OK(c, session)
}

func (s *AuthService) handleLogout(c *fiber.Ctx) error {
	var req model.LogoutRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.NewAppError(http.StatusBadRequest, "MALFORMED_JSON", "Format request tidak valid.")
	}
	if err := helper.Validate(req); err != nil {
		return err
	}

	if err := s.usecase.Logout(c.UserContext(), req.RefreshToken); err != nil {
		return err
	}
	return helper.NoContent(c)
}

func (s *AuthService) handleMe(c *fiber.Ctx) error {
	actor, ok := middleware.GetActor(c)
	if !ok {
		return helper.Unauthenticated()
	}

	var user model.User
	err := s.tx.WithTxOnce(c.UserContext(), func(ctx context.Context, db repository.DBTX) error {
		var err error
		user, err = s.users.FindByID(ctx, db, actor.UserID)
		return err
	})
	if err != nil {
		return helper.Unauthenticated()
	}

	return helper.OK(c, model.ProfileResponse{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Role:     user.RoleName,
	})
}

// limitByIP membatasi jumlah request per IP untuk satu jenis aksi auth.
func (s *AuthService) limitByIP(action string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := s.enforceLimit(c, action+":ip:"+c.IP()); err != nil {
			return err
		}
		return c.Next()
	}
}

// limitLogin membatasi percobaan login per IP. IP diambil dari koneksi socket, sehingga
// header X-Forwarded-For hanya dipercaya bila wiring utama mengaktifkan trusted proxy.
func (s *AuthService) limitLogin(c *fiber.Ctx) error {
	if err := s.enforceLimit(c, "login:ip:"+c.IP()); err != nil {
		return err
	}
	return c.Next()
}

// enforceLimit memeriksa satu kunci limiter. Jika limiter gagal, request ditolak (fail closed).
func (s *AuthService) enforceLimit(c *fiber.Ctx, key string) error {
	allowed, wait, err := s.limiter.Allow(c.UserContext(), key)
	if err != nil {
		return helper.Unavailable("SYSTEM_BUSY", "Sistem sedang sibuk, coba lagi.")
	}
	if !allowed {
		c.Set(fiber.HeaderRetryAfter, strconv.Itoa(int(math.Ceil(wait.Seconds()))))
		return helper.NewAppError(http.StatusTooManyRequests, "RATE_LIMITED", "Terlalu banyak percobaan, coba lagi nanti.").WithRetryable(true)
	}
	return nil
}

// VerifyAccess membuka verifier untuk middleware route modul lain.
func (s *AuthService) VerifyAccess(ctx context.Context, raw string) (middleware.Actor, error) {
	return s.verifyAccess(ctx, raw)
}

// verifyAccess memeriksa JWT lalu memastikan akun masih aktif, auth_version belum berubah,
// dan membaca permission terbaru dari database.
func (s *AuthService) verifyAccess(ctx context.Context, raw string) (middleware.Actor, error) {
	claims, err := s.tokens.ParseAccess(raw)
	if err != nil {
		return middleware.Actor{}, err
	}

	var actor middleware.Actor
	err = s.tx.WithTxOnce(ctx, func(ctx context.Context, db repository.DBTX) error {
		user, err := s.users.FindByID(ctx, db, claims.UserID)
		if err != nil {
			return err
		}
		if model.LoginBlockReason(user.Status) != nil || user.AuthVersion != claims.AuthVersion {
			return errSessionInvalid
		}

		permissions, err := s.users.ListPermissions(ctx, db, user.ID)
		if err != nil {
			return err
		}
		actor = middleware.Actor{UserID: user.ID, Role: user.RoleName, Permissions: permissions}
		return nil
	})
	return actor, err
}
