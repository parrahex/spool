package jobs

import "time"

// Job is the durable description and current state of one execution request
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
	MaxRetries  int           `json:"max_retries"`
	Timeout     time.Duration `json:"timeout"`
	ContainerID string        `json:"container_id"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	LeaseUntil  time.Time     `json:"lease_until"`
	StartedAt   time.Time     `json:"started_at"`
	FinishedAt  time.Time     `json:"finished_at"`
}

const (
	// A pending job is waiting to be processed by a worker
	StatusPending = "pending"
	// A running job is currently being executed by Docker
	StatusRunning = "running"
	// A leased job has been reserved for one worker attempt
	StatusLeased = "leased"
	// Completed, failed, and cancelled are terminal states
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

const (
	// DefaultTimeout is used when a job does not provide a positive timeout
	DefaultTimeout = 10 * time.Minute
	// DefaultRetries is the default retry threshold for submitted jobs
	DefaultRetries = 3
)

// IsExpired reports whether a leased or running job has lost its worker lease
func (j *Job) IsExpired(now time.Time) bool {
	return (j.Status == StatusRunning || j.Status == StatusLeased) && now.After(j.LeaseUntil)
}

// IsCancelled reports whether the job should be skipped by a worker
func (j *Job) IsCancelled() bool {
	return j.Status == StatusCancelled
}

// MarkCompleted records a successful execution and its captured output
func (j *Job) MarkCompleted(exitCode int, output string) {
	j.Status = StatusCompleted
	j.ExitCode = exitCode
	j.Output = output
	j.FinishedAt = time.Now()
	j.UpdatedAt = j.FinishedAt
}

// MarkFailed records an execution or preparation error
func (j *Job) MarkFailed(errMsg string) {
	j.Status = StatusFailed
	j.ExitCode = -1
	j.Error = errMsg
	j.FinishedAt = time.Now()
	j.UpdatedAt = j.FinishedAt
}

// MarkLeased reserves the job for a worker attempt until leaseDuration expires
func (j *Job) MarkLeased(now time.Time, leaseDuration time.Duration) {
	j.Status = StatusLeased
	j.Attempt++
	j.LeaseUntil = now.Add(leaseDuration)
	j.UpdatedAt = now
}

// MarkRunning records the time at which execution actually starts
func (j *Job) MarkRunning(now time.Time) {
	j.Status = StatusRunning
	j.StartedAt = now
	j.UpdatedAt = now
}

// MarkCancelled records that the user cancelled the job
func (j *Job) MarkCancelled() {
	j.Status = StatusCancelled
	j.FinishedAt = time.Now()
	j.UpdatedAt = j.FinishedAt
	j.Error = "cancelled by user"
}

// ExhaustedRetries reports whether the current attempt reached the configured threshold
func (j *Job) ExhaustedRetries() bool {
	return j.Attempt >= j.MaxRetries
}
