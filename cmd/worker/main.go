package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/parrahex/spool/internal/jobs"
	"github.com/parrahex/spool/internal/queue"
	"github.com/parrahex/spool/internal/runner"
	"github.com/parrahex/spool/internal/store"
)

func main() {
	ctx, cancel := setupContext()
	defer cancel()
	q, s := setupRedis("localhost:6379")
	runLoop(ctx, q, s)
}

func setupContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func setupRedis(addr string) (*queue.Queue, *store.Store) {
	return queue.NewQueue(addr),
		store.NewStore(addr)
}

func runLoop(ctx context.Context, q *queue.Queue, s *store.Store) {
	for {
		job := nextJob(ctx, q, s)
		if job == nil {
			if ctx.Err() != nil {
				fmt.Println("worker shutting down")
				return
			}
			continue
		}
		processJob(ctx, s, job)
	}
}

func nextJob(ctx context.Context, q *queue.Queue, s *store.Store) *jobs.Job {
	jobID, err := q.Dequeue(ctx)
	if err != nil {
		fmt.Println("Dequeue error:", err)
		return nil
	}
	if jobID == "" {
		return nil
	}
	job, err := s.Get(ctx, jobID)
	if err != nil {
		fmt.Println("Get Job error:", err)
		return nil
	}
	return job
}

func processJob(ctx context.Context, s *store.Store, job *jobs.Job) {
	runner.Run(ctx, job)

	if err := s.Save(ctx, job); err != nil {
		fmt.Println("save error:", err)
		return
	}
	fmt.Println("job done:", job.ID, "status:", job.Status)
}
