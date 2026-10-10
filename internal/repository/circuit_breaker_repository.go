package repository

import (
	"context"
	"fmt"

	"github.com/go-api-gateway/gateway/internal/model"
	"gorm.io/gorm"
)

// CircuitBreakerRepository provides CRUD for gw_circuit_breaker.
type CircuitBreakerRepository struct {
	db *gorm.DB
}

func NewCircuitBreakerRepository(db *gorm.DB) *CircuitBreakerRepository {
	return &CircuitBreakerRepository{db: db}
}

func (r *CircuitBreakerRepository) Create(ctx context.Context, m *model.CircuitBreaker) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *CircuitBreakerRepository) Update(ctx context.Context, m *model.CircuitBreaker) error {
	result := r.db.WithContext(ctx).Model(&model.CircuitBreaker{}).
		Where("id = ? AND version = ?", m.ID, m.Version).
		Updates(map[string]interface{}{
			"policy_name":              m.PolicyName,
			"error_rate_threshold":     m.ErrorRateThreshold,
			"slow_call_duration_ms":    m.SlowCallDurationMs,
			"slow_call_rate_threshold": m.SlowCallRateThreshold,
			"open_duration_ms":         m.OpenDurationMs,
			"half_open_max_requests":   m.HalfOpenMaxRequests,
			"enabled":                  m.Enabled,
			"description":              m.Description,
			"updated_by":               m.UpdatedBy,
			"version":                  gorm.Expr("version + 1"),
		})
	if result.Error != nil {
		return fmt.Errorf("update circuit breaker: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *CircuitBreakerRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.CircuitBreaker{}, "id = ?", id).Error
}

func (r *CircuitBreakerRepository) GetByID(ctx context.Context, id string) (*model.CircuitBreaker, error) {
	var m model.CircuitBreaker
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *CircuitBreakerRepository) List(ctx context.Context, page, pageSize int, routeID, targetService string) ([]*model.CircuitBreaker, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.CircuitBreaker{})
	if routeID != "" {
		query = query.Where("route_id = ?", routeID)
	}
	if targetService != "" {
		query = query.Where("target_service = ?", targetService)
	}
	var total int64
	query.Count(&total)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var list []*model.CircuitBreaker
	err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

func (r *CircuitBreakerRepository) ListEnabled(ctx context.Context) ([]*model.CircuitBreaker, error) {
	var list []*model.CircuitBreaker
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Find(&list).Error
	return list, err
}
