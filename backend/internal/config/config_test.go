package config

import (
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

var envKeys = []string{
	"APP_ENV", "HTTP_ADDR", "LOG_LEVEL", "API_KEY", "CORS_ALLOWED_ORIGINS", "DATABASE_URL", "DATA_DIR", "IMPORT_MIN_FILES",
}

const testDB = "postgres://shop:shop@localhost:5432/shop?sslmode=disable"

func TestLoad(t *testing.T) {
	defaults := Config{
		Env: "development", HTTPAddr: ":8080", LogLevel: "info", APIKey: "apitest",
		AllowedOrigins: []string{"*"}, DataDir: "./data", ImportMinFiles: 2,
	}
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr string
	}{
		{name: "defaults", env: map[string]string{}, want: defaults},
		{
			name: "all set",
			env: map[string]string{
				"APP_ENV": "production", "HTTP_ADDR": "127.0.0.1:9000", "LOG_LEVEL": "debug", "API_KEY": "s3cret",
				"CORS_ALLOWED_ORIGINS": " https://shop.example.com, ,http://localhost:3000 ",
				"DATABASE_URL":         testDB, "DATA_DIR": "/data", "IMPORT_MIN_FILES": "3",
			},
			want: Config{
				Env: "production", HTTPAddr: "127.0.0.1:9000", LogLevel: "debug", APIKey: "s3cret",
				AllowedOrigins: []string{"https://shop.example.com", "http://localhost:3000"},
				DatabaseURL:    testDB, DataDir: "/data", ImportMinFiles: 3,
			},
		},
		{
			name: "empty values fall back to defaults",
			env: map[string]string{
				"APP_ENV": "", "HTTP_ADDR": "", "LOG_LEVEL": "", "API_KEY": "", "CORS_ALLOWED_ORIGINS": "",
				"DATA_DIR": "", "IMPORT_MIN_FILES": "",
			},
			want: defaults,
		},
		{name: "unparsable IMPORT_MIN_FILES", env: map[string]string{"IMPORT_MIN_FILES": "two"}, wantErr: "IMPORT_MIN_FILES"},
		{name: "IMPORT_MIN_FILES out of range", env: map[string]string{"IMPORT_MIN_FILES": "4"}, wantErr: "IMPORT_MIN_FILES"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range envKeys {
				t.Setenv(k, tt.env[k])
			}
			got, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load() err = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	valid := Config{
		Env: "development", HTTPAddr: ":8080", LogLevel: "info", APIKey: "apitest",
		AllowedOrigins: []string{"*"}, DatabaseURL: testDB, DataDir: "./data", ImportMinFiles: 2,
	}
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string // substring; "" means valid
	}{
		{"defaults are valid", func(*Config) {}, ""},
		{"test env", func(c *Config) { c.Env = "test" }, ""},
		{"production with real key", func(c *Config) { c.Env = "production"; c.APIKey = "s3cret" }, ""},
		{"host and port", func(c *Config) { c.HTTPAddr = "0.0.0.0:80" }, ""},
		{"port 0 (random)", func(c *Config) { c.HTTPAddr = ":0" }, ""},
		{"each log level", func(c *Config) { c.LogLevel = "warn" }, ""},
		{"min files 1 and 3", func(c *Config) { c.ImportMinFiles = 3 }, ""},
		{"explicit origins", func(c *Config) {
			c.AllowedOrigins = []string{"https://shop.example.com", "http://localhost:3000"}
		}, ""},

		{"unknown env", func(c *Config) { c.Env = "staging" }, "APP_ENV"},
		{"env wrong case", func(c *Config) { c.Env = "Production" }, "APP_ENV"},
		{"addr without port", func(c *Config) { c.HTTPAddr = "localhost" }, "HTTP_ADDR"},
		{"non-numeric port", func(c *Config) { c.HTTPAddr = ":http" }, "HTTP_ADDR port"},
		{"port out of range", func(c *Config) { c.HTTPAddr = ":70000" }, "HTTP_ADDR port"},
		{"unknown log level", func(c *Config) { c.LogLevel = "verbose" }, "LOG_LEVEL"},
		{"log level with offset", func(c *Config) { c.LogLevel = "info+2" }, "LOG_LEVEL"},
		{"empty api key", func(c *Config) { c.APIKey = "" }, "API_KEY must not be empty"},
		{"default key in production", func(c *Config) { c.Env = "production" }, "API_KEY must be changed"},
		{"missing database url", func(c *Config) { c.DatabaseURL = "" }, "DATABASE_URL is required"},
		{"empty data dir", func(c *Config) { c.DataDir = "" }, "DATA_DIR"},
		{"min files 0", func(c *Config) { c.ImportMinFiles = 0 }, "IMPORT_MIN_FILES"},
		{"min files 4", func(c *Config) { c.ImportMinFiles = 4 }, "IMPORT_MIN_FILES"},
		{"no origins", func(c *Config) { c.AllowedOrigins = nil }, "CORS_ALLOWED_ORIGINS must not be empty"},
		{"wildcard mixed with origins", func(c *Config) { c.AllowedOrigins = []string{"*", "https://a.com"} }, "* must be the only entry"},
		{"origin with path", func(c *Config) { c.AllowedOrigins = []string{"https://a.com/app"} }, "CORS_ALLOWED_ORIGINS"},
		{"origin with trailing slash", func(c *Config) { c.AllowedOrigins = []string{"https://a.com/"} }, "CORS_ALLOWED_ORIGINS"},
		{"origin without scheme", func(c *Config) { c.AllowedOrigins = []string{"a.com"} }, "CORS_ALLOWED_ORIGINS"},
		{"origin with other scheme", func(c *Config) { c.AllowedOrigins = []string{"ftp://a.com"} }, "CORS_ALLOWED_ORIGINS"},
		{"origin upper case", func(c *Config) { c.AllowedOrigins = []string{"https://Shop.com"} }, "CORS_ALLOWED_ORIGINS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid
			tt.mutate(&c)
			err := c.Validate()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("Validate() = %v, want nil", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateReportsAllErrors(t *testing.T) {
	err := Config{Env: "x", HTTPAddr: "x", LogLevel: "x"}.Validate()
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"APP_ENV", "HTTP_ADDR", "LOG_LEVEL", "API_KEY", "CORS_ALLOWED_ORIGINS", "DATABASE_URL", "DATA_DIR", "IMPORT_MIN_FILES"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestSlogLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError,
	} {
		got, err := Config{LogLevel: in}.SlogLevel()
		if err != nil || got != want {
			t.Errorf("SlogLevel(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
}
