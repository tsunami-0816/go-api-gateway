// Package handler provides Gin HTTP handlers for the admin-plane management API.
//
// Handlers follow the pattern: parameter binding/validation -> service call -> response.
// All handlers use the unified response format and swaggo annotations for OpenAPI generation.
package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-api-gateway/gateway/internal/configsnap"
	"github.com/go-api-gateway/gateway/internal/model"
	"github.com/go-api-gateway/gateway/internal/pkg/errcode"
	"github.com/go-api-gateway/gateway/internal/pkg/logger"
	"github.com/go-api-gateway/gateway/internal/pkg/response"
	"github.com/go-api-gateway/gateway/internal/pkg/utils"
	"github.com/go-api-gateway/gateway/internal/repository"
)

// RouteHandler handles route management API endpoints.
type RouteHandler struct {
	repo        *repository.RouteRepository
	snapManager *configsnap.Manager
}

// NewRouteHandler creates a new RouteHandler.
func NewRouteHandler(repo *repository.RouteRepository, snapManager *configsnap.Manager) *RouteHandler {
	return &RouteHandler{repo: repo, snapManager: snapManager}
}

// asyncReload triggers a config snapshot reload in a background goroutine (BUG-04).
func asyncReload(snapManager *configsnap.Manager, resource string) {
	if snapManager == nil {
		return
	}
	go func() {
		if _, err := snapManager.Reload(context.Background()); err != nil {
			logger.Error("auto reload after "+resource+" failed", map[string]interface{}{"error": err.Error()})
		} else {
			logger.Info("auto reload after "+resource+" succeeded", nil)
		}
	}()
}

// CreateRouteRequest is the request body for creating a route.
type CreateRouteRequest struct {
	RouteName          string `json:"route_name" binding:"required"`
	Path               string `json:"path" binding:"required"`
	Method             string `json:"method" binding:"required"`
	TargetService      string `json:"target_service" binding:"required"`
	RewritePath        string `json:"rewrite_path"`
	TimeoutMs          int    `json:"timeout_ms"`
	RetryCount         int    `json:"retry_count"`
	RetryOnStatus      string `json:"retry_on_status"`
	RetryNonIdempotent bool   `json:"retry_non_idempotent"`
	LoadBalance        string `json:"load_balance"`
	HashHeader         string `json:"hash_header"`
	Priority           int    `json:"priority"`
	Enabled            bool   `json:"enabled"`
	Description        string `json:"description"`
}

// CreateRoute godoc
// @Summary 创建路由
// @Description 创建新的网关路由配置
// @Tags routes
// @Accept json
// @Produce json
// @Param request body CreateRouteRequest true "路由配置"
// @Success 200 {object} response.Response{data=model.Route}
// @Failure 409 {object} response.Response "路径+方法已存在"
// @Router /api/v1/routes [post]
// @Security BearerAuth
func (h *RouteHandler) CreateRoute(c *gin.Context) {
	var req CreateRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, errcode.ValidationFailed, err.Error())
		return
	}

	operator := c.GetString("operator")
	route := &model.Route{
		BaseModel: model.BaseModel{
			ID:        utils.NewUUID(),
			CreatedBy: operator,
			UpdatedBy: operator,
		},
		RouteName:          req.RouteName,
		Path:               req.Path,
		Method:             req.Method,
		TargetService:      req.TargetService,
		RewritePath:        req.RewritePath,
		TimeoutMs:          req.TimeoutMs,
		RetryCount:         req.RetryCount,
		RetryOnStatus:      req.RetryOnStatus,
		RetryNonIdempotent: req.RetryNonIdempotent,
		LoadBalance:        req.LoadBalance,
		HashHeader:         req.HashHeader,
		Priority:           req.Priority,
		Enabled:            req.Enabled,
		Description:        req.Description,
	}
	if route.TimeoutMs == 0 {
		route.TimeoutMs = 5000
	}
	if route.LoadBalance == "" {
		route.LoadBalance = "weighted_round_robin"
	}
	if route.Priority == 0 {
		route.Priority = 100
	}

	if err := h.repo.Create(c.Request.Context(), route); err != nil {
		response.Fail(c, http.StatusConflict, errcode.RoutePathMethodExists, "路由路径与方法组合已存在")
		return
	}
	asyncReload(h.snapManager, "route create")
	response.Success(c, route)
}

