package store

import (
	"context"
	"encoding/json"

	"github.com/parrahex/spool/internal/jobs"
	"github.com/redis/go-redis/v9"
)

type Store struct {
	client *redis.Client
}

func NewStore(addr string) *Store {
	return &Store{
		client: redis.NewClient(&redis.Options{
			Addr: addr,
		}),
	}
}

func (s *Store) Save(ctx context.Context, j *jobs.Job) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, "spool:job:"+j.ID, data, 0).Err()
}

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
