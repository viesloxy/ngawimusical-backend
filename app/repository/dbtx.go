package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX adalah kontrak minimum yang dipakai repository untuk menjalankan query.
// pgxpool.Pool dan pgx.Tx sama-sama memenuhi interface ini.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TxFunc adalah pekerjaan yang dijalankan di dalam satu transaksi database.
type TxFunc func(ctx context.Context, tx DBTX) error

// TxManager mengatur batas transaksi untuk usecase.
type TxManager interface {
	WithTx(ctx context.Context, fn TxFunc) error
	WithTxOnce(ctx context.Context, fn TxFunc) error
}
