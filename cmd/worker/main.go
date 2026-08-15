package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/parrahex/spool/internal/jobs"
	"github.com/parrahex/spool/internal/queue"
	"github.com/parrahex/spool/internal/runner"
	"github.com/parrahex/spool/internal/store"
)

const (
	// Lease renewals keep another worker from recovering a job that is still active
	leaseInterval = 15 * time.Second
	// Active jobs receive this much time to finish after shutdown begins
	defaultShutdownTimeout = 30 * time.Second
	// Final job state is saved with this independent timeout during shutdown
	saveTimeout = 5 * time.Second
)

func main() {
	// Intake and active execution have separate lifetimes: stop taking new jobs
	// first, then cancel active jobs only after the shutdown grace period
	intakeCtx, stopIntake := context.WithCancel(context.Background())
	jobCtx, stopJobs := context.WithCancel(context.Background())
	defer stopIntake()
	defer stopJobs()

	// Listen for Ctrl+C and the termination signal used by process managers
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	// Queue and store point to the same Redis instance so IDs and job data match
	addr := redisAddr()
	q := queue.NewQueue(addr)
	s := store.NewStore(addr)

	// Requeue jobs whose previous worker disappeared before finishing them
	recoverExpired(context.Background(), s, q)

	var wg sync.WaitGroup
	concurrency := workerCount()
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		// Each goroutine runs an independent queue consumer
		go func(id int) {
			defer wg.Done()
			workerLoop(intakeCtx, jobCtx, id, q, s)
		}(i)
	}

	// Block until the operating system asks the worker to stop
	<-signals
	fmt.Println("shutdown requested; waiting for active jobs")
	// Do not accept more jobs, but let current jobs continue for now
	stopIntake()
	// Force active jobs to stop if they exceed the graceful shutdown window
	shutdown := time.AfterFunc(shutdownTimeout(), stopJobs)
	wg.Wait()
	shutdown.Stop()
	stopJobs()
	fmt.Println("worker shutting down")
}

func shutdownTimeout() time.Duration {
	// SPOOL_SHUTDOWN_TIMEOUT accepts Go duration strings such as "45s" or "2m"
	if value := os.Getenv("SPOOL_SHUTDOWN_TIMEOUT"); value != "" {
		if timeout, err := time.ParseDuration(value); err == nil && timeout > 0 {
			return timeout
		}
	}
	return defaultShutdownTimeout
}

func redisAddr() string {
	// REDIS_ADDR lets deployments point the worker at a non-local Redis instance
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}
	return "localhost:6379"
}

func workerCount() int {
	// SPOOL_CONCURRENCY controls how many jobs can be processed in parallel
	s := os.Getenv("SPOOL_CONCURRENCY")
	if s == "" {
		return 1
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func recoverExpired(ctx context.Context, s *store.Store, q *queue.Queue) {
	// A job with an expired lease was probably interrupted by a worker crash
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
	// Remove a container left by the crashed worker before putting the job back
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

func workerLoop(intakeCtx, jobCtx context.Context, id int, q *queue.Queue, s *store.Store) {
	// Keep consuming until intake is cancelled during shutdown
	for {
		job := nextJob(intakeCtx, q, s)
		if job == nil {
			if intakeCtx.Err() != nil {
				return
			}
			continue
		}
		fmt.Printf("[worker-%d] processing job: %s\n", id, job.ID)
		processJob(jobCtx, s, q, job)
	}
}

func nextJob(ctx context.Context, q *queue.Queue, s *store.Store) *jobs.Job {
	// The queue carries only IDs; the store contains the full job definition
	jobID, err := q.Dequeue(ctx)
	if err != nil {
		fmt.Println("Dequeue error:", err)
		return nil
	}
	if jobID == "" {
		return nil
	}
	job, err := s.Get(ctx, jobID)
	// Get failed but the ID is already out of the queue; put it back
	// so a transient Redis error does not lose the job
	if err != nil {
		fmt.Println("Get Job error:", err)
		if reerr := q.Enqueue(ctx, jobID); reerr != nil {
			fmt.Println("Requeue error:", reerr)
		}
		return nil
	}
	if job.IsCancelled() {
		// A cancelled pending job can still have an old ID in the queue
		fmt.Println("skipping cancelled job:", job.ID)
		return nil
	}
	return job
}

func processJob(ctx context.Context, s *store.Store, q *queue.Queue, job *jobs.Job) {
	// Convert unexpected panics into a failed job instead of killing the worker
	defer handlePanic(job, s)

	if !acquireLease(ctx, s, q, job) {
		return
	}

	// Keep extending ownership while Docker is running
	stopLeaseRenewer := startLeaseRenewer(ctx, s, job)
	defer close(stopLeaseRenewer)

	// runner.Run mutates the job object; saveFinal persists those mutations
	runner.Run(ctx, job)

	// Use a fresh context so shutdown cancellation does not prevent the final save
	saveCtx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	saveFinal(saveCtx, s, job)
}

func handlePanic(job *jobs.Job, s *store.Store) {
	// recover catches panics raised while processing one job
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
	// Persist ownership before starting Docker so a crash can be detected later
	job.MarkLeased(time.Now(), leaseInterval*2)

	if job.ExhaustedRetries() {
		// Do not start another execution after the retry threshold is reached
		job.MarkFailed(fmt.Sprintf("exhausted %d retries", job.MaxRetries))
		if err := s.Save(ctx, job); err != nil {
			fmt.Println("retry limit save error:", err)
		}
		fmt.Println("job exhausted retries:", job.ID)
		return false
	}

	if err := s.Save(ctx, job); err != nil {
		// If ownership was not persisted, return the ID to the queue for retry
		fmt.Println("lease save error:", err)
		if reerr := q.Requeue(ctx, job.ID); reerr != nil {
			fmt.Println("requeue after lease failure:", reerr)
		}
		return false
	}
	return true
}

func startLeaseRenewer(ctx context.Context, s *store.Store, job *jobs.Job) chan struct{} {
	// Closing the returned channel tells the renewer that processing is finished
	done := make(chan struct{})
	go renewLeaseLoop(ctx, s, job, done)
	return done
}

func renewLeaseLoop(ctx context.Context, s *store.Store, job *jobs.Job, done chan struct{}) {
	// Refresh the lease before it expires so long-running jobs are not recovered
	ticker := time.NewTicker(leaseInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// The worker is still alive, so extend its ownership window
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
	// Store the state produced by runner.Run and make it visible to status clients
	if err := s.Save(ctx, job); err != nil {
		fmt.Println("save error:", err)
		return
	}
	fmt.Println("job done:", job.ID, "status:", job.Status)
}
