import pika
import json
import os
import time
from sqlalchemy.orm import Session
from .database import SessionLocal
from .models import Notificacion

RABBITMQ_HOST = os.getenv("RABBITMQ_HOST", "rabbitmq")
RESEND_API_KEY = os.getenv("RESEND_API_KEY", "")
EMAIL_FROM = os.getenv("EMAIL_FROM", "onboarding@resend.dev")

try:
    import resend
    if RESEND_API_KEY:
        resend.api_key = RESEND_API_KEY
        RESEND_DISPONIBLE = True
    else:
        RESEND_DISPONIBLE = False
except ImportError:
    RESEND_DISPONIBLE = False

def get_rabbitmq_connection():
    for i in range(5):
        try:
            return pika.BlockingConnection(pika.ConnectionParameters(host=RABBITMQ_HOST))
        except pika.exceptions.AMQPConnectionError:
            time.sleep(2)
    raise Exception("No se pudo conectar a RabbitMQ")

def enviar_email_resend(email: str, pedido_id: int, cliente: str, producto: str,
                        cantidad: int, precio_total: float, estado: str, mensaje: str) -> bool:
    """Envía un correo electrónico real usando Resend API"""
    try:
        import resend

        # Construir el HTML del correo
        html_content = f"""
        <html>
        <body style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
            <div style="background-color: #f8f9fa; border-radius: 8px; padding: 20px; margin-bottom: 20px;">
                <h2 style="color: #333; margin-top: 0;">Actualización de tu Pedido #{pedido_id}</h2>
                <p style="color: #666; font-size: 16px;">Hola <strong>{cliente}</strong>,</p>
                <p style="color: #333; font-size: 16px;">{mensaje}</p>
            </div>

            <div style="background-color: #fff; border: 1px solid #e0e0e0; border-radius: 8px; padding: 20px;">
                <h3 style="color: #333; margin-top: 0;">Detalles del Pedido</h3>
                <table style="width: 100%; border-collapse: collapse;">
                    <tr>
                        <td style="padding: 8px 0; color: #666;">Producto:</td>
                        <td style="padding: 8px 0; color: #333; font-weight: bold;">{producto} x {cantidad}</td>
                    </tr>
                    <tr>
                        <td style="padding: 8px 0; color: #666;">Total:</td>
                        <td style="padding: 8px 0; color: #333; font-weight: bold;">${precio_total:.2f}</td>
                    </tr>
                    <tr>
                        <td style="padding: 8px 0; color: #666;">Estado:</td>
                        <td style="padding: 8px 0;">
                            <span style="background-color: {'#28a745' if estado == 'COMPLETADO' else '#dc3545'};
                                         color: white; padding: 4px 12px; border-radius: 4px; font-weight: bold;">
                                {estado}
                            </span>
                        </td>
                    </tr>
                </table>
            </div>

            <div style="margin-top: 20px; padding: 15px; background-color: #e9ecef; border-radius: 8px;">
                <p style="margin: 0; color: #666; font-size: 14px;">
                    Gracias por confiar en nosotros.<br>
                    <strong>Equipo de Soporte</strong>
                </p>
            </div>
        </body>
        </html>
        """

        asunto = f"Actualización de tu Pedido #{pedido_id} - {estado}"

        params = {
            "from": EMAIL_FROM,
            "to": email,
            "subject": asunto,
            "html": html_content
        }

        email_response = resend.Emails.send(params)
        print(f"[✓] Email enviado exitosamente via Resend. ID: {email_response.get('id')}")
        return True

    except Exception as e:
        print(f"[X] Error al enviar email con Resend: {e}")
        return False


def simular_envio_email(email: str, pedido_id: int, cliente: str, producto: str,
                        cantidad: int, precio_total: float, estado: str, mensaje: str):
    """Simula el envío de un correo electrónico (fallback cuando no hay Resend)"""
    print("\n" + "="*60)
    print(f"📧 [SIMULACIÓN] ENVIANDO EMAIL A: {email}")
    print(f"Cliente: {cliente}")
    print(f"Asunto: Actualización de tu pedido #{pedido_id}")
    print(f"Producto: {producto} x {cantidad}")
    print(f"Total: ${precio_total:.2f}")
    print(f"Estado: {estado}")
    print(f"Mensaje: {mensaje}")
    print("="*60 + "\n")


