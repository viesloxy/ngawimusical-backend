package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/viesloxy/ngawimusical-backend/config"
)

// RunScheduler menjalankan pemindai expiry dan rekonsiliasi pembayaran.
// Tahap ini masih berupa loop kosong sampai scheduler diimplementasikan.
func RunScheduler(ctx context.Context, logger *slog.Logger, cfg config.WorkerConfig) error {
	logger.Info("scheduler berjalan", "expiry_scan_interval", cfg.ExpiryScanInterval)

	ticker := time.NewTicker(cfg.ExpiryScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("scheduler berhenti")
			return nil
		case <-ticker.C:
			// Tempat pemindaian booking yang mendekati release_at.
		}
	}
}
