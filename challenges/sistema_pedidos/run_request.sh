#!/bin/bash

URL="http://localhost:8000/pedidos/"

curl -X POST "$URL" \
  -H "Content-Type: application/json" \
  -d '{"cliente":"Pepito Plus_1","email":"ethan.thompson.patel@pm.me","producto":"Estabilizador 2000VA","cantidad":1,"precio_total":20000}' \
  -w "\nHTTP Peticion 1: %{http_code}\n" &

curl -X POST "$URL" \
  -H "Content-Type: application/json" \
  -d '{"cliente":"Pepito Plus_2","email":"ethan.thompson.patel@pm.me","producto":"Estabilizador 2000VA","cantidad":1,"precio_total":20000}' \
  -w "\nHTTP Peticion 2: %{http_code}\n" &

wait

echo "Ambas peticiones terminadas"
