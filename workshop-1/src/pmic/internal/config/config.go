package config

import (
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL          string
	StoragePath          string
	CORSOrigins          string
	CORSAllowCredentials bool
}

func Load() *Config {
	return &Config{
		DatabaseURL:          getEnv("DATABASE_URL", "host=localhost user=postgres password=0000 dbname=pmic port=5432 sslmode=disable"),
		StoragePath:          getEnv("STORAGE_PATH", "./storage"),
		CORSOrigins:          getEnv("CORS_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"),
		CORSAllowCredentials: getEnvBool("CORS_ALLOW_CREDENTIALS", false),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
	}
	return defaultValue
}
