package handlers

import (
    "pmic/internal/models"
    "time"

    "github.com/gofiber/fiber/v2"
    "gorm.io/gorm"
)

// RequestPayload representa el JSON que envía el cliente (RF1)
type RequestPayload struct {
    URLs             []string `json:"urls"`
    WorkersDownload  int      `json:"workers_download"`
    WorkersResize    int      `json:"workers_resize"`
    WorkersConvert   int      `json:"workers_convert"`
    WorkersWatermark int      `json:"workers_watermark"`
}

// ResponsePayload representa la respuesta exitosa
type ResponsePayload struct {
    JobID   string `json:"job_id"`
    Message string `json:"message"`
}

type JobHandler struct {
    DB *gorm.DB
}

func NewJobHandler(db *gorm.DB) *JobHandler {
    return &JobHandler{DB: db}
}

// CreateJob maneja la recepción de la solicitud
func (h *JobHandler) CreateJob(c *fiber.Ctx) error {
    var payload RequestPayload

    // Parsear el JSON
    if err := c.BodyParser(&payload); err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "error": "Body inválido o mal formado",
        })
    }

    // Validaciones básicas
    if len(payload.URLs) == 0 {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "error": "Debe enviar al menos una URL",
        })
    }

    // Configuración de workers mínima (asegurar que haya al menos 1 por etapa)
    if payload.WorkersDownload <= 0 { payload.WorkersDownload = 1 }
    if payload.WorkersResize <= 0 { payload.WorkersResize = 1 }
    if payload.WorkersConvert <= 0 { payload.WorkersConvert = 1 }
    if payload.WorkersWatermark <= 0 { payload.WorkersWatermark = 1 }

    // Crear el registro del Job en la base de datos (RF1 y RF6)
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

    // Guardar el Job (GORM generará el ID UUID automáticamente gracias al hook BeforeCreate)
    if err := h.DB.Create(&job).Error; err != nil {
        return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
            "error": "Error creando el trabajo en la base de datos",
        })
    }

    // Guardar las imágenes iniciales pendientes de descarga
    for _, url := range payload.URLs {
        image := models.Image{
            JobID:          job.ID,
            OriginalURL:    url,
            DownloadStatus: "PENDING",
            ResizeStatus:   "PENDING",
            ConvertStatus:  "PENDING",
            WatermarkStatus: "PENDING",
        }
        h.DB.Create(&image)
        
        // NOTA: En el Paso 3 enviaremos el mensaje a NATS aquí para iniciar la descarga.
    }

    // Retornamos el identificador del procesamiento [1]
    return c.Status(fiber.StatusAccepted).JSON(ResponsePayload{
        JobID:   job.ID,
        Message: "Procesamiento iniciado correctamente",
    })
}
