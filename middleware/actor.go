package middleware

import "github.com/gofiber/fiber/v2"

// Actor adalah identitas pengguna yang sudah lolos autentikasi.
// Auth middleware mengisi nilai ini, lalu service lain membacanya tanpa
// bergantung langsung pada implementasi JWT.
type Actor struct {
	UserID      int64
	Role        string
	Permissions []string
}

const ActorKey = "actor"

func SetActor(c *fiber.Ctx, actor Actor) {
	c.Locals(ActorKey, actor)
}

func GetActor(c *fiber.Ctx) (Actor, bool) {
	actor, ok := c.Locals(ActorKey).(Actor)
	return actor, ok
}

func (a Actor) HasPermission(permission string) bool {
	for _, owned := range a.Permissions {
		if owned == "*" || owned == permission {
			return true
		}
	}
	return false
}
