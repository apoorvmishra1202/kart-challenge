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

var sourceFiles = []string{"couponbase1.gz", "couponbase2.gz", "couponbase3.gz"}

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
		return err
	}

	files := make([]string, len(sourceFiles))
	for i, name := range sourceFiles {
		files[i] = filepath.Join(cfg.DataDir, name)
		if _, err := os.Stat(files[i]); err != nil {
			return fmt.Errorf("coupon file %s not found in DATA_DIR %q: %w", name, cfg.DataDir, err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	return coupon.NewImporter(pool, log).Run(ctx, files)
}
