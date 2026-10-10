// Package app assembles all gateway components and manages the startup/shutdown lifecycle.
//
// Startup sequence (9 steps, architecture 6.3.5):
//  1. Load configuration
//  2. Initialize logger
//  3. Connect config store (SQLite/MySQL) + AutoMigrate
//  4. Load config snapshot
//  5. Connect etcd + full fetch + watch
//  6. Connect Redis
//  7. Start health check
//  8. Start admin-plane HTTP server (8081)
//  9. Start data-plane HTTP server (8080) — last
package app

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-api-gateway/gateway/internal/circuitbreaker"
	"github.com/go-api-gateway/gateway/internal/config"
	"github.com/go-api-gateway/gateway/internal/configsnap"
	"github.com/go-api-gateway/gateway/internal/gray"
	"github.com/go-api-gateway/gateway/internal/handler"
	"github.com/go-api-gateway/gateway/internal/middleware"
	"github.com/go-api-gateway/gateway/internal/observability"
	"github.com/go-api-gateway/gateway/internal/pkg/logger"
	"github.com/go-api-gateway/gateway/internal/proxy"
	"github.com/go-api-gateway/gateway/internal/ratelimit"
	"github.com/go-api-gateway/gateway/internal/registry"
	"github.com/go-api-gateway/gateway/internal/repository"
	"github.com/go-api-gateway/gateway/internal/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/gorm"
)

// App holds all gateway dependencies and manages their lifecycle.
type App struct {
	cfg               *config.Config
	version           string
	db                *gorm.DB
	redisClient       *store.RedisClient
	discovery         *registry.Discovery
	snapManager       *configsnap.Manager
	router            *proxy.Router
	reverseProxy      *proxy.ReverseProxy
	rateLimiterMgr    *ratelimit.RateLimiterManager
	cbManager         *circuitbreaker.CircuitBreakerManager
	grayRouter        *gray.Router
	metrics           *observability.Metrics
	dataServer        *http.Server
	adminServer       *http.Server
	startTime         time.Time
	activeConns       int64 // atomic counter for active connections
	stopMetricsCh     chan struct{}
	errorTracker      *errorRateTracker // 修复Bug-P1-006 滑动窗口错误率统计
}

// New initializes the application following steps 1-8.
// The data-plane server (step 9) is started separately by Start().
func New(ctx context.Context, cfg *config.Config, version string) (*App, error) {
	app := &App{
		cfg:       cfg,
		version:   version,
		startTime: time.Now(),
	}

	// ② Initialize logger
	logger.Init(cfg.Log.Level, cfg.Log.Format, cfg.Log.Output, cfg.Log.Async)
	logger.Info("gateway starting", map[string]interface{}{"version": version})

	// ③ Connect config store + AutoMigrate
	db, err := store.NewDB(cfg.ConfigStore)
	if err != nil {
		return nil, fmt.Errorf("step3 connect db: %w", err)
	}
	app.db = db
	logger.Info("config store connected", map[string]interface{}{"driver": cfg.ConfigStore.Driver})

	// Initialize repositories
	routeRepo := repository.NewRouteRepository(db)
	rlRepo := repository.NewRateLimitRepository(db)
	cbRepo := repository.NewCircuitBreakerRepository(db)
	grayRepo := repository.NewGrayReleaseRepository(db)
	svcRepo := repository.NewServiceRepository(db)

	// ④ Load config snapshot
	app.snapManager = configsnap.NewManager(routeRepo, rlRepo, cbRepo, grayRepo, svcRepo)

	// Initialize core engines (before first reload so callbacks can populate them)
	app.router = proxy.NewRouter()
	app.rateLimiterMgr = ratelimit.NewRateLimiterManager()
	app.cbManager = circuitbreaker.NewCircuitBreakerManager()
	app.grayRouter = gray.NewRouter()
	app.metrics = observability.NewMetrics()
	app.errorTracker = newErrorRateTracker(10 * time.Second) // 修复Bug-P1-006 10秒滑动窗口

	// Register hot-reload callbacks: keep data-plane components in sync
	app.snapManager.OnReload(func(snap *configsnap.Snapshot) {
		app.router.Update(snap.Routes)
		app.rateLimiterMgr.Sync(snap.RateLimits)
		app.cbManager.Sync(snap.CircuitBreakers)
		app.metrics.ConfigReloadTotal.WithLabelValues("success").Inc()
	})

	snapVersion, err := app.snapManager.Reload(ctx)
	if err != nil {
		return nil, fmt.Errorf("step4 load snapshot: %w", err)
	}
	logger.Info("config snapshot loaded", map[string]interface{}{"version": snapVersion})

	// ⑤ Connect etcd + full fetch + watch
	discovery, err := registry.NewDiscovery(cfg.Etcd)
	if err != nil {
		logger.Warn("etcd connection failed, running with empty service registry", map[string]interface{}{"error": err.Error()})
	} else {
		app.discovery = discovery
		logger.Info("etcd connected and watching")
	}

	// ⑥ Connect Redis
	redisClient, err := store.NewRedisClient(cfg.Redis)
	if err != nil {
		logger.Warn("redis connection failed, rate limiter will run in local-only mode", map[string]interface{}{"error": err.Error()})
	} else {
		app.redisClient = redisClient
		logger.Info("redis connected")
		// 修复Bug-P1-007 将Redis客户端注入限流管理器，启动分布式令牌预取
		app.rateLimiterMgr.SetRedisClient(redisClient, cfg.RateLimit)
	}

	// Initialize load balancer and reverse proxy
	// 修复Bug-P1-008 即使discovery为nil也创建reverseProxy，支持静态后端降级
	staticBackends := buildStaticBackends(cfg.StaticBackends)
	app.reverseProxy = proxy.NewReverseProxy(app.discovery, staticBackends)

	// Start system metrics collector (BUG-08)
	app.startMetricsCollector()

	// ⑧ Start admin-plane HTTP server
	if err := app.startAdminServer(); err != nil {
		return nil, fmt.Errorf("step8 start admin server: %w", err)
	}
	logger.Info("admin plane started", map[string]interface{}{"port": cfg.Server.Admin.Port})

	return app, nil
}

