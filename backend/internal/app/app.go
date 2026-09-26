// Package app is the composition root: the only place that constructs and
// wires the API server's dependencies.
package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"

	"shop/internal/config"
	"shop/internal/coupon"
	"shop/internal/database"
	"shop/internal/httpapi"
	"shop/internal/order"
	"shop/internal/product"
)

// The services plug into order directly, with no adapter types.
var (
	_ order.ProductLookup   = (*product.Service)(nil)
	_ order.CouponValidator = (*coupon.Service)(nil)
)

type App struct {
	Config  config.Config
	Logger  *slog.Logger
	Handler http.Handler

	close func()
}

// New validates cfg, opens the database pool (verifying the database is
// reachable), and builds the logger and router. Logs are JSON in production
// and human-readable text otherwise, written to stdout. Call Close when done.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	a, err := newApp(cfg, os.Stdout, coupon.NewStore(pool))
	if err != nil {
		pool.Close()
		return nil, err
	}
	a.close = pool.Close
	return a, nil
}

// Close releases the database pool. It is safe to call more than once.
func (a *App) Close() {
	if a.close != nil {
		a.close()
	}
}

// newApp builds everything except the pool, so tests can pass a fake
// coupon store.
func newApp(cfg config.Config, out io.Writer, coupons coupon.CodeStore) (*App, error) {
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
	couponSvc := coupon.NewService(coupons)
	orderSvc := order.NewService(order.NewMemoryStore(), productSvc, couponSvc)

	productHandler := product.NewHandler(productSvc, logger)
	orderHandler := order.NewHandler(orderSvc, logger)
	protect := httpapi.APIKey(cfg.APIKey)

	return &App{
		Config: cfg,
		Logger: logger,
		Handler: httpapi.NewRouter(logger, cfg.AllowedOrigins,
			productHandler,
			httpapi.RegistrarFunc(func(mux *http.ServeMux) { orderHandler.Register(mux, protect) }),
		),
	}, nil
}
