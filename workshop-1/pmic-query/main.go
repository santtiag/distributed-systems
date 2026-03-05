package main

import (
    "log"
    "os"

    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/logger"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

func main() {
    // 1. Conectar a la misma base de datos del proyecto principal
    dbURL := os.Getenv("DATABASE_URL")
    if dbURL == "" {
        dbURL = "host=localhost user=postgres password=pass_postgres dbname=pmic port=5432 sslmode=disable"
    }

    db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
    if err != nil {
        log.Fatal("Error conectando a BD:", err)
    }

    // 2. Configurar Fiber
    app := fiber.New()
    app.Use(logger.New())
    
    // Habilitar CORS es vital porque el Frontend (RF6) estará en otro puerto o dominio
    app.Use(cors.New()) 

    // 3. Endpoint de consulta (RF6)
    app.Get("/api/v1/status/:job_id", func(c *fiber.Ctx) error {
        jobID := c.Params("job_id")

        var job Job
        // Preload carga automáticamente todas las imágenes relacionadas a ese Job
        result := db.Preload("Images").First(&job, "id = ?", jobID)
        
        if result.Error != nil {
            return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
                "error": "Procesamiento no encontrado",
            })
        }

        // Calcular métricas globales extra para enriquecer la respuesta
        var totalDownload, totalResize, totalConvert, totalWatermark float64
        for _, img := range job.Images {
            totalDownload += img.DownloadTimeSec
            totalResize += img.ResizeTimeSec
            totalConvert += img.ConvertTimeSec
            totalWatermark += img.WatermarkTimeSec
        }

        // Formatear la respuesta final
        response := fiber.Map{
            "job_info": job,
            "global_metrics": fiber.Map{
                "total_download_time_sec":  totalDownload,
                "total_resize_time_sec":    totalResize,
                "total_convert_time_sec":   totalConvert,
                "total_watermark_time_sec": totalWatermark,
            },
        }

        return c.JSON(response)
    })

    // 4. Iniciar servicio de lectura (Puerto distinto)
    log.Println("Microservicio de Query iniciado en puerto 3001")
    log.Fatal(app.Listen(":3001"))
}
