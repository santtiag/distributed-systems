from fastapi import FastAPI
import threading
from . import models, database, rabbitmq

app = FastAPI(title="Servicio 2 - Facturación")

models.Base.metadata.create_all(bind=database.engine)

@app.on_event("startup")
def startup_event():
    # Iniciamos el consumidor de RabbitMQ en un hilo en segundo plano (Background Thread)
    hilo_consumidor = threading.Thread(target=rabbitmq.iniciar_consumidor, daemon=True)
    hilo_consumidor.start()

@app.get("/health")
def health_check():
    return {"status": "ok", "servicio": "facturacion"}
