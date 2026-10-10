package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-api-gateway/gateway/internal/circuitbreaker"
	"github.com/go-api-gateway/gateway/internal/configsnap"
	"github.com/go-api-gateway/gateway/internal/model"
	"github.com/go-api-gateway/gateway/internal/pkg/errcode"
	"github.com/go-api-gateway/gateway/internal/pkg/response"
	"github.com/go-api-gateway/gateway/internal/pkg/utils"
	"github.com/go-api-gateway/gateway/internal/registry"
	"github.com/go-api-gateway/gateway/internal/repository"
	"gorm.io/datatypes"
)

// RateLimitHandler handles rate limit rule management API.
type RateLimitHandler struct {
	repo        *repository.RateLimitRepository
	snapManager *configsnap.Manager
}

func NewRateLimitHandler(repo *repository.RateLimitRepository, snapManager *configsnap.Manager) *RateLimitHandler {
	return &RateLimitHandler{repo: repo, snapManager: snapManager}
}

type CreateRateLimitRequest struct {
	RuleName    string `json:"rule_name" binding:"required"`
	RouteID     string `json:"route_id"`
	PathPattern string `json:"path_pattern"`
	Method      string `json:"method"`
	Algorithm   string `json:"algorithm" binding:"required"`
	Rate        int    `json:"rate" binding:"required"`
	Capacity    int    `json:"capacity" binding:"required"`
	LimitBy     string `json:"limit_by"`
	HeaderKey   string `json:"header_key"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

func (h *RateLimitHandler) Create(c *gin.Context) {
	var req CreateRateLimitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, errcode.ValidationFailed, err.Error())
		return
	}
	if req.Algorithm != "token_bucket" && req.Algorithm != "leaky_bucket" {
		response.Fail(c, http.StatusBadRequest, errcode.RateLimitAlgorithmUnsupported, "不支持的限流算法")
		return
	}
	if req.Rate <= 0 {
		response.Fail(c, http.StatusBadRequest, errcode.RateLimitRateMustBePositive, "限流速率必须大于0")
		return
	}
	operator := c.GetString("operator")
	rl := &model.RateLimit{
		BaseModel:   model.BaseModel{ID: utils.NewUUID(), CreatedBy: operator, UpdatedBy: operator},
		RuleName:    req.RuleName,
		RouteID:     req.RouteID,
		PathPattern: req.PathPattern,
		Method:      req.Method,
		Algorithm:   req.Algorithm,
		Rate:        req.Rate,
		Capacity:    req.Capacity,
		LimitBy:     req.LimitBy,
		HeaderKey:   req.HeaderKey,
		Enabled:     req.Enabled,
		Description: req.Description,
	}
	if rl.Method == "" {
		rl.Method = "*"
	}
	if rl.LimitBy == "" {
		rl.LimitBy = "route"
	}
	if err := h.repo.Create(c.Request.Context(), rl); err != nil {
		response.Fail(c, http.StatusInternalServerError, errcode.DBError, "创建限流规则失败")
		return
	}
	asyncReload(h.snapManager, "rate-limit create")
	response.Success(c, rl)
}

func (h *RateLimitHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := h.repo.List(c.Request.Context(), page, pageSize, c.Query("route_id"), nil)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, errcode.DBError, "查询失败")
		return
	}
	response.PageSuccess(c, list, page, pageSize, total)
}

func (h *RateLimitHandler) Get(c *gin.Context) {
	rl, err := h.repo.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Fail(c, http.StatusNotFound, 40400, "限流规则不存在")
		return
	}
	response.Success(c, rl)
}

func (h *RateLimitHandler) Update(c *gin.Context) {
	var req CreateRateLimitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, errcode.ValidationFailed, err.Error())
		return
	}
	operator := c.GetString("operator")
	// BUG-07: include route binding fields (RouteID, PathPattern, Method, HeaderKey)
	rl := &model.RateLimit{
		BaseModel:   model.BaseModel{ID: c.Param("id"), UpdatedBy: operator},
		RuleName:    req.RuleName,
		RouteID:     req.RouteID,
		PathPattern: req.PathPattern,
		Method:      req.Method,
		Algorithm:   req.Algorithm,
		Rate:        req.Rate,
		Capacity:    req.Capacity,
		LimitBy:     req.LimitBy,
		HeaderKey:   req.HeaderKey,
		Enabled:     req.Enabled,
		Description: req.Description,
	}
	if rl.Method == "" {
		rl.Method = "*"
	}
	if err := h.repo.Update(c.Request.Context(), rl); err != nil {
		response.Fail(c, http.StatusConflict, errcode.Conflict, "更新失败")
		return
	}
	asyncReload(h.snapManager, "rate-limit update")
	response.Success(c, rl)
}

func (h *RateLimitHandler) Delete(c *gin.Context) {
	if err := h.repo.Delete(c.Request.Context(), c.Param("id")); err != nil {
		response.Fail(c, http.StatusNotFound, 40400, "限流规则不存在")
		return
	}
	asyncReload(h.snapManager, "rate-limit delete")
	response.Success(c, gin.H{"deleted": true})
}

// CircuitBreakerHandler handles circuit breaker policy management API.
type CircuitBreakerHandler struct {
	repo        *repository.CircuitBreakerRepository
	snapManager *configsnap.Manager
}

func NewCircuitBreakerHandler(repo *repository.CircuitBreakerRepository, snapManager *configsnap.Manager) *CircuitBreakerHandler {
	return &CircuitBreakerHandler{repo: repo, snapManager: snapManager}
}

type CreateCircuitBreakerRequest struct {
	PolicyName            string  `json:"policy_name" binding:"required"`
	RouteID               string  `json:"route_id"`
	TargetService         string  `json:"target_service"`
	SlidingWindowSizeMs   int     `json:"sliding_window_size_ms"`
	MinRequestCount       int     `json:"min_request_count"`
	ErrorRateThreshold    float64 `json:"error_rate_threshold"`
	SlowCallDurationMs    int     `json:"slow_call_duration_ms"`
	SlowCallRateThreshold float64 `json:"slow_call_rate_threshold"`
	OpenDurationMs        int     `json:"open_duration_ms"`
	HalfOpenMaxRequests   int     `json:"half_open_max_requests"`
	Enabled               bool    `json:"enabled"`
	Description           string  `json:"description"`
}

func (h *CircuitBreakerHandler) Create(c *gin.Context) {
	var req CreateCircuitBreakerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, errcode.ValidationFailed, err.Error())
		return
	}
	if req.ErrorRateThreshold < 1 || req.ErrorRateThreshold > 100 {
		response.Fail(c, http.StatusBadRequest, errcode.CircuitBreakerErrorRateInvalid, "错误率阈值必须在1-100之间")
		return
	}
	operator := c.GetString("operator")
	cb := &model.CircuitBreaker{
		BaseModel:             model.BaseModel{ID: utils.NewUUID(), CreatedBy: operator, UpdatedBy: operator},
		PolicyName:            req.PolicyName,
		RouteID:               req.RouteID,
		TargetService:         req.TargetService,
		SlidingWindowSizeMs:   req.SlidingWindowSizeMs,
		MinRequestCount:       req.MinRequestCount,
		ErrorRateThreshold:    req.ErrorRateThreshold,
		SlowCallDurationMs:    req.SlowCallDurationMs,
		SlowCallRateThreshold: req.SlowCallRateThreshold,
		OpenDurationMs:        req.OpenDurationMs,
		HalfOpenMaxRequests:   req.HalfOpenMaxRequests,
		Enabled:               req.Enabled,
		Description:           req.Description,
	}
	defaults := circuitbreaker.DefaultConfig()
	if cb.SlidingWindowSizeMs == 0 {
		cb.SlidingWindowSizeMs = defaults.SlidingWindowSizeMs
	}
	if cb.MinRequestCount == 0 {
		cb.MinRequestCount = defaults.MinRequestCount
	}
	if cb.OpenDurationMs == 0 {
		cb.OpenDurationMs = defaults.OpenDurationMs
	}
	if cb.HalfOpenMaxRequests == 0 {
		cb.HalfOpenMaxRequests = defaults.HalfOpenMaxRequests
	}
	if err := h.repo.Create(c.Request.Context(), cb); err != nil {
		response.Fail(c, http.StatusInternalServerError, errcode.DBError, "创建熔断策略失败")
		return
	}
	asyncReload(h.snapManager, "circuit-breaker create")
	response.Success(c, cb)
}

func (h *CircuitBreakerHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := h.repo.List(c.Request.Context(), page, pageSize, c.Query("route_id"), c.Query("target_service"))
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, errcode.DBError, "查询失败")
		return
	}
	response.PageSuccess(c, list, page, pageSize, total)
}

func (h *CircuitBreakerHandler) Get(c *gin.Context) {
	cb, err := h.repo.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Fail(c, http.StatusNotFound, 40400, "熔断策略不存在")
		return
	}
	response.Success(c, cb)
}

func (h *CircuitBreakerHandler) Delete(c *gin.Context) {
	if err := h.repo.Delete(c.Request.Context(), c.Param("id")); err != nil {
		response.Fail(c, http.StatusNotFound, 40400, "熔断策略不存在")
		return
	}
	asyncReload(h.snapManager, "circuit-breaker delete")
	response.Success(c, gin.H{"deleted": true})
}

// GrayReleaseHandler handles gray release rule management API.
type GrayReleaseHandler struct {
	repo        *repository.GrayReleaseRepository
	snapManager *configsnap.Manager
}

func NewGrayReleaseHandler(repo *repository.GrayReleaseRepository, snapManager *configsnap.Manager) *GrayReleaseHandler {
	return &GrayReleaseHandler{repo: repo, snapManager: snapManager}
}

type CreateGrayReleaseRequest struct {
	RuleName      string          `json:"rule_name" binding:"required"`
	RouteID       string          `json:"route_id" binding:"required"`
	GrayVersion   string          `json:"gray_version" binding:"required"`
	StableVersion string          `json:"stable_version"`
	GrayPercent   int             `json:"gray_percent"`
	HashKeyType   string          `json:"hash_key_type"`
	HashKeyName   string          `json:"hash_key_name"`
	StainRules    json.RawMessage `json:"stain_rules"`
	Enabled       bool            `json:"enabled"`
	Description   string          `json:"description"`
}

func (h *GrayReleaseHandler) Create(c *gin.Context) {
	var req CreateGrayReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, errcode.ValidationFailed, err.Error())
		return
	}
	if req.GrayPercent < 0 || req.GrayPercent > 100 {
		response.Fail(c, http.StatusBadRequest, errcode.GrayReleasePercentInvalid, "灰度比例必须在0-100之间")
		return
	}
	operator := c.GetString("operator")
	gray := &model.GrayRelease{
		BaseModel:     model.BaseModel{ID: utils.NewUUID(), CreatedBy: operator, UpdatedBy: operator},
		RuleName:      req.RuleName,
		RouteID:       req.RouteID,
		GrayVersion:   req.GrayVersion,
		StableVersion: req.StableVersion,
		GrayPercent:   req.GrayPercent,
		HashKeyType:   req.HashKeyType,
		HashKeyName:   req.HashKeyName,
		StainRules:    datatypes.JSON(req.StainRules),
		Enabled:       req.Enabled,
		Description:   req.Description,
	}
	if gray.StableVersion == "" {
		gray.StableVersion = "stable"
	}
	if gray.HashKeyType == "" {
		gray.HashKeyType = "ip"
	}
	if err := h.repo.Create(c.Request.Context(), gray); err != nil {
		response.Fail(c, http.StatusInternalServerError, errcode.DBError, "创建灰度规则失败")
		return
	}
	asyncReload(h.snapManager, "gray-release create")
	response.Success(c, gray)
}

func (h *GrayReleaseHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := h.repo.List(c.Request.Context(), page, pageSize, c.Query("route_id"))
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, errcode.DBError, "查询失败")
		return
	}
	response.PageSuccess(c, list, page, pageSize, total)
}

func (h *GrayReleaseHandler) Get(c *gin.Context) {
	g, err := h.repo.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.Fail(c, http.StatusNotFound, 40400, "灰度规则不存在")
		return
	}
	response.Success(c, g)
}

func (h *GrayReleaseHandler) Delete(c *gin.Context) {
	if err := h.repo.Delete(c.Request.Context(), c.Param("id")); err != nil {
		response.Fail(c, http.StatusNotFound, 40400, "灰度规则不存在")
		return
	}
	asyncReload(h.snapManager, "gray-release delete")
	response.Success(c, gin.H{"deleted": true})
}

// PatchPercent updates gray percentage at runtime.
func (h *GrayReleaseHandler) PatchPercent(c *gin.Context) {
	var body struct {
		GrayPercent int `json:"gray_percent" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, http.StatusBadRequest, errcode.ValidationFailed, err.Error())
		return
	}
	if body.GrayPercent < 0 || body.GrayPercent > 100 {
		response.Fail(c, http.StatusBadRequest, errcode.GrayReleasePercentInvalid, "灰度比例必须在0-100之间")
		return
	}
	if err := h.repo.UpdatePercent(c.Request.Context(), c.Param("id"), body.GrayPercent); err != nil {
		response.Fail(c, http.StatusNotFound, 40400, "灰度规则不存在")
		return
	}
	asyncReload(h.snapManager, "gray-release percent change")
	response.Success(c, gin.H{"id": c.Param("id"), "gray_percent": body.GrayPercent})
}

