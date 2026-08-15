package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/parrahex/spool/internal/jobs"
	"github.com/redis/go-redis/v9"
)

// Store persists complete Job values as JSON in Redis
type Store struct {
	client *redis.Client
}

// NewStore creates a Redis-backed job store
func NewStore(addr string) *Store {
	return &Store{
		client: redis.NewClient(&redis.Options{
			Addr: addr,
		}),
	}
}

// Save serializes a job and stores it for 24 hours so completed jobs stay
// visible to status and bot clients
func (s *Store) Save(ctx context.Context, j *jobs.Job) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, "spool:job:"+j.ID, data, 24*time.Hour).Err()
}

// Get loads and deserializes a job by ID; the returned value is a local copy
// and must be saved again if the caller changes it
func (s *Store) Get(ctx context.Context, id string) (*jobs.Job, error) {
	data, err := s.client.Get(ctx, "spool:job:"+id).Bytes()
	if err != nil {
		return nil, err
	}
	var j jobs.Job
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

// ListAll scans Redis for every stored job and is used during worker recovery
func (s *Store) ListAll(ctx context.Context) ([]*jobs.Job, error) {
	iter := s.client.Scan(ctx, 0, "spool:job:*", 0).Iterator()
	var jobsList []*jobs.Job
	for iter.Next(ctx) {
		key := iter.Val()
		data, err := s.client.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		var j jobs.Job
		if err := json.Unmarshal(data, &j); err != nil {
			continue
		}
		jobsList = append(jobsList, &j)
	}
	return jobsList, iter.Err()
}
