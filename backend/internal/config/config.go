package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port           string
	MySQLDSN       string
	StoragePath    string
	LLMBaseURL     string
	LLMAPIKey      string
	LLMGuestModel  string
	TavilyAPIKey   string
	CookieSecure   bool
	SessionSecret  string
	AllowedOrigins []string
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
		Port:           getEnv("PORT", "8080"),
		MySQLDSN:       os.Getenv("MYSQL_DSN"),
		StoragePath:    getEnv("STORAGE_PATH", "data/attachments"),
		LLMBaseURL:     os.Getenv("LLM_BASE_URL"),
		LLMAPIKey:      os.Getenv("LLM_API_KEY"),
		LLMGuestModel:  os.Getenv("LLM_GUEST_MODEL"),
		TavilyAPIKey:   os.Getenv("TAVILY_API_KEY"),
		CookieSecure:   getEnvBool("COOKIE_SECURE", false),
		SessionSecret:  os.Getenv("SESSION_SECRET"),
		AllowedOrigins: []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:8080", "http://127.0.0.1:8080"},
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

	return cfg, nil
}
