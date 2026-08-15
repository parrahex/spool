package queue

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Queue is a thin Redis list of job IDs
type Queue struct {
	client *redis.Client
}

// NewQueue creates a queue client for the configured Redis address
func NewQueue(addr string) *Queue {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	return &Queue{client: client}
}

// Enqueue pushes a job ID into the waiting-job list
func (q *Queue) Enqueue(ctx context.Context, jobID string) error {
	return q.client.LPush(ctx, "spool:queue", jobID).Err()
}

// Dequeue waits for up to five seconds for the next job ID
// A timeout returns an empty ID so the worker can check shutdown state
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

// Requeue puts a job ID back into the waiting list after a recoverable failure
func (q *Queue) Requeue(ctx context.Context, jobID string) error {
	return q.client.LPush(ctx, "spool:queue", jobID).Err()
}
