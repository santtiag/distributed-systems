# PMIC - Plataforma de Procesamiento Masivo de Imágenes Concurrente

## Descripción General

PMIC es un sistema backend basado en API REST que implementa un modelo de procesamiento concurrente para la descarga y transformación masiva de imágenes. El sistema utiliza el patrón Productor-Consumidor con colas de mensajes para desacoplar las diferentes etapas de procesamiento.

---

## Arquitectura Visual del Sistema

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                                    CLIENTE                                          │
│                              (Frontend/Postman/cURL)                                │
└─────────────────────────────────────────────────────────────────────────────────────┘
                                          │
                                          │ HTTP POST /api/v1/process
                                          │ JSON: {urls[], workers_config}
                                          ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                           CAPA API REST (FIBER)                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────┐   │
│  │                         JobHandler (cmd/main.go:108)                        │   │
│  │  • Recibe solicitud (RF1)                                                   │   │
│  │  • Crea Job en PostgreSQL                                                   │   │
│  │  • Crea registros Image                                                     │   │
│  │  • Publica mensajes en NATS (images.download)                                 │   │
│  │  • Retorna job_id                                                           │   │
│  └─────────────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────────────┘
                                          │
                                          │ Mensajes: {job_id, image_id, url}
                                          ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                      SISTEMA DE MENSAJERÍA NATS (Queue)                             │
│                                                                                     │
│   ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐           │
│   │  images.download │───▶│   images.resize  │───▶│  images.convert  │───▶ ...     │
│   │   (Cola RF2)     │    │   (Cola RF3)     │    │   (Cola RF4)     │            │
│   └──────────────────┘    └──────────────────┘    └──────────────────┘           │
│                                                                                     │
└─────────────────────────────────────────────────────────────────────────────────────┘
                    │                  │                  │
                    ▼                  ▼                  ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                         CAPA DE WORKERS CONCURRENTES                                │
│                                                                                     │
│   ┌─────────────────────────┐                                                       │
│   │    DOWNLOAD WORKERS     │  ◄── IO-Bound (Red/HTTP)                              │
│   │    (5 instancias)       │      • Descarga desde URLs                             │
│   │    download_worker.go   │      • Almacena en disco                               │
│   │                         │      • Métricas: tamaño, tiempo                        │
│   │  DL-Worker-1  DL-Worker-2  ...  DL-Worker-5                                    │
│   └─────────────────────────┘                                                       │
│              │                                                                      │
│              ▼                                                                      │
│   ┌─────────────────────────┐                                                       │
│   │     RESIZE WORKERS      │  ◄── CPU-Bound (Procesamiento)                        │
│   │     (3 instancias)      │      • Redimensiona a 800px                          │
│   │     resize_worker.go     │      • Algoritmo Lanczos                             │
│   │                         │      • Mantiene proporción                             │
│   │  RSZ-Worker-1  RSZ-Worker-2  RSZ-Worker-3                                       │
│   └─────────────────────────┘                                                       │
│              │                                                                      │
│              ▼                                                                      │
│   ┌─────────────────────────┐                                                       │
│   │     CONVERT WORKERS     │  ◄── CPU-Bound Pesado                                 │
│   │     (3 instancias)      │      • Conversión a PNG                                │
│   │     convert_worker.go   │      • Compresión sin pérdida                          │
│   │                         │                                                       │
│   │  CNV-Worker-1  CNV-Worker-2  CNV-Worker-3                                       │
│   └─────────────────────────┘                                                       │
│              │                                                                      │
│              ▼                                                                      │
│   ┌─────────────────────────┐                                                       │
│   │    WATERMARK WORKERS    │  ◄── CPU-Bound (Gráficos)                             │
│   │     (2 instancias)      │      • Banner negro semi-transparente                 │
│   │    watermark_worker.go  │      • Texto "WATERMARK - PMIC"                      │
│   │                         │      • Actualiza estado final Job                      │
│   │  WMK-Worker-1  WMK-Worker-2                                                    │
│   └─────────────────────────┘                                                       │
│                                                                                     │
└─────────────────────────────────────────────────────────────────────────────────────┘
                    │
                    │ Lectura/Escritura
                    ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                    BASE DE DATOS POSTGRESQL (GORM)                                  │
