from fastapi import FastAPI, Depends
from sqlalchemy.orm import Session
import threading
from . import models, database, rabbitmq

app = FastAPI(title="Servicio 4 - Notificaciones")

models.Base.metadata.create_all(bind=database.engine)

@app.on_event("startup")
def startup_event():
    hilo_consumidor = threading.Thread(target=rabbitmq.iniciar_consumidor, daemon=True)
    hilo_consumidor.start()

# @app.get("/notificaciones/")
# def listar_notificaciones(db: Session = Depends(database.get_db)):
#     return db.query(models.Notificacion).all()

@app.get("/health")
def health_check():
    return {"status": "ok", "servicio": "notificaciones"}
