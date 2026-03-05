package main

import (
    "time"
)

// Job refleja la tabla jobs
type Job struct {
    ID               string     `gorm:"type:uuid;primaryKey" json:"job_id"`
    Status           string     `json:"status"`
    TotalFiles       int        `json:"total_files"`
    ProcessedFiles   int        `json:"processed_files"`
    FailedFiles      int        `json:"failed_files"`
    StartTime        time.Time  `json:"start_time"`
    EndTime          *time.Time `json:"end_time"`
    WorkersDownload  int        `json:"workers_download"`
    WorkersResize    int        `json:"workers_resize"`
    WorkersConvert   int        `json:"workers_convert"`
    WorkersWatermark int        `json:"workers_watermark"`
    Images           []Image    `gorm:"foreignKey:JobID" json:"images,omitempty"`
}

// Image refleja la tabla images
type Image struct {
    ID                string     `gorm:"type:uuid;primaryKey" json:"image_id"`
    JobID             string     `json:"-"` // Oculto en el JSON
    OriginalURL       string     `json:"original_url"`
    
    // Paths finales
    WatermarkPath     string     `json:"final_path"`
    
    // Estados
    DownloadStatus    string     `json:"download_status"`
    ResizeStatus      string     `json:"resize_status"`
    ConvertStatus     string     `json:"convert_status"`
    WatermarkStatus   string     `json:"watermark_status"`
    
    // Métricas por etapa (RF6)
    DownloadTimeSec   float64    `json:"download_time_sec"`
    DownloadSizeMB    float64    `json:"download_size_mb"`
    ResizeTimeSec     float64    `json:"resize_time_sec"`
    ConvertTimeSec    float64    `json:"convert_time_sec"`
    WatermarkTimeSec  float64    `json:"watermark_time_sec"`
    
    // Identificación de workers
    DownloadWorkerID  string     `json:"download_worker"`
    ResizeWorkerID    string     `json:"resize_worker"`
    ConvertWorkerID   string     `json:"convert_worker"`
    WatermarkWorkerID string     `json:"watermark_worker"`
    
    ErrorMessage      string     `json:"error_message,omitempty"`
}