// Start starts the data-plane HTTP server (step 9, last).
func (a *App) Start(ctx context.Context) error {
	if err := a.startDataServer(); err != nil {
		return fmt.Errorf("step9 start data server: %w", err)
	}
	logger.Info("data plane started", map[string]interface{}{"port": a.cfg.Server.Data.Port})
	logger.Info("gateway startup complete", map[string]interface{}{
		"startup_duration_ms": time.Since(a.startTime).Milliseconds(),
	})
	return nil
}

// startAdminServer creates and starts the admin-plane Gin engine on port 8081.
func (a *App) startAdminServer() error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.RecoveryMiddleware())
	r.Use(middleware.CORSMiddleware())

	// Health endpoints (no auth required)
	systemHandler := handler.NewSystemHandler(a.snapManager)
	r.GET("/healthz", systemHandler.Healthz)
	r.GET("/readyz", systemHandler.Readyz)

	// Prometheus metrics
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Admin API group with auth
	api := r.Group("/api/v1")
	tokenMap := buildTokenMap(a.cfg.Admin.Auth.Tokens)
	api.Use(middleware.AuthMiddleware(tokenMap, a.cfg.Admin.Auth.DefaultUsername))
	api.Use(middleware.TraceMiddleware())

	// Route management
	routeHandler := handler.NewRouteHandler(repository.NewRouteRepository(a.db), a.snapManager)
	api.POST("/routes", routeHandler.CreateRoute)
	api.GET("/routes", routeHandler.ListRoutes)
	api.GET("/routes/:id", routeHandler.GetRoute)
	api.PUT("/routes/:id", routeHandler.UpdateRoute)
	api.DELETE("/routes/:id", routeHandler.DeleteRoute)
	api.PATCH("/routes/:id/status", routeHandler.PatchRouteStatus)

	// Rate limit management
	rlHandler := handler.NewRateLimitHandler(repository.NewRateLimitRepository(a.db), a.snapManager)
	api.POST("/rate-limits", rlHandler.Create)
	api.GET("/rate-limits", rlHandler.List)
	api.GET("/rate-limits/:id", rlHandler.Get)
	api.PUT("/rate-limits/:id", rlHandler.Update)
	api.DELETE("/rate-limits/:id", rlHandler.Delete)

	// Circuit breaker management
	cbHandler := handler.NewCircuitBreakerHandler(repository.NewCircuitBreakerRepository(a.db), a.snapManager)
	api.POST("/circuit-breakers", cbHandler.Create)
	api.GET("/circuit-breakers", cbHandler.List)
	api.GET("/circuit-breakers/:id", cbHandler.Get)
	api.DELETE("/circuit-breakers/:id", cbHandler.Delete)

	// Gray release management
	grayHandler := handler.NewGrayReleaseHandler(repository.NewGrayReleaseRepository(a.db), a.snapManager)
	api.POST("/gray-releases", grayHandler.Create)
	api.GET("/gray-releases", grayHandler.List)
	api.GET("/gray-releases/:id", grayHandler.Get)
	api.DELETE("/gray-releases/:id", grayHandler.Delete)
	api.PATCH("/gray-releases/:id/percent", grayHandler.PatchPercent)

	// Service management
	svcHandler := handler.NewServiceHandler(repository.NewServiceRepository(a.db), a.snapManager)
	if a.discovery != nil {
		svcHandler.SetDiscovery(a.discovery)
	}
	api.POST("/services", svcHandler.Create)
	api.GET("/services", svcHandler.List)
	api.GET("/services/:name", svcHandler.Get)
	api.DELETE("/services/:name", svcHandler.Delete)
	api.POST("/services/:name/instances", svcHandler.RegisterTestInstance)
	api.DELETE("/services/:name/instances/:instance_id", svcHandler.DeregisterTestInstance)

	// System operations
	api.POST("/system/reload", systemHandler.Reload)
	api.GET("/system/config", systemHandler.GetConfig)

	addr := fmt.Sprintf("%s:%d", a.cfg.Server.Admin.Host, a.cfg.Server.Admin.Port)
	a.adminServer = &http.Server{
		Addr:    addr,
		Handler: r,
	}
	go func() {
		if err := a.adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("admin server error", map[string]interface{}{"error": err.Error()})
		}
	}()
	return nil
}

