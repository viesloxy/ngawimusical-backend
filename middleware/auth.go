package middleware

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/helper"
)

// AccessVerifier memverifikasi access token mentah dan mengembalikan actor-nya.
// Implementasinya disuntikkan dari service agar middleware tidak tahu detail JWT.
type AccessVerifier func(ctx context.Context, rawToken string) (Actor, error)

// RequireAuth memastikan request membawa Bearer token yang valid, lalu mengisi Actor.
// Semua kegagalan dikembalikan sebagai 401 generik.
func RequireAuth(verify AccessVerifier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		raw, ok := bearerToken(c.Get(fiber.HeaderAuthorization))
		if !ok {
			return helper.Unauthenticated()
		}

		actor, err := verify(c.UserContext(), raw)
		if err != nil {
			return helper.Unauthenticated()
		}

		SetActor(c, actor)
		return c.Next()
	}
}

// RequirePermission menolak request bila actor tidak memiliki permission yang diminta.
// Tanpa actor, request ditolak (fail closed).
func RequirePermission(permission string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		actor, ok := GetActor(c)
		if !ok {
			return helper.Unauthenticated()
		}
		if !actor.HasPermission(permission) {
			return helper.Forbidden()
		}
		return c.Next()
	}
}

func bearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}
