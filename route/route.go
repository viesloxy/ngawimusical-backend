package route

import (
	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/app/service"
)

// Register mendaftarkan seluruh endpoint HTTP.
// Endpoint auth, event, booking, payment, dan gate ditambahkan setelah modulnya tersedia.
func Register(app *fiber.App, health *service.HealthService) {
	app.Get("/health/live", health.Live)
	app.Get("/health/ready", health.Ready)

	api := app.Group("/api/v1")
	api.Get("/health/live", health.Live)
	api.Get("/health/ready", health.Ready)
}
