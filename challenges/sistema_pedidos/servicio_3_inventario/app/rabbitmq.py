import pika
import json
import os
import time
from sqlalchemy.orm import Session
from .database import SessionLocal
from .models import Producto

RABBITMQ_HOST = os.getenv("RABBITMQ_HOST", "rabbitmq")

def get_rabbitmq_connection():
    for i in range(5):
        try:
            return pika.BlockingConnection(pika.ConnectionParameters(host=RABBITMQ_HOST))
        except pika.exceptions.AMQPConnectionError:
            time.sleep(2)
    raise Exception("No se pudo conectar a RabbitMQ")

def publicar_evento_inventario(pedido_id: int, cliente: str, email: str, producto: str,
                                cantidad: int, precio_total: float, estado_pago: str,
                                estado_inventario: str):
    conexion = get_rabbitmq_connection()
    canal = conexion.channel()
    canal.queue_declare(queue='cola_inventario', durable=True)

    # Event-carried state transfer: reenviar todos los datos + estado de inventario
    evento = {
        "evento": "InventarioConfirmado",
        "pedido_id": pedido_id,
        "cliente": cliente,
        "email": email,
        "producto": producto,
        "cantidad": cantidad,
        "precio_total": precio_total,
        "estado_pago": estado_pago,
        "estado_inventario": estado_inventario
    }

    canal.basic_publish(
        exchange='',
        routing_key='cola_inventario',
        body=json.dumps(evento),
        properties=pika.BasicProperties(delivery_mode=2)
    )
    conexion.close()

def procesar_mensaje(ch, method, properties, body):
    mensaje = json.loads(body)
    print(f"[*] Recibido pago en Inventario: {mensaje}")

    # Extraer todos los datos del evento (Event-carried state transfer)
    pedido_id = mensaje.get("pedido_id")
    cliente = mensaje.get("cliente")
    email = mensaje.get("email")
    producto_nombre = mensaje.get("producto")
    cantidad_requerida = mensaje.get("cantidad")
    precio_total = mensaje.get("precio_total")
    estado_pago = mensaje.get("estado_pago")

    db: Session = SessionLocal()
    estado_inventario = "RECHAZADO_SIN_STOCK"  # Asumimos lo peor inicialmente

    try:
        # 1. Buscar el producto en la base de datos
        producto = db.query(Producto).filter(Producto.nombre == producto_nombre).first()

        # 2. Validar stock y actualizar
        if producto and producto.stock >= cantidad_requerida:
            producto.stock -= cantidad_requerida
            estado_inventario = "RESERVADO"
            db.commit()
            print(f"[✓] Stock reservado para {producto_nombre}. Stock restante: {producto.stock}")
        else:
            print(f"[X] No hay stock suficiente para {producto_nombre}.")

    except Exception as e:
        print(f"Error en BD: {e}")
        db.rollback()
    finally:
        db.close()

    # 3. Publicar el evento con todos los datos para el Servicio 4 (Notificaciones)
    publicar_evento_inventario(
        pedido_id=pedido_id,
        cliente=cliente,
        email=email,
        producto=producto_nombre,
        cantidad=cantidad_requerida,
        precio_total=precio_total,
        estado_pago=estado_pago,
        estado_inventario=estado_inventario
    )

    # 4. Confirmar procesamiento a RabbitMQ
    ch.basic_ack(delivery_tag=method.delivery_tag)

def iniciar_consumidor():
    conexion = get_rabbitmq_connection()
    canal = conexion.channel()
    
    # Nos suscribimos a la cola que viene del Servicio 2
    canal.queue_declare(queue='cola_facturacion', durable=True)
    canal.basic_qos(prefetch_count=1)
    canal.basic_consume(queue='cola_facturacion', on_message_callback=procesar_mensaje)
    
    print(" [*] Servicio de Inventario esperando mensajes en 'cola_facturacion'.")
    canal.start_consuming()
