package workers

import (
    "encoding/json"
    "fmt"
    "path/filepath"
    "strings"
    "time"

    "pmic/internal/models"
    "pmic/internal/queue"

    "github.com/disintegration/imaging"
    "github.com/nats-io/nats.go"
    "gorm.io/gorm"
)

type ResizeWorker struct {
    WorkerID string
    DB       *gorm.DB
    Queue    *queue.Queue
    Storage  string
}

func (w *ResizeWorker) Start() error {
    _, err := w.Queue.Subscribe(queue.SubjectResize, w.processMessage)
    if err != nil {
        return fmt.Errorf("error suscribiendo worker %s: %v", w.WorkerID, err)
    }
    fmt.Printf("Worker de redimensionamiento iniciado: %s\n", w.WorkerID)
    return nil
}

func (w *ResizeWorker) processMessage(msg *nats.Msg) {
    var task queue.Message
    if err := json.Unmarshal(msg.Data, &task); err != nil {
        fmt.Printf("[%s] Error decodificando mensaje: %v\n", w.WorkerID, err)
        return
    }

    fmt.Printf("[%s] Iniciando redimensionamiento: %s\n", w.WorkerID, task.FileName)

    startTime := time.Now()

    // 1. Actualizar estado en BD
    w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Update("resize_status", "PROCESSING")

    // 2. Cargar la imagen original en memoria
    img, err := imaging.Open(task.InputPath)
    if err != nil {
        w.marcarFallo(task.ImageID, fmt.Sprintf("Error abriendo imagen: %v", err))
        return
    }

    // 3. Obtener dimensiones originales
    bounds := img.Bounds()
    origWidth := float64(bounds.Dx())
    origHeight := float64(bounds.Dy())

    // 4. Aplicar la FÓRMULA EXACTA DEL USUARIO
    // Asumimos un nuevo_ancho fijo de 800px para el cálculo
    targetAncho := 800.0
    
    // nuevo_alto = alto_original * (nuevo_ancho/ancho_original)
    targetAlto := origHeight * (targetAncho / origWidth)

    newWidthIDx := int(targetAncho)
    newHeightIDx := int(targetAlto)

    // 5. Ejecutar el redimensionamiento matemático (CPU-bound) usando un algoritmo de alta calidad (Lanczos)
    // Como ya calculamos el targetAlto exacto, le pasamos las dimensiones calculadas.
    resizedImg := imaging.Resize(img, newWidthIDx, newHeightIDx, imaging.Lanczos)

    // 6. Generar nuevo nombre con el sufijo (RF3)
    // Ej: "uuid_original.jpg" -> "uuid_redimensionado.jpg"
    ext := filepath.Ext(task.FileName)
    baseName := strings.TrimSuffix(task.FileName, "_original"+ext)
    if !strings.Contains(baseName, "_original") { // Por seguridad si el nombre cambió
        baseName = strings.TrimSuffix(task.FileName, ext)
    }
    
    newFileName := fmt.Sprintf("%s_redimensionado%s", baseName, ext)
    newFilePath := filepath.Join(w.Storage, newFileName)

    // 7. Guardar la nueva imagen
    err = imaging.Save(resizedImg, newFilePath)
    if err != nil {
        w.marcarFallo(task.ImageID, "Error guardando imagen redimensionada")
        return
    }

    // 8. Calcular métricas (Requerimiento RF3)
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

    fmt.Printf("[%s] Redimensión exitosa: %s (%.2fs) - [%dx%d] -> [%dx%d]\n", 
        w.WorkerID, newFileName, resizeTimeSec, int(origWidth), int(origHeight), newWidthIDx, newHeightIDx)

    // 9. Enviar a la siguiente cola en el pipeline: Conversión de Formato (RF4)
    nextTask := queue.Message{
        JobID:      task.JobID,
        ImageID:    task.ImageID,
        InputPath:  newFilePath, // La entrada para la conversión será nuestra imagen redimensionada
        FileName:   newFileName,
    }
    
    w.Queue.Publish(queue.SubjectConvert, nextTask)
}

func (w *ResizeWorker) marcarFallo(imageID string, errorMsg string) {
    fmt.Printf("[%s] Falló redimensionamiento de %s: %s\n", w.WorkerID, imageID, errorMsg)
    w.DB.Model(&models.Image{}).Where("id = ?", imageID).Updates(map[string]any{
        "resize_status": models.StatusFallido,
        "error_message": errorMsg,
    })
    w.DB.Exec("UPDATE jobs SET failed_files = failed_files + 1 WHERE id = (SELECT job_id FROM images WHERE id = ?)", imageID)
}
