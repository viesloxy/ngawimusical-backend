package helper_test

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/viesloxy/ngawimusical-backend/helper"
)

const (
	testIssuer     = "ngawimusical"
	testAccessAud  = "ngawimusical-api"
	testAdmitAud   = "ngawimusical-admission"
	testBcryptCost = 4 // cost terendah agar test cepat, produksi memakai BCRYPT_COST
)

// randomKey membuat secret acak per test agar tidak ada nilai rahasia di kode test.
func randomKey(t *testing.T, size int) []byte {
	t.Helper()
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("buat key acak: %v", err)
	}
	return buf
}

func TestPasswordHashAndCompare(t *testing.T) {
	hash, err := helper.HashPassword("rahasia123", testBcryptCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	if !helper.ComparePassword(hash, "rahasia123") {
		t.Fatal("password benar harus cocok dengan hash")
	}
	if helper.ComparePassword(hash, "salah") {
		t.Fatal("password salah tidak boleh cocok")
	}
}

func TestPasswordLongerThan72BytesIsRejected(t *testing.T) {
	_, err := helper.HashPassword(strings.Repeat("a", 73), testBcryptCost)
	if !errors.Is(err, helper.ErrPasswordTooLong) {
		t.Fatalf("password 73 byte harus ditolak, dapat: %v", err)
	}
}

func TestAccessTokenRoundTrip(t *testing.T) {
	secret := string(randomKey(t, 32))
	issuer := helper.NewTokenIssuer(secret, testIssuer, testAccessAud, 15*time.Minute)

	raw, err := issuer.SignAccess(42, "attendee", 3, time.Now())
	if err != nil {
		t.Fatalf("sign access: %v", err)
	}
	claims, err := issuer.ParseAccess(raw)
	if err != nil {
		t.Fatalf("parse access: %v", err)
	}
	if claims.UserID != 42 || claims.Role != "attendee" || claims.AuthVersion != 3 {
		t.Fatalf("klaim tidak sesuai: %+v", claims)
	}
}

func TestAccessTokenRejections(t *testing.T) {
	secret := string(randomKey(t, 32))
	now := time.Now()

	valid := helper.NewTokenIssuer(secret, testIssuer, testAccessAud, 15*time.Minute)
	expired := helper.NewTokenIssuer(secret, testIssuer, testAccessAud, -time.Minute)
	otherAudience := helper.NewTokenIssuer(secret, testIssuer, "api-lain", 15*time.Minute)
	otherSecret := helper.NewTokenIssuer(string(randomKey(t, 32)), testIssuer, testAccessAud, 15*time.Minute)

	tests := []struct {
		name  string
		issue func() (string, error)
	}{
		{"kedaluwarsa", func() (string, error) { return expired.SignAccess(1, "attendee", 1, now) }},
		{"audience salah", func() (string, error) { return otherAudience.SignAccess(1, "attendee", 1, now) }},
		{"signature salah", func() (string, error) { return otherSecret.SignAccess(1, "attendee", 1, now) }},
		{"algoritma di luar allowlist", func() (string, error) {
			claims := helper.AccessClaims{UserID: 1, Role: "admin", AuthVersion: 1}
			claims.Issuer = testIssuer
			claims.Audience = jwt.ClaimStrings{testAccessAud}
			claims.ExpiresAt = jwt.NewNumericDate(now.Add(time.Minute))
			return jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString([]byte(secret))
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := tt.issue()
			if err != nil {
				t.Fatalf("siapkan token: %v", err)
			}
			if _, err := valid.ParseAccess(raw); err == nil {
				t.Fatal("token ini harus ditolak")
			}
		})
	}
}

func TestRefreshTokenHashDiffersFromRaw(t *testing.T) {
	raw, err := helper.NewRefreshToken()
	if err != nil {
		t.Fatalf("buat refresh token: %v", err)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) < 32 {
		t.Fatalf("refresh token harus minimal 32 byte acak, dapat %d byte (err=%v)", len(decoded), err)
	}

	hash := helper.HashRefreshToken(raw)
	if hash == raw {
		t.Fatal("hash tidak boleh sama dengan token mentah")
	}
	if len(hash) != 64 {
		t.Fatalf("hash SHA-256 hex harus 64 karakter, dapat %d", len(hash))
	}
	if _, err := hex.DecodeString(hash); err != nil {
		t.Fatalf("hash harus hex: %v", err)
	}
}

func TestTicketTokenSealAndOpen(t *testing.T) {
	key := randomKey(t, 32)
	sealed, err := helper.SealTicketToken("k1", key, "TIKET-123")
	if err != nil {
		t.Fatalf("seal tiket: %v", err)
	}

	plain, err := helper.OpenTicketToken(map[string][]byte{"k1": key}, sealed)
	if err != nil || plain != "TIKET-123" {
		t.Fatalf("open tiket: plain=%q err=%v", plain, err)
	}
}

func TestTicketTokenTamperedCiphertextFails(t *testing.T) {
	key := randomKey(t, 32)
	sealed, err := helper.SealTicketToken("k1", key, "TIKET-123")
	if err != nil {
		t.Fatalf("seal tiket: %v", err)
	}

	// Ubah satu karakter di bagian terakhir ciphertext (sebelum tag autentikasi).
	kid, encoded, _ := strings.Cut(sealed, ":")
	raw, _ := base64.RawURLEncoding.DecodeString(encoded)
	raw[len(raw)-1] ^= 0x01
	tampered := kid + ":" + base64.RawURLEncoding.EncodeToString(raw)

	if _, err := helper.OpenTicketToken(map[string][]byte{"k1": key}, tampered); err == nil {
		t.Fatal("ciphertext yang diubah harus gagal didekripsi")
	}
}

func TestTicketTokenKeyRotation(t *testing.T) {
	oldKey := randomKey(t, 32)
	newKey := randomKey(t, 32)

	sealedOld, err := helper.SealTicketToken("k1", oldKey, "TIKET-LAMA")
	if err != nil {
		t.Fatalf("seal dengan key lama: %v", err)
	}

	// Setelah rotasi, key lama tetap disimpan untuk membaca data lama.
	keys := map[string][]byte{"k1": oldKey, "k2": newKey}
	if plain, err := helper.OpenTicketToken(keys, sealedOld); err != nil || plain != "TIKET-LAMA" {
		t.Fatalf("token lama harus tetap terbaca setelah rotasi: plain=%q err=%v", plain, err)
	}

	sealedNew, err := helper.SealTicketToken("k2", newKey, "TIKET-BARU")
	if err != nil {
		t.Fatalf("seal dengan key baru: %v", err)
	}
	if _, err := helper.OpenTicketToken(map[string][]byte{"k1": oldKey}, sealedNew); err == nil {
		t.Fatal("token key baru tidak boleh terbuka dengan keyring tanpa key baru")
	}
}

func TestTicketKeyMustBe32Bytes(t *testing.T) {
	if _, err := helper.SealTicketToken("k1", randomKey(t, 16), "x"); !errors.Is(err, helper.ErrTicketKey) {
		t.Fatalf("key 16 byte harus ditolak, dapat: %v", err)
	}
}

func TestAdmissionPassRoundTripAndExpiry(t *testing.T) {
	secret := string(randomKey(t, 32))
	now := time.Now()
	issuer := helper.NewTokenIssuer(secret, testIssuer, testAdmitAud, time.Minute)

	raw, err := issuer.SignAdmission(7, 42, 3, "jti-abc", now)
	if err != nil {
		t.Fatalf("sign admission: %v", err)
	}
	claims, err := issuer.ParseAdmission(raw)
	if err != nil {
		t.Fatalf("parse admission: %v", err)
	}
	if claims.EventID != 7 || claims.UserID != 42 || claims.QueueEpoch != 3 || claims.ID != "jti-abc" {
		t.Fatalf("klaim admission tidak sesuai: %+v", claims)
	}

	expired := helper.NewTokenIssuer(secret, testIssuer, testAdmitAud, -time.Minute)
	rawExpired, err := expired.SignAdmission(7, 42, 3, "jti-lama", now)
	if err != nil {
		t.Fatalf("sign admission kedaluwarsa: %v", err)
	}
	if _, err := issuer.ParseAdmission(rawExpired); err == nil {
		t.Fatal("admission pass kedaluwarsa harus ditolak")
	}
}
