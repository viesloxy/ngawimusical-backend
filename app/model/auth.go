package model

// RegisterRequest adalah payload pendaftaran akun baru.
type RegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
	Email    string `json:"email" validate:"required,email,max=120"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

// LoginRequest adalah payload login dengan email dan password.
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email,max=120"`
	Password string `json:"password" validate:"required,max=72"`
}

// RefreshRequest adalah payload untuk menukar refresh token dengan sesi baru.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// LogoutRequest adalah payload untuk mencabut sesi.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// VerifyEmailRequest adalah payload konfirmasi email.
type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required"`
}

// ResetPasswordRequest adalah payload pengaturan ulang password.
type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8,max=72"`
}

// TokenResponse adalah respons berisi sepasang token sesi.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// ProfileResponse adalah data profil aktor yang sedang login.
type ProfileResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}
