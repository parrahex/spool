package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/parrahex/spool/internal/jobs"
	"github.com/parrahex/spool/internal/platform"
	"github.com/parrahex/spool/internal/queue"
	"github.com/parrahex/spool/internal/store"
)

const watchInterval = 3 * time.Second

type Bot struct {
	p platform.Platform
	q *queue.Queue
	s *store.Store
}

func New(p platform.Platform, q *queue.Queue, s *store.Store) *Bot {
	return &Bot{p: p, q: q, s: s}
}

func (b *Bot) Run(ctx context.Context) error {
	fmt.Println("bot listening")
	return b.p.Listen(ctx, b.handle)
}

func (b *Bot) handle(ctx context.Context, msg platform.Message) {
	image, timeout, command, err := parseArgs(msg.Text)
	if err != nil {
		b.p.Reply(ctx, msg, "usage: `run <image> <command>`  e.g.  `run alpine echo hello`")
		return
	}

	var filePath string
	if len(msg.Files) > 0 {
		filePath, err = b.saveFile(ctx, msg, msg.Files[0])
		if err != nil {
			b.p.Reply(ctx, msg, "file download failed: "+err.Error())
			return
		}
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
	timeout = 600
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", 0, nil, fmt.Errorf("empty message")
	}

	idx := 0
	if strings.ToLower(fields[0]) == "run" {
		idx = 1
	}
	if idx >= len(fields) {
		return "", 0, nil, fmt.Errorf("image is required")
	}

	image = fields[idx]
	if idx+1 >= len(fields) {
		return "", 0, nil, fmt.Errorf("command is required")
	}

	command = fields[idx+1:]

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

func (b *Bot) saveFile(ctx context.Context, msg platform.Message, f platform.File) (string, error) {
	dir, err := os.MkdirTemp("", "spool-bot-")
	if err != nil {
		return "", err
	}
	dest := filepath.Join(dir, f.Name)
	if err := b.p.Download(ctx, f, dest); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dest, nil
}

func (b *Bot) submitJob(ctx context.Context, image string, command []string, path string, timeout int) (string, error) {
	job := &jobs.Job{
		ID:         uuid.NewString(),
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
		return "", err
	}
	return job.ID, nil
}

func (b *Bot) watchJob(ctx context.Context, msg platform.Message, jobID string) {
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
		lines := strings.Split(out, "\n")
		if len(lines) > 40 {
			lines = lines[:40]
			sb.WriteString(strings.Join(lines, "\n"))
			sb.WriteString("\n... (truncated)")
		} else {
			sb.WriteString(out)
		}
	}
	return sb.String()
}
