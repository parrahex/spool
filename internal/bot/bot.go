package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/parrahex/spool/internal/jobs"
	"github.com/parrahex/spool/internal/platform"
	"github.com/parrahex/spool/internal/queue"
	"github.com/parrahex/spool/internal/store"
)

// watchInterval controls how often the bot checks Redis for a finished job
const watchInterval = 3 * time.Second

// Bot connects a messaging platform to the shared Spool queue and store
type Bot struct {
	p platform.Platform
	q *queue.Queue
	s *store.Store
}

func New(p platform.Platform, q *queue.Queue, s *store.Store) *Bot {
	return &Bot{p: p, q: q, s: s}
}

func (b *Bot) Run(ctx context.Context) error {
	// The platform invokes b.handle for each incoming message
	fmt.Println("bot listening")
	return b.p.Listen(ctx, b.handle)
}

func (b *Bot) handle(ctx context.Context, msg platform.Message) {
	// A message can submit a command, an attached file, or both
	image, timeout, command, err := parseArgs(msg.Text)
	if err != nil {
		b.p.Reply(ctx, msg, "usage: `@Spool <image> [command]`")
		return
	}

	if image == "" {
		b.p.Reply(ctx, msg, "image is required")
		return
	}

	var filePath string
	if len(msg.Files) > 0 {
		filePath, err = b.saveFiles(ctx, msg)
		if err != nil {
			b.p.Reply(ctx, msg, "file download failed: "+err.Error())
			return
		}
	}

	if len(command) == 0 && filePath == "" {
		b.p.Reply(ctx, msg, "command or file is required")
		return
	}

	jobID, err := b.submitJob(ctx, image, command, filePath, timeout)
	if err != nil {
		b.p.Reply(ctx, msg, "submit failed: "+err.Error())
		return
	}

	b.p.Reply(ctx, msg, "job submitted: "+jobID)
	go b.watchJob(ctx, msg, jobID)
}

func parseArgs(text string) (image string, timeout int, command []string, err error) {
	// Bot messages use a small parser instead of Cobra because they arrive as text
	timeout = 600
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", 0, nil, fmt.Errorf("empty message")
	}

	image = fields[0]
	if len(fields) > 1 {
		command = fields[1:]
	}

	for i := 0; i < len(command); i++ {
		if command[i] == "--timeout" && i+1 < len(command) {
			if v, e := strconv.Atoi(command[i+1]); e == nil {
				timeout = v
			}
			command = append(command[:i], command[i+2:]...)
			break
		}
	}

	return image, timeout, command, nil
}

func (b *Bot) saveFiles(ctx context.Context, msg platform.Message) (string, error) {
	// Downloaded files are kept in a temporary directory whose path is stored in Job
	dir, err := os.MkdirTemp("", "spool-bot-")
	if err != nil {
		return "", err
	}
	for _, f := range msg.Files {
		dest := filepath.Join(dir, f.Name)
		if err := b.p.Download(ctx, f, dest); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	if len(msg.Files) == 1 && isArchive(msg.Files[0].Name) {
		return filepath.Join(dir, msg.Files[0].Name), nil
	}
	return dir, nil
}

func isArchive(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".zip", ".gz", ".tgz":
		return true
	default:
		return false
	}
}

func (b *Bot) submitJob(ctx context.Context, image string, command []string, path string, timeout int) (string, error) {
	// Submission mirrors the CLI: persist the full job before enqueueing its ID
	job := &jobs.Job{
		ID:         uuid.New().String(),
		Image:      image,
		Path:       path,
		Command:    command,
		Status:     jobs.StatusPending,
		Timeout:    time.Duration(timeout) * time.Second,
		MaxRetries: jobs.DefaultRetries,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := b.s.Save(ctx, job); err != nil {
		return "", err
	}
	if err := b.q.Enqueue(ctx, job.ID); err != nil {
		if rollbackErr := b.s.Delete(ctx, job.ID); rollbackErr != nil {
			return "", fmt.Errorf("enqueue job: %w; rollback failed: %v", err,
				rollbackErr)
		}
		return "", fmt.Errorf("enqueue job: %w", err)
	}
	return job.ID, nil
}

func (b *Bot) watchJob(ctx context.Context, msg platform.Message, jobID string) {
	// Poll Redis until the worker writes a terminal status
	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			job, err := b.s.Get(ctx, jobID)
			if err != nil || !isDone(job.Status) {
				continue
			}
			b.p.Reply(ctx, msg, formatResult(job))
			return
		case <-ctx.Done():
			return
		}
	}
}

func isDone(status string) bool {
	return status == jobs.StatusCompleted ||
		status == jobs.StatusFailed ||
		status == jobs.StatusCancelled
}

func formatResult(job *jobs.Job) string {
	var sb strings.Builder
	icon := "🟢"
	if job.Status != jobs.StatusCompleted {
		icon = "🔴"
	}
	fmt.Fprintf(&sb, "%s %s (exit %d)\n", icon, job.Status, job.ExitCode)
	if job.Error != "" {
		fmt.Fprintf(&sb, "error: %s\n", job.Error)
	}
	out := strings.TrimSpace(job.Output)
	if out != "" {
		sb.WriteString(out)
	}
	return sb.String()
}