// ServiceHandler handles service management API.
type ServiceHandler struct {
	repo        *repository.ServiceRepository
	snapManager *configsnap.Manager
	discovery   *registry.Discovery // optional, for test instance registration (BUG-05)
}

func NewServiceHandler(repo *repository.ServiceRepository, snapManager *configsnap.Manager) *ServiceHandler {
	return &ServiceHandler{repo: repo, snapManager: snapManager}
}

// SetDiscovery injects the discovery client for test instance registration.
func (h *ServiceHandler) SetDiscovery(d *registry.Discovery) {
	h.discovery = d
}

type CreateServiceRequest struct {
	ServiceName         string `json:"service_name" binding:"required"`
	DisplayName         string `json:"display_name"`
	HealthCheckPath     string `json:"health_check_path"`
	HealthCheckInterval int    `json:"health_check_interval"`
	HealthCheckTimeout  int    `json:"health_check_timeout"`
	DefaultWeight       int    `json:"default_weight"`
	Description         string `json:"description"`
}

func (h *ServiceHandler) Create(c *gin.Context) {
	var req CreateServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, errcode.ValidationFailed, err.Error())
		return
	}
	operator := c.GetString("operator")
	svc := &model.Service{
		BaseModel:           model.BaseModel{ID: utils.NewUUID(), CreatedBy: operator, UpdatedBy: operator},
		ServiceName:         req.ServiceName,
		DisplayName:         req.DisplayName,
		HealthCheckPath:     req.HealthCheckPath,
		HealthCheckInterval: req.HealthCheckInterval,
		HealthCheckTimeout:  req.HealthCheckTimeout,
		DefaultWeight:       req.DefaultWeight,
		Description:         req.Description,
	}
	if svc.HealthCheckPath == "" {
		svc.HealthCheckPath = "/health"
	}
	if svc.HealthCheckInterval == 0 {
		svc.HealthCheckInterval = 10
	}
	if svc.HealthCheckTimeout == 0 {
		svc.HealthCheckTimeout = 3
	}
	if svc.DefaultWeight == 0 {
		svc.DefaultWeight = 1
	}
	if err := h.repo.Create(c.Request.Context(), svc); err != nil {
		response.Fail(c, http.StatusConflict, errcode.ServiceNameExists, "服务名已存在")
		return
	}
	asyncReload(h.snapManager, "service create")
	response.Success(c, svc)
}

