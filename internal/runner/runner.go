package runner

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/parrahex/spool/internal/artifacts"
	"github.com/parrahex/spool/internal/jobs"
)

func Run(ctx context.Context, job *jobs.Job) {
	job.MarkRunning(time.Now())

	if err := validateJob(job); err != nil {
		job.MarkFailed(err.Error())
		return
	}

	workspace, cleanup, err := artifacts.Workspace(job.ID, job.Path)
	if err != nil {
		job.MarkFailed(err.Error())
		return
	}
	defer cleanup()

	runCtx, cancel := executionContext(ctx, job.Timeout)
	defer cancel()

	containerID, err := createContainer(runCtx, job, workspace)
	if err != nil {
		job.MarkFailed(err.Error())
		return
	}
	job.ContainerID = containerID
	defer cleanupContainer(containerID)

	output, err := startContainer(runCtx, containerID)
	if err != nil {
		job.MarkFailed(err.Error())
		job.ExitCode = exitCode(err)
		job.Output = output
		return
	}

	job.MarkCompleted(0, output)
}

func validateJob(job *jobs.Job) error {
	if job.Image == "" {
		return errors.New("image not specified")
	}
	if len(job.Command) == 0 {
		return errors.New("command not specified")
	}
	return nil
}

func executionContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = jobs.DefaultTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func createContainer(ctx context.Context, job *jobs.Job, workspace string) (string, error) {
	args := dockerCreateArgs(job, workspace)
	out, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		return "", fmt.Errorf("docker create: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func dockerCreateArgs(job *jobs.Job, workspace string) []string {
	args := []string{"create"}
	if workspace != "" {
		args = append(args, "--volume", workspace+":/app:ro", "--workdir", "/app")
	}
	args = append(args, "--label", "spool-job-id="+job.ID)
	args = append(args, job.Image)
	return append(args, job.Command...)
}

func startContainer(ctx context.Context, containerID string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "start", "--attach", containerID).CombinedOutput()
	return string(out), err
}

func cleanupContainer(containerID string) {
	if containerID == "" {
		return
	}
	exec.Command("docker", "rm", "-f", containerID).Run()
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
