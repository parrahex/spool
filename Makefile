.PHONY: dev stop

REDIS_ADDR ?= localhost:6379
SPOOL_CONCURRENCY ?= 2

dev: stop
	@docker compose up -d
	@echo "redis up"
	@sleep 1
	@echo "starting worker..."
	@REDIS_ADDR=$(REDIS_ADDR) SPOOL_CONCURRENCY=$(SPOOL_CONCURRENCY) go run ./cmd/worker &
	@sleep 1
	@echo "starting bot..."
	@go run ./cmd/bot

stop:
	@-docker compose down >/dev/null 2>&1
	@-pkill -f "go run ./cmd/worker" >/dev/null 2>&1
	@-pkill -f "go run ./cmd/bot" >/dev/null 2>&1
	@echo "stopped"
