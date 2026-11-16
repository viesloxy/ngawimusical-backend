package route_test

import (
	"context"
	"github.com/viesloxy/ngawimusical-backend/app/usecase"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/app/repository"
	"github.com/viesloxy/ngawimusical-backend/app/service"
	"github.com/viesloxy/ngawimusical-backend/helper"
	"github.com/viesloxy/ngawimusical-backend/middleware"
	"github.com/viesloxy/ngawimusical-backend/route"
)

type noopTxManager struct{}

func (noopTxManager) WithTx(ctx context.Context, fn repository.TxFunc) error {
	return nil
}

func (noopTxManager) WithTxOnce(ctx context.Context, fn repository.TxFunc) error {
	return nil
}

func TestRegisterRoutes(t *testing.T) {
	app := fiber.New()
	health := service.NewHealthService("test")
	tx := noopTxManager{}
	tokens := helper.NewTokenIssuer("secret-test", "ngawimusical", "ngawimusical-api", time.Minute)

	authUsecase, err := usecase.NewAuthUsecase(
		repository.NewUserRepository(),
		tx,
		tokens,
		usecase.AuthUsecaseConfig{
			AccessTTL:  time.Minute,
			RefreshTTL: time.Hour,
			BcryptCost: 4,
		},
	)
	if err != nil {
		t.Fatalf("NewAuthUsecase() error = %v", err)
	}

	authService := service.NewAuthService(
		authUsecase,
		repository.NewUserRepository(),
		tx,
		tokens,
		middleware.NewMemoryLimiter(5, time.Minute),
	)
	eventService := service.NewEventService(
		usecase.NewEventUsecase(tx, nil, nil, nil, nil, nil),
		[]byte("secret-test"),
	)
	bookingService := service.NewBookingService(
		usecase.NewBookingUsecase(tx, nil, nil, nil, nil, nil, nil, nil, nil, time.Minute, time.Minute),
		[]byte("secret-test"),
	)

	route.Register(app, health, authService, eventService, bookingService)

	registered := make(map[string]bool)
	for _, item := range app.GetRoutes() {
		registered[item.Method+" "+item.Path] = true
	}

	expected := []string{
		"GET /api/v1/health/live",
		"POST /api/v1/auth/register",
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/refresh",
		"POST /api/v1/auth/logout",
		"GET /api/v1/auth/me",
		"GET /api/v1/events",
		"POST /api/v1/events",
		"GET /api/v1/events/:id",
		"POST /api/v1/bookings",
		"GET /api/v1/bookings",
		"POST /api/v1/bookings/:id/cancel",
	}

	for _, path := range expected {
		if !registered[path] {
			t.Errorf("route %q tidak terdaftar", path)
		}
	}
}
