package workers

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"

    "pmic/internal/models"
    "pmic/internal/queue"

    "github.com/nats-io/nats.go"
    "gorm.io/gorm"
)

type DownloadWorker struct {
    WorkerID string
    DB       *gorm.DB
    Queue    *queue.Queue
    Storage  string
}

// Start arranca la suscripción a la cola NATS
func (w *DownloadWorker) Start() error {
    _, err := w.Queue.Subscribe(queue.SubjectDownload, w.processMessage)
    if err != nil {
        return fmt.Errorf("error suscribiendo worker %s: %v", w.WorkerID, err)
    }
    fmt.Printf("Worker de descarga iniciado: %s\n", w.WorkerID)
    return nil
}

func (w *DownloadWorker) processMessage(msg *nats.Msg) {
    var task queue.Message
    if err := json.Unmarshal(msg.Data, &task); err != nil {
        fmt.Printf("[%s] Error decodificando mensaje: %v\n", w.WorkerID, err)
        return
    }

    fmt.Printf("[%s] Iniciando descarga: %s\n", w.WorkerID, task.URL)

    // Iniciar medición de tiempo (Requerimiento RF2)
    startTime := time.Now()

    // 1. Marcar estado como PROCESSING en BD
    w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Update("download_status", "PROCESSING")

    // 2. Realizar la petición HTTP con timeout
    client := http.Client{Timeout: 30 * time.Second}
    resp, err := client.Get(task.URL)
    if err != nil || resp.StatusCode != http.StatusOK {
        w.marcarFallo(task.ImageID, fmt.Sprintf("Error HTTP: %v", err))
        return
    }
    defer resp.Body.Close()

    // 3. Determinar extensión desde el Content-Type (ej: image/jpeg -> .jpg)
    ext := getExtensionFromContentType(resp.Header.Get("Content-Type"))
    
    // Generar nombre único: {ImageID}_original.jpg
    fileName := fmt.Sprintf("%s_original%s", task.ImageID, ext)
    filePath := filepath.Join(w.Storage, fileName)

    // 4. Crear el archivo en el sistema (Almacenamiento temporal RF2)
    file, err := os.Create(filePath)
    if err != nil {
        w.marcarFallo(task.ImageID, "Error creando archivo local")
        return
    }
    defer file.Close()

    // Escribir datos y calcular tamaño
    bytesWritten, err := io.Copy(file, resp.Body)
    if err != nil {
        w.marcarFallo(task.ImageID, "Error guardando bytes")
        return
    }

    // Calcular métricas exactas solicitadas (RF2)
    downloadTimeSec := time.Since(startTime).Seconds()
    sizeMB := float64(bytesWritten) / (1024 * 1024)
    now := time.Now()

    // 5. Actualizar la base de datos con todos los metadatos [1]
    w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Updates(map[string]interface{}{
        "download_status":    models.StatusCompletado,
        "file_name":          fileName,
        "download_path":      filePath,
        "download_size_mb":   sizeMB,
        "download_time_sec":  downloadTimeSec,
        "download_worker_id": w.WorkerID,
        "downloaded_at":      &now,
    })

    fmt.Printf("[%s] Descarga exitosa: %s (%.2f MB, %.2fs)\n", w.WorkerID, fileName, sizeMB, downloadTimeSec)

    // 6. Enviar a la siguiente cola en el pipeline: Redimensionamiento (RF3)
    nextTask := queue.Message{
        JobID:      task.JobID,
        ImageID:    task.ImageID,
        InputPath:  filePath, 
        FileName:   fileName,
    }
    
    // Publicamos en la cola de resize para que el siguiente worker la tome
    w.Queue.Publish(queue.SubjectResize, nextTask)
}

func (w *DownloadWorker) marcarFallo(imageID string, errorMsg string) {
    fmt.Printf("[%s] Falló la descarga de %s: %s\n", w.WorkerID, imageID, errorMsg)
    w.DB.Model(&models.Image{}).Where("id = ?", imageID).Updates(map[string]interface{}{
        "download_status": models.StatusFallido,
        "error_message":   errorMsg,
    })
    // También actualizamos el Job sumando un fallo (para RF6)
    w.DB.Exec("UPDATE jobs SET failed_files = failed_files + 1 WHERE id = (SELECT job_id FROM images WHERE id = ?)", imageID)
}

// Función auxiliar para deducir extensión
func getExtensionFromContentType(contentType string) string {
    switch {
    case strings.Contains(contentType, "image/jpeg"): return ".jpg"
    case strings.Contains(contentType, "image/png"): return ".png"
    case strings.Contains(contentType, "image/webp"): return ".webp"
    case strings.Contains(contentType, "image/gif"): return ".gif"
    default: return ".jpg" // Fallback por defecto
    }
}