def enviar_notificacion_email(email: str, pedido_id: int, cliente: str, producto: str,
                               cantidad: int, precio_total: float, estado: str, mensaje: str):
    """Envía notificación por email usando Resend o simula si no está disponible"""
    if RESEND_DISPONIBLE and RESEND_API_KEY:
        success = enviar_email_resend(
            email=email,
            pedido_id=pedido_id,
            cliente=cliente,
            producto=producto,
            cantidad=cantidad,
            precio_total=precio_total,
            estado=estado,
            mensaje=mensaje
        )
        if not success:
            # Fallback a simulación si falla Resend
            print("[!] Fallback a simulación de email")
            simular_envio_email(email, pedido_id, cliente, producto, cantidad, precio_total, estado, mensaje)
    else:
        # No hay API key de Resend configurada
        simular_envio_email(email, pedido_id, cliente, producto, cantidad, precio_total, estado, mensaje)

def procesar_mensaje(ch, method, properties, body):
    mensaje_broker = json.loads(body)
    print(f"[*] Recibido evento de Inventario en Notificaciones: {mensaje_broker}")

    # Extraer todos los datos del evento (Event-carried state transfer)
    pedido_id = mensaje_broker.get("pedido_id")
    cliente = mensaje_broker.get("cliente")
    email = mensaje_broker.get("email")
    producto = mensaje_broker.get("producto")
    cantidad = mensaje_broker.get("cantidad")
    precio_total = mensaje_broker.get("precio_total")
    estado_pago = mensaje_broker.get("estado_pago")
    estado_inventario = mensaje_broker.get("estado_inventario")

    db: Session = SessionLocal()

    try:
        # 1. VERIFICAR IDEMPOTENCIA: Si ya existe una notificación para este pedido, ignorar
        notificacion_existente = db.query(Notificacion).filter(Notificacion.pedido_id == pedido_id).first()
        if notificacion_existente:
            print(f"[!] Pedido {pedido_id} ya fue notificado. Mensaje duplicado ignorado.")
            ch.basic_ack(delivery_tag=method.delivery_tag)
            db.close()
            return

        # 2. Determinar el estado final y el mensaje basado en el inventario
        if estado_inventario == "RESERVADO":
            estado_final = "COMPLETADO"
            cuerpo_mensaje = f"¡Buenas noticias {cliente}! Tu pago fue aprobado, confirmamos stock de {producto} y tu pedido está siendo preparado para el envío."
        elif estado_inventario == "PRODUCTO_NO_ENCONTRADO":
            estado_final = "CANCELADO_PRODUCTO_NO_DISPONIBLE"
            cuerpo_mensaje = f"Lo sentimos {cliente}. Tu pago fue procesado, pero el producto '{producto}' no está disponible en nuestro catálogo. Iniciaremos el proceso de reembolso a la brevedad."
        else:  # RECHAZADO_SIN_STOCK
            estado_final = "CANCELADO_SIN_STOCK"
            cuerpo_mensaje = f"Lo sentimos {cliente}. Tu pago fue procesado, pero no tenemos stock disponible de {producto}. Iniciaremos el proceso de reembolso a la brevedad."

        # 3. Enviar el correo con información completa (usando Resend o simulación)
        enviar_notificacion_email(
            email=email,
            pedido_id=pedido_id,
            cliente=cliente,
            producto=producto,
            cantidad=cantidad,
            precio_total=precio_total,
            estado=estado_final,
            mensaje=cuerpo_mensaje
        )

        # 4. Registrar el evento final en la base de datos
        nueva_notificacion = Notificacion(
            pedido_id=pedido_id,
            email_cliente=email,
            estado_final_pedido=estado_final,
            mensaje_enviado=cuerpo_mensaje
        )
        db.add(nueva_notificacion)
        db.commit()
        print(f"[✓] Registro de notificación guardado en BD para el pedido {pedido_id}")

        # 5. Confirmar procesamiento a RabbitMQ
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
    
    # Nos suscribimos a la cola final
    canal.queue_declare(queue='cola_inventario', durable=True)
    canal.basic_qos(prefetch_count=1)
    canal.basic_consume(queue='cola_inventario', on_message_callback=procesar_mensaje)
    
    print(" [*] Servicio de Notificaciones esperando mensajes en 'cola_inventario'.")
    canal.start_consuming()
