# PMIC - Plataforma de Procesamiento Masivo de Imágenes Concurrente

## Arquitectura de Software

Este documento describe la arquitectura de software del proyecto PMIC, una plataforma de procesamiento masivo de imágenes que implementa el patrón Productor-Consumidor utilizando las primitivas de concurrencia de Go (goroutines y channels).

---

## 1. Visión General de la Arquitectura

El sistema está compuesto por dos servicios principales:

| Servicio | Puerto | Responsabilidad |
|----------|--------|-----------------|
| **pmic** | 5000 | Servicio de procesamiento de imágenes. Expone API REST para crear jobs y ejecuta el pipeline concurrente de procesamiento. |
| **pmic-query** | 3001 | Servicio de consulta de estado. Proporciona endpoints REST para consultar el estado y métricas de los jobs de procesamiento. |

### Arquitectura de Alto Nivel

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CLIENTE (FRONTEND)                             │
└───────────────────────────────┬─────────────────────────────────────────────┘
                                │ HTTP
                                ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              LAYER DE API                                   │
│  ┌─────────────────┐                          ┌─────────────────────────┐   │
│  │  pmic:5000      │                          │  pmic-query:3001       │   │
│  │  POST /process  │                          │  GET /status/:job_id   │   │
│  └────────┬────────┘                          └───────────┬─────────────┘   │
└───────────┼───────────────────────────────────────────────────┼─────────────────┘
            │                                                  │
            │ ESCRITURA                                       │ LECTURA
            ▼                                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           BASE DE DATOS                                     │
│                      PostgreSQL (DATABASE_URL)                               │
│                                                                             │
│  ┌─────────────────────┐          ┌─────────────────────────────────────────┐ │
│  │  Tabla: jobs       │          │  Tabla: images                          │ │
│  │  - id (UUID)       │◄────────►│  - id (UUID)                            │ │
│  │  - status          │   1:N    │  - job_id (FK)                          │ │
│  │  - total_files     │          │  - original_url                         │ │
│  │  - processed_files │          │  - download/resize/convert/watermark   │ │
│  │  - start/end_time  │          │    _status, _path, _time_sec,           │ │
│  │  - workers_*       │          │    _worker_id                           │ │
│  └─────────────────────┘          └─────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Arquitectura del Servicio `pmic`

El servicio `pmic` implementa un **pipeline de procesamiento concurrente** utilizando canales de Go (channels) como mecanismo de colas entre etapas.

### 2.1 Estructura del Proyecto

```
pmic/
├── cmd/
│   └── main.go                    # Punto de entrada, orquestación de componentes
├── internal/
│   ├── config/
│   │   └── config.go              # Gestión de configuración (env vars)
│   ├── database/
│   │   └── database.go            # Conexión y migraciones PostgreSQL (GORM)
│   ├── models/
│   │   └── models.go              # Modelos de datos: Job e Image
│   ├── queue/
│   │   └── queue.go               # Definición de canales del pipeline
│   ├── handlers/
│   │   └── job_handler.go         # Handlers HTTP (API REST)
│   └── workers/
│       ├── download_worker.go     # Workers de descarga (IO-bound)
│       ├── resize_worker.go       # Workers de redimensionamiento (CPU-bound)
│       ├── convert_worker.go      # Workers de conversión de formato (CPU-bound)
│       └── watermark_worker.go    # Workers de marca de agua (CPU-bound)
└── storage/                        # Almacenamiento local de imágenes
```

### 2.2 Pipeline de Procesamiento Paralelo

