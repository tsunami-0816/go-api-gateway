package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/go-api-gateway/gateway/internal/model"
	"gorm.io/gorm"
)

// ServiceRepository provides CRUD for gw_service.
type ServiceRepository struct {
	db *gorm.DB
}

func NewServiceRepository(db *gorm.DB) *ServiceRepository {
	return &ServiceRepository{db: db}
}

func (r *ServiceRepository) Create(ctx context.Context, m *model.Service) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *ServiceRepository) Update(ctx context.Context, m *model.Service) error {
	result := r.db.WithContext(ctx).Model(&model.Service{}).
		Where("id = ? AND version = ?", m.ID, m.Version).
		Updates(map[string]interface{}{
			"display_name":         m.DisplayName,
			"health_check_path":    m.HealthCheckPath,
			"health_check_interval": m.HealthCheckInterval,
			"health_check_timeout":  m.HealthCheckTimeout,
			"default_weight":       m.DefaultWeight,
			"description":          m.Description,
			"updated_by":           m.UpdatedBy,
			"version":              gorm.Expr("version + 1"),
		})
	if result.Error != nil {
		return fmt.Errorf("update service: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ServiceRepository) Delete(ctx context.Context, name string) error {
	return r.db.WithContext(ctx).Delete(&model.Service{}, "service_name = ?", name).Error
}

func (r *ServiceRepository) GetByName(ctx context.Context, name string) (*model.Service, error) {
	var m model.Service
	if err := r.db.WithContext(ctx).First(&m, "service_name = ?", name).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *ServiceRepository) List(ctx context.Context, page, pageSize int) ([]*model.Service, int64, error) {
	var total int64
	r.db.WithContext(ctx).Model(&model.Service{}).Count(&total)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var list []*model.Service
	err := r.db.WithContext(ctx).Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

// AuditRepository provides operations for gw_config_version and gw_operation_log.
type AuditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

// RecordConfigVersion writes a configuration change snapshot.
func (r *AuditRepository) RecordConfigVersion(ctx context.Context, v *model.ConfigVersion) error {
	return r.db.WithContext(ctx).Create(v).Error
}

// RecordOperationLog writes an admin operation audit entry (async-safe).
func (r *AuditRepository) RecordOperationLog(ctx context.Context, log *model.OperationLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// ListOperationLogs queries operation logs with filters.
func (r *AuditRepository) ListOperationLogs(ctx context.Context, page, pageSize int, operator, resourceType string, startTime, endTime time.Time) ([]*model.OperationLog, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.OperationLog{})
	if operator != "" {
		query = query.Where("operator = ?", operator)
	}
	if resourceType != "" {
		query = query.Where("resource_type = ?", resourceType)
	}
	if !startTime.IsZero() {
		query = query.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("created_at <= ?", endTime)
	}
	var total int64
	query.Count(&total)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var list []*model.OperationLog
	err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}

// ListConfigVersions queries configuration change history.
func (r *AuditRepository) ListConfigVersions(ctx context.Context, page, pageSize int, configType, configID string) ([]*model.ConfigVersion, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.ConfigVersion{})
	if configType != "" {
		query = query.Where("config_type = ?", configType)
	}
	if configID != "" {
		query = query.Where("config_id = ?", configID)
	}
	var total int64
	query.Count(&total)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var list []*model.ConfigVersion
	err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error
	return list, total, err
}
