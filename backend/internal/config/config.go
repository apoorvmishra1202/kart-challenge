// Package config loads settings for both binaries (cmd/api and cmd/importer)
// from the environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"shop/internal/coupon"
)

const (
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvProduction  = "production"

	// DefaultAPIKey matches the example key in api/openapi.yaml. It is
	// rejected in production.
	DefaultAPIKey = "apitest"

	// AnyOrigin in CORS_ALLOWED_ORIGINS allows every browser origin. That is
	// safe here because the API authenticates with the api_key header, not
	// cookies, so no credentials are sent cross-site.
	AnyOrigin = "*"
)

type Config struct {
	Env      string
	HTTPAddr string
	LogLevel string
	APIKey   string
	// AllowedOrigins are the browser origins allowed to call the API (CORS),
	// e.g. ["https://shop.example.com"], or ["*"] for any origin.
	AllowedOrigins []string

	// DatabaseURL is required: the API reads valid_codes and the importer
	// writes it.
	DatabaseURL string
	// DataDir holds the coupon .gz files (importer only).
	DataDir string
	// ImportMinFiles is how many files a code must appear in to be valid
	// (importer only).
	ImportMinFiles int
}

// Load reads the environment, applying defaults for unset variables. It fails
// only when a value can't be parsed; call Validate for the remaining checks.
func Load() (Config, error) {
	minFiles, err := coupon.ParseMinFiles(os.Getenv("IMPORT_MIN_FILES"))
	if err != nil {
		return Config{}, err
	}
	return Config{
		Env:            getenv("APP_ENV", EnvDevelopment),
		HTTPAddr:       getenv("HTTP_ADDR", ":8080"),
		LogLevel:       getenv("LOG_LEVEL", "info"),
		APIKey:         getenv("API_KEY", DefaultAPIKey),
		AllowedOrigins: splitList(getenv("CORS_ALLOWED_ORIGINS", AnyOrigin)),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		DataDir:        getenv("DATA_DIR", "./data"),
		ImportMinFiles: minFiles,
	}, nil
}

// Validate reports every invalid setting at once.
func (c Config) Validate() error {
	var errs []error

	switch c.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		errs = append(errs, fmt.Errorf("APP_ENV must be %q, %q or %q, got %q",
			EnvDevelopment, EnvTest, EnvProduction, c.Env))
	}

	if _, port, err := net.SplitHostPort(c.HTTPAddr); err != nil {
		errs = append(errs, fmt.Errorf("HTTP_ADDR must look like host:port or :port, got %q", c.HTTPAddr))
	} else if n, err := strconv.Atoi(port); err != nil || n < 0 || n > 65535 {
		errs = append(errs, fmt.Errorf("HTTP_ADDR port must be 0-65535, got %q", port))
	}

	if _, err := c.SlogLevel(); err != nil {
		errs = append(errs, err)
	}

	switch {
	case c.APIKey == "":
		errs = append(errs, errors.New("API_KEY must not be empty"))
	case c.Env == EnvProduction && c.APIKey == DefaultAPIKey:
		errs = append(errs, errors.New("API_KEY must be changed from the default in production"))
	}

	errs = append(errs, validateOrigins(c.AllowedOrigins)...)

	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if c.DataDir == "" {
		errs = append(errs, errors.New("DATA_DIR must not be empty"))
	}
	if c.ImportMinFiles < 1 || c.ImportMinFiles > coupon.SourceFiles {
		errs = append(errs, fmt.Errorf("IMPORT_MIN_FILES must be from 1 to %d, got %d", coupon.SourceFiles, c.ImportMinFiles))
	}

	return errors.Join(errs...)
}

// SlogLevel parses LogLevel ("debug", "info", "warn" or "error").
func (c Config) SlogLevel() (slog.Level, error) {
	var l slog.Level
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
		if err := l.UnmarshalText([]byte(c.LogLevel)); err != nil {
			return 0, err
		}
		return l, nil
	}
	return 0, fmt.Errorf("LOG_LEVEL must be debug, info, warn or error, got %q", c.LogLevel)
}

// validateOrigins requires either exactly ["*"] or a list of origins in the
// exact form browsers send: scheme://host[:port], lower case, with no path,
// query or trailing slash (matching is an exact string comparison).
func validateOrigins(origins []string) []error {
	if len(origins) == 0 {
		return []error{errors.New("CORS_ALLOWED_ORIGINS must not be empty (use * to allow any origin)")}
	}
	var errs []error
	for _, o := range origins {
		if o == AnyOrigin {
			if len(origins) > 1 {
				errs = append(errs, errors.New("CORS_ALLOWED_ORIGINS: * must be the only entry"))
			}
			continue
		}
		u, err := url.Parse(o)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			o != u.Scheme+"://"+u.Host || o != strings.ToLower(o) {
			errs = append(errs, fmt.Errorf("CORS_ALLOWED_ORIGINS: %q must look like https://host[:port] (lower case, no path or trailing slash)", o))
		}
	}
	return errs
}

// splitList splits a comma-separated value, trimming spaces and dropping
// empty entries.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
