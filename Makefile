# ─────────────────────────────────────────────
# Shell — use Git Bash on Windows, /bin/bash everywhere else.
# This is required for until/do/done loops, pipes and redirects.
# Note: use the 8.3 short path (PROGRA~1) to avoid spaces breaking SHELL.
# Also prepend Git usr/bin so Unix tools (sleep, etc.) are on PATH.
# ─────────────────────────────────────────────
ifeq ($(OS),Windows_NT)
    SHELL := C:/PROGRA~1/Git/bin/bash.exe
    export PATH := C:/PROGRA~1/Git/usr/bin:$(PATH)
else
    SHELL := /bin/bash
endif
.SHELLFLAGS := -c

.PHONY: all build run test test-unit test-integration lint coverage \
        docker-up docker-down wait \
        migrate seed register-connector init-es-index \
        setup teardown restart verify-pipeline \
        check-postgres check-elasticsearch check-kafka check-debezium check-app check-alertmanager check-all \
        logs logs-all logs-debezium logs-kafka logs-alertmanager \
        clean deps help

# ─────────────────────────────────────────────
# Default
# ─────────────────────────────────────────────

all: build

# ─────────────────────────────────────────────
# Build & Run
# ─────────────────────────────────────────────

build:
	@echo "Building application..."
	go build -o bin/server ./cmd/server

run:
	@echo "Running application..."
	go run ./cmd/server

# ─────────────────────────────────────────────
# Tests
# ─────────────────────────────────────────────

test: test-unit test-integration
	@echo "All tests completed"

test-unit:
	@echo "Running unit tests..."
	go test ./tests/unit/... -v -race -coverprofile=coverage.out

test-integration:
	@echo "Running integration tests..."
	go test ./tests/integration/... -v -tags=integration

lint:
	@echo "Running linter..."
	golangci-lint run ./...

coverage:
	@echo "Generating coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# ─────────────────────────────────────────────
# Docker lifecycle
# ─────────────────────────────────────────────

docker-up:
	@echo "⏳ Starting Docker services..."
	@docker compose up -d
	@echo "✅ Docker services started"

docker-down:
	@echo "⏳ Stopping Docker services..."
	@docker compose down -v
	@echo "✅ Docker services stopped"

# ─────────────────────────────────────────────
# Health-check wait loops (works on Mac/Linux/Git Bash on Windows)
# ─────────────────────────────────────────────

wait:
	@echo "⏳ Waiting for Postgres to be ready..."
	@until docker exec cdc-postgres pg_isready -U postgres > /dev/null 2>&1; do \
		echo "  Postgres not ready yet, retrying in 2s..."; \
		sleep 2; \
	done
	@echo "✅ Postgres is ready"
	@echo "⏳ Waiting for Elasticsearch to be ready..."
	@until curl -s http://localhost:9200/_cluster/health > /dev/null 2>&1; do \
		echo "  Elasticsearch not ready yet, retrying in 2s..."; \
		sleep 2; \
	done
	@echo "✅ Elasticsearch is ready"
	@echo "⏳ Waiting for Kafka Connect to be ready..."
	@until curl -s http://localhost:8083/ > /dev/null 2>&1; do \
		echo "  Kafka Connect not ready yet, retrying in 2s..."; \
		sleep 2; \
	done
	@echo "✅ Kafka Connect is ready"
	@echo "✅ All services are healthy and ready"

# ─────────────────────────────────────────────
# Data setup
# ─────────────────────────────────────────────

migrate:
	@echo "⏳ Running database migrations..."
	@docker exec -i cdc-postgres psql -U postgres -d orders_db < ./internal/db/migrations/001_create_orders.sql
	@echo "✅ Database migrations completed"

init-es-index:
	@echo "⏳ Initializing Elasticsearch index..."
	@chmod +x ./docker/elasticsearch/init-index.sh
	@./docker/elasticsearch/init-index.sh
	@echo "✅ Elasticsearch index initialized"

