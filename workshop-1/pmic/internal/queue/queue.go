package queue

// Message es la estructura de datos que viaja por los canales
type Message struct {
    JobID      string `json:"job_id"`
    ImageID    string `json:"image_id"`
    InputPath  string `json:"input_path"`
    OutputPath string `json:"output_path"`
    URL        string `json:"url"`
    FileName   string `json:"file_name"`
}

// PipelineChannels contiene todos los canales (colas) del sistema
type PipelineChannels struct {
    DownloadChan  chan Message
    ResizeChan    chan Message
    ConvertChan   chan Message
    WatermarkChan chan Message
}

// NewPipeline crea las colas en memoria con un buffer para no bloquear
func NewPipeline() *PipelineChannels {
    return &PipelineChannels{
        DownloadChan:  make(chan Message, 1000),
        ResizeChan:    make(chan Message, 1000),
        ConvertChan:   make(chan Message, 1000),
        WatermarkChan: make(chan Message, 1000),
    }
}
