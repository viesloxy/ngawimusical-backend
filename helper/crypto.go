package helper

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// ErrPasswordTooLong menandakan password melebihi batas 72 byte milik bcrypt.
var ErrPasswordTooLong = errors.New("password melebihi 72 byte")

const (
	bcryptMaxBytes    = 72
	refreshTokenBytes = 32
)

// HashPassword membuat hash bcrypt dengan cost yang diberikan pemanggil.
func HashPassword(password string, cost int) (string, error) {
	if len(password) > bcryptMaxBytes {
		return "", ErrPasswordTooLong
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// ComparePassword memeriksa password terhadap hash bcrypt.
func ComparePassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// NewRefreshToken membuat refresh token acak 32 byte dalam base64url.
// Nilai mentah hanya dikirim ke client, sedangkan server menyimpan HashRefreshToken-nya.
func NewRefreshToken() (string, error) {
	buf := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashRefreshToken menghasilkan SHA-256 dalam hex (64 karakter) untuk kolom token_hash.
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ErrTicketKey menandakan key tiket bukan 32 byte (AES-256).
var ErrTicketKey = errors.New("key tiket harus 32 byte")

// ErrTicketDecrypt dikembalikan untuk semua kegagalan dekripsi tanpa membocorkan penyebabnya.
var ErrTicketDecrypt = errors.New("token tiket tidak valid")

// SealTicketToken mengenkripsi token tiket dengan AES-256-GCM.
// Hasil berbentuk "kid:base64(nonce+ciphertext)"; kid ikut diikat sebagai data tambahan
// sehingga rotasi key cukup menambah key baru dan menyimpan key lama untuk dekripsi.
func SealTicketToken(keyID string, key []byte, plaintext string) (string, error) {
	gcm, err := newTicketGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), []byte(keyID))
	return keyID + ":" + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// OpenTicketToken mendekripsi token tiket memakai key sesuai kid pada nilai sealed.
// Keys berisi seluruh key aktif dan key lama yang masih dibutuhkan.
func OpenTicketToken(keys map[string][]byte, sealed string) (string, error) {
	keyID, encoded, ok := strings.Cut(sealed, ":")
	if !ok {
		return "", ErrTicketDecrypt
	}
	key, ok := keys[keyID]
	if !ok {
		return "", ErrTicketDecrypt
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrTicketDecrypt
	}
	gcm, err := newTicketGCM(key)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", ErrTicketDecrypt
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(keyID))
	if err != nil {
		return "", ErrTicketDecrypt
	}
	return string(plaintext), nil
}

func newTicketGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, ErrTicketKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// AccessClaims adalah isi JWT access token.
type AccessClaims struct {
	UserID      int64  `json:"uid"`
	Role        string `json:"role"`
	AuthVersion int    `json:"av"`
	jwt.RegisteredClaims
}

// AdmissionClaims adalah isi admission pass untuk satu event.
type AdmissionClaims struct {
	EventID    int64 `json:"eid"`
	UserID     int64 `json:"uid"`
	QueueEpoch int   `json:"qe"`
	jwt.RegisteredClaims
}

// TokenIssuer menandatangani dan memverifikasi JWT HS256.
// Audience ditentukan saat dibuat, sehingga access token dan admission pass
// memakai instance berbeda dan tidak bisa saling dipakai.
type TokenIssuer struct {
	secret   []byte
	issuer   string
	audience string
	ttl      time.Duration
}

func NewTokenIssuer(secret, issuer, audience string, ttl time.Duration) *TokenIssuer {
	return &TokenIssuer{secret: []byte(secret), issuer: issuer, audience: audience, ttl: ttl}
}

// SignAccess membuat JWT access token dengan user id, role, dan auth version.
func (t *TokenIssuer) SignAccess(userID int64, role string, authVersion int, now time.Time) (string, error) {
	claims := AccessClaims{
		UserID:           userID,
		Role:             role,
		AuthVersion:      authVersion,
		RegisteredClaims: t.registered(strconv.FormatInt(userID, 10), now),
	}
	return t.sign(claims)
}

// ParseAccess memverifikasi signature, algoritma, issuer, audience, dan expiry.
func (t *TokenIssuer) ParseAccess(raw string) (AccessClaims, error) {
	var claims AccessClaims
	if err := t.parse(raw, &claims); err != nil {
		return AccessClaims{}, err
	}
	return claims, nil
}

// SignAdmission membuat admission pass untuk satu user pada satu event.
// jti dipakai sebagai ID token agar bisa ditandai sekali pakai.
func (t *TokenIssuer) SignAdmission(eventID, userID int64, queueEpoch int, jti string, now time.Time) (string, error) {
	reg := t.registered(strconv.FormatInt(userID, 10), now)
	reg.ID = jti
	return t.sign(AdmissionClaims{EventID: eventID, UserID: userID, QueueEpoch: queueEpoch, RegisteredClaims: reg})
}

// ParseAdmission memverifikasi admission pass dengan aturan yang sama seperti access token.
func (t *TokenIssuer) ParseAdmission(raw string) (AdmissionClaims, error) {
	var claims AdmissionClaims
	if err := t.parse(raw, &claims); err != nil {
		return AdmissionClaims{}, err
	}
	return claims, nil
}

func (t *TokenIssuer) registered(subject string, now time.Time) jwt.RegisteredClaims {
	return jwt.RegisteredClaims{
		Issuer:    t.issuer,
		Subject:   subject,
		Audience:  jwt.ClaimStrings{t.audience},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
	}
}

func (t *TokenIssuer) sign(claims jwt.Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

func (t *TokenIssuer) parse(raw string, claims jwt.Claims) error {
	_, err := jwt.ParseWithClaims(raw, claims,
		func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(t.issuer),
		jwt.WithAudience(t.audience),
		jwt.WithExpirationRequired(),
	)
	return err
}
