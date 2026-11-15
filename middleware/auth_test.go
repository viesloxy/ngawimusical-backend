package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/helper"
	"github.com/viesloxy/ngawimusical-backend/middleware"
)

// newProtectedApp memasang route yang butuh auth dan permission "event:update:own".
func newProtectedApp(actor middleware.Actor, verifyErr error) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: helper.ErrorHandler})
	verify := func(_ context.Context, raw string) (middleware.Actor, error) {
		if raw != "token-valid" || verifyErr != nil {
			return middleware.Actor{}, errors.New("token ditolak")
		}
		return actor, nil
	}
	app.Get("/protected",
		middleware.RequireAuth(verify),
		middleware.RequirePermission("event:update:own"),
		func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) },
	)
	return app
}

func statusFor(t *testing.T, app *fiber.App, authHeader string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("jalankan request: %v", err)
	}
	return resp.StatusCode
}

func TestRequireAuthRejectsMissingOrInvalidToken(t *testing.T) {
	app := newProtectedApp(middleware.Actor{Permissions: []string{"*"}}, nil)

	if got := statusFor(t, app, ""); got != http.StatusUnauthorized {
		t.Fatalf("tanpa token harus 401, dapat %d", got)
	}
	if got := statusFor(t, app, "Bearer token-salah"); got != http.StatusUnauthorized {
		t.Fatalf("token tidak valid harus 401, dapat %d", got)
	}
}

func TestRequirePermissionReturns403WithoutScope(t *testing.T) {
	app := newProtectedApp(middleware.Actor{UserID: 1, Role: "attendee", Permissions: []string{"booking:read:own"}}, nil)

	if got := statusFor(t, app, "Bearer token-valid"); got != http.StatusForbidden {
		t.Fatalf("permission kurang harus 403, dapat %d", got)
	}
}

func TestRequirePermissionAllowsMatchingScope(t *testing.T) {
	app := newProtectedApp(middleware.Actor{UserID: 1, Role: "organizer", Permissions: []string{"event:update:own"}}, nil)

	if got := statusFor(t, app, "Bearer token-valid"); got != http.StatusOK {
		t.Fatalf("permission sesuai harus 200, dapat %d", got)
	}
}
