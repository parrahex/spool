package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/parrahex/spool/internal/jobs"
	"github.com/parrahex/spool/internal/queue"
	"github.com/parrahex/spool/internal/runner"
	"github.com/parrahex/spool/internal/store"
)

const leaseInterval = 15 * time.Second

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	addr := redisAddr()
	q := queue.NewQueue(addr)
	s := store.NewStore(addr)

	recoverExpired(ctx, s, q)
	runLoop(ctx, q, s)
}

func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "localhost:6379"
}

func recoverExpired(ctx context.Context, s *store.Store, q *queue.Queue) {
	all, err := s.ListAll(ctx)
	if err != nil {
		fmt.Println("recovery scan error:", err)
		return
	}
	for _, job := range all {
		if job.IsExpired(time.Now()) {
			recoverJob(ctx, s, q, job)
		}
	}
}

func recoverJob(ctx context.Context, s *store.Store, q *queue.Queue, job *jobs.Job) {
	runner.CleanupOrphaned(job.ID)
	job.Status = jobs.StatusPending
	job.Error = "worker crashed or lease expired"
	job.UpdatedAt = time.Now()

	if err := s.Save(ctx, job); err != nil {
		fmt.Println("recovery save error:", err)
		return
	}
	if err := q.Requeue(ctx, job.ID); err != nil {
		fmt.Println("recovery requeue error:", err)
		return
	}
	fmt.Println("recovered expired job:", job.ID)
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
		processJob(ctx, s, q, job)
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
	if job.IsCancelled() {
		fmt.Println("skipping cancelled job:", job.ID)
		return nil
	}
	return job
}

func processJob(ctx context.Context, s *store.Store, q *queue.Queue, job *jobs.Job) {
	defer handlePanic(job, s)

	if !acquireLease(ctx, s, q, job) {
		return
	}

	stopLeaseRenewer := startLeaseRenewer(ctx, s, job)
	defer close(stopLeaseRenewer)

	runner.Run(ctx, job)

	saveFinal(ctx, s, job)
}

func handlePanic(job *jobs.Job, s *store.Store) {
	if r := recover(); r != nil {
		msg := fmt.Sprintf("panic: %v", r)
		if err, ok := r.(error); ok {
			msg = "panic: " + err.Error()
		}
		job.MarkFailed(msg)
		s.Save(context.Background(), job)
	}
}

func acquireLease(ctx context.Context, s *store.Store, q *queue.Queue, job *jobs.Job) bool {
	job.MarkLeased(time.Now(), leaseInterval*2)

	if job.ExhaustedRetries() {
		job.MarkFailed(fmt.Sprintf("exhausted %d retries", job.MaxRetries))
		if err := s.Save(ctx, job); err != nil {
			fmt.Println("retry limit save error:", err)
		}
		fmt.Println("job exhausted retries:", job.ID)
		return false
	}

	if err := s.Save(ctx, job); err != nil {
		fmt.Println("lease save error:", err)
		if reerr := q.Requeue(ctx, job.ID); reerr != nil {
			fmt.Println("requeue after lease failure:", reerr)
		}
		return false
	}
	return true
}

func startLeaseRenewer(ctx context.Context, s *store.Store, job *jobs.Job) chan struct{} {
	done := make(chan struct{})
	go renewLeaseLoop(ctx, s, job, done)
	return done
}

func renewLeaseLoop(ctx context.Context, s *store.Store, job *jobs.Job, done chan struct{}) {
	ticker := time.NewTicker(leaseInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			job.LeaseUntil = time.Now().Add(leaseInterval * 2)
			if err := s.Save(ctx, job); err != nil {
				fmt.Println("lease refresh error:", err)
			}
		case <-done:
			return
		case <-ctx.Done():
			return
		}
	}
}

func saveFinal(ctx context.Context, s *store.Store, job *jobs.Job) {
	if err := s.Save(ctx, job); err != nil {
		fmt.Println("save error:", err)
		return
	}
	fmt.Println("job done:", job.ID, "status:", job.Status)
}
