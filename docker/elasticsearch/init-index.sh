#!/bin/bash

# Wait for Elasticsearch to be ready
echo "Waiting for Elasticsearch to be ready..."
until curl -s -f http://localhost:9200/_cluster/health > /dev/null; do
    echo "Elasticsearch is not ready yet. Waiting..."
    sleep 5
done

echo "Elasticsearch is ready. Creating index..."

# Create the orders index with mapping
curl -X PUT "http://localhost:9200/orders" \
  -H "Content-Type: application/json" \
  -d '{
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 0,
    "analysis": {
      "analyzer": {
        "order_analyzer": {
          "type": "custom",
          "tokenizer": "standard",
          "filter": ["lowercase", "stop", "snowball"]
        }
      }
    }
  },
  "mappings": {
    "properties": {
      "id": { "type": "keyword" },
      "customer_id": { "type": "keyword" },
      "customer_name": {
        "type": "text",
        "analyzer": "order_analyzer",
        "fields": { "keyword": { "type": "keyword" } }
      },
      "customer_email": { "type": "keyword" },
      "product_name": {
        "type": "text",
        "analyzer": "order_analyzer",
        "fields": { "keyword": { "type": "keyword" } }
      },
      "product_sku": { "type": "keyword" },
      "quantity": { "type": "integer" },
      "amount": { "type": "double" },
      "currency": { "type": "keyword" },
      "status": { "type": "keyword" },
      "shipping_address": { "type": "text" },
      "notes": { "type": "text", "analyzer": "order_analyzer" },
      "created_at": { "type": "date" },
      "updated_at": { "type": "date" }
    }
  }
}'

echo ""
echo "Index created successfully!"

# Check index health
echo "Checking index status..."
curl -s "http://localhost:9200/orders/_settings?pretty"
