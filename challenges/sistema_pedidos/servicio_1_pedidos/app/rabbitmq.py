import pika
import json
import os

RABBITMQ_HOST = os.getenv("RABBITMQ_HOST", "rabbitmq")

def get_rabbitmq_connection():
    connection = pika.BlockingConnection(pika.ConnectionParameters(host=RABBITMQ_HOST))
    return connection

def publicar_evento_pedido(pedido_id: int, cliente: str, email: str, producto: str, cantidad: int, precio_total: float):
    try:
        connection = get_rabbitmq_connection()
        channel = connection.channel()

        # Declaramos la cola por si no existe aún
        channel.queue_declare(queue='cola_pedidos', durable=True)

        # Event-carried state transfer: incluir todos los datos del pedido
        evento = {
            "evento": "PedidoCreado",
            "pedido_id": pedido_id,
            "cliente": cliente,
            "email": email,
            "producto": producto,
            "cantidad": cantidad,
            "precio_total": precio_total
        }

        channel.basic_publish(
            exchange='',
            routing_key='cola_pedidos',
            body=json.dumps(evento),
            properties=pika.BasicProperties(
                delivery_mode=2, # Hace que el mensaje sea persistente
            )
        )
        connection.close()
    except Exception as e:
        print(f"Error al publicar en RabbitMQ: {e}")