El sistema implementa un **pipeline de 4 etapas** donde las últimas 3 etapas trabajan **en paralelo e independientemente**, leyendo todas directamente desde la imagen original en la base de datos:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      PIPELINE DE PROCESAMIENTO PARALELO                       │
│                                                                             │
│   ┌─────────────┐                                                           │
│   │   Request   │  POST /api/v1/process                                     │
│   │   HTTP      │  {urls: [...], workers_download: N, ...}                  │
│   └──────┬──────┘                                                           │
│          │                                                                  │
│          ▼                                                                  │
│   ┌─────────────┐    ┌─────────────────────────────────────────────────────┐  │
│   │  Creación  │    │  Base de Datos (PostgreSQL)                         │  │
│   │  de Job    │───►│  - INSERT INTO jobs (id, status, total_files...)   │  │
│   │            │    │  - INSERT INTO images (job_id, original_url...)     │  │
│   └──────┬──────┘    └─────────────────────────────────────────────────────┘  │
│          │                                                                  │
│          ▼                                                                  │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │     DISTRIBUCIÓN PARALELA A TODAS LAS COLAS (mismo mensaje)        │   │
│   │                                                                     │   │
│   │  ┌─────────────────────────────────────────────────────────────┐   │   │
│   │  │  for each image:                                            │   │   │
│   │  │    DownloadChan  <- msg  ─────────────────────────────┐     │   │   │
│   │  │    ResizeChan    <- msg  ─────────────────────────────┼──┐  │   │   │
│   │  │    ConvertChan   <- msg  ─────────────────────────────┼──┼──┐  │   │   │
│   │  │    WatermarkChan <- msg  ─────────────────────────────┼──┼──┼──┐  │   │   │
│   │  │                                                       │  │  │  │  │   │   │
│   │  └───────────────────────────────────────────────────────┼──┼──┼──┘  │   │   │
│   │                                                      ▼  ▼  ▼  ▼   │   │
│   │              CADA COLA ES INDEPENDIENTE - NO HAY CADENAS           │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                 WORKERS (GOROUTINES) - EJECUCIÓN PARALELA          │   │
│   │                                                                     │   │
│   │  ┌─────────────────────────────────────────────────────────────┐   │   │
│   │  │           POOL DE DOWNLOAD WORKERS (N configurables)          │   │   │
│   │  │  ┌─────────┐  ┌─────────┐  ┌─────────┐                       │   │   │
│   │  │  │Worker 1 │  │Worker 2 │  │Worker N │  ...                    │   │   │
│   │  │  └────┬────┘  └────┬────┘  └────┬────┘                       │   │   │
│   │  │       └────────────┴────────────┘                            │   │   │
│   │  │                    │                                          │   │   │
│   │  │         for msg := range DownloadChan                        │   │   │
│   │  │                    │                                          │   │   │
│   │  │  Proceso: 1. Descargar imagen desde URL                     │   │   │
│   │  │           2. Guardar en disco (_original)                     │   │   │
│   │  │           3. Actualizar DB (download_path, tiempo, etc)       │   │   │
│   │  │           [NO envía a siguiente cola - trabajan en paralelo] │   │   │
│   │  └───────────────────────────────────────────────────────────────┘   │   │
│   │                                                                     │   │
│   │  ┌─────────────────────────────────────────────────────────────┐   │   │
│   │  │           POOL DE RESIZE WORKERS (N configurables)            │   │   │
│   │  │                    │                                          │   │   │
│   │  │         for msg := range ResizeChan                          │   │   │
│   │  │                    │                                          │   │   │
│   │  │  Proceso: 1. Leer download_path desde DB (esperar si PENDING)│   │   │
│   │  │           2. Abrir imagen ORIGINAL desde download_path      │   │   │
│   │  │           3. Redimensionar a 800px (manteniendo proporción) │   │   │
│   │  │           4. Guardar _redimensionado                          │   │   │
│   │  │           5. Actualizar DB (resize_path, etc)                 │   │   │
│   │  │           [LEE DIRECTO DE DB - NO depende de download]      │   │   │
│   │  └───────────────────────────────────────────────────────────────┘   │   │
│   │                                                                     │   │
│   │  ┌─────────────────────────────────────────────────────────────┐   │   │
│   │  │           POOL DE CONVERT WORKERS (N configurables)           │   │   │
│   │  │                    │                                          │   │   │
│   │  │         for msg := range ConvertChan                         │   │   │
│   │  │                    │                                          │   │   │
│   │  │  Proceso: 1. Leer download_path desde DB (esperar si PENDING)│   │   │
│   │  │           2. Abrir imagen ORIGINAL desde download_path      │   │   │
│   │  │           3. Convertir a PNG                                  │   │   │
│   │  │           4. Guardar _formato_cambiado                      │   │   │
│   │  │           5. Actualizar DB (convert_path, etc)              │   │   │
│   │  │           [LEE DIRECTO DE DB - NO depende de resize]        │   │   │
│   │  └───────────────────────────────────────────────────────────────┘   │   │
│   │                                                                     │   │
│   │  ┌─────────────────────────────────────────────────────────────┐   │   │
│   │  │          POOL DE WATERMARK WORKERS (N configurables)           │   │   │
│   │  │                    │                                          │   │   │
│   │  │         for msg := range WatermarkChan                       │   │   │
│   │  │                    │                                          │   │   │
│   │  │  Proceso: 1. Leer download_path desde DB (esperar si PENDING)│   │   │
│   │  │           2. Abrir imagen ORIGINAL desde download_path      │   │   │
│   │  │           3. Aplicar marca de agua                          │   │   │
│   │  │           4. Guardar _marca_agua                            │   │   │
│   │  │           5. Actualizar DB (watermark_path, etc)            │   │   │
│   │  │           6. Verificar si Job completado                    │   │   │
│   │  │           [LEE DIRECTO DE DB - NO depende de convert]       │   │   │
│   │  └───────────────────────────────────────────────────────────────┘   │   │
│   └───────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                    FLUJO DE DATOS                                   │   │
│   │                                                                     │   │
│   │  Resize ──► Lee download_path de DB ──► Procesa imagen original    │   │
│   │  Convert ─► Lee download_path de DB ──► Procesa imagen original   │   │
│   │  Watermark ► Lee download_path de DB ► Procesa imagen original     │   │
│   │                                                                     │   │
│   │  Cada worker consulta la DB periódicamente hasta que:             │   │
│   │  - download_status = "COMPLETADO" → Procesa                       │   │
│   │  - download_status = "FALLIDO"    → Marca fallo y termina         │   │
│   │                                                                     │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 Patrones de Concurrencia Implementados

