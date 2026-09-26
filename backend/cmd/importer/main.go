package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

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
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	files := make([]string, len(sourceFiles))
	for i, name := range sourceFiles {
		files[i] = filepath.Join(dataDir, name)
		if _, err := os.Stat(files[i]); err != nil {
			return fmt.Errorf("coupon file %s not found in DATA_DIR %q: %w", name, dataDir, err)
		}
	}

	minFiles, err := coupon.ParseMinFiles(os.Getenv("IMPORT_MIN_FILES"))
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	return coupon.NewImporter(pool, log, minFiles).Run(ctx, files)
}
