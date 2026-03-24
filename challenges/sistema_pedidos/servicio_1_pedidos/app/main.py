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

    # 2. Publicar evento en RabbitMQ
    rabbitmq.publicar_evento_pedido(nuevo_pedido.id, nuevo_pedido.cliente)
    return nuevo_pedido
