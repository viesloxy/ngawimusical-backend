package helper_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/helper"
	"github.com/viesloxy/ngawimusical-backend/middleware"
)

type successEnvelope struct {
	Status string `json:"status"`
	Msg    string `json:"msg"`
	Data   struct {
		Message string `json:"message"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

type errorEnvelope struct {
	Status string `json:"status"`
	Msg    string `json:"msg"`
	Error  struct {
		Code      string            `json:"code"`
		Message   string            `json:"message"`
		Fields    map[string]string `json:"fields"`
		Retryable bool              `json:"retryable"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

func newTestApp(setup func(app *fiber.App)) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: helper.ErrorHandler})
	app.Use(middleware.RequestID())
	if setup != nil {
		setup(app)
	}
	return app
}

func TestSuccessResponseIncludesRequestID(t *testing.T) {
	app := newTestApp(func(app *fiber.App) {
		app.Get("/contoh", func(c *fiber.Ctx) error {
			return helper.OK(c, fiber.Map{"message": "halo"})
		})
	})

	req := httptest.NewRequest("GET", "/contoh", nil)
	req.Header.Set("X-Request-ID", "req_test_123")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	var body successEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "success" {
		t.Fatalf("status = %q, want success", body.Status)
	}
	if body.Msg != "Berhasil." {
		t.Fatalf("msg = %q, want Berhasil.", body.Msg)
	}
	if body.RequestID != "req_test_123" {
		t.Fatalf("request_id = %q, want req_test_123", body.RequestID)
	}
	if body.Data.Message != "halo" {
		t.Fatalf("message = %q, want halo", body.Data.Message)
	}
}

func TestErrorHandlerMapsValidationError(t *testing.T) {
	app := newTestApp(func(app *fiber.App) {
		app.Get("/gagal", func(c *fiber.Ctx) error {
			return helper.NewValidationError(map[string]string{"quantity": "Harus antara 1 dan 4."})
		})
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/gagal", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}

	var body errorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "error" {
		t.Fatalf("status = %q, want error", body.Status)
	}
	if body.Msg == "" || body.Msg != body.Error.Message {
		t.Fatalf("msg = %q, want sama dengan error.message %q", body.Msg, body.Error.Message)
	}
	if body.Error.Code != "VALIDATION_FAILED" {
		t.Fatalf("code = %q, want VALIDATION_FAILED", body.Error.Code)
	}
	if body.Error.Fields["quantity"] == "" {
		t.Fatal("fields.quantity kosong")
	}
	if body.RequestID == "" {
		t.Fatal("request_id kosong")
	}
}

func TestErrorHandlerHidesInternalError(t *testing.T) {
	app := newTestApp(func(app *fiber.App) {
		app.Get("/internal", func(c *fiber.Ctx) error {
			return errors.New("detail internal yang tidak boleh bocor")
		})
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/internal", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("baca response: %v", err)
	}
	if strings.Contains(string(raw), "detail internal") {
		t.Fatalf("response membocorkan detail internal: %s", raw)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

func TestValidateMapsFieldErrors(t *testing.T) {
	type registerInput struct {
		Email string `json:"email" validate:"required,email"`
		Age   int    `json:"age" validate:"gte=17"`
	}

	err := helper.Validate(registerInput{Email: "bukan-email", Age: 10})
	if err == nil {
		t.Fatal("Validate() error = nil, want error")
	}

	var appErr *helper.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error bukan AppError: %v", err)
	}
	if appErr.Code != "VALIDATION_FAILED" {
		t.Fatalf("code = %q, want VALIDATION_FAILED", appErr.Code)
	}
	if appErr.Fields["email"] == "" || appErr.Fields["age"] == "" {
		t.Fatalf("fields tidak lengkap: %#v", appErr.Fields)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	secret := []byte("rahasia-cursor")
	want := helper.Cursor{CreatedAt: time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC), ID: 42}

	encoded, err := helper.EncodeCursor(secret, want)
	if err != nil {
		t.Fatalf("EncodeCursor() error = %v", err)
	}

	got, err := helper.DecodeCursor(secret, encoded)
	if err != nil {
		t.Fatalf("DecodeCursor() error = %v", err)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || got.ID != want.ID {
		t.Fatalf("cursor = %#v, want %#v", got, want)
	}

	if _, err := helper.DecodeCursor([]byte("secret-lain"), encoded); !errors.Is(err, helper.ErrInvalidCursor) {
		t.Fatalf("DecodeCursor() error = %v, want ErrInvalidCursor", err)
	}
	if _, err := helper.DecodeCursor(secret, encoded+"x"); !errors.Is(err, helper.ErrInvalidCursor) {
		t.Fatalf("DecodeCursor() error = %v, want ErrInvalidCursor", err)
	}
}

func TestWriteCSVSanitizesFormula(t *testing.T) {
	app := newTestApp(func(app *fiber.App) {
		app.Get("/laporan.csv", func(c *fiber.Ctx) error {
			return helper.WriteCSV(c, "laporan.csv", []string{"nama"}, [][]string{
				{"=2+2"},
				{"aman"},
			})
		})
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/laporan.csv", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("baca response: %v", err)
	}
	if !strings.Contains(string(raw), "'=2+2") {
		t.Fatalf("formula tidak disanitasi: %s", raw)
	}
	if !strings.Contains(string(raw), "aman") {
		t.Fatalf("isi CSV tidak lengkap: %s", raw)
	}
}
