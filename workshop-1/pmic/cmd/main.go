package main

import (
    "fmt"
    "log"
    "os"

    "pmic/internal/config"
    "pmic/internal/database"
    "pmic/internal/handlers"
    "pmic/internal/queue"
    "pmic/internal/workers"

    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/logger"
)

func main() {
    cfg := config.Load()

    if err := os.MkdirAll(cfg.StoragePath, 0755); err != nil {
        log.Fatal("Error creando storage:", err)
    }

    db, err := database.Init(cfg)
    if err != nil {
        log.Fatal("Error conectando a BD:", err)
    }
    fmt.Println("✓ Base de datos conectada")

    pipeline := queue.NewPipeline()
    fmt.Println("✓ Pipeline Channels (Nativo) creados")

    // Crear el pool de workers dinámico
    workerPool := workers.NewDynamicWorkerPool(db, cfg.StoragePath, pipeline)
    fmt.Println("✓ Worker Pool Dinámico inicializado")

    app := fiber.New()
    app.Use(logger.New())

    jobHandler := handlers.NewJobHandler(db, pipeline, workerPool)

    api := app.Group("/api/v1")
    api.Post("/process", jobHandler.CreateJob)

    log.Fatal(app.Listen(":5000"))
}
