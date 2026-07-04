package store

import (
	"context"
	"encoding/json"

	"github.com/go-redis/redis/v9"
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
	return s.client.Set(ctx, j.ID, data, 0).Err()
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