│                                                                                     │
│   ┌──────────────────────────┐        ┌─────────────────────────────────────────┐    │
│   │        jobs              │        │              images                   │    │
│   │  ┌────────────────────┐  │        │  ┌─────────────────────────────────┐   │    │
│   │  │ id (UUID) PK       │──┼────────┼──┤ job_id FK                       │   │    │
│   │  │ status             │  │        │  │ id (UUID) PK                    │   │    │
│   │  │ total_files        │  │        │  │ original_url                    │   │    │
│   │  │ processed_files    │  │        │  │ file_name                       │   │    │
│   │  │ failed_files       │  │        │  │                                 │   │    │
│   │  │ start_time         │  │        │  │ download_status                 │   │    │
│   │  │ end_time           │  │        │  │ download_path                   │   │    │
│   │  │                    │  │        │  │ download_size_mb                │   │    │
│   │  │ workers_download   │  │        │  │ download_time_sec               │   │    │
│   │  │ workers_resize     │  │        │  │ download_worker_id              │   │    │
│   │  │ workers_convert    │  │        │  │                                 │   │    │
│   │  │ workers_watermark  │  │        │  │ resize_status                   │   │    │
│   │  └────────────────────┘  │        │  │ resize_path                     │   │    │
│   │                          │        │  │ resize_time_sec                 │   │    │
│   │                          │        │  │ resize_worker_id                │   │    │
│   │                          │        │  │ original_width/height           │   │    │
│   │                          │        │  │ new_width/height                │   │    │
│   │                          │        │  │                                 │   │    │
│   │                          │        │  │ convert_status                  │   │    │
│   │                          │        │  │ convert_path                    │   │    │
│   │                          │        │  │ convert_time_sec                │   │    │
│   │                          │        │  │ convert_worker_id               │   │    │
│   │                          │        │  │                                 │   │    │
│   │                          │        │  │ watermark_status                │   │    │
│   │                          │        │  │ watermark_path                  │   │    │
│   │                          │        │  │ watermark_time_sec              │   │    │
│   │                          │        │  │ watermark_worker_id             │   │    │
│   │                          │        │  │                                 │   │    │
│   │                          │        │  │ error_message                   │   │    │
│   │                          │        │  └─────────────────────────────────┘   │    │
│   │                          │        └─────────────────────────────────────────┘    │
│   └──────────────────────────┘                                                       │
│                                                                                     │
└─────────────────────────────────────────────────────────────────────────────────────┘
                                          │
                                          ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              ALMACENAMIENTO LOCAL                                   │
│                              (Directorio storage/)                                  │
│                                                                                     │
│   Formato de archivos generados:                                                    │
│   • {uuid}_original.jpg          - Archivo descargado original                        │
│   • {uuid}_redimensionado.jpg    - Imagen redimensionada (800px ancho)                │
│   • {uuid}_formato_cambiado.png - Conversión a formato PNG                          │
│   • {uuid}_marca_agua.png        - Versión final con watermark                      │
│                                                                                     │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Flujo de Datos del Pipeline

```
┌─────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│ Cliente │────▶│  Descarga   │────▶│  Resize     │────▶│  Convert    │────▶│  Watermark  │
│   POST  │     │  (IO-Bound) │     │ (CPU-Bound) │     │(CPU-Bound)  │     │(CPU-Bound)  │
└─────────┘     └─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
     │                │                  │                  │                  │
     │           PENDING ──►         PENDING ──►         PENDING ──►         PENDING
     │          PROCESSING           PROCESSING          PROCESSING          PROCESSING
     │          COMPLETED            COMPLETED           COMPLETED           COMPLETED
     │                │                  │                  │                  │
     │           images.download    images.resize      images.convert    images.watermark
     │           (NATS subject)     (NATS subject)     (NATS subject)    (NATS subject)
```

---

## Descripción de Componentes

### 1. Capa API REST (`internal/handlers/`)

**JobHandler** (`job_handler.go`)
- **Endpoint**: `POST /api/v1/process`
- **Responsabilidades**:
  - Recibe payload JSON con URLs y configuración de workers
  - Valida datos de entrada
  - Crea registro Job en PostgreSQL con estado `EN_PROCESO`
  - Crea registros Image para cada URL con estado `PENDING`
  - Publica mensajes en NATS subject `images.download`
  - Retorna `job_id` para consulta posterior

**Estructura del Payload de Entrada**:
```json
{
  "urls": ["https://ejemplo.com/img1.jpg", "https://ejemplo.com/img2.png"],
  "workers_download": 5,
  "workers_resize": 3,
  "workers_convert": 3,
  "workers_watermark": 2
}
```

### 2. Sistema de Colas NATS (`internal/queue/`)

**Queue** (`nats.go`)
- **Propósito**: Desacoplamiento asincrónico entre etapas del pipeline
- **Subjects (Topics)**:
  - `images.download`: Cola para descargas concurrentes
  - `images.resize`: Cola para redimensionamiento
  - `images.convert`: Cola para conversión de formato
  - `images.watermark`: Cola para marca de agua

