from pydantic import BaseModel
from datetime import datetime

class PedidoCreate(BaseModel):
    cliente: str
    email: str
    producto: str
    cantidad: int
    precio_total: float

class PedidoResponse(PedidoCreate):
    id: int
    estado: str
    fecha_creacion: datetime

    class Config:
        from_attributes = True
