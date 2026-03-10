package workers

import (
    "fmt"
    "path/filepath"
    "strings"
    "time"

    "pmic/internal/models"
    "pmic/internal/queue"

    "github.com/disintegration/imaging"
    "github.com/fogleman/gg"
    "gorm.io/gorm"
)

type WatermarkWorker struct {
    WorkerID string
    DB       *gorm.DB
    InChan   <-chan queue.Message // Canal de Marca de agua
    Storage  string
}

func (w *WatermarkWorker) Start() {
    fmt.Printf("Worker de marca de agua iniciado: %s\n", w.WorkerID)
    go func() {
        for task := range w.InChan {
            w.processMessage(task)
        }
    }()
}

func (w *WatermarkWorker) processMessage(task queue.Message) {
    fmt.Printf("[%s] Aplicando marca de agua a imagen: %s\n", w.WorkerID, task.ImageID)
    startTime := time.Now()

    // Actualizar estado a PROCESSING
    w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Update("watermark_status", "PROCESSING")

    // Leer de la DB el download_path y esperar si es necesario
    var image models.Image
    if err := w.DB.First(&image, "id = ?", task.ImageID).Error; err != nil {
        w.marcarFallo(task.JobID, task.ImageID, fmt.Sprintf("Error leyendo imagen de DB: %v", err))
        return
    }

    // Si la descarga falló, no podemos procesar
    if image.DownloadStatus == models.StatusFallido {
        w.marcarFallo(task.JobID, task.ImageID, "No se puede aplicar marca de agua: la descarga falló")
        return
    }

    // Esperar a que la descarga esté completada
    for image.DownloadStatus != models.StatusCompletado {
        time.Sleep(100 * time.Millisecond)
        if err := w.DB.First(&image, "id = ?", task.ImageID).Error; err != nil {
            w.marcarFallo(task.JobID, task.ImageID, fmt.Sprintf("Error leyendo imagen de DB: %v", err))
            return
        }
        if image.DownloadStatus == models.StatusFallido {
            w.marcarFallo(task.JobID, task.ImageID, "No se puede aplicar marca de agua: la descarga falló")
            return
        }
    }

    inputPath := image.DownloadPath
    img, err := imaging.Open(inputPath)
    if err != nil {
        w.marcarFallo(task.JobID, task.ImageID, fmt.Sprintf("Error abriendo imagen: %v", err))
        return
    }

    bounds := img.Bounds()
    width := float64(bounds.Dx())
    height := float64(bounds.Dy())

    dc := gg.NewContextForImage(img)
    dc.SetRGBA(0, 0, 0, 0.6)
    dc.DrawRectangle(0, height-50, width, 50)
    dc.Fill()

    dc.SetRGB(1, 1, 1)
    dc.DrawStringAnchored("CECAR, HOY TE AMO MENOS QUE AYER", width/2, height-25, 0.5, 0.5)
    outImg := dc.Image()

    // Usar el nombre de archivo desde la base de datos
    extOriginal := filepath.Ext(image.FileName)
    base := strings.TrimSuffix(image.FileName, "_original"+extOriginal)
    if base == image.FileName {
        base = strings.TrimSuffix(image.FileName, extOriginal)
    }

    newFileName := fmt.Sprintf("%s_marca_agua%s", base)
    newFilePath := filepath.Join(w.Storage, newFileName)

    err = imaging.Save(outImg, newFilePath)
    if err != nil {
        w.marcarFallo(task.JobID, task.ImageID, "Error guardando imagen con marca de agua")
        return
    }

    watermarkTimeSec := time.Since(startTime).Seconds()
    now := time.Now()

    w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Updates(map[string]any{
        "watermark_status":    models.StatusCompletado,
        "watermark_path":      newFilePath,
        "watermark_time_sec":  watermarkTimeSec,
        "watermark_worker_id": w.WorkerID,
        "watermarked_at":      &now,
    })

    fmt.Printf("[%s] Marca de agua finalizada: %s (%.2fs)\n", w.WorkerID, newFileName, watermarkTimeSec)
    w.actualizarProgresoJob(task.JobID)
}

func (w *WatermarkWorker) marcarFallo(jobID string, imageID string, errorMsg string) {
    fmt.Printf("[%s] Falló marca de agua de %s: %s\n", w.WorkerID, imageID, errorMsg)
    w.DB.Model(&models.Image{}).Where("id = ?", imageID).Updates(map[string]any{
        "watermark_status": models.StatusFallido,
        "error_message":    errorMsg,
    })
    w.DB.Exec("UPDATE jobs SET failed_files = failed_files + 1 WHERE id = ?", jobID)
    w.actualizarProgresoJob(jobID)
}

func (w *WatermarkWorker) actualizarProgresoJob(jobID string) {
    w.DB.Exec("UPDATE jobs SET processed_files = processed_files + 1 WHERE id = ?", jobID)
    var job models.Job
    w.DB.First(&job, "id = ?", jobID)

    if job.ProcessedFiles+job.FailedFiles >= job.TotalFiles {
        status := models.StatusCompletado
        if job.FailedFiles > 0 {
            status = models.StatusCompletadoConErrores
        }

        now := time.Now()
        w.DB.Model(&job).Updates(map[string]any{
            "status":   status,
            "end_time": &now,
        })
        fmt.Printf("🎯 [SISTEMA] JOB FINALIZADO: %s | Estado: %s | Procesadas: %d | Fallidas: %d\n",
            jobID, status, job.ProcessedFiles, job.FailedFiles)
    }
}