// ListRoutes godoc
// @Summary 路由列表
// @Description 分页查询路由列表，支持路径、方法、目标服务筛选
// @Tags routes
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页条数" default(20)
// @Param path query string false "路径模糊匹配"
// @Param method query string false "HTTP方法"
// @Param target_service query string false "目标服务"
// @Success 200 {object} response.Response
// @Router /api/v1/routes [get]
// @Security BearerAuth
func (h *RouteHandler) ListRoutes(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	filter := repository.ListFilter{
		Page:          page,
		PageSize:      pageSize,
		Path:          c.Query("path"),
		Method:        c.Query("method"),
		TargetService: c.Query("target_service"),
	}
	routes, total, err := h.repo.List(c.Request.Context(), filter)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, errcode.DBError, "查询路由失败")
		return
	}
	response.PageSuccess(c, routes, page, pageSize, total)
}

// GetRoute godoc
// @Summary 路由详情
// @Description 根据ID获取路由详情
// @Tags routes
// @Produce json
// @Param id path string true "路由ID"
// @Success 200 {object} response.Response{data=model.Route}
// @Router /api/v1/routes/{id} [get]
// @Security BearerAuth
func (h *RouteHandler) GetRoute(c *gin.Context) {
	id := c.Param("id")
	route, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, http.StatusNotFound, errcode.RouteNotFound, "路由不存在")
		return
	}
	response.Success(c, route)
}

// UpdateRoute godoc
// @Summary 更新路由
// @Description 更新路由配置（乐观锁）
// @Tags routes
// @Accept json
// @Produce json
// @Param id path string true "路由ID"
// @Param request body CreateRouteRequest true "路由配置"
// @Success 200 {object} response.Response{data=model.Route}
// @Router /api/v1/routes/{id} [put]
// @Security BearerAuth
func (h *RouteHandler) UpdateRoute(c *gin.Context) {
	id := c.Param("id")
	var req CreateRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, errcode.ValidationFailed, err.Error())
		return
	}

	// BUG-06: read existing record first to preserve immutable audit fields
	existing, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, http.StatusNotFound, errcode.RouteNotFound, "路由不存在")
		return
	}

	operator := c.GetString("operator")
	// Preserve created_at, created_by from existing record; only update mutable fields
	route := &model.Route{
		BaseModel: model.BaseModel{
			ID:        id,
			CreatedAt: existing.CreatedAt,
			CreatedBy: existing.CreatedBy,
			UpdatedBy: operator,
			Version:   existing.Version,
		},
		RouteName:          req.RouteName,
		Path:               req.Path,
		Method:             req.Method,
		TargetService:      req.TargetService,
		RewritePath:        req.RewritePath,
		TimeoutMs:          req.TimeoutMs,
		RetryCount:         req.RetryCount,
		RetryOnStatus:      req.RetryOnStatus,
		RetryNonIdempotent: req.RetryNonIdempotent,
		LoadBalance:        req.LoadBalance,
		HashHeader:         req.HashHeader,
		Priority:           req.Priority,
		Enabled:            req.Enabled,
		Description:        req.Description,
	}
	if err := h.repo.Update(c.Request.Context(), route); err != nil {
		response.Fail(c, http.StatusConflict, errcode.Conflict, "更新失败或版本冲突")
		return
	}
	// Re-read to get accurate updated_at from DB
	updated, err := h.repo.GetByID(c.Request.Context(), id)
	if err == nil {
		route = updated
	}
	asyncReload(h.snapManager, "route update")
	response.Success(c, route)
}

// DeleteRoute godoc
// @Summary 删除路由
// @Description 逻辑删除路由
// @Tags routes
// @Produce json
// @Param id path string true "路由ID"
// @Success 200 {object} response.Response
// @Router /api/v1/routes/{id} [delete]
// @Security BearerAuth
func (h *RouteHandler) DeleteRoute(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, http.StatusNotFound, errcode.RouteNotFound, "路由不存在")
		return
	}
	asyncReload(h.snapManager, "route delete")
	response.Success(c, gin.H{"id": id, "deleted": true})
}

// PatchRouteStatus godoc
// @Summary 启用/禁用路由
// @Description 切换路由启用状态
// @Tags routes
// @Accept json
// @Produce json
// @Param id path string true "路由ID"
// @Param request body object{enabled=bool} true "状态"
// @Success 200 {object} response.Response
// @Router /api/v1/routes/{id}/status [patch]
// @Security BearerAuth
func (h *RouteHandler) PatchRouteStatus(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, http.StatusBadRequest, errcode.ValidationFailed, err.Error())
		return
	}
	if err := h.repo.UpdateStatus(c.Request.Context(), id, body.Enabled); err != nil {
		response.Fail(c, http.StatusNotFound, errcode.RouteNotFound, "路由不存在")
		return
	}
	asyncReload(h.snapManager, "route status change")
	response.Success(c, gin.H{"id": id, "enabled": body.Enabled})
}
