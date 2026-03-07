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
    OutChan  chan<- queue.Message // Canal de marca de agua
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
    fmt.Printf("[%s] Iniciando conversión formato: %s\n", w.WorkerID, task.FileName)
    startTime := time.Now()

    w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Update("convert_status", "PROCESSING")

    img, err := imaging.Open(task.InputPath)
    if err != nil {
        w.marcarFallo(task.ImageID, fmt.Sprintf("Error abriendo imagen para conversión: %v", err))
        return
    }

    extOriginal := filepath.Ext(task.FileName)
    base := strings.TrimSuffix(task.FileName, "_redimensionado"+extOriginal)
    if strings.Contains(base, "_redimensionado") {
        base = strings.Split(base, "_redimensionado")[0]
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

    nextTask := queue.Message{
        JobID:     task.JobID,
        ImageID:   task.ImageID,
        InputPath: newFilePath,
        FileName:  newFileName,
    }
    w.OutChan <- nextTask
}

func (w *ConvertWorker) marcarFallo(imageID string, errorMsg string) {
    fmt.Printf("[%s] Falló conversión de %s: %s\n", w.WorkerID, imageID, errorMsg)
    w.DB.Model(&models.Image{}).Where("id = ?", imageID).Updates(map[string]interface{}{
        "convert_status": models.StatusFallido,
        "error_message":  errorMsg,
    })
    w.DB.Exec("UPDATE jobs SET failed_files = failed_files + 1 WHERE id = (SELECT job_id FROM images WHERE id = ?)", imageID)
}