#### 2.3.1 Worker Pool (Fan-Out)

Cada etapa del pipeline tiene un pool de workers que leen del mismo canal. Esto permite procesar múltiples tareas en paralelo:

```go
// Creación de workers en main.go
for i := 0; i < config.WorkersDownload; i++ {
    worker := workers.NewDownloadWorker(
        fmt.Sprintf("download-worker-%d", i),
        db,
        pipeline.DownloadChan,
        pipeline.ResizeChan,
        storagePath,
    )
    worker.Start() // Inicia goroutine por cada worker
}
```

#### 2.3.2 Patrón de Procesamiento Paralelo

Los workers de cada etapa (resize, convert, watermark) leen independientemente de la base de datos. No hay conexión directa entre las colas - cada worker espera a que la descarga esté completada consultando la DB:

```go
// DownloadWorker: Solo descarga y guarda en DB
func (w *DownloadWorker) Start() {
    go func() {
        for msg := range w.InChan {
            err := w.process(msg)
            if err != nil {
                w.handleError(msg, err)
            }
            // NO envía a siguiente cola - las demás colas ya recibieron el msg
        }
    }()
}

// ResizeWorker, ConvertWorker, WatermarkWorker: Leen de DB directamente
func (w *ResizeWorker) Start() {
    go func() {
        for msg := range w.InChan {
            // 1. Consultar DB hasta que download_status = "COMPLETADO"
            image := w.waitForDownload(msg.ImageID)
            // 2. Procesar desde image.DownloadPath (imagen original)
            result, err := w.process(image)
            // 3. Guardar resultado en DB
            // NO envía a siguiente cola
        }
    }()
}
```

