package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"shop/internal/config"
	"shop/internal/coupon"
	"shop/internal/database"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("import failed", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	files := make([]string, 0, len(coupon.SourceFileNames))
	for _, name := range coupon.SourceFileNames {
		path := filepath.Join(cfg.DataDir, name)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("coupon file %s not found in DATA_DIR %q: %w", name, cfg.DataDir, err)
		}
		files = append(files, path)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	return coupon.NewImporter(pool, log, cfg.ImportMinFiles).Run(ctx, files)
}
