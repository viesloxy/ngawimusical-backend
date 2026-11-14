package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/app/service"
)

type healthEnvelope struct {
	Data struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	} `json:"data"`
}

func TestHealthServiceLive(t *testing.T) {
	app := fiber.New()
	health := service.NewHealthService("test")
	app.Get("/health/live", health.Live)

	resp, err := app.Test(httptest.NewRequest("GET", "/health/live", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestHealthServiceReadyWithAllDependenciesUp(t *testing.T) {
	app := fiber.New()
	health := service.NewHealthService("test",
		service.DependencyCheck{Name: "postgres", Check: func(ctx context.Context) error { return nil }},
		service.DependencyCheck{Name: "redis", Check: func(ctx context.Context) error { return nil }},
	)
	app.Get("/health/ready", health.Ready)

	resp, err := app.Test(httptest.NewRequest("GET", "/health/ready", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body healthEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Status != "ok" {
		t.Fatalf("status = %q, want ok", body.Data.Status)
	}
	if body.Data.Checks["postgres"] != "up" {
		t.Fatalf("postgres = %q, want up", body.Data.Checks["postgres"])
	}
}

func TestHealthServiceReadyWithDependencyDown(t *testing.T) {
	app := fiber.New()
	health := service.NewHealthService("test",
		service.DependencyCheck{Name: "postgres", Check: func(ctx context.Context) error { return nil }},
		service.DependencyCheck{Name: "redis", Check: func(ctx context.Context) error { return errors.New("mati") }},
	)
	app.Get("/health/ready", health.Ready)

	resp, err := app.Test(httptest.NewRequest("GET", "/health/ready", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}

	var body healthEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Status != "degraded" {
		t.Fatalf("status = %q, want degraded", body.Data.Status)
	}
	if body.Data.Checks["redis"] != "down" {
		t.Fatalf("redis = %q, want down", body.Data.Checks["redis"])
	}
}
