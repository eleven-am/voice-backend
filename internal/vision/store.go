package vision

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type Store struct {
	redis    *redis.Client
	frameTTL time.Duration
}

func NewStore(redisClient *redis.Client, frameTTL time.Duration) *Store {
	if frameTTL == 0 {
		frameTTL = 60 * time.Second
	}
	return &Store{
		redis:    redisClient,
		frameTTL: frameTTL,
	}
}

func (s *Store) StoreFrame(ctx context.Context, frame *Frame) error {
	key := fmt.Sprintf("session:%s:frames", frame.SessionID)
	member := redis.Z{
		Score:  float64(frame.Timestamp),
		Member: frame.Data,
	}

	pipe := s.redis.Pipeline()
	pipe.ZAdd(ctx, key, member)
	pipe.Expire(ctx, key, s.frameTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) GetLatestFrame(ctx context.Context, sessionID string) (*Frame, error) {
	key := fmt.Sprintf("session:%s:frames", sessionID)
	results, err := s.redis.ZRevRangeWithScores(ctx, key, 0, 0).Result()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}

	data, ok := results[0].Member.(string)
	if !ok {
		return nil, fmt.Errorf("invalid frame data type")
	}

	return &Frame{
		SessionID: sessionID,
		Timestamp: int64(results[0].Score),
		Data:      []byte(data),
	}, nil
}

func (s *Store) GetFrames(ctx context.Context, sessionID string, startTime, endTime int64, limit int) ([]*Frame, error) {
	key := fmt.Sprintf("session:%s:frames", sessionID)

	opt := &redis.ZRangeBy{
		Min:   strconv.FormatInt(startTime, 10),
		Max:   strconv.FormatInt(endTime, 10),
		Count: int64(limit),
	}

	results, err := s.redis.ZRangeByScoreWithScores(ctx, key, opt).Result()
	if err != nil {
		return nil, err
	}

	frames := make([]*Frame, 0, len(results))
	for _, r := range results {
		data, ok := r.Member.(string)
		if !ok {
			continue
		}
		frames = append(frames, &Frame{
			SessionID: sessionID,
			Timestamp: int64(r.Score),
			Data:      []byte(data),
		})
	}

	return frames, nil
}

func (s *Store) DeleteFrames(ctx context.Context, sessionID string) error {
	key := fmt.Sprintf("session:%s:frames", sessionID)
	return s.redis.Del(ctx, key).Err()
}
