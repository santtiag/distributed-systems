package queue

import (
    "encoding/json"
    "pmic/internal/config"

    "github.com/nats-io/nats.go"
)

const (
    SubjectDownload   = "images.download"
    SubjectResize     = "images.resize"
    SubjectConvert    = "images.convert"
    SubjectWatermark  = "images.watermark"
)

type Queue struct {
    Conn *nats.Conn
}

// Message estructura base para comunicación entre workers
type Message struct {
    JobID      string `json:"job_id"`
    ImageID    string `json:"image_id"`
    InputPath  string `json:"input_path"`  // Para etapas después de descarga
    OutputPath string `json:"output_path"`
    URL        string `json:"url"`         // Solo para descarga
    FileName   string `json:"file_name"`
}

func New(cfg *config.Config) (*Queue, error) {
    nc, err := nats.Connect(cfg.NATSURL)
    if err != nil {
        return nil, err
    }
    return &Queue{Conn: nc}, nil
}

func (q *Queue) Publish(subject string, msg Message) error {
    data, err := json.Marshal(msg)
    if err != nil {
        return err
    }
    return q.Conn.Publish(subject, data)
}

func (q *Queue) Subscribe(subject string, handler func(msg *nats.Msg)) (*nats.Subscription, error) {
    return q.Conn.Subscribe(subject, handler)
}

func (q *Queue) Close() {
    q.Conn.Close()
}
