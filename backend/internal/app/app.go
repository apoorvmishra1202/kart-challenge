// Package app is the composition root: the only place that constructs and
// wires the API server's dependencies.
package app

import (
	"io"
	"log/slog"
	"net/http"
	"os"

	"shop/internal/config"
	"shop/internal/coupon"
	"shop/internal/httpapi"
	"shop/internal/order"
	"shop/internal/product"
)

type App struct {
	Config  config.Config
	Logger  *slog.Logger
	Handler http.Handler
}

// New validates cfg and builds the logger and router. Logs are JSON in
// production and human-readable text otherwise, written to stdout.
func New(cfg config.Config) (*App, error) {
	return newApp(cfg, os.Stdout)
}

func newApp(cfg config.Config, out io.Writer) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	level, _ := cfg.SlogLevel() // validated above

	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler = slog.NewTextHandler(out, opts)
	if cfg.Env == config.EnvProduction {
		h = slog.NewJSONHandler(out, opts)
	}
	logger := slog.New(h).With("env", cfg.Env)

	productSvc := product.NewService(product.NewMemoryStore(product.SeedProducts()))
	// Empty store: every coupon is rejected until the API is wired to
	// coupon.PostgresStore (valid_codes, filled by cmd/importer).
	couponSvc := coupon.NewService(coupon.NewMemoryStore())
	orderSvc := order.NewService(order.NewMemoryStore(), productSvc, couponSvc)

	productHandler := product.NewHandler(productSvc)
	orderHandler := order.NewHandler(orderSvc, logger)
	protect := httpapi.APIKey(cfg.APIKey)

	return &App{
		Config: cfg,
		Logger: logger,
		Handler: httpapi.NewRouter(logger,
			productHandler,
			httpapi.RegistrarFunc(func(mux *http.ServeMux) { orderHandler.Register(mux, protect) }),
		),
	}, nil
}
