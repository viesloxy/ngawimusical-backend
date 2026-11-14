package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/viesloxy/ngawimusical-backend/helper"
)

// RequestID membuat atau meneruskan request id untuk setiap request.
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID := strings.TrimSpace(c.Get("X-Request-ID"))
		if requestID == "" || len(requestID) > 64 {
			requestID = "req_" + uuid.NewString()
		}

		c.Set("X-Request-ID", requestID)
		c.Locals(helper.RequestIDKey, requestID)

		return c.Next()
	}
}
