package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port            string
	MySQLDSN        string
	StoragePath     string
	LLMBaseURL      string
	LLMAPIKey       string
	LLMGuestModel   string
	LLMDefaultModel string
	ExtraProviders  []ExtraProviderConfig
	TavilyAPIKey    string
	ComfyUIURL      string
	ImageAPIURL     string
	ImageAPIKey     string
	ImageModels     []string
	CookieSecure    bool
	SessionSecret   string
	AllowedOrigins  []string
}

// ExtraProviderConfig mendefinisikan satu endpoint OpenAI-compatible tambahan
// (Ollama di mesin lain, LM Studio, OpenRouter, dll).
type ExtraProviderConfig struct {
	Name      string   `json:"name"`
	BaseURL   string   `json:"base_url"`
	APIKey    string   `json:"api_key"`
	APIKeyEnv string   `json:"api_key_env"` // nama env var; lebih aman daripada api_key
	Models    []string `json:"models"`      // allowlist opsional
}

// loadExtraProviders membaca providers.json (atau path dari LLM_PROVIDERS_FILE).
// File yang tidak ada bukan error; file yang rusak adalah error.
func loadExtraProviders() ([]ExtraProviderConfig, error) {
	path := os.Getenv("LLM_PROVIDERS_FILE")
	explicit := path != ""
	candidates := []string{path}
	if !explicit {
		candidates = []string{"providers.json", "../providers.json"}
	}
	for _, c := range candidates {
		data, err := os.ReadFile(c)
		if errors.Is(err, os.ErrNotExist) {
			if explicit {
				return nil, fmt.Errorf("LLM_PROVIDERS_FILE %q tidak ditemukan", c)
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("baca %s: %w", c, err)
		}
		var file struct {
			Providers []ExtraProviderConfig `json:"providers"`
		}
		if err := json.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("%s tidak valid: %w", c, err)
		}
		seen := map[string]bool{}
		for i := range file.Providers {
			p := &file.Providers[i]
			p.Name, p.BaseURL = strings.TrimSpace(p.Name), strings.TrimSpace(p.BaseURL)
			if p.Name == "" || p.BaseURL == "" {
				return nil, fmt.Errorf("%s: provider #%d wajib punya name dan base_url", c, i+1)
			}
			if strings.Contains(p.Name, "::") {
				return nil, fmt.Errorf("%s: nama provider %q tidak boleh mengandung '::'", c, p.Name)
			}
			if seen[p.Name] {
				return nil, fmt.Errorf("%s: nama provider %q duplikat", c, p.Name)
			}
			seen[p.Name] = true
			if p.APIKeyEnv != "" {
				p.APIKey = os.Getenv(p.APIKeyEnv)
			}
		}
		return file.Providers, nil
	}
	return nil, nil
}

func loadEnvFile(paths ...string) {
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				// Strip quotes if present
				val = strings.Trim(val, `"'`)
				if _, exists := os.LookupEnv(key); !exists {
					_ = os.Setenv(key, val)
				}
			}
		}
		break // loaded first found
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return fallback
	}
	return b
}

func Load() (*Config, error) {
	// Attempt to load .env from current directory or parent directory
	loadEnvFile(".env", "../.env")

	cfg := &Config{
		Port:            getEnv("PORT", "8080"),
		MySQLDSN:        os.Getenv("MYSQL_DSN"),
		StoragePath:     getEnv("STORAGE_PATH", "data/attachments"),
		LLMBaseURL:      os.Getenv("LLM_BASE_URL"),
		LLMAPIKey:       os.Getenv("LLM_API_KEY"),
		LLMGuestModel:   os.Getenv("LLM_GUEST_MODEL"),
		LLMDefaultModel: os.Getenv("LLM_DEFAULT_MODEL"),
		TavilyAPIKey:    os.Getenv("TAVILY_API_KEY"),
		ComfyUIURL:      os.Getenv("COMFYUI_URL"),
		ImageAPIURL:     strings.TrimSpace(os.Getenv("IMAGE_API_URL")),
		ImageAPIKey:     strings.TrimSpace(os.Getenv("IMAGE_API_KEY")),
		ImageModels:     splitList(getEnv("IMAGE_MODELS", "cx/gpt-image-2.5")),
		CookieSecure:    getEnvBool("COOKIE_SECURE", false),
		SessionSecret:   os.Getenv("SESSION_SECRET"),
		AllowedOrigins:  []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:8080", "http://127.0.0.1:8080"},
	}

	if origins := os.Getenv("ALLOWED_ORIGINS"); origins != "" {
		cfg.AllowedOrigins = strings.Split(origins, ",")
		for i := range cfg.AllowedOrigins {
			cfg.AllowedOrigins[i] = strings.TrimSpace(cfg.AllowedOrigins[i])
		}
	}

	// Strict validations (fail-fast on startup)
	if cfg.MySQLDSN == "" {
		return nil, fmt.Errorf("MYSQL_DSN wajib diisi melalui environment variable")
	}
	if cfg.SessionSecret == "" || len(cfg.SessionSecret) < 32 {
		return nil, fmt.Errorf("SESSION_SECRET wajib diisi melalui environment variable (minimal 32 karakter)")
	}
	if cfg.LLMBaseURL == "" {
		return nil, fmt.Errorf("LLM_BASE_URL wajib diisi melalui environment variable")
	}

	extras, err := loadExtraProviders()
	if err != nil {
		return nil, err
	}
	cfg.ExtraProviders = extras

	return cfg, nil
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
