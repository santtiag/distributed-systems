package main

import (
    "fmt"
    "log"
    "os"
    
    "pmic/internal/config"
    "pmic/internal/database"
    "pmic/internal/handlers"
    "pmic/internal/queue"

    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/logger"
)

func main() {
    cfg := config.Load()

    // 1. Crear directorio de storage si no existe
    if err := os.MkdirAll(cfg.StoragePath, 0755); err != nil {
        log.Fatal("Error creando storage:", err)
    }

    // 2. Conectar BD
    db, err := database.Init(cfg)
    if err != nil {
        log.Fatal("Error conectando a BD:", err)
    }
    fmt.Println("✓ Base de datos conectada")

    // 3. Conectar NATS
    q, err := queue.New(cfg)
    if err != nil {
        log.Fatal("Error conectando a NATS:", err)
    }
    defer q.Close()
    fmt.Println("✓ NATS conectado")

    // 4. Inicializar Fiber
    app := fiber.New()
    app.Use(logger.New()) // Middleware para ver las peticiones en consola

    // 5. Configurar Rutas
    jobHandler := handlers.NewJobHandler(db)
    
    api := app.Group("/api/v1")
    api.Post("/process", jobHandler.CreateJob) // RF1: Recepción de Solicitud

    // 6. Arrancar servidor
    log.Fatal(app.Listen(":5000"))
}
