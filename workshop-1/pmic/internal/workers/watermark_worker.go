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
    "github.com/fogleman/gg"
    "github.com/nats-io/nats.go"
    "gorm.io/gorm"
)

type WatermarkWorker struct {
    WorkerID string
    DB       *gorm.DB
    Queue    *queue.Queue
    Storage  string
}

func (w *WatermarkWorker) Start() error {
    _, err := w.Queue.Subscribe(queue.SubjectWatermark, w.processMessage)
    if err != nil {
        return fmt.Errorf("error suscribiendo worker %s: %v", w.WorkerID, err)
    }
    fmt.Printf("Worker de marca de agua iniciado: %s\n", w.WorkerID)
    return nil
}

func (w *WatermarkWorker) processMessage(msg *nats.Msg) {
    var task queue.Message
    if err := json.Unmarshal(msg.Data, &task); err != nil {
        fmt.Printf("[%s] Error decodificando mensaje: %v\n", w.WorkerID, err)
        return
    }

    fmt.Printf("[%s] Aplicando marca de agua a: %s\n", w.WorkerID, task.FileName)

    startTime := time.Now()

    // 1. Actualizar estado
    w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Update("watermark_status", "PROCESSING")

    // 2. Cargar imagen origen (la PNG del paso anterior)
    img, err := imaging.Open(task.InputPath)
    if err != nil {
        w.marcarFallo(task.JobID, task.ImageID, fmt.Sprintf("Error abriendo imagen: %v", err))
        return
    }

    // 3. Crear el contexto para dibujar la marca de agua con 'gg'
    bounds := img.Bounds()
    width := float64(bounds.Dx())
    height := float64(bounds.Dy())
    
    dc := gg.NewContextForImage(img)
    
    // Dibujar un borde/banner negro semi-transparente abajo (alto de 50px)
    dc.SetRGBA(0, 0, 0, 0.6) // rgba(R, G, B, Opacidad)
    dc.DrawRectangle(0, height-50, width, 50)
    dc.Fill()

    // Escribir el texto encima del banner
    dc.SetRGB(1, 1, 1) // Blanco
    // DrawStringAnchored centra el texto en las coordenadas dadas
    dc.DrawStringAnchored("WATERMARK - PMIC", width/2, height-25, 0.5, 0.5)

    outImg := dc.Image()

    // 4. Nombre del archivo final (Sufijo exigido por RF5)
    extOriginal := filepath.Ext(task.FileName)
    base := strings.TrimSuffix(task.FileName, "_formato_cambiado"+extOriginal)
    
    // Limpieza por seguridad
    base = strings.Split(base, "_formato_cambiado")[0]
    base = strings.Split(base, "_redimensionado")[0]
    base = strings.Split(base, "_original")[0]

    // Guardaremos como PNG para no perder la calidad de los pixeles editados
    newFileName := fmt.Sprintf("%s_marca_agua.png", base)
    newFilePath := filepath.Join(w.Storage, newFileName)

    // 5. Guardar la imagen final
    err = imaging.Save(outImg, newFilePath)
    if err != nil {
        w.marcarFallo(task.JobID, task.ImageID, "Error guardando imagen con marca de agua")
        return
    }

    // 6. Calcular métricas (RF5)
    watermarkTimeSec := time.Since(startTime).Seconds()
    now := time.Now()

    // 7. Actualizar el registro de la Imagen
    w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Updates(map[string]any{
        "watermark_status":    models.StatusCompletado,
        "watermark_path":      newFilePath,
        "watermark_time_sec":  watermarkTimeSec,
        "watermark_worker_id": w.WorkerID,
        "watermarked_at":      &now,
    })

    fmt.Printf("[%s] Marca de agua finalizada: %s (%.2fs)\n", w.WorkerID, newFileName, watermarkTimeSec)

    // 8. LOGICA DE CIERRE: Actualizar el contador del Job y verificar si terminó
    w.actualizarProgresoJob(task.JobID)
}

func (w *WatermarkWorker) marcarFallo(jobID string, imageID string, errorMsg string) {
    fmt.Printf("[%s] Falló marca de agua de %s: %s\n", w.WorkerID, imageID, errorMsg)
    w.DB.Model(&models.Image{}).Where("id = ?", imageID).Updates(map[string]any{
        "watermark_status": models.StatusFallido,
        "error_message":    errorMsg,
    })
    w.DB.Exec("UPDATE jobs SET failed_files = failed_files + 1 WHERE id = ?", jobID)
    w.actualizarProgresoJob(jobID) // Aún si falla, hay que verificar si era la última
}

// actualizarProgresoJob incrementa el contador y cierra el job si es necesario
func (w *WatermarkWorker) actualizarProgresoJob(jobID string) {
    // Incrementar procesados (se asume que si llegó aquí y no falló, se procesó)
    // Nota: esta es una forma simple usando transacciones atómicas de DB
    w.DB.Exec("UPDATE jobs SET processed_files = processed_files + 1 WHERE id = ?", jobID)

    var job models.Job
    w.DB.First(&job, "id = ?", jobID)

    // Si el total de archivos se completó (ya sea por éxito o fallo en el camino)
    if job.ProcessedFiles+job.FailedFiles >= job.TotalFiles {
        status := models.StatusCompletado
        if job.FailedFiles > 0 {
            status = models.StatusCompletadoConErrores // Agregamos este matiz al estado
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
