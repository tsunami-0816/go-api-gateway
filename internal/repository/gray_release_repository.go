package repository

import (
	"context"
	"fmt"

	"github.com/go-api-gateway/gateway/internal/model"
	"gorm.io/gorm"
)

// GrayReleaseRepository provides CRUD for gw_gray_release.
type GrayReleaseRepository struct {
	db *gorm.DB
}

func NewGrayReleaseRepository(db *gorm.DB) *GrayReleaseRepository {
	return &GrayReleaseRepository{db: db}
}

func (r *GrayReleaseRepository) Create(ctx context.Context, m *model.GrayRelease) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *GrayReleaseRepository) Update(ctx context.Context, m *model.GrayRelease) error {
	result := r.db.WithContext(ctx).Model(&model.GrayRelease{}).
		Where("id = ? AND version = ?", m.ID, m.Version).
		Updates(map[string]interface{}{
			"rule_name":      m.RuleName,
			"gray_version":   m.GrayVersion,
			"stable_version": m.StableVersion,
			"gray_percent":   m.GrayPercent,
			"stain_rules":    m.StainRules,
			"enabled":        m.Enabled,
			"description":    m.Description,
			"updated_by":     m.UpdatedBy,
			"version":        gorm.Expr("version + 1"),
		})
	if result.Error != nil {
		return fmt.Errorf("update gray release: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *GrayReleaseRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.GrayRelease{}, "id = ?", id).Error
}

func (r *GrayReleaseRepository) GetByID(ctx context.Context, id string) (*model.GrayRelease, error) {
	var m model.GrayRelease
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *GrayReleaseRepository) List(ctx context.Context, page, pageSize int, routeID string) ([]*model.GrayRelease, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.GrayRelease{})
	if routeID != "" {
		query = query.Where("route_id = ?", routeID)
	}
	var total int64
	query.Count(&total)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var list []*model.GrayRelease
	err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

func (r *GrayReleaseRepository) ListEnabled(ctx context.Context) ([]*model.GrayRelease, error) {
	var list []*model.GrayRelease
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Find(&list).Error
	return list, err
}

// UpdatePercent updates the gray percentage at runtime (PATCH endpoint).
func (r *GrayReleaseRepository) UpdatePercent(ctx context.Context, id string, percent int) error {
	return r.db.WithContext(ctx).Model(&model.GrayRelease{}).
		Where("id = ?", id).Update("gray_percent", percent).Error
}
