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
		job.MarkFailed(err.Error())
		job.ExitCode = exitCode(err)
		job.Output = output
		return
	}

	job.MarkCompleted(0, output)
}

func executionContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = jobs.DefaultTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func runDocker(ctx context.Context, job *jobs.Job, workspace string) (string, error) {
	args := dockerArgs(job, workspace)
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func dockerArgs(job *jobs.Job, workspace string) []string {
	args := []string{"run", "--rm", "--label", "spool-job-id=" + job.ID}
	if workspace != "" {
		args = append(args, "--volume", workspace+":/app:ro", "--workdir", "/app")
	}
	args = append(args, job.Image)
	return append(args, job.Command...)
}

func CleanupOrphaned(jobID string) {
	out, err := exec.Command("docker", "ps", "-q", "--filter", "label=spool-job-id="+jobID).Output()
	if err != nil {
		return
	}
	for _, id := range strings.Fields(string(out)) {
		exec.Command("docker", "rm", "-f", id).Run()
	}
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
