import pika
import json
import os
import time
from sqlalchemy.orm import Session
from .database import SessionLocal
from .models import Factura

RABBITMQ_HOST = os.getenv("RABBITMQ_HOST", "rabbitmq")

def get_rabbitmq_connection():
    # Reintentos de conexión por si RabbitMQ tarda en levantar
    for i in range(5):
        try:
            return pika.BlockingConnection(pika.ConnectionParameters(host=RABBITMQ_HOST))
        except pika.exceptions.AMQPConnectionError:
            time.sleep(2)
    raise Exception("No se pudo conectar a RabbitMQ")

def publicar_evento_facturacion(pedido_id: int, email: str, estado_pago: str):
    conexion = get_rabbitmq_connection()
    canal = conexion.channel()
    canal.queue_declare(queue='cola_facturacion', durable=True)

    evento = {
        "evento": "PagoProcesado",
        "pedido_id": pedido_id,
        "email": email,
        "estado_pago": estado_pago
    }

    canal.basic_publish(
        exchange='',
        routing_key='cola_facturacion',
        body=json.dumps(evento),
        properties=pika.BasicProperties(delivery_mode=2)
    )
    conexion.close()

def procesar_mensaje(ch, method, body):
    mensaje = json.loads(body)
    print(f"[*] Recibido pedido en Facturación: {mensaje}")
    
    properties = pika.BasicProperties(delivery_mode=2)
    pedido_id = mensaje.get("pedido_id")
    email = mensaje.get("email")
    
    estado_pago = "APROBADO"
    monto = 100000.0 

    db: Session = SessionLocal()
    try:
        nueva_factura = Factura(pedido_id=pedido_id, monto=monto, estado_pago=estado_pago)
        db.add(nueva_factura)
        db.commit()
    except Exception as e:
        print(f"Error en BD: {e}")
        db.rollback()
    finally:
        db.close()

    publicar_evento_facturacion(pedido_id, email, estado_pago)
    
    # Confirmar a RabbitMQ que el mensaje fue procesado (Acknowledge)
    ch.basic_ack(delivery_tag=method.delivery_tag)
    print(f"[*] Pago procesado y evento publicado para el pedido {pedido_id}")

def iniciar_consumidor():
    conexion = get_rabbitmq_connection()
    canal = conexion.channel()
    canal.queue_declare(queue='cola_pedidos', durable=True)
    
    # Prevenir que RabbitMQ envíe más de 1 mensaje a la vez a este worker
    canal.basic_qos(prefetch_count=1)
    canal.basic_consume(queue='cola_pedidos', on_message_callback=procesar_mensaje)
    
    print(" [*] Esperando mensajes en 'cola_pedidos'.")
    canal.start_consuming()
