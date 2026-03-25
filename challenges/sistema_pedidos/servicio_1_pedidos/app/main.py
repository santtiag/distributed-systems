from fastapi import FastAPI, Depends, HTTPException
from sqlalchemy.orm import Session
from . import models, schemas, database, rabbitmq

app = FastAPI(title="Servicio 1 - API de Pedidos")

models.Base.metadata.create_all(bind=database.engine)

@app.post("/pedidos/", response_model=schemas.PedidoResponse, status_code=201)
def crear_pedido(pedido: schemas.PedidoCreate, db: Session = Depends(database.get_db)):
    # 1. Registrar pedido en la base de datos
    nuevo_pedido = models.Pedido(**pedido.model_dump())
    db.add(nuevo_pedido)
    db.commit()
    db.refresh(nuevo_pedido)

    # 2. Publicar evento en RabbitMQ con Event-carried state transfer
    rabbitmq.publicar_evento_pedido(
        pedido_id=nuevo_pedido.id,
        cliente=nuevo_pedido.cliente,
        email=nuevo_pedido.email,
        producto=nuevo_pedido.producto,
        cantidad=nuevo_pedido.cantidad,
        precio_total=nuevo_pedido.precio_total
    )
    return nuevo_pedido
