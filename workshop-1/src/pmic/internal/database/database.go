package database

import (
    "pmic/internal/config"
    "pmic/internal/models"

    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
)

func Init(cfg *config.Config) (*gorm.DB, error) {
    db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Info),
    })
    if err != nil {
        return nil, err
    }

    // Auto-migración
    err = db.AutoMigrate(&models.Job{}, &models.Image{})
    if err != nil {
        return nil, err
    }

    return db, nil
}