#### 2.3.3 Buffered Channels

Los canales tienen un buffer de 1000 mensajes para evitar bloqueos cuando los workers están ocupados:

```go
// internal/queue/queue.go
func NewPipeline() *PipelineChannels {
    return &PipelineChannels{
        DownloadChan:  make(chan Message, 1000),
        ResizeChan:    make(chan Message, 1000),
        ConvertChan:   make(chan Message, 1000),
        WatermarkChan: make(chan Message, 1000),
    }
}
```

### 2.4 Flujo de Datos

```go
// Estructura del mensaje que viaja por las colas
// internal/queue/queue.go
type Message struct {
    JobID   string  // UUID del job padre
    ImageID string  // UUID de la imagen específica
    URL     string  // URL original de la imagen (solo para download)
}

// NOTA: Los workers de resize, convert y watermark obtienen
// el download_path directamente de la base de datos, no del mensaje.
// Esto permite que procesen en paralelo desde la imagen original.
```

### 2.5 Tipos de Workers

#### Download Worker (IO-Bound)

- **Tipo:** IO-bound (espera respuesta de red)
- **Responsabilidad:** Descargar imágenes desde URLs HTTP
- **Métricas guardadas:** Tamaño en MB, tiempo de descarga, worker ID

```go
func (w *DownloadWorker) process(msg Message) (Message, error) {
    // 1. HTTP GET a la URL
    // 2. Determinar extensión desde Content-Type
    // 3. Guardar en storage/{uuid}_original.{ext}
    // 4. Actualizar DB: download_path, download_time_sec, download_worker_id
    // 5. Retornar msg para siguiente etapa
}
```

#### Resize Worker (CPU-Bound)

- **Tipo:** CPU-bound (procesamiento de imagen)
- **Responsabilidad:** Redimensionar a 800px de ancho manteniendo proporción, **trabaja directamente desde la imagen original**
- **Métricas guardadas:** Tiempo de procesamiento, worker ID

```go
func (w *ResizeWorker) process(msg Message) error {
    // 1. Consultar DB: SELECT download_path, download_status FROM images WHERE id = ?
    // 2. Esperar polling hasta que download_status = "COMPLETADO"
    // 3. Abrir imagen ORIGINAL desde download_path (no de entrada anterior)
    // 4. Calcular nueva altura: newHeight = (originalHeight * 800) / originalWidth
    // 5. Aplicar Lanczos resampling
    // 6. Guardar en storage/{uuid}_redimensionado.{ext}
    // 7. Actualizar DB: resize_path, resize_time_sec, resize_worker_id
    // [NO envía a siguiente etapa - trabaja independientemente]
}
```

#### Convert Worker (CPU-Bound)

- **Tipo:** CPU-bound (codificación de imagen)
- **Responsabilidad:** Convertir a formato PNG, **trabaja directamente desde la imagen original**
- **Métricas guardadas:** Tiempo de procesamiento, worker ID

```go
func (w *ConvertWorker) process(msg Message) error {
    // 1. Consultar DB: SELECT download_path, download_status FROM images WHERE id = ?
    // 2. Esperar polling hasta que download_status = "COMPLETADO"
    // 3. Abrir imagen ORIGINAL desde download_path
    // 4. Guardar como PNG (compresión natural)
    // 5. Guardar en storage/{uuid}_formato_cambiado.png
    // 6. Actualizar DB: convert_path, convert_time_sec, convert_worker_id
    // [NO depende del resize - trabaja independientemente desde la original]
}
```

#### Watermark Worker (CPU-Bound)

