package main

import (
	"fmt"
	"os"
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

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a job",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := viper.GetString("redis_addr")
			q := queue.NewQueue(addr)
			s := store.NewStore(addr)

			job := &jobs.Job{
				ID:        uuid.NewString(),
				Image:     image,
				Command:   args,
				Status:    jobs.StatusPending,
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
