package config_test

import (
	"os"
	"testing"

	"chat/backend/internal/config"
)

func TestConfigValidation(t *testing.T) {
	// Clear envs
	os.Unsetenv("MYSQL_DSN")
	os.Unsetenv("SESSION_SECRET")
	os.Unsetenv("LLM_BASE_URL")

	// Missing MYSQL_DSN
	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when MYSQL_DSN is missing")
	}

	// Set MYSQL_DSN but missing SESSION_SECRET
	os.Setenv("MYSQL_DSN", "user:pass@tcp(127.0.0.1:3306)/chat")
	_, err = config.Load()
	if err == nil {
		t.Fatal("expected error when SESSION_SECRET is missing")
	}

	// Short SESSION_SECRET (< 32 chars)
	os.Setenv("SESSION_SECRET", "too-short")
	_, err = config.Load()
	if err == nil {
		t.Fatal("expected error when SESSION_SECRET is too short")
	}

	// Valid SESSION_SECRET but missing LLM_BASE_URL
	os.Setenv("SESSION_SECRET", "this-is-a-valid-session-secret-longer-than-32-bytes")
	_, err = config.Load()
	if err == nil {
		t.Fatal("expected error when LLM_BASE_URL is missing")
	}

	// All valid
	os.Setenv("LLM_BASE_URL", "http://127.0.0.1:11434/v1")
	os.Setenv("LLM_GUEST_MODEL", "guest-model")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected config to load successfully, got: %v", err)
	}

	if cfg.Port != "8080" {
		t.Fatalf("expected default port 8080, got %s", cfg.Port)
	}
	if cfg.LLMGuestModel != "guest-model" {
		t.Fatalf("expected guest model from env, got %q", cfg.LLMGuestModel)
	}
}
