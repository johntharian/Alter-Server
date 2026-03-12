.PHONY: dev-up dev-down migrate run-api run-worker build clean

# Start infrastructure
dev-up:
	docker compose up -d
	@echo "Waiting for services to be healthy..."
	@sleep 5
	@echo "PostgreSQL: localhost:5432"
	@echo "Redis:      localhost:6379"
	@echo "RabbitMQ:   localhost:5672 (management: localhost:15672)"

# Stop infrastructure
dev-down:
	docker compose down

# Run database migrations
migrate:
	go run cmd/migrate/main.go

# Run API server
run-api:
	go run cmd/api/main.go

# Run delivery worker
run-worker:
	go run cmd/worker/main.go

# Build binaries
build:
	go build -o bin/api cmd/api/main.go
	go build -o bin/worker cmd/worker/main.go

# Clean build artifacts
clean:
	rm -rf bin/
