package jobs

import (
	"testing"
	"time"
)

func TestJobIsExpired(t *testing.T) {
	now := time.Now()

	job := Job{
		Status:     StatusLeased,
		LeaseUntil: now.Add(-time.Second),
	}

	if !job.IsExpired(now) {
		t.Fatal("expected leased job with expired lease to be expired")
	}
}