// startDataServer creates and starts the data-plane Gin engine on port 8080.
func (a *App) startDataServer() error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.RecoveryMiddleware())

	// Data-plane middleware chain: Trace -> RateLimit -> CircuitBreaker -> Gray -> Route -> Proxy
	r.Use(middleware.TraceMiddleware())

	// Track active connections for metrics (BUG-08)
	r.Use(func(c *gin.Context) {
		atomic.AddInt64(&a.activeConns, 1)
		defer atomic.AddInt64(&a.activeConns, -1)
		c.Next()
	})

	// Route matching + proxy handler
	r.NoRoute(func(c *gin.Context) {
		start := time.Now()

		// Match route
		route := a.router.Match(c.Request.URL.Path, c.Request.Method)
		if route == nil {
			a.recordMetrics(c, "", "", http.StatusNotFound, start)
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "route not found"})
			return
		}
		c.Set("route_id", route.ID)
		c.Set("matched_route", route)
		c.Set("target_service", route.TargetService)
		c.Set("client_ip", c.ClientIP())

		snap := a.snapManager.Get()

		// --- Rate limit check (BUG-02: use snapshot rules) ---
		rateRules := snap.GetRateLimitsByRouteID(route.ID)
		a.metrics.RateLimitTotal.WithLabelValues(route.ID).Inc()
		// 修复Bug-P1-001 传入clientIP和请求Header，按LimitBy维度分桶限流
		if rejected := a.rateLimiterMgr.Allow(rateRules, c.ClientIP(), c.Request.Header); rejected != nil {
			a.metrics.RateLimitRejected.WithLabelValues(route.ID).Inc()
			// BUG-10: dynamic rate limit headers from matched rule
			c.Header("X-RateLimit-Limit", strconv.Itoa(rejected.Rate))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("Retry-After", "1")
			a.recordMetrics(c, route.ID, route.TargetService, http.StatusTooManyRequests, start)
			c.JSON(http.StatusTooManyRequests, gin.H{"code": 429, "message": "rate limit exceeded"})
			return
		}
		a.metrics.RateLimitPassed.WithLabelValues(route.ID).Inc()

		// --- Circuit breaker check (BUG-03: use snapshot policy) ---
		cbPolicy := snap.GetCircuitBreakerByRouteID(route.ID)
		var cb *circuitbreaker.CircuitBreaker
		if cbPolicy != nil {
			cb = a.cbManager.Get(cbPolicy.ID)
		}
		if cb != nil && !cb.Allow() {
			a.metrics.CircuitBreakerOpenTotal.WithLabelValues(route.TargetService, route.ID).Inc()
			a.metrics.CircuitBreakerState.WithLabelValues(route.TargetService, route.ID).Set(float64(circuitbreaker.StateOpen))
			c.Header("X-Circuit-Breaker", "open")
			a.recordMetrics(c, route.ID, route.TargetService, http.StatusServiceUnavailable, start)
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "circuit breaker open"})
			return
		}
		if cb != nil {
			a.metrics.CircuitBreakerState.WithLabelValues(route.TargetService, route.ID).Set(float64(cb.State()))
		}

		// --- Gray version determination ---
		grayRule := snap.GetGrayRuleByRouteID(route.ID)
		version := "stable"
		if grayRule != nil {
			stainRules, _ := gray.ParseStainRules(grayRule.StainRules)
			rule := &gray.Rule{
				GrayVersion:   grayRule.GrayVersion,
				StableVersion: grayRule.StableVersion,
				GrayPercent:   grayRule.GrayPercent,
				HashKeyType:   grayRule.HashKeyType,
				StainRules:    stainRules,
			}
			version, _ = a.grayRouter.Calculate(rule, c.Request, c.ClientIP())
		}
		c.Set("gray_version", version)
		a.metrics.GrayRequestsTotal.WithLabelValues(route.ID, version).Inc()

		// Path rewriting
		rewriter := proxy.NewRewriter()
		c.Request.URL.Path = rewriter.Rewrite(route, c.Request.URL.Path)

		// 修复Bug-P0-001 将gin context中的值注入到req.Context()，
		// Gin的c.Set不会自动同步到标准库context，proxy.director从req.Context()读取
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, "route_id", route.ID)
		ctx = context.WithValue(ctx, "route", route)
		ctx = context.WithValue(ctx, "target_service", route.TargetService)
		ctx = context.WithValue(ctx, "gray_version", version)
		ctx = context.WithValue(ctx, "client_ip", c.ClientIP())
		ctx = context.WithValue(ctx, "trace_id", c.GetString("trace_id"))

		// 修复Bug-P1-002 设置路由级超时，通过context传递给ReverseProxy的RoundTrip
		timeout := time.Duration(route.TimeoutMs) * time.Millisecond
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		c.Request = c.Request.WithContext(ctx)

		// Reverse proxy (修复Bug-P1-008 reverseProxy总是创建，支持静态后端降级)
		a.reverseProxy.ServeHTTP(c.Writer, c.Request)

		status := c.Writer.Status()
		duration := time.Since(start)

		// Record circuit breaker result
		if cb != nil {
			cb.Record(status < 500, duration)
			a.metrics.CircuitBreakerState.WithLabelValues(route.TargetService, route.ID).Set(float64(cb.State()))
		}

		// BUG-09: record business metrics
		a.recordMetrics(c, route.ID, route.TargetService, status, start)
	})

	addr := fmt.Sprintf("%s:%d", a.cfg.Server.Data.Host, a.cfg.Server.Data.Port)
	a.dataServer = &http.Server{
		Addr:           addr,
		Handler:        r,
		ReadTimeout:    a.cfg.Server.Data.ReadTimeout,
		WriteTimeout:   a.cfg.Server.Data.WriteTimeout,
		IdleTimeout:    a.cfg.Server.Data.IdleTimeout,
		MaxHeaderBytes: a.cfg.Server.Data.MaxHeaderBytes,
	}
	go func() {
		if err := a.dataServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("data server error", map[string]interface{}{"error": err.Error()})
		}
	}()
	return nil
}

