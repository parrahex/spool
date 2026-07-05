package runner

import (
	"context"
	"os/exec"
	"time"

	"github.com/parrahex/spool/internal/jobs"
)

func Run(ctx context.Context, job *jobs.Job) {
	job.Status = jobs.StatusRunning
	job.UpdatedAt = time.Now()

	if job.Image == "" {
		fail(job, "Image not found")
		return
	}
}

func dockerArgs(job *jobs.Job) []string {
	args := []string{"run", "--rm", job.Image}
	return append(args, job.Command...)
}

func runDocker(ctx context.Context, job *jobs.Job, args []string) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()

	job.Output = string(output)
	job.UpdatedAt = time.Now()

	if err != nil {
		job.Status = jobs.StatusFailed
		job.ExitCode = exitCode(err)
		job.Error = err.Error()
		return
	}

	job.Status = jobs.StatusCompleted
	job.ExitCode = 0
}

func fail(job *jobs.Job, msg string) {
	job.Status = jobs.StatusFailed
	job.Error = msg
	job.UpdatedAt = time.Now()
	job.ExitCode = -1
}

func exitCode(err error) int {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}
