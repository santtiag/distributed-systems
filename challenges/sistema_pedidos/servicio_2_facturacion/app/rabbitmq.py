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

def publicar_evento_facturacion(pedido_id: int, cliente: str, email: str, producto: str,
                                  cantidad: int, precio_total: float, estado_pago: str, monto: float):
    conexion = get_rabbitmq_connection()
    canal = conexion.channel()
    canal.queue_declare(queue='cola_facturacion', durable=True)

    # Event-carried state transfer: reenviar todos los datos del pedido + datos de facturación
    evento = {
        "evento": "PagoProcesado",
        "pedido_id": pedido_id,
        "cliente": cliente,
        "email": email,
        "producto": producto,
        "cantidad": cantidad,
        "precio_total": precio_total,
        "estado_pago": estado_pago,
        "monto": monto
    }

    canal.basic_publish(
        exchange='',
        routing_key='cola_facturacion',
        body=json.dumps(evento),
        properties=pika.BasicProperties(delivery_mode=2)
    )
    conexion.close()

def procesar_mensaje(ch, method, properties, body):
    mensaje = json.loads(body)
    print(f"[*] Recibido pedido en Facturación: {mensaje}")

    # Extraer todos los datos del evento (Event-carried state transfer)
    pedido_id = mensaje.get("pedido_id")
    cliente = mensaje.get("cliente")
    email = mensaje.get("email")
    producto = mensaje.get("producto")
    cantidad = mensaje.get("cantidad")
    precio_total = mensaje.get("precio_total")

    db: Session = SessionLocal()
    try:
        # 1. VERIFICAR IDEMPOTENCIA: Si ya existe una factura para este pedido, ignorar
        factura_existente = db.query(Factura).filter(Factura.pedido_id == pedido_id).first()
        if factura_existente:
            print(f"[!] Pedido {pedido_id} ya fue facturado (Factura ID: {factura_existente.id}). Mensaje duplicado ignorado.")
            ch.basic_ack(delivery_tag=method.delivery_tag)
            return

        # 2. Procesar pago
        estado_pago = "APROBADO"
        monto = precio_total  # Usar el precio total del pedido

        # 3. Guardar en BD primero (para validar idempotencia)
        nueva_factura = Factura(pedido_id=pedido_id, monto=monto, estado_pago=estado_pago)
        db.add(nueva_factura)
        db.commit()
        print(f"[✓] Factura creada para pedido {pedido_id} - Monto: {monto}")

        # 4. Solo publicar evento si el guardado fue exitoso
        publicar_evento_facturacion(
            pedido_id=pedido_id,
            cliente=cliente,
            email=email,
            producto=producto,
            cantidad=cantidad,
            precio_total=precio_total,
            estado_pago=estado_pago,
            monto=monto
        )

        # 5. Confirmar a RabbitMQ que el mensaje fue procesado (Acknowledge)
        ch.basic_ack(delivery_tag=method.delivery_tag)
        print(f"[*] Pago procesado y evento publicado para el pedido {pedido_id}")

    except Exception as e:
        print(f"[X] Error en BD: {e}")
        db.rollback()
        # No hacer ACK en caso de error - el mensaje será reencolado
        # Pero para mensajes duplicados con IntegrityError, el try-catch anterior ya lo maneja
        # Si llegamos aquí con IntegrityError, es una condición de carrera rara
        # Hacemos ACK para evitar loops infinitos
        ch.basic_ack(delivery_tag=method.delivery_tag)
    finally:
        db.close()

def iniciar_consumidor():
    conexion = get_rabbitmq_connection()
    canal = conexion.channel()
    canal.queue_declare(queue='cola_pedidos', durable=True)
    
    # Prevenir que RabbitMQ envíe más de 1 mensaje a la vez a este worker
    canal.basic_qos(prefetch_count=1)
    canal.basic_consume(queue='cola_pedidos', on_message_callback=procesar_mensaje)
    
    print(" [*] Esperando mensajes en 'cola_pedidos'.")
    canal.start_consuming()