// recordMetrics records QPS, duration, and error rate for a request (BUG-09).
func (a *App) recordMetrics(c *gin.Context, routeID, service string, status int, start time.Time) {
	duration := time.Since(start).Seconds()
	method := c.Request.Method
	statusStr := strconv.Itoa(status)

	a.metrics.QPSTotal.WithLabelValues(routeID, service, method, statusStr).Inc()
	a.metrics.RequestDuration.WithLabelValues(routeID, service, method).Observe(duration)

	// 修复Bug-P1-006 原代码每次Set(100)或Set(0)，错误率由最后一个请求决定。
	// 改为滑动窗口统计：记录最近窗口内的请求，计算错误请求占比。
	key := routeID + "|" + service
	a.errorTracker.Record(key, status >= 500)
	rate := a.errorTracker.Rate(key)
	a.metrics.ErrorRate.WithLabelValues(routeID, service).Set(rate)
}

// startMetricsCollector launches a background goroutine that updates
// system-level gauges (goroutines, memory, active connections) every 5s (BUG-08).
func (a *App) startMetricsCollector() {
	a.stopMetricsCh = make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		var memStats runtime.MemStats
		for {
			select {
			case <-ticker.C:
				a.metrics.Goroutines.Set(float64(runtime.NumGoroutine()))
				runtime.ReadMemStats(&memStats)
				a.metrics.MemoryUsageBytes.Set(float64(memStats.Alloc))
				a.metrics.ActiveConnections.Set(float64(atomic.LoadInt64(&a.activeConns)))
			case <-a.stopMetricsCh:
				return
			}
		}
	}()
}

