package config

import (
	"log/slog"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Config
	}{
		{
			name: "defaults",
			env:  map[string]string{},
			want: Config{Env: "development", HTTPAddr: ":8080", LogLevel: "info", APIKey: "apitest"},
		},
		{
			name: "all set",
			env: map[string]string{
				"APP_ENV": "production", "HTTP_ADDR": "127.0.0.1:9000", "LOG_LEVEL": "debug", "API_KEY": "s3cret",
			},
			want: Config{Env: "production", HTTPAddr: "127.0.0.1:9000", LogLevel: "debug", APIKey: "s3cret"},
		},
		{
			name: "empty values fall back to defaults",
			env:  map[string]string{"APP_ENV": "", "HTTP_ADDR": "", "LOG_LEVEL": "", "API_KEY": ""},
			want: Config{Env: "development", HTTPAddr: ":8080", LogLevel: "info", APIKey: "apitest"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"APP_ENV", "HTTP_ADDR", "LOG_LEVEL", "API_KEY"} {
				t.Setenv(k, tt.env[k])
			}
			if got := Load(); got != tt.want {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	valid := Config{Env: "development", HTTPAddr: ":8080", LogLevel: "info", APIKey: "apitest"}
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

		{"unknown env", func(c *Config) { c.Env = "staging" }, "APP_ENV"},
		{"env wrong case", func(c *Config) { c.Env = "Production" }, "APP_ENV"},
		{"addr without port", func(c *Config) { c.HTTPAddr = "localhost" }, "HTTP_ADDR"},
		{"non-numeric port", func(c *Config) { c.HTTPAddr = ":http" }, "HTTP_ADDR port"},
		{"port out of range", func(c *Config) { c.HTTPAddr = ":70000" }, "HTTP_ADDR port"},
		{"unknown log level", func(c *Config) { c.LogLevel = "verbose" }, "LOG_LEVEL"},
		{"log level with offset", func(c *Config) { c.LogLevel = "info+2" }, "LOG_LEVEL"},
		{"empty api key", func(c *Config) { c.APIKey = "" }, "API_KEY must not be empty"},
		{"default key in production", func(c *Config) { c.Env = "production" }, "API_KEY must be changed"},
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
	err := Config{Env: "x", HTTPAddr: "x", LogLevel: "x", APIKey: ""}.Validate()
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"APP_ENV", "HTTP_ADDR", "LOG_LEVEL", "API_KEY"} {
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
