package middleware_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/middleware"
)

func TestActorContext(t *testing.T) {
	app := fiber.New()
	app.Get("/aktor", func(c *fiber.Ctx) error {
		middleware.SetActor(c, middleware.Actor{
			UserID:      7,
			Role:        "organizer",
			Permissions: []string{"event:update:own"},
		})

		actor, ok := middleware.GetActor(c)
		if !ok {
			return c.Status(fiber.StatusInternalServerError).SendString("aktor tidak ditemukan")
		}
		if !actor.HasPermission("event:update:own") {
			return c.Status(fiber.StatusForbidden).SendString("permission tidak cocok")
		}
		if actor.HasPermission("event:update:any") {
			return c.Status(fiber.StatusForbidden).SendString("permission tidak boleh ada")
		}

		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/aktor", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
