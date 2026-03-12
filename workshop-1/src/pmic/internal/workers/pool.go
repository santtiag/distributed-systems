package workers

import (
	"fmt"
	"sync"

	"pmic/internal/queue"

	"gorm.io/gorm"
)

// DynamicWorkerPool gestiona los workers de forma dinámica
type DynamicWorkerPool struct {
	db      *gorm.DB
	storage string
	pipeline *queue.PipelineChannels

	// Contadores de workers activos por tipo
	downloadWorkers  int
	resizeWorkers    int
	convertWorkers   int
	watermarkWorkers int

	// Mutex para proteger los contadores
	mu sync.RWMutex
}

// NewDynamicWorkerPool crea un nuevo pool de workers dinámico
func NewDynamicWorkerPool(db *gorm.DB, storage string, pipeline *queue.PipelineChannels) *DynamicWorkerPool {
	return &DynamicWorkerPool{
		db:       db,
		storage:  storage,
		pipeline: pipeline,
	}
}

// EnsureWorkers garantiza que haya al menos el número especificado de workers por tipo
func (p *DynamicWorkerPool) EnsureWorkers(downloadCount, resizeCount, convertCount, watermarkCount int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Escala workers de descarga
	if downloadCount > p.downloadWorkers {
		workersToAdd := downloadCount - p.downloadWorkers
		for i := 0; i < workersToAdd; i++ {
			p.downloadWorkers++
			workerID := fmt.Sprintf("DL-Worker-%d", p.downloadWorkers)
			worker := &DownloadWorker{
				WorkerID: workerID,
				DB:       p.db,
				InChan:   p.pipeline.DownloadChan,
				Storage:  p.storage,
			}
			worker.Start()
		}
	}

	// Escala workers de redimensión
	if resizeCount > p.resizeWorkers {
		workersToAdd := resizeCount - p.resizeWorkers
		for i := 0; i < workersToAdd; i++ {
			p.resizeWorkers++
			workerID := fmt.Sprintf("RSZ-Worker-%d", p.resizeWorkers)
			worker := &ResizeWorker{
				WorkerID: workerID,
				DB:       p.db,
				InChan:   p.pipeline.ResizeChan,
				Storage:  p.storage,
			}
			worker.Start()
		}
	}

	// Escala workers de conversión
	if convertCount > p.convertWorkers {
		workersToAdd := convertCount - p.convertWorkers
		for i := 0; i < workersToAdd; i++ {
			p.convertWorkers++
			workerID := fmt.Sprintf("CNV-Worker-%d", p.convertWorkers)
			worker := &ConvertWorker{
				WorkerID: workerID,
				DB:       p.db,
				InChan:   p.pipeline.ConvertChan,
				Storage:  p.storage,
			}
			worker.Start()
		}
	}

	// Escala workers de marca de agua
	if watermarkCount > p.watermarkWorkers {
		workersToAdd := watermarkCount - p.watermarkWorkers
		for i := 0; i < workersToAdd; i++ {
			p.watermarkWorkers++
			workerID := fmt.Sprintf("WMK-Worker-%d", p.watermarkWorkers)
			worker := &WatermarkWorker{
				WorkerID: workerID,
				DB:       p.db,
				InChan:   p.pipeline.WatermarkChan,
				Storage:  p.storage,
			}
			worker.Start()
		}
	}
}

// GetWorkerCounts devuelve el número actual de workers por tipo
func (p *DynamicWorkerPool) GetWorkerCounts() (download, resize, convert, watermark int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.downloadWorkers, p.resizeWorkers, p.convertWorkers, p.watermarkWorkers
}
