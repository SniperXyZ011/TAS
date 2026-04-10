package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	DatabaseURL      string
	ServerPort       string
	AdminAPIKey      string
	NodeRateLimitRPS int
	LogLevel         string
	Env              string
}

// Load reads configuration from environment variables and validates required fields.
// It returns an error if any required variable is missing or invalid.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		ServerPort:   getEnvOr("SERVER_PORT", "8080"),
		AdminAPIKey:  os.Getenv("ADMIN_API_KEY"),
		LogLevel:     getEnvOr("LOG_LEVEL", "info"),
		Env:          getEnvOr("ENV", "production"),
	}

	// Validate required fields
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.AdminAPIKey == "" {
		return nil, fmt.Errorf("ADMIN_API_KEY is required")
	}
	if len(cfg.AdminAPIKey) < 32 {
		return nil, fmt.Errorf("ADMIN_API_KEY must be at least 32 characters")
	}

	rps, err := strconv.Atoi(getEnvOr("NODE_RATE_LIMIT_RPS", "10"))
	if err != nil || rps <= 0 {
		return nil, fmt.Errorf("NODE_RATE_LIMIT_RPS must be a positive integer, got: %q", os.Getenv("NODE_RATE_LIMIT_RPS"))
	}
	cfg.NodeRateLimitRPS = rps

	return cfg, nil
}

func getEnvOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