**Patrón**: Competitive Consumers (Consumidores Competitivos)
- Múltiples workers suscritos al mismo subject
- NATS distribuye mensajes usando round-robin
- Permite escalado horizontal de workers

**Estructura del Mensaje**:
```go
type Message struct {
    JobID      string  // ID del job padre
    ImageID    string  // ID de la imagen específica
    InputPath  string  // Path del archivo de entrada
    OutputPath string  // Path del archivo de salida
    URL        string  // URL original (solo para descarga)
    FileName   string  // Nombre base del archivo
}
```

### 3. Workers Concurrentes (`internal/workers/`)

Cada worker implementa la interfaz:
```go
type Worker interface {
    Start() error
    processMessage(msg *nats.Msg)
    marcarFallo(...)
}
```

#### 3.1 DownloadWorker (`download_worker.go`)
- **Tipo**: IO-Bound (Operaciones de red)
- **Cantidad**: 5 instancias (`DL-Worker-1` a `DL-Worker-5`)
- **Funciones**:
  - Descarga imagen desde URL vía HTTP GET
  - Determina extensión desde Content-Type
  - Guarda archivo como `{uuid}_original.{ext}`
  - Calcula: tamaño en MB, tiempo de descarga
  - Actualiza BD: status, path, métricas, worker_id
  - Publica siguiente mensaje a `images.resize`

**Métricas RF2 Almacenadas**:
- `download_size_mb`: Tamaño en megabytes
- `download_time_sec`: Tiempo transcurrido
- `download_worker_id`: Identificador del worker
- `downloaded_at`: Timestamp

#### 3.2 ResizeWorker (`resize_worker.go`)
- **Tipo**: CPU-Bound (Procesamiento de imagen)
- **Cantidad**: 3 instancias (`RSZ-Worker-1` a `RSZ-Worker-3`)
- **Funciones**:
  - Carga imagen original con `imaging.Open()`
  - Calcula nuevas dimensiones: `nuevo_alto = alto_original * (800 / ancho_original)`
  - Redimensiona usando algoritmo Lanczos
  - Guarda como `{uuid}_redimensionado.{ext}`
  - Almacena dimensiones originales y nuevas

**Fórmula de Redimensionamiento RF3**:
```
nuevo_ancho = 800px (fijo)
nuevo_alto = alto_original * (nuevo_ancho / ancho_original)
```

#### 3.3 ConvertWorker (`convert_worker.go`)
- **Tipo**: CPU-Bound Pesado (Compresión)
- **Cantidad**: 3 instancias (`CNV-Worker-1` a `CNV-Worker-3`)
- **Funciones**:
  - Convierte imagen a formato PNG
  - Usa encoder PNG de `imaging.Save()`
  - Guarda como `{uuid}_formato_cambiado.png`
  - Nota: Potencial cuello de botella por compresión

#### 3.4 WatermarkWorker (`watermark_worker.go`)
- **Tipo**: CPU-Bound (Gráficos/Renderizado)
- **Cantidad**: 2 instancias (`WMK-Worker-1`, `WMK-Worker-2`)
- **Funciones**:
  - Carga imagen PNG con `imaging.Open()`
  - Crea contexto de dibujo con `gg.NewContextForImage()`
  - Dibuja banner negro semi-transparente (y=height-50, alto=50px)
  - Escribe texto "WATERMARK - PMIC" en blanco centrado
  - Guarda como `{uuid}_marca_agua.png`
  - **Actualiza contador del Job** y verifica finalización

**Lógica de Cierre**:
```go
if job.ProcessedFiles + job.FailedFiles >= job.TotalFiles {
    // Marcar Job como COMPLETADO o COMPLETADO_CON_ERRORES
    // Guardar end_time
}
```

### 4. Base de Datos PostgreSQL (`internal/database/`, `internal/models/`)

**Conexión** (`database.go`):
- ORM: GORM (v2)
- Driver: `gorm.io/driver/postgres`
- Auto-migración de esquemas al iniciar
- URL configurable vía variable de entorno `DATABASE_URL`

#### Modelo Job (`models/models.go:18-38`)
Representa un procesamiento completo:

| Campo | Tipo | Descripción |
|-------|------|-------------|
| `id` | UUID PK | Identificador único del job |
| `status` | VARCHAR | EN_PROCESO, COMPLETADO, COMPLETADO_CON_ERRORES, FALLIDO |
| `total_files` | INT | Total de imágenes a procesar |
| `processed_files` | INT | Contador de completadas exitosamente |
| `failed_files` | INT | Contador de fallos |
| `start_time` | TIMESTAMP | Inicio del procesamiento |
| `end_time` | TIMESTAMP | Finalización (nullable) |
| `workers_*` | INT | Configuración de workers por etapa |

