from sqlalchemy import Column, Integer, String, DateTime
from datetime import datetime
from .database import Base

class Notificacion(Base):
    __tablename__ = "notificaciones"

    id = Column(Integer, primary_key=True, index=True)
    pedido_id = Column(Integer, unique=True, index=True)
    email_cliente = Column(String)
    estado_final_pedido = Column(String) # Ejemplo: "COMPLETADO" o "CANCELADO_SIN_STOCK"
    mensaje_enviado = Column(String)
    fecha_envio = Column(DateTime, default=datetime.now())
