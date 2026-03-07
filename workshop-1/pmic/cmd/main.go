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

    // Pool Workers de Descargas
    for i := 1; i <= 5; i++ {
        worker := &workers.DownloadWorker{
            WorkerID: fmt.Sprintf("DL-Worker-%d", i),
            DB:       db,
            InChan:   pipeline.DownloadChan,
            OutChan:  pipeline.ResizeChan,
            Storage:  cfg.StoragePath,
        }
        worker.Start()
    }

    // Pool Workers de Redimensionamiento
    for i := 1; i <= 3; i++ {
        worker := &workers.ResizeWorker{
            WorkerID: fmt.Sprintf("RSZ-Worker-%d", i),
            DB:       db,
            InChan:   pipeline.ResizeChan,
            OutChan:  pipeline.ConvertChan,
            Storage:  cfg.StoragePath,
        }
        worker.Start()
    }

    // Pool Workers de Conversión
    for i := 1; i <= 3; i++ {
        worker := &workers.ConvertWorker{
            WorkerID: fmt.Sprintf("CNV-Worker-%d", i),
            DB:       db,
            InChan:   pipeline.ConvertChan,
            OutChan:  pipeline.WatermarkChan,
            Storage:  cfg.StoragePath,
        }
        worker.Start()
    }

    // Pool Workers de Marca de Agua
    for i := 1; i <= 2; i++ {
        worker := &workers.WatermarkWorker{
            WorkerID: fmt.Sprintf("WMK-Worker-%d", i),
            DB:       db,
            InChan:   pipeline.WatermarkChan,
            Storage:  cfg.StoragePath,
        }
        worker.Start()
    }

    app := fiber.New()
    app.Use(logger.New())

    jobHandler := handlers.NewJobHandler(db, pipeline)

    api := app.Group("/api/v1")
    api.Post("/process", jobHandler.CreateJob)

    log.Fatal(app.Listen(":5000"))
}
