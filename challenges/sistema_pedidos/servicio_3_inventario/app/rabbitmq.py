import pika
import json
import os
import time
from sqlalchemy.orm import Session
from .database import SessionLocal
from .models import Producto, ReservaInventario

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

    try:
        # 1. VERIFICAR IDEMPOTENCIA: Si ya existe una reserva para este pedido, ignorar
        reserva_existente = db.query(ReservaInventario).filter(ReservaInventario.pedido_id == pedido_id).first()
        if reserva_existente:
            print(f"[!] Pedido {pedido_id} ya fue procesado en Inventario. Mensaje duplicado ignorado.")
            ch.basic_ack(delivery_tag=method.delivery_tag)
            db.close()
            return

        estado_inventario = None

        # 2. Buscar el producto en la base de datos
        producto = db.query(Producto).filter(Producto.nombre == producto_nombre).first()

        # 3. Validar existencia y stock
        if not producto:
            estado_inventario = "PRODUCTO_NO_ENCONTRADO"
            print(f"[X] Producto {producto_nombre} no encontrado en la base de datos.")
        elif producto.stock >= cantidad_requerida:
            producto.stock -= cantidad_requerida
            estado_inventario = "RESERVADO"
            db.commit()
            print(f"[✓] Stock reservado para {producto_nombre}. Stock restante: {producto.stock}")
        else:
            estado_inventario = "RECHAZADO_SIN_STOCK"
            print(f"[X] No hay stock suficiente para {producto_nombre}.")

        # 4. Registrar la reserva (para idempotencia)
        nueva_reserva = ReservaInventario(
            pedido_id=pedido_id,
            producto_nombre=producto_nombre,
            cantidad=cantidad_requerida,
            estado=estado_inventario
        )
        db.add(nueva_reserva)
        db.commit()

        # 5. Publicar el evento con todos los datos para el Servicio 4 (Notificaciones)
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

        # 6. Confirmar procesamiento a RabbitMQ
        ch.basic_ack(delivery_tag=method.delivery_tag)

    except Exception as e:
        print(f"[X] Error en BD: {e}")
        db.rollback()
        # Hacer ACK para evitar loops infinitos en caso de error de datos duplicados
        ch.basic_ack(delivery_tag=method.delivery_tag)
    finally:
        db.close()

def iniciar_consumidor():
    conexion = get_rabbitmq_connection()
    canal = conexion.channel()
    
    # Nos suscribimos a la cola que viene del Servicio 2
    canal.queue_declare(queue='cola_facturacion', durable=True)
    canal.basic_qos(prefetch_count=1)
    canal.basic_consume(queue='cola_facturacion', on_message_callback=procesar_mensaje)
    
    print(" [*] Servicio de Inventario esperando mensajes en 'cola_facturacion'.")
    canal.start_consuming()