- **Tipo:** CPU-bound (dibuja sobre imagen)
- **Responsabilidad:** Aplicar marca de agua visible, **trabaja directamente desde la imagen original**
- **Métricas guardadas:** Tiempo de procesamiento, worker ID
- **Responsabilidad adicional:** Detectar fin del job

```go
func (w *WatermarkWorker) process(msg Message) error {
    // 1. Consultar DB: SELECT download_path, download_status FROM images WHERE id = ?
    // 2. Esperar polling hasta que download_status = "COMPLETADO"
    // 3. Abrir imagen ORIGINAL desde download_path
    // 4. Dibujar rectángulo semi-transparente en la parte inferior
    // 5. Dibujar texto: "CECAR, HOY TE AMO MENOS QUE AYER"
    // 6. Guardar en storage/{uuid}_marca_agua.png
    // 7. Actualizar DB: watermark_path, watermark_time_sec, watermark_worker_id
    // 8. Verificar: si processed_files + failed_files >= total_files
    //    → Marcar job como COMPLETADO o COMPLETADO_CON_ERRORES
    // [NO depende del convert - trabaja independientemente desde la original]
}
```

---

## 3. Arquitectura del Servicio `pmic-query`

El servicio `pmic-query` es un servicio de solo lectura que consulta el estado de los jobs desde la misma base de datos PostgreSQL.

### 3.1 Estructura del Proyecto

```
pmic-query/
├── main.go              # Punto de entrada y servidor HTTP
├── models.go            # Modelos de datos (lectura)
├── go.mod
└── go.sum
```

### 3.2 Responsabilidades

- **Consulta de estado:** Recuperar información completa de un job por su ID
- **Cálculo de métricas:** Agregar métricas de todas las imágenes del job
- **CORS:** Habilitado para permitir acceso desde el frontend

### 3.3 Endpoint de Consulta

```
GET /api/v1/status/:job_id

Response:
{
  "job_info": {
    "id": "uuid",
    "status": "EN_PROCESO|COMPLETADO|COMPLETADO_CON_ERRORES|FALLIDO",
    "start_time": "2024-01-01T00:00:00Z",
    "end_time": "2024-01-01T00:05:00Z",
    "total_time_seconds": 300
  },
  "workers_config": {
    "workers_download": 5,
    "workers_resize": 3,
    "workers_convert": 3,
    "workers_watermark": 2
  },
  "metrics": {
    "downloads": {
      "total": 10,
      "successful": 9,
      "failed": 1,
      "total_time_sec": 45.5,
      "avg_time_sec": 5.06
    },
    "resize": { ... },
    "convert": { ... },
    "watermark": { ... }
  },
  "summary": {
    "total_files": 10,
    "processed_files": 9,
    "failed_files": 1,
    "success_rate": 90.0
  }
}
```

---

## 4. Modelo de Datos

### 4.1 Tabla `jobs`

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `id` | UUID | Identificador único del job |
| `status` | VARCHAR(50) | Estado: EN_PROCESO, COMPLETADO, COMPLETADO_CON_ERRORES, FALLIDO |
| `total_files` | INT | Total de imágenes a procesar |
| `processed_files` | INT | Imágenes procesadas exitosamente |
| `failed_files` | INT | Imágenes con error |
| `start_time` | TIMESTAMP | Inicio del procesamiento |
| `end_time` | TIMESTAMP | Fin del procesamiento |
| `workers_download` | INT | Workers configurados para descarga |
| `workers_resize` | INT | Workers configurados para redimensionamiento |
| `workers_convert` | INT | Workers configurados para conversión |
| `workers_watermark` | INT | Workers configurados para marca de agua |

