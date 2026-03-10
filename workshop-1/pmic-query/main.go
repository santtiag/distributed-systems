package main

import (
    "log"
    "os"
    "time"

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
        var downloadCompleted, downloadFailed int
        var resizeCompleted, resizeFailed int
        var convertCompleted, convertFailed int
        var watermarkCompleted, watermarkFailed int

        for _, img := range job.Images {
            // Tiempos totales
            totalDownload += img.DownloadTimeSec
            totalResize += img.ResizeTimeSec
            totalConvert += img.ConvertTimeSec
            totalWatermark += img.WatermarkTimeSec

            // Contadores por etapa (descarga) - estados en español según modelo
            if img.DownloadStatus == "COMPLETADO" {
                downloadCompleted++
            } else if img.DownloadStatus == "FALLIDO" {
                downloadFailed++
            }

            // Contadores por etapa (resize)
            if img.ResizeStatus == "COMPLETADO" {
                resizeCompleted++
            } else if img.ResizeStatus == "FALLIDO" {
                resizeFailed++
            }

            // Contadores por etapa (convert)
            if img.ConvertStatus == "COMPLETADO" {
                convertCompleted++
            } else if img.ConvertStatus == "FALLIDO" {
                convertFailed++
            }

            // Contadores por etapa (watermark) - incluye COMPLETADO_CON_ERRORES como completado
            if img.WatermarkStatus == "COMPLETADO" || img.WatermarkStatus == "COMPLETADO_CON_ERRORES" {
                watermarkCompleted++
            } else if img.WatermarkStatus == "FALLIDO" {
                watermarkFailed++
            }
        }

        // Calcular tiempo total de ejecución del job
        var totalExecutionTime float64
        if job.EndTime != nil {
            totalExecutionTime = job.EndTime.Sub(job.StartTime).Seconds()
        } else {
            totalExecutionTime = time.Since(job.StartTime).Seconds()
        }

        // Calcular porcentajes de éxito y fallo
        var successPercentage, failurePercentage float64
        if job.TotalFiles > 0 {
            successPercentage = float64(job.ProcessedFiles) / float64(job.TotalFiles) * 100
            failurePercentage = float64(job.FailedFiles) / float64(job.TotalFiles) * 100
        }

        // Calcular promedios por etapa (evitar división por cero)
        downloadAvg := 0.0
        if downloadCompleted > 0 {
            downloadAvg = totalDownload / float64(downloadCompleted)
        }
        resizeAvg := 0.0
        if resizeCompleted > 0 {
            resizeAvg = totalResize / float64(resizeCompleted)
        }
        convertAvg := 0.0
        if convertCompleted > 0 {
            convertAvg = totalConvert / float64(convertCompleted)
        }
        watermarkAvg := 0.0
        if watermarkCompleted > 0 {
            watermarkAvg = totalWatermark / float64(watermarkCompleted)
        }

        // Formatear la respuesta final
        response := fiber.Map{
            "job_info": job,
            "global_metrics": fiber.Map{
                "total_download_time_sec":  totalDownload,
                "total_resize_time_sec":    totalResize,
                "total_convert_time_sec":   totalConvert,
                "total_watermark_time_sec": totalWatermark,
                "total_execution_time_sec": totalExecutionTime,
                "success_percentage":       successPercentage,
                "failure_percentage":       failurePercentage,
            },
            "stage_metrics": fiber.Map{
                "download": fiber.Map{
                    "total_processed": downloadCompleted,
                    "total_failed":    downloadFailed,
                    "total_time_sec":  totalDownload,
                    "avg_time_sec":    downloadAvg,
                },
                "resize": fiber.Map{
                    "total_processed": resizeCompleted,
                    "total_failed":    resizeFailed,
                    "total_time_sec":  totalResize,
                    "avg_time_sec":    resizeAvg,
                },
                "convert": fiber.Map{
                    "total_processed": convertCompleted,
                    "total_failed":    convertFailed,
                    "total_time_sec":  totalConvert,
                    "avg_time_sec":    convertAvg,
                },
                "watermark": fiber.Map{
                    "total_processed": watermarkCompleted,
                    "total_failed":    watermarkFailed,
                    "total_time_sec":  totalWatermark,
                    "avg_time_sec":    watermarkAvg,
                },
            },
        }

        return c.JSON(response)
    })

    // 4. Iniciar servicio de lectura (Puerto distinto)
    log.Println("Microservicio de Query iniciado en puerto 3001")
    log.Fatal(app.Listen(":3001"))
}
