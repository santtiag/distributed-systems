from sqlalchemy import Column, Integer, String, Float, DateTime
from datetime import datetime
from .database import Base

class Factura(Base):
    __tablename__ = "facturas"

    id = Column(Integer, primary_key=True, index=True)
    pedido_id = Column(Integer, unique=True, index=True)
    monto = Column(Float, default=50000.0)
    estado_pago = Column(String, default="PROCESANDO")
    fecha_procesamiento = Column(DateTime, default=datetime.now())
