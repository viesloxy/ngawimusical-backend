package service_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/app/service"
	"github.com/viesloxy/ngawimusical-backend/helper"
	"github.com/viesloxy/ngawimusical-backend/middleware"
)

const testRegisterBody = `{"username":"arsalan","email":"a@example.com","password":"rahasia123"}`

// newAuthApp memasang route auth dengan repository dalam memori dan limiter yang diberikan.
func newAuthApp(t *testing.T, limiter middleware.Limiter) *fiber.App {
	t.Helper()

	repo := newFakeRepo()
	secret, err := helper.NewRefreshToken()
	if err != nil {
		t.Fatalf("buat secret acak: %v", err)
	}
	tokens := helper.NewTokenIssuer(secret, "ngawimusical", "ngawimusical-api", 15*time.Minute)
	uc, err := service.NewAuthUsecase(repo, fakeTx{}, tokens, service.AuthUsecaseConfig{
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 24 * time.Hour,
		BcryptCost: 4,
	})
	if err != nil {
		t.Fatalf("buat usecase: %v", err)
	}

	app := fiber.New(fiber.Config{ErrorHandler: helper.ErrorHandler})
	service.NewAuthService(uc, repo, fakeTx{}, tokens, limiter).Register(app.Group("/api/v1"))
	return app
}

func postJSON(t *testing.T, app *fiber.App, path, body string, headers map[string]string) (*http.Response, []byte) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return doRequest(t, app, req)
}

func doRequest(t *testing.T, app *fiber.App, req *http.Request) (*http.Response, []byte) {
	t.Helper()

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("jalankan request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

func errorCode(t *testing.T, body []byte) string {
	t.Helper()

	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("body bukan error envelope: %s", body)
	}
	return payload.Error.Code
}

func TestRegisterReturns201WithTokens(t *testing.T) {
	app := newAuthApp(t, middleware.NewMemoryLimiter(5, time.Minute))

	resp, body := postJSON(t, app, "/api/v1/auth/register", testRegisterBody, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register harus 201, dapat %d: %s", resp.StatusCode, body)
	}
	if !bytes.Contains(body, []byte(`"access_token"`)) || !bytes.Contains(body, []byte(`"refresh_token"`)) {
		t.Fatalf("respons register harus berisi token: %s", body)
	}
}

func TestRegisterWithInvalidPayloadReturns422(t *testing.T) {
	app := newAuthApp(t, middleware.NewMemoryLimiter(5, time.Minute))

	resp, _ := postJSON(t, app, "/api/v1/auth/register", `{"username":"arsalan","password":"rahasia123"}`, nil)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("email kosong harus 422, dapat %d", resp.StatusCode)
	}
}

func TestMeWithoutTokenReturns401(t *testing.T) {
	app := newAuthApp(t, middleware.NewMemoryLimiter(5, time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	resp, body := doRequest(t, app, req)
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "UNAUTHENTICATED" {
		t.Fatalf("/me tanpa token harus 401 UNAUTHENTICATED, dapat %d: %s", resp.StatusCode, body)
	}
}

func TestMeWithValidTokenReturns200(t *testing.T) {
	app := newAuthApp(t, middleware.NewMemoryLimiter(5, time.Minute))

	_, body := postJSON(t, app, "/api/v1/auth/register", testRegisterBody, nil)
	var session struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatalf("decode register: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+session.Data.AccessToken)
	resp, body := doRequest(t, app, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/me dengan token valid harus 200, dapat %d: %s", resp.StatusCode, body)
	}
}

func TestLoginWrongPasswordReturns401(t *testing.T) {
	app := newAuthApp(t, middleware.NewMemoryLimiter(5, time.Minute))
	postJSON(t, app, "/api/v1/auth/register", testRegisterBody, nil)

	resp, body := postJSON(t, app, "/api/v1/auth/login", `{"email":"a@example.com","password":"salah-sekali"}`, nil)
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "UNAUTHENTICATED" {
		t.Fatalf("login salah harus 401, dapat %d: %s", resp.StatusCode, body)
	}
}

func TestLoginOverLimitReturns429WithRetryAfter(t *testing.T) {
	app := newAuthApp(t, middleware.NewMemoryLimiter(5, time.Minute))
	headers := map[string]string{"X-Forwarded-For": "203.0.113.7"}
	attempt := `{"email":"a@example.com","password":"salah-sekali"}`

	for i := 0; i < 5; i++ {
		postJSON(t, app, "/api/v1/auth/login", attempt, headers)
	}

	resp, body := postJSON(t, app, "/api/v1/auth/login", attempt, headers)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("percobaan ke-6 harus 429, dapat %d: %s", resp.StatusCode, body)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("respons 429 harus menyertakan Retry-After")
	}
}
