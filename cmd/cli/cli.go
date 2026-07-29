package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/parrahex/spool/internal/jobs"
	"github.com/parrahex/spool/internal/queue"
	"github.com/parrahex/spool/internal/runner"
	"github.com/parrahex/spool/internal/store"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func runCmd() *cobra.Command {
	var image string
	var path string
	var timeout int
	var retries int

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a job",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("command is required")
			}

			addr := viper.GetString("redis_addr")
			q := queue.NewQueue(addr)
			s := store.NewStore(addr)

			if path != "" {
				absolutePath, err := filepath.Abs(path)
				if err != nil {
					return err
				}
				path = absolutePath
			}

			job := &jobs.Job{
				ID:         uuid.NewString(),
				Image:      image,
				Path:       path,
				Command:    args,
				Status:     jobs.StatusPending,
				Timeout:    time.Duration(timeout) * time.Second,
				MaxRetries: retries,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}

			ctx := cmd.Context()

			if err := s.Save(ctx, job); err != nil {
				return err
			}
			if err := q.Enqueue(ctx, job.ID); err != nil {
				return err
			}

			fmt.Println("enqueued job:", job.ID, "image:", image, "command:", args)
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

			var duration string
			if !job.StartedAt.IsZero() && !job.FinishedAt.IsZero() {
				duration = fmt.Sprint(job.FinishedAt.Sub(job.StartedAt).Round(time.Second))
			} else if !job.StartedAt.IsZero() {
				duration = fmt.Sprint(time.Since(job.StartedAt).Round(time.Second)) + " (running)"
			}

			fmt.Println("ID:       ", job.ID)
			fmt.Println("Image:    ", job.Image)
			fmt.Println("Command:  ", strings.Join(job.Command, " "))
			fmt.Println("Status:   ", job.Status)
			fmt.Println("Attempt:  ", job.Attempt)
			fmt.Println("ExitCode: ", job.ExitCode)
			fmt.Println("Duration: ", duration)
			fmt.Println("Error:    ", job.Error)
			fmt.Println("--- output ---")
			out := strings.TrimSpace(job.Output)
			if out == "" {
				fmt.Println("(no output)")
			} else {
				lines := strings.Split(out, "\n")
				if len(lines) > 30 {
					lines = lines[:30]
				}
				fmt.Println(strings.Join(lines, "\n"))
				if len(lines) < len(strings.Split(out, "\n")) {
					fmt.Println("... (truncated)")
				}
			}
			return nil
		},
	}
}

func cancelCmd() *cobra.Command {
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

			runner.CleanupOrphaned(job.ID)
			job.MarkCancelled()

			if err := s.Save(cmd.Context(), job); err != nil {
				return fmt.Errorf("save error: %w", err)
			}

			fmt.Println("cancelled job:", job.ID)
			return nil
		},
	}
}

func main() {
	viper.SetDefault("redis_addr", "localhost:6379")

	rootCmd := &cobra.Command{
		Use: "spool",
	}

	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(cancelCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
