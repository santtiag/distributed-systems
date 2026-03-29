from sqlalchemy import Column, Integer, String
from .database import Base

class Producto(Base):
    __tablename__ = "productos"

    id = Column(Integer, primary_key=True, index=True)
    nombre = Column(String, unique=True, index=True)
    stock = Column(Integer, default=0)


class ReservaInventario(Base):
    """Registra cada reserva de inventario para garantizar idempotencia"""
    __tablename__ = "reservas_inventario"

    id = Column(Integer, primary_key=True, index=True)
    pedido_id = Column(Integer, unique=True, index=True)  # Unico para evitar duplicados
    producto_nombre = Column(String)
    cantidad = Column(Integer)
    estado = Column(String)  # RESERVADO, RECHAZADO_SIN_STOCK, PRODUCTO_NO_ENCONTRADO
