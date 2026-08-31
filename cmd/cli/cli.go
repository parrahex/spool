package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	"uuid"

	"github.com/parrahex/spool/internal/jobs"
	"github.com/parrahex/spool/internal/queue"
	"github.com/parrahex/spool/internal/runner"
	"github.com/parrahex/spool/internal/store"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runCmd builds the `run` command; Cobra calls RunE later
// when the user actually enters `spool run`
func runCmd() *cobra.Command {
	// Cobra writes parsed flag values into these variables through their pointers
	var image string
	var path string
	var timeout int
	var retries int

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a job",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Positional arguments become the command that will run in Docker
			if len(args) == 0 {
				return fmt.Errorf("command is required")
			}

			// The CLI stores job metadata and queue entries in the same Redis instance
			// that the worker uses to discover and process jobs
			addr := viper.GetString("redis_addr")
			q := queue.NewQueue(addr)
			s := store.NewStore(addr)

			if path != "" {
				// Store an absolute path so the worker does not depend on the caller's
				// current working directory
				absolutePath, err := filepath.Abs(path)
				if err != nil {
					return err
				}
				path = absolutePath
			}

			// A Job is the durable description of the work; save it before its ID
			// is pushed into the queue so a worker can load it immediately
			job := &jobs.Job{
				ID:         uuid.New().String(),
				Image:      image,
				Path:       path,
				Command:    args,
				Status:     jobs.StatusPending,
				Timeout:    time.Duration(timeout) * time.Second,
				MaxRetries: retries,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}

			// The command context carries cancellation and deadlines to Redis calls
			ctx := cmd.Context()

			if err := s.Save(ctx, job); err != nil {
				return err
			}

			if err := q.Enqueue(ctx, job.ID); err != nil {
				if rollbackErr := s.Delete(ctx, job.ID); rollbackErr != nil {
					return fmt.Errorf("enqueue job: %w; rollback failed: %v", err, rollbackErr)
				}
				return fmt.Errorf("enqueue job: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "enqueued job:", job.ID, "image:", image, "command:", args)

			lastStatus := ""
			finished, err := waitForJob(ctx, s, job.ID, func(current *jobs.Job) {
				if current.Status != lastStatus {
					fmt.Fprintln(cmd.OutOrStdout(), "status:", current.Status)
					lastStatus = current.Status
				}
			})
			if errors.Is(err, context.Canceled) {
				fmt.Fprintln(cmd.ErrOrStderr(), "stopped waiting; job continues:", job.ID)
				return nil
			}
			if err != nil {
				return fmt.Errorf("wait for job %s: %w", job.ID, err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "--- result ---")
			writeJob(cmd.OutOrStdout(), finished)
			if finished.Status != jobs.StatusCompleted {
				return fmt.Errorf("job finished with status %s", finished.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&image, "image", "", "Docker image")
	cmd.Flags().StringVar(&path, "path", "", "Path to the code directory")
	cmd.Flags().IntVar(&timeout, "timeout", 600, "Execution timeout in seconds")
	cmd.Flags().IntVar(&retries, "retries", jobs.DefaultRetries, "Max retry attempts")
	cmd.MarkFlagRequired("image")

	return cmd
}

func statusCmd() *cobra.Command {
	// status reads the latest job snapshot from Redis and formats it for a human
	return &cobra.Command{
		Use:   "status <id>",
		Short: "Show job status and result",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := store.NewStore(viper.GetString("redis_addr"))
			job, err := s.Get(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("job not found: %w", err)
			}

			writeJob(cmd.OutOrStdout(), job)
			return nil
		},
	}
}

func cancelCmd() *cobra.Command {
	// cancel marks a job as cancelled and removes any matching Docker container
	return &cobra.Command{
		Use:   "cancel <id>",
		Short: "Cancel a pending or running job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := store.NewStore(viper.GetString("redis_addr"))
			job, err := s.Get(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("job not found: %w", err)
			}

			if job.Status == jobs.StatusCompleted || job.Status == jobs.StatusFailed || job.Status == jobs.StatusCancelled {
				return fmt.Errorf("job already finished with status: %s", job.Status)
			}

			// A pending job may not have a container, while a running job may need
			// its container forcefully removed before the cancelled state is saved
			runner.CleanupOrphaned(job.ID)
			job.MarkCancelled()

			if err := s.Save(cmd.Context(), job); err != nil {
				return fmt.Errorf("save error: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "cancelled job:", job.ID)
			return nil
		},
	}
}

func main() {
	// The CLI uses localhost unless a different Viper value is configured
	viper.SetDefault("redis_addr", "localhost:6379")

	rootCmd := &cobra.Command{
		Use:          "spool",
		SilenceUsage: true,
	}

	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(cancelCmd())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
