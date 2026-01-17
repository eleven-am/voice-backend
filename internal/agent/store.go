package agent

import (
	"context"
	"errors"

	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/qdrant/go-client/qdrant"
	"gorm.io/gorm"
)

type Store struct {
	db     *gorm.DB
	qdrant *qdrant.Client
}

func NewStore(db *gorm.DB, qdrantClient *qdrant.Client) *Store {
	return &Store{
		db:     db,
		qdrant: qdrantClient,
	}
}

func (s *Store) Migrate() error {
	return s.db.AutoMigrate(&Agent{}, &AgentInstall{}, &AgentReview{})
}

func (s *Store) Create(ctx context.Context, a *Agent) error {
	if a.ID == "" {
		a.ID = shared.NewID("agent_")
	}
	return s.db.WithContext(ctx).Create(a).Error
}

func (s *Store) GetByID(ctx context.Context, id string) (*Agent, error) {
	var a Agent
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, shared.ErrNotFound
	}
	return &a, err
}

func (s *Store) GetByDeveloper(ctx context.Context, developerID string) ([]*Agent, error) {
	var agents []*Agent
	err := s.db.WithContext(ctx).Where("developer_id = ?", developerID).Find(&agents).Error
	return agents, err
}

func (s *Store) ListPublic(ctx context.Context, category *shared.AgentCategory, limit, offset int) ([]*Agent, error) {
	var agents []*Agent
	q := s.db.WithContext(ctx).Where("is_public = ?", true)
	if category != nil {
		q = q.Where("category = ?", *category)
	}
	err := q.Order("total_installs DESC").Limit(limit).Offset(offset).Find(&agents).Error
	return agents, err
}

func (s *Store) Update(ctx context.Context, a *Agent) error {
	return s.db.WithContext(ctx).Save(a).Error
}

