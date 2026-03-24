from fastapi import FastAPI, Depends
from sqlalchemy.orm import Session
import threading
from . import models, database, rabbitmq

app = FastAPI(title="Servicio 3 - Inventario")

models.Base.metadata.create_all(bind=database.engine)

@app.on_event("startup")
def startup_event():
    hilo_consumidor = threading.Thread(target=rabbitmq.iniciar_consumidor, daemon=True)
    hilo_consumidor.start()

# Endpoint administrativo para cargar inventario inicial
# @app.post("/admin/productos/")
# def crear_producto(producto: ProductoCreate, db: Session = Depends(database.get_db)):
#     nuevo_producto = models.Producto(nombre=producto.nombre, stock=producto.stock)
#     db.add(nuevo_producto)
#     db.commit()
#     return {"mensaje": "Producto creado", "producto": producto.nombre, "stock": producto.stock}

@app.get("/health")
def health_check():
    return {"status": "ok", "servicio": "inventario"}
