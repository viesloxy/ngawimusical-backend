package service

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/viesloxy/ngawimusical-backend/helper"
)

// DependencyCheck adalah pemeriksaan satu dependency pada readiness probe.
type DependencyCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// HealthService menangani liveness dan readiness API.
type HealthService struct {
	version   string
	startedAt time.Time
	checks    []DependencyCheck
}

func NewHealthService(version string, checks ...DependencyCheck) *HealthService {
	return &HealthService{
		version:   version,
		startedAt: time.Now().UTC(),
		checks:    checks,
	}
}

func (h *HealthService) Live(c *fiber.Ctx) error {
	return helper.OK(c, fiber.Map{
		"status":  "ok",
		"version": h.version,
		"time":    time.Now().UTC(),
	})
}

func (h *HealthService) Ready(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
	defer cancel()

	results := make(map[string]string, len(h.checks))
	ready := true

	for _, check := range h.checks {
		if err := check.Check(ctx); err != nil {
			results[check.Name] = "down"
			ready = false
			continue
		}
		results[check.Name] = "up"
	}

	payload := fiber.Map{
		"status":         "ok",
		"version":        h.version,
		"checks":         results,
		"uptime_seconds": int64(time.Since(h.startedAt).Seconds()),
	}

	if !ready {
		payload["status"] = "degraded"
		return helper.JSON(c, fiber.StatusServiceUnavailable, payload)
	}

	return helper.OK(c, payload)
}