func (s *Store) Delete(ctx context.Context, id string) error {
	result := s.db.WithContext(ctx).Delete(&Agent{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (s *Store) IncrementInstalls(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Model(&Agent{}).Where("id = ?", id).
		UpdateColumns(map[string]any{
			"total_installs":  gorm.Expr("total_installs + 1"),
			"active_installs": gorm.Expr("active_installs + 1"),
		}).Error
}

func (s *Store) DecrementActiveInstalls(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Model(&Agent{}).Where("id = ?", id).
		UpdateColumn("active_installs", gorm.Expr("GREATEST(active_installs - 1, 0)")).Error
}

func (s *Store) Install(ctx context.Context, install *AgentInstall) error {
	if install.ID == "" {
		install.ID = shared.NewID("install_")
	}
	err := s.db.WithContext(ctx).Create(install).Error
	if err != nil {
		return err
	}
	return s.IncrementInstalls(ctx, install.AgentID)
}

func (s *Store) Uninstall(ctx context.Context, userID, agentID string) error {
	result := s.db.WithContext(ctx).Delete(&AgentInstall{}, "user_id = ? AND agent_id = ?", userID, agentID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return shared.ErrNotFound
	}
	return s.DecrementActiveInstalls(ctx, agentID)
}

func (s *Store) GetInstall(ctx context.Context, userID, agentID string) (*AgentInstall, error) {
	var install AgentInstall
	err := s.db.WithContext(ctx).Where("user_id = ? AND agent_id = ?", userID, agentID).First(&install).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, shared.ErrNotFound
	}
	return &install, err
}

func (s *Store) GetUserInstalls(ctx context.Context, userID string) ([]*AgentInstall, error) {
	var installs []*AgentInstall
	err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&installs).Error
	return installs, err
}

func (s *Store) GetInstalledAgents(ctx context.Context, userID string) ([]*Agent, error) {
	var agents []*Agent
	err := s.db.WithContext(ctx).
		Joins("JOIN agent_installs ON agent_installs.agent_id = agents.id").
		Where("agent_installs.user_id = ?", userID).
		Find(&agents).Error
	return agents, err
}

func (s *Store) UpdateInstallScopes(ctx context.Context, userID, agentID string, scopes []string) error {
	result := s.db.WithContext(ctx).Model(&AgentInstall{}).
		Where("user_id = ? AND agent_id = ?", userID, agentID).
		Update("granted_scopes", shared.StringSlice(scopes))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (s *Store) CreateReview(ctx context.Context, review *AgentReview) error {
	if review.ID == "" {
		review.ID = shared.NewID("review_")
	}
	err := s.db.WithContext(ctx).Create(review).Error
	if err != nil {
		return err
	}
	return s.recalculateRating(ctx, review.AgentID)
}

func (s *Store) GetReviews(ctx context.Context, agentID string, limit, offset int) ([]*AgentReview, error) {
	var reviews []*AgentReview
	err := s.db.WithContext(ctx).Where("agent_id = ?", agentID).
		Order("created_at DESC").Limit(limit).Offset(offset).Find(&reviews).Error
	return reviews, err
}

func (s *Store) GetUserReview(ctx context.Context, userID, agentID string) (*AgentReview, error) {
	var review AgentReview
	err := s.db.WithContext(ctx).Where("user_id = ? AND agent_id = ?", userID, agentID).First(&review).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, shared.ErrNotFound
	}
	return &review, err
}

func (s *Store) AddDeveloperReply(ctx context.Context, agentID, reviewID, reply string) error {
	result := s.db.WithContext(ctx).Model(&AgentReview{}).
		Where("id = ? AND agent_id = ?", reviewID, agentID).
		Updates(map[string]any{
			"developer_reply": reply,
			"replied_at":      gorm.Expr("NOW()"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return shared.ErrNotFound
	}
	return nil
}

func (s *Store) recalculateRating(ctx context.Context, agentID string) error {
	var result struct {
		Avg   float32
		Count int64
	}
	err := s.db.WithContext(ctx).Model(&AgentReview{}).
		Select("AVG(rating) as avg, COUNT(*) as count").
		Where("agent_id = ?", agentID).
		Scan(&result).Error
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Model(&Agent{}).Where("id = ?", agentID).
		Updates(map[string]any{
			"avg_rating":    result.Avg,
			"total_reviews": result.Count,
		}).Error
}

func (s *Store) SearchByEmbedding(ctx context.Context, embedding []float32, limit int) ([]*Agent, error) {
	if s.qdrant == nil {
		return nil, errors.New("qdrant client not configured")
	}

	results, err := s.qdrant.Query(ctx, &qdrant.QueryPoints{
		CollectionName: "agents",
		Query:          qdrant.NewQuery(embedding...),
		Limit:          qdrant.PtrOf(uint64(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(results))
	for _, r := range results {
		if r.Id != nil {
			if uuid := r.Id.GetUuid(); uuid != "" {
				ids = append(ids, uuid)
			}
		}
	}

	if len(ids) == 0 {
		return []*Agent{}, nil
	}

	var agents []*Agent
	err = s.db.WithContext(ctx).Where("id IN ?", ids).Find(&agents).Error
	return agents, err
}

func (s *Store) UpsertEmbedding(ctx context.Context, agentID string, embedding []float32) error {
	if s.qdrant == nil {
		return errors.New("qdrant client not configured")
	}

	_, err := s.qdrant.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: "agents",
		Points: []*qdrant.PointStruct{
			{
				Id:      qdrant.NewID(agentID),
				Vectors: qdrant.NewVectors(embedding...),
			},
		},
	})
	return err
}

func (s *Store) DeleteEmbedding(ctx context.Context, agentID string) error {
	if s.qdrant == nil {
		return errors.New("qdrant client not configured")
	}

	_, err := s.qdrant.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: "agents",
		Points:         qdrant.NewPointsSelector(qdrant.NewID(agentID)),
	})
	return err
}
