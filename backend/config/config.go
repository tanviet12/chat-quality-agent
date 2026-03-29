package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	// Server
	ServerPort string
	ServerHost string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	// Security
	JWTSecret     string
	EncryptionKey string // 32 bytes for AES-256-GCM

	// Rate limiting
	RateLimitPerIP   int // requests per minute
	RateLimitPerUser int

	// AI
	AIMaxTokens int // max tokens for AI responses

	// Guesty API
	GuestyClientID     string
	GuestyClientSecret string

	// Environment
	Env string // "development" | "production"
}

func Load() (*Config, error) {
	cfg := &Config{
		ServerPort:       getEnv("SERVER_PORT", "8080"),
		ServerHost:       getEnv("SERVER_HOST", "127.0.0.1"),
		DBHost:           getEnv("DB_HOST", "localhost"),
		DBPort:           getEnv("DB_PORT", "3306"),
		DBUser:           getEnv("DB_USER", "cqa"),
		DBPassword:       getEnv("DB_PASSWORD", ""),
		DBName:           getEnv("DB_NAME", "cqa"),
		JWTSecret:        getEnv("JWT_SECRET", ""),
		EncryptionKey:    getEnv("ENCRYPTION_KEY", ""),
		RateLimitPerIP:   getEnvInt("RATE_LIMIT_PER_IP", 500),
		RateLimitPerUser: getEnvInt("RATE_LIMIT_PER_USER", 1000),
		AIMaxTokens:      getEnvInt("AI_MAX_TOKENS", 16384),
		GuestyClientID:   getEnv("GUESTY_CLIENT_ID", ""),
		GuestyClientSecret: getEnv("GUESTY_CLIENT_SECRET", ""),
		Env:              getEnv("APP_ENV", "development"),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters for HS256 security")
	}
	if cfg.EncryptionKey == "" {
		return nil, fmt.Errorf("ENCRYPTION_KEY is required")
	}
	if len(cfg.EncryptionKey) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be exactly 32 bytes for AES-256-GCM")
	}
	if cfg.DBPassword == "" {
		return nil, fmt.Errorf("DB_PASSWORD is required")
	}

	return cfg, nil
}

func (c *Config) DSN() string {
	// Check if using PostgreSQL (Render uses pgsql:// prefix)
	dbType := os.Getenv("DB_TYPE")
	if dbType == "postgres" {
		// PostgreSQL format: postgres://user:pass@host:port/dbname
		return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
	}
	// MySQL format
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

func (c *Config) ListenAddr() string {
	return fmt.Sprintf("%s:%s", c.ServerHost, c.ServerPort)
}

func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