register-connector:
	@echo "⏳ Registering Debezium connector..."
	@chmod +x ./docker/debezium/register-connector.sh
	@./docker/debezium/register-connector.sh
	@echo "✅ Debezium connector registered"

seed:
	@echo "⏳ Seeding database..."
	@POSTGRES_HOST=localhost POSTGRES_PORT=5432 POSTGRES_USER=postgres \
		POSTGRES_PASSWORD=postgres POSTGRES_DB=orders_db \
		go run ./cmd/seed/main.go
	@echo "✅ Database seeded"

# ─────────────────────────────────────────────
# Pipeline verification
# ─────────────────────────────────────────────

verify-pipeline:
	@echo "⏳ Waiting 5s for pipeline to propagate data..."
	@sleep 5
	@echo ""
	@echo "──────────────────────────────────────"
	@echo " Pipeline Verification"
	@echo "──────────────────────────────────────"
	@echo ""
	@echo "📦 Postgres order count:"
	@docker exec cdc-postgres psql -U postgres -d orders_db -t -c "SELECT COUNT(*) FROM orders;" 2>/dev/null || echo "  (unable to query)"
	@echo ""
	@echo "🔍 Elasticsearch document count:"
	@curl -s http://localhost:9200/orders/_count 2>/dev/null || echo "  (unable to query)"
	@echo ""
	@echo "🔗 Debezium connector status:"
	@curl -s http://localhost:8083/connectors?expand=status 2>/dev/null || echo "  (unable to query)"
	@echo ""
	@echo "✅ Verification complete"

# ─────────────────────────────────────────────
# Full setup / teardown / restart
# ─────────────────────────────────────────────

setup: docker-up wait migrate init-es-index register-connector seed verify-pipeline
	@echo ""
	@echo "============================================"
	@echo "       ✅  CDC Pipeline is ready!           "
	@echo "============================================"
	@echo ""
	@echo "  Service URLs:"
	@echo "    App API        → http://localhost:8080"
	@echo "    Kibana         → http://localhost:5601"
	@echo "    Grafana        → http://localhost:3000  (admin / admin)"
	@echo "    Prometheus     → http://localhost:9090"
	@echo "    Alertmanager   → http://localhost:9093"
	@echo "    Elasticsearch  → http://localhost:9200"
	@echo "    Kafka Connect  → http://localhost:8083"
	@echo "    PostgreSQL     → localhost:5432  (postgres / postgres)"
	@echo ""

teardown:
	@echo "⏳ Tearing down all services and volumes..."
	@docker compose down -v
	@echo "✅ Teardown complete — all containers and volumes removed"

restart: teardown setup

# ─────────────────────────────────────────────
# Individual health checks
# ─────────────────────────────────────────────

check-postgres:
	@echo "⏳ Checking Postgres..."
	@docker exec cdc-postgres psql -U postgres -d orders_db -c "SELECT COUNT(*) AS order_count FROM orders;"
	@echo "✅ Postgres check done"

check-elasticsearch:
	@echo "⏳ Checking Elasticsearch..."
	@curl -s http://localhost:9200/orders/_count | python3 -m json.tool 2>/dev/null || curl -s http://localhost:9200/orders/_count
	@echo ""
	@echo "✅ Elasticsearch check done"

check-kafka:
	@echo "⏳ Listing Kafka topics..."
	@docker exec cdc-kafka kafka-topics --bootstrap-server localhost:9092 --list
	@echo "✅ Kafka check done"

check-debezium:
	@echo "⏳ Checking Debezium connector status..."
	@curl -s http://localhost:8083/connectors?expand=status | python3 -m json.tool 2>/dev/null || curl -s http://localhost:8083/connectors?expand=status
	@echo ""
	@echo "✅ Debezium check done"