func (h *ServiceHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := h.repo.List(c.Request.Context(), page, pageSize)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, errcode.DBError, "查询失败")
		return
	}
	response.PageSuccess(c, list, page, pageSize, total)
}

func (h *ServiceHandler) Get(c *gin.Context) {
	svc, err := h.repo.GetByName(c.Request.Context(), c.Param("name"))
	if err != nil {
		response.Fail(c, http.StatusNotFound, 40400, "服务不存在")
		return
	}
	response.Success(c, svc)
}

func (h *ServiceHandler) Delete(c *gin.Context) {
	if err := h.repo.Delete(c.Request.Context(), c.Param("name")); err != nil {
		response.Fail(c, http.StatusNotFound, 40400, "服务不存在")
		return
	}
	asyncReload(h.snapManager, "service delete")
	response.Success(c, gin.H{"deleted": true})
}

// RegisterTestInstance registers a test backend instance with etcd for local debugging (BUG-05).
// Production services should self-register; this API is for gateway admin testing only.
func (h *ServiceHandler) RegisterTestInstance(c *gin.Context) {
	if h.discovery == nil {
		response.Fail(c, http.StatusServiceUnavailable, errcode.DBError, "etcd 未连接，无法注册测试实例")
		return
	}
	var req struct {
		ServiceName string `json:"service_name" binding:"required"`
		Address     string `json:"address" binding:"required"`
		Weight      int    `json:"weight"`
		Version     string `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, errcode.ValidationFailed, err.Error())
		return
	}
	if req.Weight == 0 {
		req.Weight = 1
	}
	if req.Version == "" {
		req.Version = "stable"
	}
	inst := &registry.Instance{
		ID:      utils.NewUUID(),
		Address: req.Address,
		Weight:  req.Weight,
		Version: req.Version,
		Healthy: true,
	}
	if err := h.discovery.RegisterTestInstance(c.Request.Context(), req.ServiceName, inst); err != nil {
		response.Fail(c, http.StatusInternalServerError, errcode.DBError, "注册测试实例失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"service_name": req.ServiceName, "instance": inst})
}

// DeregisterTestInstance removes a test instance from etcd (BUG-05).
func (h *ServiceHandler) DeregisterTestInstance(c *gin.Context) {
	if h.discovery == nil {
		response.Fail(c, http.StatusServiceUnavailable, errcode.DBError, "etcd 未连接")
		return
	}
	serviceName := c.Param("name")
	instanceID := c.Param("instance_id")
	if err := h.discovery.DeregisterTestInstance(c.Request.Context(), serviceName, instanceID); err != nil {
		response.Fail(c, http.StatusInternalServerError, errcode.DBError, "注销测试实例失败: "+err.Error())
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// SystemHandler handles system operations (health, reload, config).
type SystemHandler struct {
	snapManager *configsnap.Manager
}

func NewSystemHandler(snap *configsnap.Manager) *SystemHandler {
	return &SystemHandler{snapManager: snap}
}

// Healthz returns liveness probe (always 200).
func (h *SystemHandler) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Readyz returns readiness probe.
func (h *SystemHandler) Readyz(c *gin.Context) {
	snap := h.snapManager.Get()
	if snap.Version == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "reason": "config not loaded"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready", "config_version": snap.Version})
}

// Reload triggers a manual config reload.
func (h *SystemHandler) Reload(c *gin.Context) {
	version, err := h.snapManager.Reload(c.Request.Context())
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, errcode.ConfigReloadFailed, "配置热更新失败")
		return
	}
	response.Success(c, gin.H{"config_version": version, "reloaded": true})
}

// GetConfig returns current config snapshot version and summary.
func (h *SystemHandler) GetConfig(c *gin.Context) {
	snap := h.snapManager.Get()
	response.Success(c, gin.H{
		"version":                snap.Version,
		"routes_count":           len(snap.Routes),
		"rate_limits_count":      len(snap.RateLimits),
		"circuit_breakers_count": len(snap.CircuitBreakers),
		"gray_releases_count":    len(snap.GrayReleases),
		"services_count":         len(snap.Services),
	})
}
