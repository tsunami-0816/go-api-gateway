package repository

import (
	"context"
	"fmt"

	"github.com/go-api-gateway/gateway/internal/model"
	"gorm.io/gorm"
)

// RateLimitRepository provides CRUD operations for gw_rate_limit.
type RateLimitRepository struct {
	db *gorm.DB
}

// NewRateLimitRepository creates a new RateLimitRepository.
func NewRateLimitRepository(db *gorm.DB) *RateLimitRepository {
	return &RateLimitRepository{db: db}
}

func (r *RateLimitRepository) Create(ctx context.Context, m *model.RateLimit) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *RateLimitRepository) Update(ctx context.Context, m *model.RateLimit) error {
	result := r.db.WithContext(ctx).Model(&model.RateLimit{}).
		Where("id = ? AND version = ?", m.ID, m.Version).
		Updates(map[string]interface{}{
			"rule_name":    m.RuleName,
			"route_id":     m.RouteID,
			"path_pattern": m.PathPattern,
			"method":       m.Method,
			"algorithm":    m.Algorithm,
			"rate":         m.Rate,
			"capacity":     m.Capacity,
			"limit_by":     m.LimitBy,
			"header_key":   m.HeaderKey,
			"enabled":      m.Enabled,
			"description":  m.Description,
			"updated_by":   m.UpdatedBy,
			"version":      gorm.Expr("version + 1"),
		})
	if result.Error != nil {
		return fmt.Errorf("update rate limit: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *RateLimitRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.RateLimit{}, "id = ?", id).Error
}

func (r *RateLimitRepository) GetByID(ctx context.Context, id string) (*model.RateLimit, error) {
	var m model.RateLimit
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *RateLimitRepository) List(ctx context.Context, page, pageSize int, routeID string, enabled *bool) ([]*model.RateLimit, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.RateLimit{})
	if routeID != "" {
		query = query.Where("route_id = ?", routeID)
	}
	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}
	var total int64
	query.Count(&total)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var list []*model.RateLimit
	err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

func (r *RateLimitRepository) ListEnabled(ctx context.Context) ([]*model.RateLimit, error) {
	var list []*model.RateLimit
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Find(&list).Error
	return list, err
}

func (r *RateLimitRepository) UpdateStatus(ctx context.Context, id string, enabled bool) error {
	return r.db.WithContext(ctx).Model(&model.RateLimit{}).Where("id = ?", id).Update("enabled", enabled).Error
}
