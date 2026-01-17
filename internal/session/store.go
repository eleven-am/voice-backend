package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/redis/go-redis/v9"
)

const (
	sessionTTL = 24 * time.Hour
	metricsTTL = 7 * 24 * time.Hour
)

type Store struct {
	redis *redis.Client
}

func NewStore(redisClient *redis.Client) *Store {
	return &Store{redis: redisClient}
}

func (s *Store) CreateSession(ctx context.Context, sess *Session) error {
	if sess.ID == "" {
		sess.ID = shared.NewID("sess_")
	}
	sess.Status = StatusActive
	sess.StartedAt = time.Now()
	sess.LastActiveAt = time.Now()

	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}

	return s.redis.Set(ctx, sess.RedisKey(), data, sessionTTL).Err()
}

func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	data, err := s.redis.Get(ctx, "session:"+id).Bytes()
	if err == redis.Nil {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) UpdateSession(ctx context.Context, sess *Session) error {
	sess.LastActiveAt = time.Now()
	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	return s.redis.Set(ctx, sess.RedisKey(), data, sessionTTL).Err()
}

func (s *Store) EndSession(ctx context.Context, id string, status Status) error {
	sess, err := s.GetSession(ctx, id)
	if err != nil {
		return err
	}
	sess.Status = status
	return s.UpdateSession(ctx, sess)
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	return s.redis.Del(ctx, "session:"+id).Err()
}

func (s *Store) GetActiveSessions(ctx context.Context, userID string) ([]*Session, error) {
	pattern := "session:sess_*"
	keys, err := s.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	var sessions []*Session
	for _, key := range keys {
		data, err := s.redis.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		if sess.UserID == userID && sess.Status == StatusActive {
			sessions = append(sessions, &sess)
		}
	}
	return sessions, nil
}

func (s *Store) IncrementMetric(ctx context.Context, agentID string, field string, value int64) error {
	now := time.Now().UTC()
	key := MetricsRedisKey(agentID, now.Format("2006-01-02"), now.Hour())

	pipe := s.redis.Pipeline()
	pipe.HIncrBy(ctx, key, field, value)
	pipe.Expire(ctx, key, metricsTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) IncrementSessions(ctx context.Context, agentID string) error {
	return s.IncrementMetric(ctx, agentID, "sessions", 1)
}

func (s *Store) IncrementResponses(ctx context.Context, agentID string) error {
	return s.IncrementMetric(ctx, agentID, "responses", 1)
}

func (s *Store) IncrementUtterances(ctx context.Context, agentID string) error {
	return s.IncrementMetric(ctx, agentID, "utterances", 1)
}

func (s *Store) IncrementErrors(ctx context.Context, agentID string) error {
	return s.IncrementMetric(ctx, agentID, "error_count", 1)
}

func (s *Store) TrackUniqueUser(ctx context.Context, agentID, userID string) error {
	now := time.Now().UTC()
	key := fmt.Sprintf("agent:%s:users:%s:%d", agentID, now.Format("2006-01-02"), now.Hour())

	added, err := s.redis.SAdd(ctx, key, userID).Result()
	if err != nil {
		return err
	}
	s.redis.Expire(ctx, key, metricsTTL)

	if added > 0 {
		return s.IncrementMetric(ctx, agentID, "unique_users", 1)
	}
	return nil
}

func (s *Store) RecordLatency(ctx context.Context, agentID string, latencyMs int64) error {
	now := time.Now().UTC()
	key := MetricsRedisKey(agentID, now.Format("2006-01-02"), now.Hour())

	pipe := s.redis.Pipeline()
	pipe.HIncrBy(ctx, key, "total_latency_ms", latencyMs)
	pipe.HIncrBy(ctx, key, "latency_count", 1)
	pipe.Expire(ctx, key, metricsTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) GetMetrics(ctx context.Context, agentID string, hours int) ([]*Metrics, error) {
	now := time.Now().UTC()
	var metrics []*Metrics

	for i := 0; i < hours; i++ {
		t := now.Add(-time.Duration(i) * time.Hour)
		key := MetricsRedisKey(agentID, t.Format("2006-01-02"), t.Hour())

		data, err := s.redis.HGetAll(ctx, key).Result()
		if err != nil || len(data) == 0 {
			continue
		}

		m := &Metrics{
			AgentID: agentID,
			Date:    t.Format("2006-01-02"),
			Hour:    t.Hour(),
		}

		if v, ok := data["sessions"]; ok {
			m.Sessions, _ = strconv.ParseInt(v, 10, 64)
		}
		if v, ok := data["utterances"]; ok {
			m.Utterances, _ = strconv.ParseInt(v, 10, 64)
		}
		if v, ok := data["responses"]; ok {
			m.Responses, _ = strconv.ParseInt(v, 10, 64)
		}
		if v, ok := data["unique_users"]; ok {
			m.UniqueUsers, _ = strconv.ParseInt(v, 10, 64)
		}
		if v, ok := data["error_count"]; ok {
			m.ErrorCount, _ = strconv.ParseInt(v, 10, 64)
		}
		if v, ok := data["new_installs"]; ok {
			m.NewInstalls, _ = strconv.ParseInt(v, 10, 64)
		}
		if v, ok := data["uninstalls"]; ok {
			m.Uninstalls, _ = strconv.ParseInt(v, 10, 64)
		}

		totalLatency, _ := strconv.ParseInt(data["total_latency_ms"], 10, 64)
		latencyCount, _ := strconv.ParseInt(data["latency_count"], 10, 64)
		if latencyCount > 0 {
			m.AvgLatencyMs = totalLatency / latencyCount
		}

		metrics = append(metrics, m)
	}

	return metrics, nil
}

func (s *Store) GetMetricsForLast7Days(ctx context.Context, agentID string) ([]*Metrics, error) {
	return s.GetMetrics(ctx, agentID, 7*24)
}
