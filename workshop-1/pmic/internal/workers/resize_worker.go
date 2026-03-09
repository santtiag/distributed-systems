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
    InChan   <-chan queue.Message // Canal de lectura (Redimensión)
    OutChan  chan<- queue.Message // Canal de escritura (Conversión)
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
    fmt.Printf("[%s] Iniciando redimensionamiento: %s\n", w.WorkerID, task.FileName)
    startTime := time.Now()

    w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Update("resize_status", "PROCESSING")

    img, err := imaging.Open(task.InputPath)
    if err != nil {
        w.marcarFallo(task.ImageID, fmt.Sprintf("Error abriendo imagen: %v", err))
        return
    }

    bounds := img.Bounds()
    origWidth := float64(bounds.Dx())
    origHeight := float64(bounds.Dy())
	
	// WARNING: Tener en cuenta
    targetAncho := 800.0
    targetAlto := origHeight * (targetAncho / origWidth)
    newWidthIDx := int(targetAncho)
    newHeightIDx := int(targetAlto)

    resizedImg := imaging.Resize(img, newWidthIDx, newHeightIDx, imaging.Lanczos)

    ext := filepath.Ext(task.FileName)
    baseName := strings.TrimSuffix(task.FileName, "_original"+ext)
    if !strings.Contains(baseName, "_original") {
        baseName = strings.TrimSuffix(task.FileName, ext)
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

    nextTask := queue.Message{
        JobID:     task.JobID,
        ImageID:   task.ImageID,
        InputPath: newFilePath,
        FileName:  newFileName,
    }

    w.OutChan <- nextTask
}

func (w *ResizeWorker) marcarFallo(imageID string, errorMsg string) {
    fmt.Printf("[%s] Falló redimensionamiento de %s: %s\n", w.WorkerID, imageID, errorMsg)
    w.DB.Model(&models.Image{}).Where("id = ?", imageID).Updates(map[string]any{
        "resize_status": models.StatusFallido,
        "error_message": errorMsg,
    })
    w.DB.Exec("UPDATE jobs SET failed_files = failed_files + 1 WHERE id = (SELECT job_id FROM images WHERE id = ?)", imageID)
}
