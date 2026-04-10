package config

import (
	"os"
	"testing"
)

func TestLoad_MissingDatabaseURL(t *testing.T) {
	os.Unsetenv("DATABASE_URL")
	os.Setenv("ADMIN_API_KEY", "a_very_long_admin_key_at_least_32_ch")
	os.Setenv("NODE_RATE_LIMIT_RPS", "10")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
}

func TestLoad_MissingAdminAPIKey(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Unsetenv("ADMIN_API_KEY")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing ADMIN_API_KEY")
	}
}

func TestLoad_ShortAdminAPIKey(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("ADMIN_API_KEY", "short_key")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for short ADMIN_API_KEY")
	}
}

func TestLoad_InvalidRateLimitRPS(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("ADMIN_API_KEY", "a_very_long_admin_key_at_least_32_ch")
	os.Setenv("NODE_RATE_LIMIT_RPS", "abc")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid NODE_RATE_LIMIT_RPS")
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("ADMIN_API_KEY", "a_very_long_admin_key_at_least_32_ch")
	os.Setenv("NODE_RATE_LIMIT_RPS", "5")
	os.Setenv("SERVER_PORT", "9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ServerPort != "9090" {
		t.Errorf("expected port 9090, got %s", cfg.ServerPort)
	}
	if cfg.NodeRateLimitRPS != 5 {
		t.Errorf("expected RPS 5, got %d", cfg.NodeRateLimitRPS)
	}
}
