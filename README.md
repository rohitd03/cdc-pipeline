# CDC Pipeline — Change Data Capture with Go, Kafka, Debezium, PostgreSQL, and Elasticsearch

A production-ready CDC (Change Data Capture) pipeline that automatically synchronizes data from PostgreSQL to Elasticsearch via Kafka and Debezium, built with Go.

## Architecture

```
┌──────────────┐         ┌──────────────┐         ┌──────────────┐
│  PostgreSQL  │──CDC──▶ │   Debezium   │──────▶ │    Kafka     │
│ (Source DB)  │         │  Connector   │         │   Stream     │
└──────────────┘         └──────────────┘         └──────────────┘
                                                           │
                                                           │ CDC Events
                                                           ▼
┌──────────────┐         ┌──────────────┐         ┌──────────────┐
│   REST API   │◀────────│ Go Consumer  │──────▶ │Elasticsearch │
│ (Chi Router) │   Read  │   Service    │  Sync  │ (Search DB)  │
└──────────────┘         └──────────────┘         └──────────────┘
       │
       │ Write
       ▼
┌──────────────┐
│  PostgreSQL  │
│ (Source DB)  │
└──────────────┘

Observability Stack:
├─ Prometheus (Metrics)
├─ Grafana (Dashboards)
└─ Zap (Structured Logging)
```

## Key Features

- **Real-time CDC**: Automatic synchronization of PostgreSQL changes to Elasticsearch
- **Event-driven**: Uses Kafka for reliable message streaming with at-least-once delivery
- **Full-text Search**: Elasticsearch provides powerful search capabilities
- **Graceful Shutdown**: Handles termination signals properly
- **Retry & DLQ**: Exponential backoff retry with Dead Letter Queue for failed messages
- **Metrics & Observability**: Prometheus metrics, Grafana dashboards, structured logging
- **Type-safe**: Strongly typed Go with comprehensive error handling
- **Tested**: Unit tests, integration tests with testcontainers
- **Production-ready**: Docker Compose setup with all dependencies

## Prerequisites

- **Docker** & **Docker Compose** — for running all services
- **Go 1.22+** — for building and running the application
- **Make** — for running build commands (optional)
- **Git** — for version control

## Quick Start

### 1. Clone the Repository

```bash
git clone https://github.com/rohit/cdc-pipeline.git
cd cdc-pipeline
```

### 2. Copy Environment Variables

```bash
cp .env.example .env
```

### 3. Start All Services

```bash
make docker-up
```

This starts:

- PostgreSQL (port 5432)
- Kafka + Zookeeper (port 9092, 2181)
- Kafka Connect + Debezium (port 8083)
- Elasticsearch (port 9200)
- Kibana (port 5601)
- Prometheus (port 9090)
- Grafana (port 3000)
- CDC Pipeline App (port 8080)

### 4. Run Database Migrations

```bash
# Migrations run automatically on app startup
# Or run manually:
docker compose exec app /app/server
```

### 5. Create Elasticsearch Index

```bash
make init-es-index
```

### 6. Register Debezium Connector

```bash
make register-connector
```

This configures Debezium to capture changes from the `orders` table in PostgreSQL.

### 7. Seed Sample Data

```bash
make seed
```

Seeds 50 sample orders into PostgreSQL. Changes will automatically flow to Elasticsearch via CDC.

### 8. Test the API

```bash
# Health check
curl http://localhost:8080/health

# Create an order
curl -X POST http://localhost:8080/api/v1/orders \
  -H "Content-Type: application/json" \
  -d '{
    "customer_id": "550e8400-e29b-41d4-a716-446655440000",
    "customer_name": "John Doe",
    "customer_email": "john@example.com",
    "product_name": "Wireless Headphones",
    "product_sku": "WH-1000X",
    "quantity": 1,
    "amount": 299.99,
    "currency": "USD",
    "shipping_address": "123 Main St, New York, NY 10001"
  }'

# Search orders in Elasticsearch
curl "http://localhost:8080/api/v1/search/orders?q=wireless"

# Get pipeline status
curl http://localhost:8080/api/v1/pipeline/status
```

## API Documentation

### Orders (Write Operations - PostgreSQL)

| Method | Endpoint                     | Description         |
| ------ | ---------------------------- | ------------------- |
| POST   | `/api/v1/orders`             | Create a new order  |
| PUT    | `/api/v1/orders/{id}/status` | Update order status |
| GET    | `/api/v1/orders/{id}`        | Get order by ID     |
| DELETE | `/api/v1/orders/{id}`        | Delete an order     |

### Search (Read Operations - Elasticsearch)

| Method | Endpoint                     | Description                  |
| ------ | ---------------------------- | ---------------------------- |
| GET    | `/api/v1/search/orders`      | Search orders with filters   |
| GET    | `/api/v1/search/orders/{id}` | Get order from Elasticsearch |

**Search Query Parameters:**

- `q` — Full-text search (product name, customer name, notes)
- `status` — Filter by status
- `customer_id` — Filter by customer
- `min_amount`, `max_amount` — Price range
- `from`, `to` — Date range (ISO 8601)
- `page`, `page_size` — Pagination
- `sort_by`, `sort_order` — Sorting

