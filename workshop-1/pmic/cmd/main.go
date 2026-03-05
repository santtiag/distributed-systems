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

	// NOTE: Workers de Descargas
	// NUEVO: Instanciar fijos 5 workers de descarga (simulando pool concurrente)
	// NATS distribuirá los mensajes entre ellos (Patrón Consumidor Competitivo)
	for i := 1; i <= 5; i++ {
		worker := &workers.DownloadWorker{
			WorkerID: fmt.Sprintf("DL-Worker-%d", i),
			DB:       db,
			Queue:    q,
			Storage:  cfg.StoragePath,
		}
		if err := worker.Start(); err != nil {
			log.Fatalf("Error iniciando worker de descarga: %v", err)
		}
	}
	
	// NOTE: Workers de redimensionamiento
	// NUEVO: Instanciar workers de Redimensionamiento (RF3 CPU-bound)
	// Generalmente menor cantidad que los IO-bound
	for i := 1; i <= 3; i++ {
		worker := &workers.ResizeWorker{
			WorkerID: fmt.Sprintf("RSZ-Worker-%d", i),
			DB:       db,
			Queue:    q,
			Storage:  cfg.StoragePath,
		}
		if err := worker.Start(); err != nil {
			log.Fatalf("Error iniciando worker de redimensionamiento: %v", err)
		}
	}

	// NOTE: Worker de conversión de imagenes
	//  NUEVO: Instanciar workers de Conversión (RF4 CPU-bound pesado)
	// Para talleres, esto suele ser el cuello de botella principal
    for i := 1; i <= 3; i++ {
        worker := &workers.ConvertWorker{
            WorkerID: fmt.Sprintf("CNV-Worker-%d", i),
            DB:       db,
            Queue:    q,
            Storage:  cfg.StoragePath,
        }
        if err := worker.Start(); err != nil {
            log.Fatalf("Error iniciando worker de conversión: %v", err)
        }
    }
	
	// NOTE: Workder de marca de agua
	// NUEVO: Instanciar workers de Marca de Agua (RF5 CPU-bound)
	for i := 1; i <= 2; i++ {
        worker := &workers.WatermarkWorker{
            WorkerID: fmt.Sprintf("WMK-Worker-%d", i),
            DB:       db,
            Queue:    q,
            Storage:  cfg.StoragePath,
        }
        if err := worker.Start(); err != nil {
            log.Fatalf("Error iniciando worker de marca de agua: %v", err)
        }
    }

	// 4. Inicializar Fiber
	app := fiber.New()
	app.Use(logger.New()) // Middleware para ver las peticiones en consola

	// 5. Configurar Rutas
	jobHandler := handlers.NewJobHandler(db, q)

	api := app.Group("/api/v1")
	api.Post("/process", jobHandler.CreateJob) // RF1: Recepción de Solicitud

	// 6. Arrancar servidor
	log.Fatal(app.Listen(":5000"))
}
