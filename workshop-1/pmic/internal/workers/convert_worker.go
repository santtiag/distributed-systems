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

type ConvertWorker struct {
	WorkerID string
	DB       *gorm.DB
	Queue    *queue.Queue
	Storage  string
}

func (w *ConvertWorker) Start() error {
	_, err := w.Queue.Subscribe(queue.SubjectConvert, w.processMessage)
	if err != nil {
		return fmt.Errorf("error suscribiendo worker %s: %v", w.WorkerID, err)
	}
	fmt.Printf("Worker de conversión iniciado: %s\n", w.WorkerID)
	return nil
}

func (w *ConvertWorker) processMessage(msg *nats.Msg) {
	var task queue.Message
	if err := json.Unmarshal(msg.Data, &task); err != nil {
		fmt.Printf("[%s] Error decodificando mensaje: %v\n", w.WorkerID, err)
		return
	}

	fmt.Printf("[%s] Iniciando conversión formato: %s\n", w.WorkerID, task.FileName)

	startTime := time.Now()

	// 1. Marca estado de conversión a PROCESSING en BD
	w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Update("convert_status", "PROCESSING")

	// 2. Abrir la imagen redimensionada (pesada en memoria RAM)
	img, err := imaging.Open(task.InputPath)
	if err != nil {
		w.marcarFallo(task.ImageID, fmt.Sprintf("Error abriendo imagen para conversión: %v", err))
		return
	}

	// 3. Generar el nuevo nombre de archivo (RF4)
	// Quitamos la extensión vieja y cualquier sufijo previo que traiga la tarea de NATS
	extOriginal := filepath.Ext(task.FileName)
	base := strings.TrimSuffix(task.FileName, "_redimensionado"+extOriginal) // si venía del resize standard

	// Prevenir si el nombre base igual trae sufijos rebeldes
	if strings.Contains(base, "_redimensionado") {
		base = strings.Split(base, "_redimensionado")[0]
	}

	// Forzamos el nombre con el sufijo solicitado y extensión .png [1]
	newFileName := fmt.Sprintf("%s_formato_cambiado.png", base)
	newFilePath := filepath.Join(w.Storage, newFileName)

	// 4. Guardar como PNG (CPU-bound)
	// imaging.Save usa automáticamente el encoder PNG al ver la extensión ".png"
	err = imaging.Save(img, newFilePath)
	if err != nil {
		w.marcarFallo(task.ImageID, "Error ejecutando compresión PNG")
		return
	}

	// 5. Calcular métricas (RF4)
	convertTimeSec := time.Since(startTime).Seconds()
	now := time.Now()

	// 6. Actualizar BD con metadatos de conversión
	w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Updates(map[string]interface{}{
		"convert_status":    models.StatusCompletado,
		"convert_path":      newFilePath, // Path de la imagen .png
		"convert_time_sec":  convertTimeSec,
		"convert_worker_id": w.WorkerID,
		"converted_at":      &now,
	})

	fmt.Printf("[%s] Conversión PNG exitosa: %s (%.2fs)\n", w.WorkerID, newFileName, convertTimeSec)

	// 7. Enviar a la última etapa del pipeline: Marca de Agua (RF5)
	nextTask := queue.Message{
		JobID:     task.JobID,
		ImageID:   task.ImageID,
		InputPath: newFilePath, // Entrada para el próximo worker: el PNG gordo
		FileName:  newFileName,
	}

	w.Queue.Publish(queue.SubjectWatermark, nextTask)
}

func (w *ConvertWorker) marcarFallo(imageID string, errorMsg string) {
	fmt.Printf("[%s] Falló conversión de %s: %s\n", w.WorkerID, imageID, errorMsg)
	w.DB.Model(&models.Image{}).Where("id = ?", imageID).Updates(map[string]interface{}{
		"convert_status": models.StatusFallido,
		"error_message":  errorMsg,
	})
	w.DB.Exec("UPDATE jobs SET failed_files = failed_files + 1 WHERE id = (SELECT job_id FROM images WHERE id = ?)", imageID)
}
