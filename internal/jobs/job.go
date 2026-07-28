package jobs

import "time"

type Job struct {
	ID          string        `json:"id"`
	Image       string        `json:"image"`
	Path        string        `json:"path"`
	Command     []string      `json:"command"`
	Status      string        `json:"status"`
	ExitCode    int           `json:"exit_code"`
	Output      string        `json:"output"`
	Error       string        `json:"error"`
	Attempt     int           `json:"attempt"`
	Timeout     time.Duration `json:"timeout"`
	ContainerID string        `json:"container_id"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	LeaseUntil  time.Time     `json:"lease_until"`
	StartedAt   time.Time     `json:"started_at"`
	FinishedAt  time.Time     `json:"finished_at"`
}

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusLeased    = "leased"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

const DefaultTimeout = 10 * time.Minute

func (j *Job) IsExpired(now time.Time) bool {
	return (j.Status == StatusRunning || j.Status == StatusLeased) && now.After(j.LeaseUntil)
}

func (j *Job) MarkCompleted(exitCode int, output string) {
	j.Status = StatusCompleted
	j.ExitCode = exitCode
	j.Output = output
	j.FinishedAt = time.Now()
	j.UpdatedAt = j.FinishedAt
}

func (j *Job) MarkFailed(errMsg string) {
	j.Status = StatusFailed
	j.ExitCode = -1
	j.Error = errMsg
	j.FinishedAt = time.Now()
	j.UpdatedAt = j.FinishedAt
}

func (j *Job) MarkLeased(now time.Time, leaseDuration time.Duration) {
	j.Status = StatusLeased
	j.Attempt++
	j.LeaseUntil = now.Add(leaseDuration)
	j.UpdatedAt = now
}

func (j *Job) MarkRunning(now time.Time) {
	j.Status = StatusRunning
	j.StartedAt = now
	j.UpdatedAt = now
}
