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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	p := platform.NewSlack(env("SLACK_APP_TOKEN"), env("SLACK_BOT_TOKEN"))
	b := bot.New(p, setupQueue(), setupStore())
	if err := b.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

func env(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatal(key + " is required")
	}
	return v
}

func setupQueue() *queue.Queue {
	return queue.NewQueue(redisAddr())
}

func setupStore() *store.Store {
	return store.NewStore(redisAddr())
}

func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "localhost:6379"
}