#### Modelo Image (`models/models.go:41-84`)
Representa cada imagen en el pipeline:

**Estados por Etapa**:
- `download_status`: PENDING → PROCESSING → COMPLETED/FAILED
- `resize_status`: PENDING → PROCESSING → COMPLETED/FAILED
- `convert_status`: PENDING → PROCESSING → COMPLETED/FAILED
- `watermark_status`: PENDING → PROCESSING → COMPLETED/FAILED

**Metadatos por Etapa**:
- Paths de archivos generados
- Tiempos de procesamiento (segundos)
- Identificadores de workers
- Dimensiones originales y nuevas (resize)
- Mensaje de error (si aplica)

---

## Patrones de Diseño Implementados

### 1. Productor-Consumidor
- **Productor**: JobHandler publica mensajes en NATS
- **Colas**: Subjects NATS actúan como buffers
- **Consumidores**: Workers que procesan mensajes concurrentemente

### 2. Pipeline de Procesamiento
- Las imágenes fluyen secuencialmente por 4 etapas
- Cada etapa publica en la siguiente cola al completarse
- Desacoplamiento total entre etapas

### 3. Competitive Consumers
- Múltiples workers por etapa compiten por mensajes
- NATS distribuye mensajes usando round-robin
- Escalado horizontal por etapa

### 4. Event-Driven Architecture
- Comunicación asíncrona vía mensajes
- Workers reactivos a eventos de la cola
- Actualizaciones de estado reactivas

---

## Tipos de Operaciones y Cuellos de Botella

```
┌─────────────────────────────────────────────────────────────────┐
│                    CLASIFICACIÓN POR ETAPA                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   DESCARGA (DownloadWorker)                                      │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━                                      │
│   Tipo: IO-Bound                                                 │
│   Recursos: Red HTTP, Disco                                      │
│   Workers: 5 (más workers = más concurrencia de red)             │
│   Cuello: Ancho de banda de red                                  │
│                                                                  │
│   REDIMENSIONAMIENTO (ResizeWorker)                              │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━                              │
│   Tipo: CPU-Bound                                                │
│   Recursos: CPU (algoritmo Lanczos matemático)                  │
│   Workers: 3 (recomendado: núcleos físicos del CPU)              │
│   Cuello: Capacidad de cómputo                                   │
│                                                                  │
│   CONVERSIÓN (ConvertWorker)                                     │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━                                    │
│   Tipo: CPU-Bound Pesado                                         │
│   Recursos: CPU (compresión PNG sin pérdida)                     │
│   Workers: 3                                                     │
│   Cuello: Principal cuello de botella del sistema                │
│                                                                  │
│   MARCA DE AGUA (WatermarkWorker)                                │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━                                │
│   Tipo: CPU-Bound (Gráficos)                                     │
│   Recursos: CPU (renderizado con gg)                           │
│   Workers: 2                                                     │
│   Cuello: Capacidad de renderizado                               │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Secuencia de Ejecución

```
1. INICIALIZACIÓN (cmd/main.go)
   ├─ Crear directorio storage/
   ├─ Conectar PostgreSQL
   ├─ Conectar NATS
   ├─ Iniciar 5 DownloadWorkers
   ├─ Iniciar 3 ResizeWorkers
   ├─ Iniciar 3 ConvertWorkers
   ├─ Iniciar 2 WatermarkWorkers
   └─ Iniciar servidor Fiber en :5000

2. RECEPCIÓN DE SOLICITUD (RF1)
   ├─ POST /api/v1/process
   ├─ Validar payload
   ├─ Crear Job en BD (status: EN_PROCESO)
   ├─ Crear registros Image (status: PENDING)
   ├─ Publicar mensajes en images.download
   └─ Responder: {job_id, message}

3. PIPELINE DE PROCESAMIENTO
   Para cada imagen:
   ├─ ETAPA 2 (RF2): Descarga
   │   ├─ Recibir mensaje images.download
   │   ├─ Descargar vía HTTP
   │   ├─ Guardar archivo
   │   ├─ Actualizar BD con métricas
   │   └─ Publicar mensaje images.resize
   │
   ├─ ETAPA 3 (RF3): Redimensionamiento
   │   ├─ Recibir mensaje images.resize
   │   ├─ Calcular nuevas dimensiones (800px)
   │   ├─ Aplicar algoritmo Lanczos
   │   ├─ Guardar archivo
   │   ├─ Actualizar BD con métricas
   │   └─ Publicar mensaje images.convert
   │
   ├─ ETAPA 4 (RF4): Conversión
   │   ├─ Recibir mensaje images.convert
   │   ├─ Convertir a formato PNG
   │   ├─ Guardar archivo
   │   ├─ Actualizar BD con métricas
   │   └─ Publicar mensaje images.watermark
   │
   └─ ETAPA 5 (RF5): Marca de Agua
       ├─ Recibir mensaje images.watermark
       ├─ Dibujar banner + texto
       ├─ Guardar archivo final
       ├─ Actualizar BD con métricas
       └─ Actualizar contador Job y verificar fin

