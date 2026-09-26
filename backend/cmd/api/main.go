package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"shop/internal/app"
	"shop/internal/config"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}

func run() error {
	a, err := app.New(config.Load())
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := a.Logger

	// Listen before serving so a bad or busy address fails startup with a
	// non-zero exit instead of surfacing later.
	ln, err := net.Listen("tcp", a.Config.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", a.Config.HTTPAddr, err)
	}

	srv := &http.Server{
		Handler:           a.Handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", ln.Addr().String())
		serveErr <- srv.Serve(ln)
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	log.Info("shutting down", "timeout", shutdownTimeout)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	log.Info("server stopped")
	return nil
}
