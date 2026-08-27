package runner

import (
	"slices"
	"testing"

	"github.com/parrahex/spool/internal/jobs"
)

func TestDockerArgsMountsWorkspaceReadOnly(t *testing.T) {
	job := &jobs.Job{
		ID:      "job-123",
		Image:   "alpine",
		Command: []string{"echo", "hello"},
	}

	got := dockerArgs(job, "/workspace")
	want := []string{
		"run",
		"--rm",
		"--label", "spool-job-id=job-123",
		"--volume", "/workspace:/app:ro",
		"--workdir", "/app",
		"alpine",
		"echo", "hello",
	}

	if !slices.Equal(got, want) {
		t.Fatalf("unexpected Docker arguments:\ngot:  %v\nwant: %v", got, want)
	}
}
