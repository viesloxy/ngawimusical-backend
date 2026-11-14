package config

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoadReadsEnvironment(t *testing.T) {
	t.Setenv("APP_PORT", "9090")
	t.Setenv("DB_NAME", "ngawimusical_test")
	t.Setenv("HOLD_TTL", "5m")
	t.Setenv("JWT_ACCESS_TTL", "20m")
	t.Setenv("METRICS_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.App.Port != 9090 {
		t.Fatalf("App.Port = %d, want 9090", cfg.App.Port)
	}
	if cfg.DB.Name != "ngawimusical_test" {
		t.Fatalf("DB.Name = %q, want ngawimusical_test", cfg.DB.Name)
	}
	if cfg.Business.HoldTTL != 5*time.Minute {
		t.Fatalf("HoldTTL = %v, want 5m", cfg.Business.HoldTTL)
	}
	if cfg.Auth.AccessTTL != 20*time.Minute {
		t.Fatalf("AccessTTL = %v, want 20m", cfg.Auth.AccessTTL)
	}
	if cfg.Observability.MetricsEnabled {
		t.Fatal("MetricsEnabled = true, want false")
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("HOLD_TTL", "sepuluh menit")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	t.Setenv("APP_PORT", "70000")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestDBConfigDSN(t *testing.T) {
	cfg := DBConfig{
		Host:     "127.0.0.1",
		Port:     "5432",
		User:     "ngawimusical",
		Password: "p@ss word",
		Name:     "ngawimusical",
		SSLMode:  "disable",
	}

	parsed, err := url.Parse(cfg.DSN())
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	if parsed.Scheme != "postgres" {
		t.Fatalf("scheme = %q, want postgres", parsed.Scheme)
	}
	if parsed.Host != "127.0.0.1:5432" {
		t.Fatalf("host = %q, want 127.0.0.1:5432", parsed.Host)
	}
	if parsed.User.Username() != "ngawimusical" {
		t.Fatalf("user = %q, want ngawimusical", parsed.User.Username())
	}
	password, _ := parsed.User.Password()
	if password != "p@ss word" {
		t.Fatalf("password = %q, want p@ss word", password)
	}
	if !strings.HasSuffix(parsed.Path, "/ngawimusical") {
		t.Fatalf("path = %q, want /ngawimusical", parsed.Path)
	}
	if parsed.Query().Get("sslmode") != "disable" {
		t.Fatalf("sslmode = %q, want disable", parsed.Query().Get("sslmode"))
	}
}
