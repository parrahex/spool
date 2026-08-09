.PHONY: dev stop

REDIS_ADDR ?= localhost:6379
SPOOL_CONCURRENCY ?= 2

dev: stop
	@docker compose up -d --wait
	@echo "redis up"
	@echo "starting worker..."
	@REDIS_ADDR=$(REDIS_ADDR) SPOOL_CONCURRENCY=$(SPOOL_CONCURRENCY) go run ./cmd/worker &
	@echo "starting bot..."
	@go run ./cmd/bot

stop:
	@pkill -TERM -f "go run ./cmd/worker" >/dev/null 2>&1 || true
	@pkill -TERM -f "go run ./cmd/bot" >/dev/null 2>&1 || true
	@for i in $$(seq 1 320); do \
		worker=$$(pgrep -f '[g]o run ./cmd/worker' || true); \
		bot=$$(pgrep -f '[g]o run ./cmd/bot' || true); \
		if [ -z "$$worker" ] && [ -z "$$bot" ]; then break; fi; \
		sleep 0.1; \
	done
	@docker compose down >/dev/null 2>&1 || true
	@echo "stopped"