4. FINALIZACIÓN
   └─ Cuando ProcessedFiles + FailedFiles == TotalFiles
       ├─ Marcar Job como COMPLETADO o COMPLETADO_CON_ERRORES
       └─ Guardar end_time
```

---

## Estructura del Proyecto

```
pmic/
├── cmd/
│   └── main.go                    # Punto de entrada, inicialización
├── internal/
│   ├── config/
│   │   └── config.go              # Variables de entorno
│   ├── database/
│   │   └── database.go            # Conexión PostgreSQL + GORM
│   ├── handlers/
│   │   └── job_handler.go         # Handler REST (RF1)
│   ├── models/
│   │   └── models.go              # Job, Image (estructuras BD)
│   ├── queue/
│   │   └── nats.go                # Cliente NATS, mensajes
│   └── workers/
│       ├── download_worker.go     # Worker de descarga (RF2)
│       ├── resize_worker.go       # Worker redimensionamiento (RF3)
│       ├── convert_worker.go      # Worker conversión (RF4)
│       └── watermark_worker.go    # Worker marca de agua (RF5)
├── storage/                       # Directorio de imágenes generadas
├── tmp/                           # Archivos temporales
├── go.mod                         # Dependencias Go
├── go.sum                         # Checksums
└── README.md                      # Este documento
```

---

## Tecnologías Utilizadas

| Categoría | Tecnología | Versión | Propósito |
|-----------|-----------|---------|-----------|
| **Lenguaje** | Go | 1.25 | Lógica de negocio, concurrencia nativa |
| **Framework Web** | Fiber | v2 | API REST de alto rendimiento |
| **Base de Datos** | PostgreSQL | 14+ | Persistencia de jobs e imágenes |
| **ORM** | GORM | v2 | Mapeo objeto-relacional |
| **Mensajería** | NATS | Latest | Colas de mensajes asíncronas |
| **Imágenes** | imaging | latest | Procesamiento de imágenes (resize, convert) |
| **Gráficos** | gg | latest | Dibujo de marca de agua |
| **IDs** | google/uuid | latest | Generación de UUIDs |

---

## Variables de Entorno

| Variable | Valor por Defecto | Descripción |
|----------|-------------------|-------------|
| `DATABASE_URL` | `host=localhost user=postgres password=pass_postgres dbname=pmic port=5432 sslmode=disable` | Conexión PostgreSQL |
| `NATS_URL` | `nats://localhost:4222` | URL del servidor NATS |
| `STORAGE_PATH` | `./storage` | Directorio de almacenamiento de imágenes |

---

## Cumplimiento de Requisitos Funcionales

| RF | Descripción | Implementación |
|----|-------------|----------------|
| **RF1** | Recepción de Solicitud | `job_handler.go:37-109` - Endpoint POST, validaciones, creación de Job, retorno de job_id |
| **RF2** | Descarga Concurrente | `download_worker.go` - Cola NATS, 5 workers, métricas de tamaño/tiempo/worker |
| **RF3** | Redimensionamiento | `resize_worker.go` - Fórmula 800px, algoritmo Lanczos, sufijo `_redimensionado` |
| **RF4** | Conversión Formato | `convert_worker.go` - Conversión a PNG, sufijo `_formato_cambiado` |
| **RF5** | Marca de Agua | `watermark_worker.go` - Banner negro + texto "WATERMARK - PMIC", sufijo `_marca_agua` |
| **RF6** | Consulta de Estado | Estructura de BD soporta métricas; endpoint adicional requerido |

---

## Notas de Arquitectura

1. **Desacoplamiento**: Cada etapa es independiente y se comunica solo vía NATS
2. **Escalabilidad**: Los workers pueden escalar horizontalmente independientemente
3. **Tolerancia a Fallos**: Fallos en una etapa no afectan otras imágenes
4. **Observabilidad**: Todos los metadatos se almacenan en PostgreSQL
5. **Persistencia**: Las imágenes se almacenan en disco local
6. **Concurrencia Real**: Los workers son goroutines independientes con suscripciones NATS
