from sqlalchemy import Column, Integer, String, Float, DateTime
from datetime import datetime
from .database import Base

class Pedido(Base):
    __tablename__ = "pedidos"

    id = Column(Integer, primary_key=True, index=True)
    cliente = Column(String, index=True)
    email = Column(String)
    producto = Column(String)
    cantidad = Column(Integer)
    precio_total = Column(Float)
    estado = Column(String, default="CREADO")
    fecha_creacion = Column(DateTime, default=datetime.now())
