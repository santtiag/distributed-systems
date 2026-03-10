package workers

import (
    "fmt"
    "path/filepath"
    "strings"
    "time"

    "pmic/internal/models"
    "pmic/internal/queue"

    "github.com/disintegration/imaging"
    "gorm.io/gorm"
)

type ConvertWorker struct {
    WorkerID string
    DB       *gorm.DB
    InChan   <-chan queue.Message // Canal de conversión
    Storage  string
}

func (w *ConvertWorker) Start() {
    fmt.Printf("Worker de conversión iniciado: %s\n", w.WorkerID)
    go func() {
        for task := range w.InChan {
            w.processMessage(task)
        }
    }()
}

func (w *ConvertWorker) processMessage(task queue.Message) {
    fmt.Printf("[%s] Iniciando conversión formato para imagen: %s\n", w.WorkerID, task.ImageID)
    startTime := time.Now()

    // Actualizar estado a PROCESSING
    w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Update("convert_status", "PROCESSING")

    // Leer de la DB el download_path y esperar si es necesario
    var image models.Image
    if err := w.DB.First(&image, "id = ?", task.ImageID).Error; err != nil {
        w.marcarFallo(task.ImageID, fmt.Sprintf("Error leyendo imagen de DB: %v", err))
        return
    }

    // Si la descarga falló, no podemos procesar
    if image.DownloadStatus == models.StatusFallido {
        w.marcarFallo(task.ImageID, "No se puede convertir: la descarga falló")
        return
    }

    // Esperar a que la descarga esté completada
    for image.DownloadStatus != models.StatusCompletado {
        time.Sleep(100 * time.Millisecond)
        if err := w.DB.First(&image, "id = ?", task.ImageID).Error; err != nil {
            w.marcarFallo(task.ImageID, fmt.Sprintf("Error leyendo imagen de DB: %v", err))
            return
        }
        if image.DownloadStatus == models.StatusFallido {
            w.marcarFallo(task.ImageID, "No se puede convertir: la descarga falló")
            return
        }
    }

    inputPath := image.DownloadPath
    img, err := imaging.Open(inputPath)
    if err != nil {
        w.marcarFallo(task.ImageID, fmt.Sprintf("Error abriendo imagen para conversión: %v", err))
        return
    }

    // Usar el nombre de archivo desde la base de datos
    extOriginal := filepath.Ext(image.FileName)
    base := strings.TrimSuffix(image.FileName, "_original"+extOriginal)
    if base == image.FileName {
        base = strings.TrimSuffix(image.FileName, extOriginal)
    }

    newFileName := fmt.Sprintf("%s_formato_cambiado.png", base)
    newFilePath := filepath.Join(w.Storage, newFileName)

    err = imaging.Save(img, newFilePath)
    if err != nil {
        w.marcarFallo(task.ImageID, "Error ejecutando compresión PNG")
        return
    }

    convertTimeSec := time.Since(startTime).Seconds()
    now := time.Now()

    w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Updates(map[string]interface{}{
        "convert_status":    models.StatusCompletado,
        "convert_path":      newFilePath,
        "convert_time_sec":  convertTimeSec,
        "convert_worker_id": w.WorkerID,
        "converted_at":      &now,
    })

    fmt.Printf("[%s] Conversión PNG exitosa: %s (%.2fs)\n", w.WorkerID, newFileName, convertTimeSec)
}

func (w *ConvertWorker) marcarFallo(imageID string, errorMsg string) {
    fmt.Printf("[%s] Falló conversión de %s: %s\n", w.WorkerID, imageID, errorMsg)
    w.DB.Model(&models.Image{}).Where("id = ?", imageID).Updates(map[string]interface{}{
        "convert_status": models.StatusFallido,
        "error_message":  errorMsg,
    })
    w.DB.Exec("UPDATE jobs SET failed_files = failed_files + 1 WHERE id = (SELECT job_id FROM images WHERE id = ?)", imageID)
}
