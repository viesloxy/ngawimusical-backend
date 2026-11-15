package route

import (
	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/app/service"
	"github.com/viesloxy/ngawimusical-backend/middleware"
)

// Register mendaftarkan seluruh endpoint HTTP.
// Service auth, event, dan booking mendaftarkan route masing-masing.
func Register(
	app *fiber.App,
	health *service.HealthService,
	auth *service.AuthService,
	event *service.EventService,
	booking *service.BookingService,
) {
	app.Get("/health/live", health.Live)
	app.Get("/health/ready", health.Ready)

	api := app.Group("/api/v1")
	api.Get("/health/live", health.Live)
	api.Get("/health/ready", health.Ready)

	auth.Register(api)

	// Endpoint event dan booking boleh diakses tanpa token untuk operasi publik.
	// Middleware ini hanya mengisi Actor bila token valid, lalu handler memutuskan
	// apakah operasi tersebut membutuhkan login.
	modules := api.Group("", middleware.OptionalAuth(auth.VerifyAccess))
	event.Register(modules)
	booking.Register(modules)
}
