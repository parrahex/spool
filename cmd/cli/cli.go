package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/parrahex/spool/internal/jobs"
	"github.com/parrahex/spool/internal/queue"
	"github.com/parrahex/spool/internal/store"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func runCmd() *cobra.Command {
	var image string
	var path string
	var timeout int

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
				ID:        uuid.NewString(),
				Image:     image,
				Path:      path,
				Command:   args,
				Status:    jobs.StatusPending,
				Timeout:   time.Duration(timeout) * time.Second,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
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
	cmd.MarkFlagRequired("image")

	return cmd
}

func main() {
	viper.SetDefault("redis_addr", "localhost:6379")

	rootCmd := &cobra.Command{
		Use: "spool",
	}

	rootCmd.AddCommand(runCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