### 4.2 Tabla `images`

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `id` | UUID | Identificador único de la imagen |
| `job_id` | UUID | FK al job padre |
| `original_url` | TEXT | URL original de descarga |
| `download_status` | VARCHAR(50) | Estado de la descarga |
| `download_path` | TEXT | Ruta del archivo descargado |
| `download_time_sec` | FLOAT | Tiempo de descarga |
| `download_worker_id` | VARCHAR | ID del worker que descargó |
| `resize_status` | VARCHAR(50) | Estado del redimensionamiento |
| `resize_path` | TEXT | Ruta del archivo redimensionado |
| `resize_time_sec` | FLOAT | Tiempo de redimensionamiento |
| `resize_worker_id` | VARCHAR | ID del worker que redimensionó |
| `convert_status` | VARCHAR(50) | Estado de la conversión |
| `convert_path` | TEXT | Ruta del archivo convertido |
| `convert_time_sec` | FLOAT | Tiempo de conversión |
| `convert_worker_id` | VARCHAR | ID del worker que convirtió |
| `watermark_status` | VARCHAR(50) | Estado de la marca de agua |
| `watermark_path` | TEXT | Ruta del archivo final |
| `watermark_time_sec` | FLOAT | Tiempo de aplicación de marca de agua |
| `watermark_worker_id` | VARCHAR | ID del worker que aplicó marca de agua |

---

## 5. Comunicación entre Servicios

Los servicios se comunican mediante **Base de Datos Compartida** (Shared Database pattern), no mediante llamadas HTTP directas:

```
┌─────────────────┐           ┌──────────────────┐
│     pmic       │           │   pmic-query     │
│                │           │                  │
│  ┌───────────┐ │           │  ┌───────────┐   │
│  │  ESCRIBE  │─┼──────────►│  │   LEE     │   │
│  │  Jobs     │ │   Misma   │  │   Jobs    │   │
│  │  Images   │ │   DB      │  │   Images  │   │
│  └───────────┘ │           │  └───────────┘   │
└─────────────────┘           └──────────────────┘
```

**Ventajas:**
- Desacoplamiento de servicios
- No hay dependencia de disponibilidad entre servicios
- Lecturas y escrituras pueden escalar independientemente

---

## 6. Configuración y Variables de Entorno

| Variable | Descripción | Default |
|----------|-------------|---------|
| `DATABASE_URL` | Connection string PostgreSQL | `host=localhost user=postgres password=pass_postgres dbname=pmic port=5432 sslmode=disable` |
| `STORAGE_PATH` | Ruta de almacenamiento de imágenes | `./storage` |

---

## 7. Tipos de Procesamiento: IO-Bound vs CPU-Bound

| Etapa | Tipo | Descripción | Estrategia de Workers |
|-------|------|-------------|----------------------|
| **Descarga** | IO-Bound | Espera respuesta de red HTTP | Más workers (N >= conexiones HTTP concurrentes) |
| **Redimensionamiento** | CPU-Bound | Procesamiento de imagen | Workers ~ núcleos CPU |
| **Conversión** | CPU-Bound | Codificación PNG | Workers ~ núcleos CPU |
| **Marca de Agua** | CPU-Bound | Dibujado sobre imagen | Workers ~ núcleos CPU |

En Go, los goroutines son eficientes para ambos tipos porque:
- **IO-Bound:** El scheduler de Go puede manejar miles de goroutines bloqueadas en IO
- **CPU-Bound:** El runtime utiliza GOMAXPROCS para paralelizar en múltiples núcleos

---

## 8. Diagrama de Secuencia (Procesamiento Paralelo)

