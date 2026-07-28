package queue

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Queue struct {
	client *redis.Client
}

func NewQueue(addr string) *Queue {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	return &Queue{client: client}
}

func (q *Queue) Enqueue(ctx context.Context, jobID string) error {
	return q.client.LPush(ctx, "spool:queue", jobID).Err()
}

func (q *Queue) Dequeue(ctx context.Context) (string, error) {
	result, err := q.client.BRPop(ctx, 5*time.Second, "spool:queue").Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return result[1], nil
}

func (q *Queue) Requeue(ctx context.Context, jobID string) error {
	return q.client.LPush(ctx, "spool:queue", jobID).Err()
}
