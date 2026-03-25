# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A microservices-based order management system using FastAPI, PostgreSQL, and RabbitMQ for asynchronous event-driven communication.

## Architecture

Four services process orders sequentially via RabbitMQ queues:

```
Service 1 (Pedidos)     → cola_pedidos     → Service 2 (Facturación)
Service 2 (Facturación) → cola_facturacion → Service 3 (Inventario)
Service 3 (Inventario)  → cola_inventario  → Service 4 (Notificaciones)
```

Each service has its own PostgreSQL database (database-per-service pattern).

### Service Endpoints
| Service | Port | Endpoint | Purpose |
|---------|------|----------|---------|
| Pedidos | 8000 | POST /pedidos/ | Create orders |
| Facturación | 8001 | Consumer only | Process payments |
| Inventario | 8002 | Consumer only | Update stock |
| Notificaciones | 8003 | Consumer only | Send email notifications |

### Infrastructure
- **RabbitMQ**: Ports 5672 (AMQP), 15672 (Management UI, guest/guest)
- **PostgreSQL**: Each service has its own DB on ports 5432-5435

## Commands

```bash
# Start all services
docker-compose up --build

# Start in background
docker-compose up -d

# Stop services
docker-compose down

# View logs for a specific service
docker-compose logs -f servicio_pedidos
docker-compose logs -f servicio_facturacion
docker-compose logs -f servicio_inventario
docker-compose logs -f servicio_notificaciones

# Rebuild a single service
docker-compose up --build servicio_pedidos
```

## Service Structure

Each service follows this pattern:
```
servicio_X_name/
├── app/
│   ├── main.py        # FastAPI app + consumer thread (services 2-4)
│   ├── models.py      # SQLAlchemy models
│   ├── schemas.py     # Pydantic schemas
│   ├── database.py    # DB connection
│   └── rabbitmq.py    # Publisher/consumer logic
├── Dockerfile
└── requirements.txt
```

## Key Patterns

- **Message Durability**: All messages use `delivery_mode=2`
- **QoS**: `prefetch_count=1` for fair load balancing
- **Retry Logic**: 5 attempts with 2s delay for RabbitMQ connections
- **Background Consumers**: Services 2-4 start consumer threads on startup
- **Health Checks**: Each service has a `/health` endpoint

## Environment Variables

Each service uses:
- `DATABASE_URL`: PostgreSQL connection string
- `RABBITMQ_HOST`: RabbitMQ hostname (defaults to "rabbitmq")