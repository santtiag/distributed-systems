from pydantic import BaseModel

class ProductoCreate(BaseModel):
    nombre: str
    stock: int