```
Cliente                    pmic                    PostgreSQL      Download   Resize     Convert    Watermark
  |                         |                          │            Workers    Workers    Workers    Workers
  │ POST /process           │                          │               │          │          │          │
  │ {urls: [...]}           │                          │               │          │          │          │
  ├────────────────────────►│                          │               │          │          │          │
  │                         │ INSERT jobs, images      │               │          │          │          │
  │                         ├─────────────────────────►│               │          │          │          │
  │                         │                          │               │          │          │          │
  │                         │ Enviar mensajes a        │               │          │          │          │
  │                         │ TODAS las colas          │               │          │          │          │
  │                         │ (Download, Resize,       │               │          │          │          │
  │                         │  Convert, Watermark)     │               │          │          │          │
  │                         ├──────────────────────────────────────────►│          │          │          │
  │                         ├────────────────────────────────────────────────────►│          │          │
  │                         ├───────────────────────────────────────────────────────────────►│          │
  │                         ├──────────────────────────────────────────────────────────────────────────►│
  │                         │                          │               │          │          │          │
  │     {job_id: uuid}      │                          │               │          │          │          │
  │◄────────────────────────┤                          │               │          │          │          │
  │                         │                          │               │          │          │          │
  │                         │                          │    Descargar  │          │          │          │
  │                         │                          │◄──────────────┤          │          │          │
  │                         │ UPDATE download_status   │               │          │          │          │
  │                         │ download_path            │               │          │          │          │
  │                         ├◄─────────────────────────│               │          │          │          │
  │                         │                          │               │          │          │          │
  │                         │                          │               │   Leer DB (espera)│          │
  │                         │                          │               │◄─────────│          │          │
  │                         │                          │               │          │          │          │
  │                         │                          │               │   Leer DB (espera)│          │
  │                         │                          │               │◄─────────────────────│          │
  │                         │                          │               │          │          │          │
  │                         │                          │               │   Leer DB (espera)│          │
  │                         │                          │               │◜────────────────────────────────│
  │                         │                          │               │          │          │          │
  │                         │                          │               │ [Procesar paralelo desde img original]
  │                         │                          │               │          │          │          │
  │                         │                          │◄──────────────┤          │          │          │
  │                         │ UPDATE resize_status     │               │          │          │          │
  │                         │ resize_path, etc       │◄──────────────────────────┤          │          │
  │                         │ UPDATE convert_status    │◜───────────────────────────────────────┤          │
  │                         │ UPDATE watermark_status  │◜──────────────────────────────────────────────────│
  │                         │                          │               │          │          │          │
  │ GET /status/:job_id     │                          │               │          │          │          │
  ├────────────────────────►│                          │               │          │          │          │
  │                         │ SELECT jobs, images      │               │          │          │          │
  │                         ├─────────────────────────►│               │          │          │          │
  │                         │◄─────────────────────────│               │          │          │          │
  │  JSON con status y      │                          │               │          │          │          │
  │  métricas de todas      │                          │               │          │          │          │
  │  las etapas             │                          │               │          │          │          │
  │◄────────────────────────┤                          │               │          │          │          │
  │                         │                          │               │          │          │          │
  │          ... (polling)  │                          │               │          │          │          │
  │                         │                          │               │          │          │          │
  │                         │ UPDATE job status        │               │          │          │          │
  │                         │ COMPLETADO               │               │          │          │          │
  │                         ├◄─────────────────────────│               │          │          │          │
  │                         │                          │               │          │          │          │
```

---

## 9. Dependencias Principales

| Dependencia | Versión | Uso |
|-------------|---------|-----|
| `github.com/gofiber/fiber/v2` | v2.x | Framework HTTP |
| `gorm.io/gorm` | v1.x | ORM para PostgreSQL |
| `gorm.io/driver/postgres` | v1.x | Driver PostgreSQL |
| `github.com/disintegration/imaging` | v1.x | Procesamiento de imágenes |
| `github.com/google/uuid` | v1.x | Generación de UUIDs |

---

## 10. Ejecución

### Servicio pmic (Puerto 5000)

```bash
cd pmic
go run cmd/main.go
```

### Servicio pmic-query (Puerto 3001)

```bash
cd pmic-query
go run main.go
```

---

## 11. Referencias

- [Go Concurrency Patterns](https://go.dev/blog/pipelines)
- [Effective Go - Concurrency](https://go.dev/doc/effective_go#concurrency)
- [GORM Documentation](https://gorm.io/docs/)
- [Fiber Framework](https://docs.gofiber.io/)
