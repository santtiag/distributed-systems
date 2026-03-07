package workers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pmic/internal/models"
	"pmic/internal/queue"

	"gorm.io/gorm"
)

type DownloadWorker struct {
	WorkerID string
	DB       *gorm.DB
	InChan   <-chan queue.Message // Canal de lectura
	OutChan  chan<- queue.Message // Canal de escritura
	Storage  string
}

func (w *DownloadWorker) Start() {
	fmt.Printf("Worker de descarga iniciado: %s\n", w.WorkerID)
	go func() {
		// Leemos continuamente del canal nativo
		for task := range w.InChan {
			w.processMessage(task)
		}
	}()
}

func (w *DownloadWorker) processMessage(task queue.Message) {
	fmt.Printf("[%s] Iniciando descarga: %s\n", w.WorkerID, task.URL)
	startTime := time.Now()

	w.DB.Model(&models.Image{}).Where("id = ?", task.ImageID).Update("download_status", "PROCESSING")

	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(task.URL)
	if err != nil || resp.StatusCode != http.StatusOK {
		w.marcarFallo(task.ImageID, fmt.Sprintf("Error HTTP: %v", err))
		return
	}
	defer resp.Body.Close()

	ext := getExtensionFromContentType(resp.Header.Get("Content-Type"))
	fileName := fmt.Sprintf("%s_original%s", task.ImageID, ext)
	filePath := filepath.Join(w.Storage, fileName)

	file, err := os.Create(filePath)
	if err != nil {
		w.marcarFallo(task.ImageID, "Error creando archivo local")
		return
	}
	defer file.Close()

	bytesWritten, err := io.Copy(file, resp.Body)
	if err != nil {
		w.marcarFallo(task.ImageID, "Error guardando bytes")
		return
	}

	downloadTimeSec := time.Since(startTime).Seconds()
	sizeMB := float64(bytesWritten) / (1024 * 1024)
	now := time.Now()

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

	nextTask := queue.Message{
		JobID:     task.JobID,
		ImageID:   task.ImageID,
		InputPath: filePath,
		FileName:  fileName,
	}

	// Enviar al siguiente canal de manera nativa
	w.OutChan <- nextTask
}

func (w *DownloadWorker) marcarFallo(imageID string, errorMsg string) {
	fmt.Printf("[%s] Falló la descarga de %s: %s\n", w.WorkerID, imageID, errorMsg)
	w.DB.Model(&models.Image{}).Where("id = ?", imageID).Updates(map[string]interface{}{
		"download_status": models.StatusFallido,
		"error_message":   errorMsg,
	})
	w.DB.Exec("UPDATE jobs SET failed_files = failed_files + 1 WHERE id = (SELECT job_id FROM images WHERE id = ?)", imageID)
}

func getExtensionFromContentType(contentType string) string {
	switch {
	case strings.Contains(contentType, "image/jpeg"):
		return ".jpg"
	case strings.Contains(contentType, "image/png"):
		return ".png"
	case strings.Contains(contentType, "image/webp"):
		return ".webp"
	case strings.Contains(contentType, "image/gif"):
		return ".gif"
	default:
		return ".jpg"
	}
}
