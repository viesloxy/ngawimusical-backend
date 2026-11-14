package helper

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalidCursor = errors.New("cursor tidak valid")

// Cursor adalah posisi halaman berbasis created_at dan id.
type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
}

// EncodeCursor menandatangani cursor agar tidak bisa diubah client.
func EncodeCursor(secret []byte, cursor Cursor) (string, error) {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}

	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := signCursor(secret, encoded)

	return encoded + "." + signature, nil
}

// DecodeCursor memeriksa tanda tangan dan mengembalikan isi cursor.
func DecodeCursor(secret []byte, raw string) (Cursor, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return Cursor{}, ErrInvalidCursor
	}

	expected := signCursor(secret, parts[0])
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return Cursor{}, ErrInvalidCursor
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}

	var cursor Cursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	if cursor.CreatedAt.IsZero() || cursor.ID <= 0 {
		return Cursor{}, ErrInvalidCursor
	}

	return cursor, nil
}

func signCursor(secret []byte, payload string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
