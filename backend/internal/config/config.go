package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv         string
	Port           string
	DatabaseURL    string
	FrontendOrigin string

	JWTAccessSecret  string
	JWTRefreshSecret string
	EncryptionKey    string

	AccessTokenTTL        time.Duration
	RefreshTokenTTL       time.Duration
	HTTPCheckTimeout      time.Duration
	MonitorWorkerInterval time.Duration
}

func Load() (Config, error) {
	_ = godotenv.Load()

	accessTTLMinutes, err := getPositiveInt("ACCESS_TOKEN_TTL_MINUTES", 15)
	if err != nil {
		return Config{}, err
	}

	refreshTTLDays, err := getPositiveInt("REFRESH_TOKEN_TTL_DAYS", 7)
	if err != nil {
		return Config{}, err
	}

	checkTimeoutSeconds, err := getPositiveInt("HTTP_CHECK_TIMEOUT_SECONDS", 10)
	if err != nil {
		return Config{}, err
	}

	workerIntervalSeconds, err := getPositiveInt("MONITOR_WORKER_INTERVAL_SECONDS", 15)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppEnv:         getEnv("APP_ENV", "development"),
		Port:           getEnv("PORT", "8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		FrontendOrigin: getEnv("FRONTEND_ORIGIN", "http://localhost:3000"),

		JWTAccessSecret:  os.Getenv("JWT_ACCESS_SECRET"),
		JWTRefreshSecret: os.Getenv("JWT_REFRESH_SECRET"),
		EncryptionKey:    os.Getenv("WEBHOOK_ENCRYPTION_KEY"),

		AccessTokenTTL:        time.Duration(accessTTLMinutes) * time.Minute,
		RefreshTokenTTL:       time.Duration(refreshTTLDays) * 24 * time.Hour,
		HTTPCheckTimeout:      time.Duration(checkTimeoutSeconds) * time.Second,
		MonitorWorkerInterval: time.Duration(workerIntervalSeconds) * time.Second,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	if cfg.JWTAccessSecret == "" {
		return Config{}, fmt.Errorf("JWT_ACCESS_SECRET is required")
	}

	if cfg.JWTRefreshSecret == "" {
		return Config{}, fmt.Errorf("JWT_REFRESH_SECRET is required")
	}

	if cfg.EncryptionKey == "" {
		return Config{}, fmt.Errorf("WEBHOOK_ENCRYPTION_KEY is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func getPositiveInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}

	return parsed, nil
}
