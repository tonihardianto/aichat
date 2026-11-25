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

func TestExtraProvidersFile(t *testing.T) {
	os.Setenv("MYSQL_DSN", "user:pass@tcp(127.0.0.1:3306)/chat")
	os.Setenv("SESSION_SECRET", "this-is-a-valid-session-secret-longer-than-32-bytes")
	os.Setenv("LLM_BASE_URL", "http://127.0.0.1:11434/v1")
	t.Cleanup(func() { os.Unsetenv("LLM_PROVIDERS_FILE"); os.Unsetenv("TEST_EXTRA_KEY") })

	write := func(content string) {
		path := t.TempDir() + "/providers.json"
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		os.Setenv("LLM_PROVIDERS_FILE", path)
	}

	os.Setenv("TEST_EXTRA_KEY", "rahasia")
	write(`{"providers":[{"name":"mac","base_url":"http://10.0.0.5:11434/v1","api_key_env":"TEST_EXTRA_KEY","models":["qwen3"]}]}`)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ExtraProviders) != 1 || cfg.ExtraProviders[0].APIKey != "rahasia" || cfg.ExtraProviders[0].Models[0] != "qwen3" {
		t.Fatalf("unexpected providers: %+v", cfg.ExtraProviders)
	}

	for name, bad := range map[string]string{
		"nama kosong": `{"providers":[{"name":"","base_url":"http://x"}]}`,
		"prefix ::":   `{"providers":[{"name":"a::b","base_url":"http://x"}]}`,
		"duplikat":    `{"providers":[{"name":"a","base_url":"http://x"},{"name":"a","base_url":"http://y"}]}`,
		"json rusak":  `{`,
	} {
		write(bad)
		if _, err := config.Load(); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}

	os.Setenv("LLM_PROVIDERS_FILE", "/tidak/ada/providers.json")
	if _, err := config.Load(); err == nil {
		t.Fatal("expected error for explicit missing file")
	}
}
