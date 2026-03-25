from fastapi import FastAPI
import threading
from . import models, database, rabbitmq

app = FastAPI(title="Servicio 3 - Inventario")

models.Base.metadata.create_all(bind=database.engine)

@app.on_event("startup")
def startup_event():
    hilo_consumidor = threading.Thread(target=rabbitmq.iniciar_consumidor, daemon=True)
    hilo_consumidor.start()

@app.get("/health")
def health_check():
    return {"status": "ok", "servicio": "inventario"}
