package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/viesloxy/ngawimusical-backend/config"
)

// Run menjalankan worker pembayaran, refund, dan notifikasi.
// Tahap ini masih berupa loop kosong sampai processor dan relay diimplementasikan.
func Run(ctx context.Context, logger *slog.Logger, cfg config.WorkerConfig) error {
	logger.Info("worker berjalan", "outbox_relay_interval", cfg.OutboxRelayInterval)

	ticker := time.NewTicker(cfg.OutboxRelayInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("worker berhenti")
			return nil
		case <-ticker.C:
			// Tempat pemrosesan outbox dan pekerjaan Asynq.
		}
	}
}
