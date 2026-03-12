package config

import (
    "os"
)

type Config struct {
    DatabaseURL string
    StoragePath string
}

func Load() *Config {
    return &Config{
        DatabaseURL: getEnv("DATABASE_URL", "host=localhost user=postgres password=pass_postgres dbname=pmic port=5432 sslmode=disable"),
        StoragePath: getEnv("STORAGE_PATH", "./storage"),
    }
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
