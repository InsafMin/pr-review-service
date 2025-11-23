.PHONY: help
help:
	@echo "Available commands:"
	@echo "  build           - Build the application"
	@echo "  run             - Run the application"
	@echo "  clean           - Clean build artifacts"
	@echo "  fmt             - Format Go code"
	@echo "  mod-tidy        - Tidy Go modules"
	@echo "  mod-download    - Download Go modules"
	@echo "  docker-build    - Build Docker images"
	@echo "  docker-up       - Start Docker containers"
	@echo "  docker-down     - Stop Docker containers"
	@echo "  docker-restart  - Restart Docker containers"
	@echo "  docker-logs     - Show all Docker logs"
	@echo "  docker-logs-app - Show application logs"
	@echo "  docker-logs-db  - Show database logs"
	@echo "  docker-clean    - Clean Docker containers and volumes"

.PHONY: build
build:
	go build -o bin/server ./cmd/server

.PHONY: run
run:
	go run ./cmd/server

.PHONY: clean
clean:
	rm -rf bin/

.PHONY: docker-build
docker-build:
	docker-compose build

.PHONY: docker-up
docker-up:
	docker-compose up -d

.PHONY: docker-down
docker-down:
	docker-compose down

.PHONY: docker-restart
docker-restart:
	docker-compose restart

.PHONY: docker-logs
docker-logs:
	docker-compose logs -f

.PHONY: docker-logs-app
docker-logs-app:
	docker-compose logs -f app

.PHONY: docker-logs-db
docker-logs-db:
	docker-compose logs -f postgres

.PHONY: docker-clean
docker-clean:
	docker-compose down -v
	docker system prune -f

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: mod-tidy
mod-tidy:
	go mod tidy

.PHONY: mod-download
mod-download:
	go mod download

.DEFAULT_GOAL := help