// Package repository provides data access layer for all gateway configuration tables.
//
// Each repository encapsulates GORM operations for a specific table,
// following the Repository pattern. Business logic lives in the service layer.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-api-gateway/gateway/internal/model"
	"gorm.io/gorm"
)

// RouteRepository provides CRUD operations for the gw_route table.
type RouteRepository struct {
	db *gorm.DB
}

// NewRouteRepository creates a new RouteRepository.
func NewRouteRepository(db *gorm.DB) *RouteRepository {
	return &RouteRepository{db: db}
}

// Create inserts a new route record.
// Returns error if path+method combination already exists (unique constraint).
func (r *RouteRepository) Create(ctx context.Context, route *model.Route) error {
	if err := r.db.WithContext(ctx).Create(route).Error; err != nil {
		return fmt.Errorf("create route: %w", err)
	}
	return nil
}

// Update updates an existing route with optimistic locking (version field).
func (r *RouteRepository) Update(ctx context.Context, route *model.Route) error {
	result := r.db.WithContext(ctx).
		Model(&model.Route{}).
		Where("id = ? AND version = ?", route.ID, route.Version).
		Updates(map[string]interface{}{
			"route_name":           route.RouteName,
			"path":                 route.Path,
			"method":               route.Method,
			"target_service":       route.TargetService,
			"rewrite_path":         route.RewritePath,
			"timeout_ms":           route.TimeoutMs,
			"retry_count":          route.RetryCount,
			"retry_on_status":      route.RetryOnStatus,
			"retry_non_idempotent": route.RetryNonIdempotent,
			"load_balance":         route.LoadBalance,
			"hash_header":          route.HashHeader,
			"priority":             route.Priority,
			"enabled":              route.Enabled,
			"description":          route.Description,
			"updated_by":           route.UpdatedBy,
			"version":              gorm.Expr("version + 1"),
		})
	if result.Error != nil {
		return fmt.Errorf("update route: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return errors.New("route not found or version conflict")
	}
	return nil
}

// Delete performs a soft delete on the route with the given ID.
func (r *RouteRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&model.Route{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("delete route: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetByID retrieves a route by its ID.
func (r *RouteRepository) GetByID(ctx context.Context, id string) (*model.Route, error) {
	var route model.Route
	if err := r.db.WithContext(ctx).First(&route, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("get route by id: %w", err)
	}
	return &route, nil
}

// GetByPathMethod retrieves a route by exact path and method match.
func (r *RouteRepository) GetByPathMethod(ctx context.Context, path, method string) (*model.Route, error) {
	var route model.Route
	err := r.db.WithContext(ctx).
		Where("path = ? AND method = ?", path, method).
		First(&route).Error
	if err != nil {
		return nil, fmt.Errorf("get route by path method: %w", err)
	}
	return &route, nil
}

// ListFilter defines filter conditions for route listing.
type ListFilter struct {
	Page          int
	PageSize      int
	Path          string
	Method        string
	TargetService string
	Enabled       *bool
}

// List retrieves a paginated list of routes with optional filters.
// Returns the list, total count, and error.
func (r *RouteRepository) List(ctx context.Context, filter ListFilter) ([]*model.Route, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.Route{})

	if filter.Path != "" {
		query = query.Where("path LIKE ?", "%"+filter.Path+"%")
	}
	if filter.Method != "" {
		query = query.Where("method = ?", filter.Method)
	}
	if filter.TargetService != "" {
		query = query.Where("target_service = ?", filter.TargetService)
	}
	if filter.Enabled != nil {
		query = query.Where("enabled = ?", *filter.Enabled)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count routes: %w", err)
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var routes []*model.Route
	if err := query.Order("priority ASC, created_at DESC").
		Offset(offset).Limit(pageSize).Find(&routes).Error; err != nil {
		return nil, 0, fmt.Errorf("list routes: %w", err)
	}
	return routes, total, nil
}

// ListEnabled retrieves all enabled routes (used for config snapshot building).
func (r *RouteRepository) ListEnabled(ctx context.Context) ([]*model.Route, error) {
	var routes []*model.Route
	if err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("priority ASC").
		Find(&routes).Error; err != nil {
		return nil, fmt.Errorf("list enabled routes: %w", err)
	}
	return routes, nil
}

// UpdateStatus toggles the enabled status of a route.
func (r *RouteRepository) UpdateStatus(ctx context.Context, id string, enabled bool) error {
	result := r.db.WithContext(ctx).
		Model(&model.Route{}).
		Where("id = ?", id).
		Update("enabled", enabled)
	if result.Error != nil {
		return fmt.Errorf("update route status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
