package jobs

import "time"

type Job struct {
	ID        string
	Image     string
	Path      string
	Command   []string
	Status    string // pending, running, completed, failed, etc
	ExitCode  int
	Output    string
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)
