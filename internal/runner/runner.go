package runner

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/parrahex/spool/internal/artifacts"
	"github.com/parrahex/spool/internal/jobs"
)

// Run prepares the job workspace, executes Docker, and updates the Job object
// with its final status, output, and exit code; the caller persists the object
func Run(ctx context.Context, job *jobs.Job) {
	job.MarkRunning(time.Now())

	if job.Image == "" {
		job.MarkFailed("image not specified")
		return
	}

	workspace, cleanup, err := artifacts.Workspace(job.ID, job.Path)
	if err != nil {
		job.MarkFailed(err.Error())
		return
	}
	defer cleanup()

	if len(job.Command) == 0 && job.Path == "" {
		job.MarkFailed("command or file is required")
		return
	}

	runCtx, cancel := executionContext(ctx, job.Timeout)
	defer cancel()

	output, err := runDocker(runCtx, job, workspace)
	if err != nil {
		if runCtx.Err() != nil {
			CleanupOrphaned(job.ID)
		}
		job.MarkFailed(err.Error())
		job.ExitCode = exitCode(err)
		job.Output = output
		return
	}

	job.MarkCompleted(0, output)
}

// executionContext applies the job timeout and falls back to the package default
func executionContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = jobs.DefaultTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

// runDocker starts the external Docker CLI and captures both stdout and stderr
func runDocker(ctx context.Context, job *jobs.Job, workspace string) (string, error) {
	args := dockerArgs(job, workspace)
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// dockerArgs maps the Job model to `docker run` arguments
// The user's Command is appended after the image as the container process
func dockerArgs(job *jobs.Job, workspace string) []string {
	args := []string{"run", "--rm", "--label", "spool-job-id=" + job.ID}
	if workspace != "" {
		args = append(args, "--volume", workspace+":/app:ro", "--workdir", "/app")
	}
	args = append(args, job.Image)
	return append(args, job.Command...)
}

// CleanupOrphaned removes Docker containers labeled with the job ID
// It is used when cancellation or an expired worker lease leaves a container behind
func CleanupOrphaned(jobID string) {
	out, err := exec.Command("docker", "ps", "-aq", "--filter", "label=spool-job-id="+jobID).Output()
	if err != nil {
		return
	}
	for _, id := range strings.Fields(string(out)) {
		exec.Command("docker", "rm", "-f", id).Run()
	}
}

// exitCode extracts a process exit code when Docker returns an exec error
func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
