#!/bin/bash

# Wait for Kafka Connect to be ready
echo "Waiting for Kafka Connect to be ready..."
until curl -s -f http://localhost:8083/ > /dev/null; do
    echo "Kafka Connect is not ready yet. Waiting..."
    sleep 5
done

echo "Kafka Connect is ready. Registering Debezium connector..."

# Register the Debezium PostgreSQL connector
curl -X POST http://localhost:8083/connectors \
  -H "Content-Type: application/json" \
  -d '{
  "name": "orders-connector",
  "config": {
    "connector.class": "io.debezium.connector.postgresql.PostgresConnector",
    "database.hostname": "postgres",
    "database.port": "5432",
    "database.user": "postgres",
    "database.password": "postgres",
    "database.dbname": "orders_db",
    "database.server.name": "postgres",
    "table.include.list": "public.orders",
    "plugin.name": "pgoutput",
    "slot.name": "debezium_orders_slot",
    "publication.name": "debezium_orders_publication",
    "topic.prefix": "postgres",
    "key.converter": "org.apache.kafka.connect.json.JsonConverter",
    "value.converter": "org.apache.kafka.connect.json.JsonConverter",
    "key.converter.schemas.enable": "false",
    "value.converter.schemas.enable": "false",
    "snapshot.mode": "initial",
    "heartbeat.interval.ms": "5000"
  }
}'

echo ""
echo "Debezium connector registered successfully!"

# Check connector status
echo "Checking connector status..."
curl -s http://localhost:8083/connectors/orders-connector/status | python3 -m json.tool 2>/dev/null || \
  curl -s http://localhost:8083/connectors/orders-connector/status
echo ""
