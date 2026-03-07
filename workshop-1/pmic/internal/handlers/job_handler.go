package handlers

import (
    "pmic/internal/models"
    "pmic/internal/queue"
    "time"

    "github.com/gofiber/fiber/v2"
    "gorm.io/gorm"
)

type RequestPayload struct {
    URLs             []string `json:"urls"`
    WorkersDownload  int      `json:"workers_download"`
    WorkersResize    int      `json:"workers_resize"`
    WorkersConvert   int      `json:"workers_convert"`
    WorkersWatermark int      `json:"workers_watermark"`
}

type ResponsePayload struct {
    JobID   string `json:"job_id"`
    Message string `json:"message"`
}

type JobHandler struct {
    DB       *gorm.DB
    Pipeline *queue.PipelineChannels
}

func NewJobHandler(db *gorm.DB, p *queue.PipelineChannels) *JobHandler {
    return &JobHandler{DB: db, Pipeline: p}
}

func (h *JobHandler) CreateJob(c *fiber.Ctx) error {
    var payload RequestPayload

    if err := c.BodyParser(&payload); err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Payload inválido"})
    }

    if len(payload.URLs) == 0 {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Se requiere al menos una URL"})
    }

    // Crear Job en BD
    job := models.Job{
        Status:           models.StatusEnProceso,
        TotalFiles:       len(payload.URLs),
        ProcessedFiles:   0,
        FailedFiles:      0,
        StartTime:        time.Now(),
        WorkersDownload:  payload.WorkersDownload,
        WorkersResize:    payload.WorkersResize,
        WorkersConvert:   payload.WorkersConvert,
        WorkersWatermark: payload.WorkersWatermark,
    }

    if err := h.DB.Create(&job).Error; err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Error creando el Job"})
    }

    // Guardar las imágenes y mandar al canal nativo de Go
    for _, url := range payload.URLs {
        image := models.Image{
            JobID:           job.ID,
            OriginalURL:     url,
            DownloadStatus:  "PENDING",
            ResizeStatus:    "PENDING",
            ConvertStatus:   "PENDING",
            WatermarkStatus: "PENDING",
        }

        if err := h.DB.Create(&image).Error; err == nil {
            taskMsg := queue.Message{
                JobID:   job.ID,
                ImageID: image.ID,
                URL:     url,
            }
            // Enviar a la cola en memoria
            h.Pipeline.DownloadChan <- taskMsg
        }
    }

    return c.Status(fiber.StatusAccepted).JSON(ResponsePayload{
        JobID:   job.ID,
        Message: "Procesamiento en memoria iniciado correctamente",
    })
}