// Stop gracefully shuts down all components in reverse order.
func (a *App) Stop(ctx context.Context) error {
	logger.Info("shutting down gateway...")

	// Stop metrics collector
	if a.stopMetricsCh != nil {
		close(a.stopMetricsCh)
	}

	// Stop data plane first (reverse order of startup)
	if a.dataServer != nil {
		if err := a.dataServer.Shutdown(ctx); err != nil {
			logger.Error("data server shutdown error", map[string]interface{}{"error": err.Error()})
		}
	}
	// Stop admin plane
	if a.adminServer != nil {
		if err := a.adminServer.Shutdown(ctx); err != nil {
			logger.Error("admin server error", map[string]interface{}{"error": err.Error()})
		}
	}
	// Close dependencies
	if a.discovery != nil {
		a.discovery.Close()
	}
	if a.rateLimiterMgr != nil {
		a.rateLimiterMgr.Stop() // 修复Bug-P1-007 停止Redis预取后台goroutine
	}
	if a.redisClient != nil {
		a.redisClient.Close()
	}
	logger.Sync()
	return nil
}

// buildTokenMap converts config token entries to a map[token]username.
func buildTokenMap(tokens []config.TokenEntry) map[string]string {
	m := make(map[string]string, len(tokens))
	for _, t := range tokens {
		m[t.Token] = t.Username
	}
	return m
}

// buildStaticBackends converts config static backends to registry.Instance slices.
// 修复Bug-P1-008 将配置文件中的静态后端转换为registry.Instance，供discovery不可用时降级使用
func buildStaticBackends(cfg map[string][]config.StaticBackend) map[string][]*registry.Instance {
	if len(cfg) == 0 {
		return nil
	}
	result := make(map[string][]*registry.Instance, len(cfg))
	for svcName, backends := range cfg {
		instances := make([]*registry.Instance, 0, len(backends))
		for i, b := range backends {
			weight := b.Weight
			if weight <= 0 {
				weight = 1
			}
			instances = append(instances, &registry.Instance{
				ID:          fmt.Sprintf("static-%s-%d", svcName, i),
				ServiceName: svcName,
				Address:     b.Address,
				Weight:      weight,
				Version:     "stable",
				Healthy:     true,
			})
		}
		result[svcName] = instances
	}
	return result
}

// errorRateTracker implements a sliding-window error rate calculator.
// 修复Bug-P1-006 维护时间窗口内的请求记录，计算错误请求占比，而非由最后一个请求决定
type errorRateTracker struct {
	mu      sync.Mutex
	windows map[string]*errorWindow
	window  time.Duration
}

type errorWindow struct {
	events []errorEvent
}

type errorEvent struct {
	ts      time.Time
	isError bool
}

func newErrorRateTracker(window time.Duration) *errorRateTracker {
	return &errorRateTracker{
		windows: make(map[string]*errorWindow),
		window:  window,
	}
}

// Record adds a request result to the sliding window and evicts expired entries.
func (t *errorRateTracker) Record(key string, isError bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	w, ok := t.windows[key]
	if !ok {
		w = &errorWindow{}
		t.windows[key] = w
	}
	now := time.Now()
	w.events = append(w.events, errorEvent{ts: now, isError: isError})
	// Evict events outside the window
	cutoff := now.Add(-t.window)
	valid := w.events[:0]
	for _, e := range w.events {
		if e.ts.After(cutoff) {
			valid = append(valid, e)
		}
	}
	w.events = valid
}

// Rate returns the error rate percentage (0-100) for the given key.
func (t *errorRateTracker) Rate(key string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	w, ok := t.windows[key]
	if !ok || len(w.events) == 0 {
		return 0
	}
	errors := 0
	for _, e := range w.events {
		if e.isError {
			errors++
		}
	}
	return float64(errors) / float64(len(w.events)) * 100
}