### System

| Method | Endpoint                  | Description                        |
| ------ | ------------------------- | ---------------------------------- |
| GET    | `/health`                 | Health check (Postgres, Kafka, ES) |
| GET    | `/metrics`                | Prometheus metrics                 |
| GET    | `/api/v1/pipeline/status` | CDC pipeline status & consumer lag |

## Project Structure

```
cdc-pipeline/
├── cmd/
│   ├── server/         # Main application entry point
│   └── seed/           # Database seeder
├── internal/
│   ├── api/            # HTTP handlers, middleware, router
│   ├── config/         # Configuration loading
│   ├── consumer/       # Kafka consumer & Debezium handler
│   ├── db/             # PostgreSQL connection & queries
│   ├── elasticsearch/  # Elasticsearch client & repository
│   ├── errors/         # Sentinel errors
│   ├── metrics/        # Prometheus metrics
│   ├── model/          # Domain models
│   └── service/        # Business logic layer
├── tests/
│   ├── unit/           # Unit tests
│   └── integration/    # Integration tests
├── docker/             # Docker scripts
├── postman/            # Postman collection & environment
├── docker-compose.yml  # Infrastructure setup
├── Dockerfile          # Multi-stage Go build
├── Makefile            # Build automation
└── README.md
```

## Running Tests

### Unit Tests

```bash
make test-unit
```

### Integration Tests

Requires test containers (Postgres, Kafka, Elasticsearch):

```bash
docker compose -f docker-compose.test.yml up -d
make test-integration
docker compose -f docker-compose.test.yml down
```

### Coverage Report

```bash
make coverage
# Opens coverage.html in browser
```

## Observability

### Prometheus

Access Prometheus at http://localhost:9090

Key metrics:

- `kafka_messages_consumed_total` — Messages processed
- `kafka_consumer_lag` — Consumer lag per partition
- `es_index_operations_total` — Elasticsearch operations
- `http_requests_total` — HTTP request counts
- `pipeline_lag_seconds` — Pipeline processing lag

### Grafana

Access Grafana at http://localhost:3000 (admin/admin)

Preconfigured dashboards for:

- HTTP request metrics
- Kafka consumer performance
- Elasticsearch indexing rates
- Pipeline lag monitoring

### Kibana

Access Kibana at http://localhost:5601

View and query orders indexed in Elasticsearch.

### Logs

```bash
# View application logs
make logs

# Or use docker compose
docker compose logs -f app
```

## Key Design Decisions

### 1. CDC Pattern

- **Why**: Eliminates dual-write problem, ensures data consistency
- **How**: Debezium captures row-level changes from PostgreSQL WAL

### 2. CQRS (Command Query Responsibility Segregation)

- **Writes**: Go directly to PostgreSQL
- **Reads**: Served from Elasticsearch for fast search
- **Sync**: Automatic via CDC pipeline

### 3. At-Least-Once Delivery

- Consumer commits Kafka offsets only after successful processing
- Idempotent Elasticsearch operations (index by ID)
- Retry logic with exponential backoff

### 4. Dead Letter Queue

- Failed messages after 3 retries go to DLQ topic
- Prevents blocking of consumer on poison messages
- Enables later investigation and reprocessing

### 5. Graceful Shutdown

- Listens for OS signals (SIGTERM, SIGINT)
- Finishes processing current message before shutdown
- Prevents data loss during deployment

### 6. Status Transition Validation

- Enforces valid order status flows at service layer
- Prevents invalid state transitions (e.g., delivered → pending)

## Environment Variables

See [.env.example](.env.example) for all configuration options.

## Postman Collection

Import the collection and environment from the `postman/` directory:

- `CDC_Pipeline.postman_collection.json`
- `CDC_Pipeline.postman_environment.json`

## Development

### Build Locally

```bash
make build
```

### Run Locally (without Docker)

```bash
# Ensure PostgreSQL, Kafka, and Elasticsearch are running
make run
```

### Lint

```bash
make lint
```

### Clean

```bash
make clean
```

## Troubleshooting

### Debezium connector not working

```bash
# Check connector status
curl http://localhost:8083/connectors/orders-connector/status

# Restart connector
curl -X POST http://localhost:8083/connectors/orders-connector/restart
```

### Consumer lag increasing

Check Kafka consumer health:

```bash
curl http://localhost:8080/api/v1/pipeline/status
```

View consumer logs:

```bash
docker compose logs -f app
```

### Elasticsearch not syncing

1. Check if Debezium connector is running
2. Verify Kafka topic has messages: `docker compose exec kafka kafka-console-consumer --bootstrap-server localhost:9092 --topic postgres.public.orders --from-beginning`
3. Check application logs for processing errors

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes with tests
4. Run `make test lint`
5. Submit a pull request

## License

MIT License - see LICENSE file for details

## Author

Rohit — [github.com/rohit](https://github.com/rohit)

## Acknowledgments

- Debezium for CDC capabilities
- Confluent for Kafka platform
- Elastic for search technology
- Go community for excellent libraries
