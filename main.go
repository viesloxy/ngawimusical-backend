package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/viesloxy/ngawimusical-backend/app/usecase"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/viesloxy/ngawimusical-backend/app/repository"
	"github.com/viesloxy/ngawimusical-backend/app/service"
	"github.com/viesloxy/ngawimusical-backend/config"
	"github.com/viesloxy/ngawimusical-backend/database"
	"github.com/viesloxy/ngawimusical-backend/helper"
	"github.com/viesloxy/ngawimusical-backend/middleware"
	"github.com/viesloxy/ngawimusical-backend/route"
	"github.com/viesloxy/ngawimusical-backend/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("aplikasi berhenti", "error", err)
		os.Exit(1)
	}
}

func run() error {
	roleFlag := flag.String("role", "all", "proses yang dijalankan: api, worker, scheduler, all")
	versionFlag := flag.String("version", "dev", "versi aplikasi")
	flag.Parse()

	role := strings.ToLower(strings.TrimSpace(*roleFlag))
	if !validRole(role) {
		return fmt.Errorf("role %q tidak dikenal, pilih api, worker, scheduler, atau all", role)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg.App.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgres(ctx, cfg.DB)
	if err != nil {
		return fmt.Errorf("siapkan postgres: %w", err)
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("jalankan migrasi: %w", err)
	}

	admissionRedis, err := database.NewAdmissionRedis(ctx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("siapkan redis admission: %w", err)
	}
	defer func() {
		_ = admissionRedis.Close()
	}()

	queueRedis, err := database.NewQueueRedis(ctx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("siapkan redis queue: %w", err)
	}
	defer func() {
		_ = queueRedis.Close()
	}()

	mongoClient, err := database.NewMongo(ctx, cfg.Mongo)
	if err != nil {
		return fmt.Errorf("siapkan mongodb: %w", err)
	}
	defer func() {
		_ = mongoClient.Disconnect(context.Background())
	}()

	if err := database.EnsureReviewIndexes(ctx, mongoClient.Database(cfg.Mongo.Database)); err != nil {
		return fmt.Errorf("siapkan index ulasan: %w", err)
	}

	health := service.NewHealthService(
		*versionFlag,
		service.DependencyCheck{Name: "postgres", Check: pool.Ping},
		service.DependencyCheck{Name: "redis_admission", Check: func(ctx context.Context) error {
			return admissionRedis.Ping(ctx).Err()
		}},
		service.DependencyCheck{Name: "redis_queue", Check: func(ctx context.Context) error {
			return queueRedis.Ping(ctx).Err()
		}},
		service.DependencyCheck{Name: "mongodb", Check: func(ctx context.Context) error {
			return mongoClient.Ping(ctx, readpref.Primary())
		}},
	)

	txManager := database.NewTxManager(pool, cfg.DB.RetryMax)

	userRepo := repository.NewUserRepository()
	tokens := helper.NewTokenIssuer(cfg.Auth.AccessSecret, cfg.Auth.Issuer, cfg.Auth.Audience, cfg.Auth.AccessTTL)
	limiter := middleware.NewRedisLimiter(admissionRedis, cfg.Auth.LoginRateLimitPerMin, time.Minute)
	registerLimiter := middleware.NewRedisLimiter(admissionRedis, cfg.Auth.RegisterRateLimitPerMin, time.Minute)

	authUsecase, err := usecase.NewAuthUsecase(userRepo, txManager, tokens, usecase.AuthUsecaseConfig{
		AccessTTL:  cfg.Auth.AccessTTL,
		RefreshTTL: cfg.Auth.RefreshTTL,
		BcryptCost: cfg.Auth.BcryptCost,
	})
	if err != nil {
		return fmt.Errorf("siapkan usecase auth: %w", err)
	}
	authService := service.NewAuthService(authUsecase, userRepo, txManager, tokens, limiter, registerLimiter)

	eventRepo := repository.NewPostgresEventRepository(pool)
	tierRepo := repository.NewPostgresTicketTierRepository(pool)
	staffRepo := repository.NewPostgresEventStaffRepository(pool)
	auditRepo := repository.NewPostgresAuditLogRepository(pool)
	outboxRepo := repository.NewPostgresOutboxRepository(pool)
	inventoryRepo := repository.NewPostgresInventoryMovementRepository(pool)

	eventUsecase := usecase.NewEventUsecase(txManager, eventRepo, tierRepo, staffRepo, auditRepo, outboxRepo)
	eventService := service.NewEventService(eventUsecase, []byte(cfg.Auth.AccessSecret))

	bookingRepo := repository.NewPostgresBookingRepository(pool)
	userLimitRepo := repository.NewPostgresUserEventLimitRepository(pool)
	idempotencyRepo := repository.NewPostgresIdempotencyRepository(pool)

	bookingUsecase := usecase.NewBookingUsecase(
		txManager,
		eventRepo,
		tierRepo,
		bookingRepo,
		userLimitRepo,
		idempotencyRepo,
		inventoryRepo,
		outboxRepo,
		auditRepo,
		cfg.Business.HoldTTL,
		cfg.Business.ReleaseGrace,
	)
	bookingService := service.NewBookingService(bookingUsecase, []byte(cfg.Auth.AccessSecret))

	app := fiber.New(fiber.Config{
		AppName:               cfg.App.Name,
		BodyLimit:             1 << 20,
		ReadTimeout:           5 * time.Second,
		WriteTimeout:          15 * time.Second,
		IdleTimeout:           60 * time.Second,
		ErrorHandler:          helper.ErrorHandler,
		DisableStartupMessage: cfg.App.Env == "production",
	})
	app.Use(recover.New())
	app.Use(middleware.RequestID())
	route.Register(app, health, authService, eventService, bookingService)

	errCh := make(chan error, 2)

	if role == "api" || role == "all" {
		go func() {
			address := fmt.Sprintf(":%d", cfg.App.Port)
			logger.Info("api mendengarkan", "address", address, "env", cfg.App.Env)
			if err := app.Listen(address); err != nil {
				errCh <- fmt.Errorf("http server: %w", err)
			}
		}()
	}

	if role == "worker" || role == "all" {
		go func() {
			if err := worker.Run(ctx, logger, cfg.Worker); err != nil {
				errCh <- fmt.Errorf("worker: %w", err)
			}
		}()
	}

	if role == "scheduler" || role == "all" {
		go func() {
			if err := worker.RunScheduler(ctx, logger, cfg.Worker); err != nil {
				errCh <- fmt.Errorf("scheduler: %w", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		logger.Info("sinyal berhenti diterima")
	case err := <-errCh:
		stop()
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if role == "api" || role == "all" {
		if err := app.ShutdownWithContext(shutdownCtx); err != nil {
			return fmt.Errorf("hentikan http server: %w", err)
		}
	}

	logger.Info("aplikasi berhenti")
	return nil
}

func validRole(role string) bool {
	switch role {
	case "api", "worker", "scheduler", "all":
		return true
	default:
		return false
	}
}

func newLogger(levelName string) *slog.Logger {
	var level slog.Level

	switch strings.ToLower(levelName) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
