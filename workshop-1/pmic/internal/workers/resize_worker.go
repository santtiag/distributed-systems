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

type ResizeWorker struct {
    WorkerID string
    DB       *gorm.DB
    InChan   <-chan queue.Message // Canal de lectura (Resize)
    Storage  string
}

func (w *ResizeWorker) Start() {
    fmt.Printf("Worker de redimensionamiento iniciado: %s\n", w.WorkerID)
    go func() {
        for task := range w.InChan {
            w.processMessage(task)
        }
    }()
}

func (w *ResizeWorker) processMessage(task queue.Message) {
    fmt.Printf("[%s] Iniciando redimensionamiento para imagen: %s\n", w.WorkerID, task.ImageID)
    startTime := time.Now()

    // Actualizar estado a PROCESSING
    w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Update("resize_status", "PROCESSING")

    // Leer de la DB el download_path y esperar si es necesario
    var image models.Image
    if err := w.DB.First(&image, "id = ?", task.ImageID).Error; err != nil {
        w.marcarFallo(task.ImageID, fmt.Sprintf("Error leyendo imagen de DB: %v", err))
        return
    }

    // Si la descarga falló, no podemos procesar
    if image.DownloadStatus == models.StatusFallido {
        w.marcarFallo(task.ImageID, "No se puede redimensionar: la descarga falló")
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
            w.marcarFallo(task.ImageID, "No se puede redimensionar: la descarga falló")
            return
        }
    }

    inputPath := image.DownloadPath
    img, err := imaging.Open(inputPath)
    if err != nil {
        w.marcarFallo(task.ImageID, fmt.Sprintf("Error abriendo imagen: %v", err))
        return
    }

    bounds := img.Bounds()
    origWidth := float64(bounds.Dx())
    origHeight := float64(bounds.Dy())
	
    targetAncho := 1200.0
    targetAlto := origHeight * (targetAncho / origWidth)
    newWidthIDx := int(targetAncho)
    newHeightIDx := int(targetAlto)

    resizedImg := imaging.Resize(img, newWidthIDx, newHeightIDx, imaging.Lanczos)

    // Usar el nombre de archivo desde la base de datos
    ext := filepath.Ext(image.FileName)
    baseName := strings.TrimSuffix(image.FileName, "_original"+ext)
    if baseName == image.FileName {
        baseName = strings.TrimSuffix(image.FileName, ext)
    }

    newFileName := fmt.Sprintf("%s_redimensionado%s", baseName, ext)
    newFilePath := filepath.Join(w.Storage, newFileName)

    err = imaging.Save(resizedImg, newFilePath)
    if err != nil {
        w.marcarFallo(task.ImageID, "Error guardando imagen redimensionada")
        return
    }

    resizeTimeSec := time.Since(startTime).Seconds()
    now := time.Now()
    w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Updates(map[string]any{
        "resize_status":    models.StatusCompletado,
        "resize_path":      newFilePath,
        "resize_time_sec":  resizeTimeSec,
        "resize_worker_id": w.WorkerID,
        "resized_at":       &now,
        "original_width":   int(origWidth),
        "original_height":  int(origHeight),
        "new_width":        newWidthIDx,
        "new_height":       newHeightIDx,
    })

    fmt.Printf("[%s] Redimensión exitosa: %s (%.2fs)\n", w.WorkerID, newFileName, resizeTimeSec)
}

func (w *ResizeWorker) marcarFallo(imageID string, errorMsg string) {
    fmt.Printf("[%s] Falló redimensionamiento de %s: %s\n", w.WorkerID, imageID, errorMsg)
    w.DB.Model(&models.Image{}).Where("id = ?", imageID).Updates(map[string]any{
        "resize_status": models.StatusFallido,
        "error_message": errorMsg,
    })
    w.DB.Exec("UPDATE jobs SET failed_files = failed_files + 1 WHERE id = (SELECT job_id FROM images WHERE id = ?)", imageID)
}
