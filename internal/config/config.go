package config

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

type Config struct {
	Port string

	DatabaseURL string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	TelegramToken   string
	TelegramWebhook string

	WhatsAppToken       string
	WhatsAppPhoneID     string
	WhatsAppVerifyToken string

	// AIProvider selects the backend: "gemini" (default) or "openai".
	AIProvider string

	// Gemini
	GeminiAPIKey         string
	GeminiModel          string
	GeminiEmbeddingModel string

	// OpenAI
	OpenAIAPIKey         string
	OpenAIBaseURL        string // optional; empty = default OpenAI endpoint
	OpenAIModel          string
	OpenAIEmbeddingModel string

	DefaultTenantID string

	AdminJWTSecret         string
	AdminBootstrapEmail    string
	AdminBootstrapPassword string
}

// loadDotEnv reads a .env file and sets any variables not already present in
// the environment. Real env exports always take precedence over the file.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // no .env file is fine
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		// Strip optional surrounding quotes
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}

func Load() (*Config, error) {
	loadDotEnv(".env")
	cfg := &Config{
		Port:            getEnv("PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		RedisAddr:       getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:   os.Getenv("REDIS_PASSWORD"),
		RedisDB:         0,
		TelegramToken:   os.Getenv("TELEGRAM_TOKEN"),
		TelegramWebhook: os.Getenv("TELEGRAM_WEBHOOK"),
		WhatsAppToken:       os.Getenv("WHATSAPP_TOKEN"),
		WhatsAppPhoneID:     os.Getenv("WHATSAPP_PHONE_ID"),
		WhatsAppVerifyToken: os.Getenv("WHATSAPP_VERIFY_TOKEN"),
		AIProvider:           getEnv("AI_PROVIDER", "gemini"),
		GeminiAPIKey:         os.Getenv("GEMINI_API_KEY"),
		GeminiModel:          getEnv("GEMINI_MODEL", "gemini-1.5-flash"),
		GeminiEmbeddingModel: getEnv("GEMINI_EMBEDDING_MODEL", "text-embedding-004"),
		OpenAIAPIKey:         os.Getenv("OPENAI_API_KEY"),
		OpenAIBaseURL:        os.Getenv("OPENAI_BASE_URL"),
		OpenAIModel:          getEnv("OPENAI_MODEL", "gpt-4o"),
		OpenAIEmbeddingModel: getEnv("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
		DefaultTenantID:        getEnv("DEFAULT_TENANT_ID", "default"),
		AdminJWTSecret:         os.Getenv("ADMIN_JWT_SECRET"),
		AdminBootstrapEmail:    os.Getenv("ADMIN_BOOTSTRAP_EMAIL"),
		AdminBootstrapPassword: os.Getenv("ADMIN_BOOTSTRAP_PASSWORD"),
	}

	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}

	switch cfg.AIProvider {
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, errors.New("OPENAI_API_KEY is required when AI_PROVIDER=openai")
		}
	default: // "gemini"
		if cfg.GeminiAPIKey == "" {
			return nil, errors.New("GEMINI_API_KEY is required when AI_PROVIDER=gemini")
		}
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
