package models

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/gorm"
)

// Estados del proceso
const (
    StatusEnProceso          = "EN_PROCESO"
    StatusCompletado         = "COMPLETADO"
    StatusCompletadoConErrores = "COMPLETADO_CON_ERRORES"
    StatusFallido            = "FALLIDO"
)

// Job representa un procesamiento completo (RF1, RF6)
type Job struct {
    ID               string         `gorm:"type:uuid;primaryKey" json:"id"`
    Status           string         `gorm:"type:varchar(50)" json:"status"`
    TotalFiles       int            `json:"total_files"`
    ProcessedFiles   int            `json:"processed_files"`
    FailedFiles      int            `json:"failed_files"`
    StartTime        time.Time      `json:"start_time"`
    EndTime          *time.Time     `json:"end_time,omitempty"`
    CreatedAt        time.Time      `json:"created_at"`
    UpdatedAt        time.Time      `json:"updated_at"`
    DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
    
    // Configuración de workers (del payload RF1)
    WorkersDownload      int `json:"workers_download"`
    WorkersResize        int `json:"workers_resize"`
    WorkersConvert       int `json:"workers_convert"`
    WorkersWatermark     int `json:"workers_watermark"`
    
    // Relación
    Images []Image `json:"images,omitempty" gorm:"foreignKey:JobID"`
}

// Image representa cada archivo procesado a través del pipeline (RF2-RF5)
type Image struct {
    ID              string         `gorm:"type:uuid;primaryKey" json:"id"`
    JobID           string         `gorm:"type:uuid;index" json:"job_id"`
    OriginalURL     string         `gorm:"type:text" json:"original_url"`
    FileName        string         `json:"file_name"` // Nombre base sin extensiones
    
    // Estado por etapa
    DownloadStatus    string     `gorm:"type:varchar(50)" json:"download_status"`    // PENDING, PROCESSING, COMPLETED, FAILED
    ResizeStatus      string     `gorm:"type:varchar(50)" json:"resize_status"`
    ConvertStatus     string     `gorm:"type:varchar(50)" json:"convert_status"`
    WatermarkStatus   string     `gorm:"type:varchar(50)" json:"watermark_status"`
    
    // Metadatos RF2: Descarga
    DownloadPath      string         `json:"download_path"`
    DownloadSizeMB    float64        `json:"download_size_mb"`
    DownloadTimeSec   float64        `json:"download_time_sec"`
    DownloadWorkerID  string         `json:"download_worker_id"`
    DownloadedAt      *time.Time     `json:"downloaded_at"`
    
    // Metadatos RF3: Redimensionamiento
    ResizePath        string         `json:"resize_path"`
    ResizeTimeSec     float64        `json:"resize_time_sec"`
    ResizeWorkerID    string         `json:"resize_worker_id"`
    ResizedAt         *time.Time     `json:"resized_at"`
    OriginalWidth     int            `json:"original_width"`
    OriginalHeight    int            `json:"original_height"`
    NewWidth          int            `json:"new_width"`
    NewHeight         int            `json:"new_height"`
    
    // Metadatos RF4: Conversión
    ConvertPath       string         `json:"convert_path"`
    ConvertTimeSec    float64        `json:"convert_time_sec"`
    ConvertWorkerID   string         `json:"convert_worker_id"`
    ConvertedAt       *time.Time     `json:"converted_at"`
    
    // Metadatos RF5: Marca de agua
    WatermarkPath     string         `json:"watermark_path"`
    WatermarkTimeSec  float64        `json:"watermark_time_sec"`
    WatermarkWorkerID string         `json:"watermark_worker_id"`
    WatermarkedAt     *time.Time     `json:"watermarked_at"`
    
    ErrorMessage      string         `json:"error_message,omitempty"`
    CreatedAt         time.Time      `json:"created_at"`
}

// BeforeCreate genera UUIDs automáticamente
func (j *Job) BeforeCreate(tx *gorm.DB) error {
    if j.ID == "" {
        j.ID = uuid.New().String()
    }
    return nil
}

func (i *Image) BeforeCreate(tx *gorm.DB) error {
    if i.ID == "" {
        i.ID = uuid.New().String()
    }
    return nil
}
