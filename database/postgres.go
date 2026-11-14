package database

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/viesloxy/ngawimusical-backend/app/repository"
	"github.com/viesloxy/ngawimusical-backend/config"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// NewPostgres membuat connection pool PostgreSQL sesuai batas yang ada di konfigurasi.
func NewPostgres(ctx context.Context, cfg config.DBConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse konfigurasi postgres: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	poolCfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	poolCfg.ConnConfig.RuntimeParams["statement_timeout"] = cfg.StatementTimeout.String()
	poolCfg.ConnConfig.RuntimeParams["lock_timeout"] = cfg.LockTimeout.String()

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("buat connection pool postgres: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}

// Migrate menjalankan seluruh file migrasi SQL yang belum tercatat di schema_migrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	const createTracker = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
		)`

	if _, err := pool.Exec(ctx, createTracker); err != nil {
		return fmt.Errorf("buat tabel schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("baca folder migrasi: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		var applied bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`,
			entry.Name(),
		).Scan(&applied); err != nil {
			return fmt.Errorf("periksa status migrasi %s: %w", entry.Name(), err)
		}
		if applied {
			continue
		}

		content, err := migrationFiles.ReadFile(path.Join("migrations", entry.Name()))
		if err != nil {
			return fmt.Errorf("baca migrasi %s: %w", entry.Name(), err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("mulai transaksi migrasi %s: %w", entry.Name(), err)
		}

		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("jalankan migrasi %s: %w", entry.Name(), err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`,
			entry.Name(),
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("catat migrasi %s: %w", entry.Name(), err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migrasi %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// TxManager menjalankan pekerjaan usecase dalam transaksi PostgreSQL.
type TxManager struct {
	pool       *pgxpool.Pool
	maxRetries int
}

func NewTxManager(pool *pgxpool.Pool, maxRetries int) *TxManager {
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &TxManager{pool: pool, maxRetries: maxRetries}
}

var _ repository.TxManager = (*TxManager)(nil)

func (m *TxManager) WithTx(ctx context.Context, fn repository.TxFunc) error {
	return m.run(ctx, fn, true)
}

func (m *TxManager) WithTxOnce(ctx context.Context, fn repository.TxFunc) error {
	return m.run(ctx, fn, false)
}

func (m *TxManager) run(ctx context.Context, fn repository.TxFunc, retry bool) error {
	var lastErr error

	for attempt := 0; ; attempt++ {
		lastErr = m.runOnce(ctx, fn)
		if lastErr == nil {
			return nil
		}
		if !retry || attempt >= m.maxRetries || !isRetryable(lastErr) {
			return lastErr
		}

		backoff := time.Duration(1<<attempt) * 25 * time.Millisecond
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (m *TxManager) runOnce(ctx context.Context, fn repository.TxFunc) error {
	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("mulai transaksi: %w", err)
	}

	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	if err := fn(ctx, tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaksi: %w", err)
	}

	return nil
}

func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40001" || pgErr.Code == "40P01"
	}
	return false
}
