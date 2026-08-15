package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/parrahex/spool/internal/bot"
	"github.com/parrahex/spool/internal/platform"
	"github.com/parrahex/spool/internal/queue"
	"github.com/parrahex/spool/internal/store"
)

func main() {
	// Keep process-level error handling in one place so the bot exits non-zero
	// when startup or the Slack listener fails
	if err := run(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

func run() error {
	// NotifyContext stops the Slack listener when the process receives a signal
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Slack credentials are required; the bot and worker share Redis job storage
	p := platform.NewSlack(env("SLACK_APP_TOKEN"), env("SLACK_BOT_TOKEN"))
	b := bot.New(p, setupQueue(), setupStore())

	return b.Run(ctx)
}

func env(key string) string {
	// Required environment variables fail fast instead of starting a misconfigured bot
	v := os.Getenv(key)
	if v == "" {
		log.Fatal(key + " is required")
	}
	return v
}

func setupQueue() *queue.Queue {
	// The bot submits IDs to the same Redis queue consumed by workers
	return queue.NewQueue(redisAddr())
}

func setupStore() *store.Store {
	// The bot stores complete jobs in the same Redis store used by workers and CLI
	return store.NewStore(redisAddr())
}

func redisAddr() string {
	// REDIS_ADDR allows the bot to use a shared or remote Redis instance
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "localhost:6379"
}