check-app:
	@echo "⏳ Checking app health..."
	@curl -s http://localhost:8080/health | python3 -m json.tool 2>/dev/null || curl -s http://localhost:8080/health
	@echo ""
	@echo "✅ App health check done"

check-alertmanager:
	@echo "⏳ Checking Alertmanager health..."
	@curl -s http://localhost:9093/-/healthy
	@echo ""
	@echo "✅ Alertmanager health check done"

check-all: check-postgres check-elasticsearch check-kafka check-debezium check-app check-alertmanager
	@echo "✅ All checks completed"

# ─────────────────────────────────────────────
# Logs
# ─────────────────────────────────────────────

logs:
	@echo "⏳ Tailing application logs..."
	docker compose logs -f app

logs-all:
	@echo "⏳ Tailing all service logs..."
	docker compose logs -f

logs-debezium:
	@echo "⏳ Tailing Kafka Connect / Debezium logs..."
	docker compose logs -f kafka-connect

logs-kafka:
	@echo "⏳ Tailing Kafka logs..."
	docker compose logs -f kafka

logs-alertmanager:
	@echo "⏳ Tailing Alertmanager logs..."
	docker compose logs -f alertmanager

# ─────────────────────────────────────────────
# Utilities
# ─────────────────────────────────────────────

clean:
	@echo "Cleaning up..."
	rm -f bin/server
	rm -f coverage.out coverage.html
	go clean -cache

deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

help:
	@echo ""
	@echo "==================================================================="
	@echo "                  CDC Pipeline - Makefile Targets                  "
	@echo "==================================================================="
	@echo ""
	@echo "  WORKFLOW"
	@echo "    setup                - Full pipeline setup (docker-up → wait → migrate → ...)"
	@echo "    teardown             - Stop and remove all containers and volumes"
	@echo "    restart              - teardown then setup"
	@echo ""
	@echo "  DOCKER"
	@echo "    docker-up            - Start Docker services"
	@echo "    docker-down          - Stop Docker services and remove volumes"
	@echo "    wait                 - Wait for Postgres, Elasticsearch, Kafka Connect"
	@echo ""
	@echo "  DATA SETUP"
	@echo "    migrate              - Run SQL migrations inside the Postgres container"
	@echo "    init-es-index        - Create Elasticsearch index with mappings"
	@echo "    register-connector   - Register Debezium connector with Kafka Connect"
	@echo "    seed                 - Seed database with sample orders"
	@echo "    verify-pipeline      - Print counts and connector status to verify CDC flow"
	@echo ""
	@echo "  HEALTH CHECKS"
	@echo "    check-all            - Run all checks below"
	@echo "    check-postgres       - Order count from Postgres"
	@echo "    check-elasticsearch  - Document count from Elasticsearch"
	@echo "    check-kafka          - List Kafka topics"
	@echo "    check-debezium       - Debezium connector status"
	@echo "    check-app            - App /health endpoint"
	@echo "    check-alertmanager   - Alertmanager /health endpoint"
	@echo ""
	@echo "  LOGS"
	@echo "    logs                 - Tail app logs"
	@echo "    logs-all             - Tail all service logs"
	@echo "    logs-debezium        - Tail Kafka Connect / Debezium logs"
	@echo "    logs-kafka           - Tail Kafka logs"
	@echo "    logs-alertmanager    - Tail Alertmanager logs"
	@echo ""
	@echo "  BUILD & TEST"
	@echo "    build                - Build the application binary"
	@echo "    run                  - Run the application locally"
	@echo "    test                 - Run all tests"
	@echo "    test-unit            - Run unit tests"
	@echo "    test-integration     - Run integration tests"
	@echo "    lint                 - Run linter"
	@echo "    coverage             - Generate HTML coverage report"
	@echo ""
	@echo "  OTHER"
	@echo "    clean                - Remove build artifacts and Go cache"
	@echo "    deps                 - Download and tidy Go modules"
	@echo ""
