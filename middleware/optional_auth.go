package middleware

import "github.com/gofiber/fiber/v2"

// OptionalAuth mengisi Actor bila request membawa Bearer token yang valid.
// Request tanpa token atau dengan token tidak valid tetap dilanjutkan, lalu
// handler yang membutuhkan aktor mengembalikan 401 sendiri.
func OptionalAuth(verify AccessVerifier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		raw, ok := bearerToken(c.Get(fiber.HeaderAuthorization))
		if ok {
			if actor, err := verify(c.UserContext(), raw); err == nil {
				SetActor(c, actor)
			}
		}
		return c.Next()
	}
}
