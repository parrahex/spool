package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/parrahex/spool/internal/jobs"
)

const jobPollInterval = 250 * time.Millisecond

type jobReader interface {
	Get(context.Context, string) (*jobs.Job, error)
}

func waitForJob(ctx context.Context, reader jobReader, id string, report func(*jobs.Job)) (*jobs.Job, error) {
	ticker := time.NewTicker(jobPollInterval)
	defer ticker.Stop()

	for {
		job, err := reader.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		report(job)
		if terminal(job.Status) {
			return job, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func terminal(status string) bool {
	return status == jobs.StatusCompleted ||
		status == jobs.StatusFailed ||
		status == jobs.StatusCancelled
}

func writeJob(w io.Writer, job *jobs.Job) {
	duration := ""
	if !job.StartedAt.IsZero() && !job.FinishedAt.IsZero() {
		duration = job.FinishedAt.Sub(job.StartedAt).Round(time.Second).String()
	} else if !job.StartedAt.IsZero() {
		duration = time.Since(job.StartedAt).Round(time.Second).String() + " (running)"
	}

	fmt.Fprintln(w, "ID:       ", job.ID)
	fmt.Fprintln(w, "Image:    ", job.Image)
	fmt.Fprintln(w, "Command:  ", strings.Join(job.Command, " "))
	fmt.Fprintln(w, "Status:   ", job.Status)
	fmt.Fprintln(w, "Attempt:  ", job.Attempt)
	fmt.Fprintln(w, "ExitCode: ", job.ExitCode)
	fmt.Fprintln(w, "Duration: ", duration)
	fmt.Fprintln(w, "Error:    ", job.Error)
	fmt.Fprintln(w, "--- output ---")

	output := strings.TrimSpace(job.Output)
	if output == "" {
		fmt.Fprintln(w, "(no output)")
		return
	}

	fmt.Fprintln(w, output)
}
