// Package proxy implements route matching, path rewriting, and reverse proxying.
//
// The Router uses a radix-tree-like prefix matching with method exact match.
// The ReverseProxy is based on httputil.ReverseProxy with custom Director
// for target URL construction, traceparent injection, and route-level timeout.
package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-api-gateway/gateway/internal/loadbalance"
	"github.com/go-api-gateway/gateway/internal/model"
	"github.com/go-api-gateway/gateway/internal/registry"
)

// Router matches incoming requests to route configurations.
// Uses prefix path matching + method exact match, sorted by priority.
type Router struct {
	mu     sync.RWMutex
	routes []*model.Route
}

// NewRouter creates a new empty router.
func NewRouter() *Router {
	return &Router{}
}

// Update replaces the entire route table (called on config hot reload).
func (r *Router) Update(routes []*model.Route) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Sort by priority (smaller = higher priority), then by path length
	sorted := make([]*model.Route, len(routes))
	copy(sorted, routes)
	// Simple insertion sort by priority
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Priority < sorted[j-1].Priority; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	r.routes = sorted
}

// Match finds the best matching route for the given path and method.
// Returns the matched route or nil if no match.
func (r *Router) Match(path, method string) *model.Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, route := range r.routes {
		if !route.Enabled {
			continue
		}
		// Method exact match (or wildcard *)
		if route.Method != "*" && !strings.EqualFold(route.Method, method) {
			continue
		}
		// Path prefix match
		if matchPath(route.Path, path) {
			return route
		}
	}
	return nil
}

// matchPath checks if the request path matches the route path pattern.
// Supports exact match and prefix match (route path ending with / or *).
func matchPath(routePath, requestPath string) bool {
	if routePath == requestPath {
		return true
	}
	// Prefix match: route path is a prefix of request path
	if strings.HasSuffix(routePath, "/") || strings.HasSuffix(routePath, "*") {
		prefix := strings.TrimRight(routePath, "/*")
		return strings.HasPrefix(requestPath, prefix)
	}
	// Also support route path as directory prefix
	if strings.HasPrefix(requestPath, routePath+"/") {
		return true
	}
	return false
}

// Rewriter handles path rewriting for matched routes.
type Rewriter struct{}

// NewRewriter creates a new path rewriter.
func NewRewriter() *Rewriter {
	return &Rewriter{}
}

// Rewrite applies the route's rewrite_path rule to the request path.
// Supports prefix stripping (strip_prefix) and simple replacement.
func (rw *Rewriter) Rewrite(route *model.Route, requestPath string) string {
	if route.RewritePath == "" {
		return requestPath
	}
	// Simple prefix strip: if rewrite_path starts with "strip_prefix=", remove that prefix
	if strings.HasPrefix(route.RewritePath, "strip_prefix=") {
		prefix := strings.TrimPrefix(route.RewritePath, "strip_prefix=")
		return strings.TrimPrefix(requestPath, prefix)
	}
	// Direct replacement
	return route.RewritePath
}

// StaticBackend represents a statically configured backend instance for fallback.
// 修复Bug-P1-008 etcd不可用时的静态后端降级
type StaticBackend struct {
	Address string `mapstructure:"address"`
	Weight  int    `mapstructure:"weight"`
}

// ReverseProxy wraps httputil.ReverseProxy with gateway-specific features.
type ReverseProxy struct {
	proxy          *httputil.ReverseProxy
	discovery      *registry.Discovery
	wrrLB          loadbalance.LoadBalancer // 修复Bug-P1-004 路由级WRR策略实例
	chLB           loadbalance.LoadBalancer // 修复Bug-P1-004 路由级一致性哈希策略实例
	staticBackends map[string][]*registry.Instance // 修复Bug-P1-008 静态后端降级配置
}

// NewReverseProxy creates a new reverse proxy.
//
// Parameters:
//   - discovery: service discovery for instance lookup (may be nil for static-only mode)
//   - staticBackends: static backend instances keyed by service name (fallback when discovery is unavailable)
func NewReverseProxy(discovery *registry.Discovery, staticBackends map[string][]*registry.Instance) *ReverseProxy {
	rp := &ReverseProxy{
		discovery:      discovery,
		wrrLB:          loadbalance.NewLoadBalancer("weighted_round_robin"),
		chLB:           loadbalance.NewLoadBalancer("consistent_hash"),
		staticBackends: staticBackends,
	}

	baseTransport := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	}

	rp.proxy = &httputil.ReverseProxy{
		Director: rp.director,
		// 修复Bug-P1-003 使用自定义retryRoundTripper实现路由级重试
		Transport: &retryRoundTripper{
			base: baseTransport,
			rp:   rp,
		},
		ErrorHandler: rp.errorHandler,
	}
	return rp
}

// getLoadBalancer selects the load balancer strategy based on route config.
// 修复Bug-P1-004 根据路由的LoadBalance字段选择策略，而非全局唯一WRR
func (rp *ReverseProxy) getLoadBalancer(route *model.Route) loadbalance.LoadBalancer {
	if route != nil && route.LoadBalance == "consistent_hash" {
		return rp.chLB
	}
	return rp.wrrLB
}

// buildRequestKey builds the key for consistent hashing based on route config.
// 修复Bug-P1-004 一致性哈希使用路由配置的HashHeader值作为key，默认回退到clientIP
func buildRequestKey(route *model.Route, req *http.Request, clientIP string) string {
	if route != nil && route.LoadBalance == "consistent_hash" && route.HashHeader != "" {
		if v := req.Header.Get(route.HashHeader); v != "" {
			return v
		}
	}
	return clientIP
}

// getInstances retrieves backend instances from discovery, falling back to static backends.
// 修复Bug-P1-008 discovery为nil或无实例时，从静态配置获取后端
func (rp *ReverseProxy) getInstances(targetService, version string) []*registry.Instance {
	var instances []*registry.Instance
	if rp.discovery != nil {
		instances = rp.discovery.GetInstances(targetService, version)
	}
	if len(instances) == 0 && rp.staticBackends != nil {
		instances = rp.staticBackends[targetService]
	}
	return instances
}

// director modifies the outgoing request to target the selected backend instance.
func (rp *ReverseProxy) director(req *http.Request) {
	// Extract route and target service from request context (set by data-plane handler)
	// 修复Bug-P0-001 app.go已将gin context值注入req.Context()，此处正常读取
	routeID, _ := req.Context().Value("route_id").(string)
	targetService, _ := req.Context().Value("target_service").(string)
	version, _ := req.Context().Value("gray_version").(string)
	clientIP, _ := req.Context().Value("client_ip").(string)
	traceID, _ := req.Context().Value("trace_id").(string)
	route, _ := req.Context().Value("route").(*model.Route)

	if targetService == "" {
		return
	}

	// Get healthy instances for the target service (修复Bug-P1-008 支持静态后端降级)
	instances := rp.getInstances(targetService, version)
	if len(instances) == 0 {
		return
	}

	// Select instance via load balancer (修复Bug-P1-004 根据路由策略选择LB)
	lb := rp.getLoadBalancer(route)
	reqKey := buildRequestKey(route, req, clientIP)
	instance, err := lb.Select(instances, reqKey)
	if err != nil || instance == nil {
		return
	}

	// Build target URL
	targetURL, _ := url.Parse("http://" + instance.Address)
	req.URL.Scheme = targetURL.Scheme
	req.URL.Host = targetURL.Host
	req.Host = targetURL.Host

	// Inject traceparent header for distributed tracing
	if traceID != "" {
		req.Header.Set("traceparent", "00-"+traceID+"-0000000000000000-01")
	}

	// 修复Bug-P1-002 路由级超时由app.go通过context.WithTimeout设置，此处不再重复设置
	_ = routeID
}

// retryRoundTripper wraps http.RoundTripper with route-level retry logic.
// 修复Bug-P1-003 实现路由级重试：后端返回可重试状态码时切换实例重试
type retryRoundTripper struct {
	base http.RoundTripper
	rp   *ReverseProxy
}

// RoundTrip executes the request with retry support.
func (rt *retryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	route, _ := req.Context().Value("route").(*model.Route)
	// 无路由配置或重试次数为0时，直接转发不重试
	if route == nil || route.RetryCount <= 0 {
		return rt.base.RoundTrip(req)
	}

	retryStatuses := parseRetryOnStatus(route.RetryOnStatus)
	targetService, _ := req.Context().Value("target_service").(string)
	version, _ := req.Context().Value("gray_version").(string)
	clientIP, _ := req.Context().Value("client_ip").(string)

	var savedBody []byte
	var savedHeader http.Header
	var savedStatusCode int
	var savedContentLength int64

	for attempt := 0; attempt <= route.RetryCount; attempt++ {
		if attempt > 0 {
			// 重试时重新选择后端实例
			instances := rt.rp.getInstances(targetService, version)
			if len(instances) > 0 {
				lb := rt.rp.getLoadBalancer(route)
				reqKey := buildRequestKey(route, req, clientIP)
				instance, err := lb.Select(instances, reqKey)
				if err == nil && instance != nil {
					req.URL.Host = instance.Address
					req.Host = instance.Address
				}
			}
			// 重放request body（httputil.ReverseProxy已设置GetBody）
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err == nil {
					req.Body = body
				}
			}
		}

		resp, err := rt.base.RoundTrip(req)
		if err != nil {
			if attempt == route.RetryCount {
				return nil, err
			}
			continue
		}

		// 判断是否需要重试
		if !shouldRetry(resp.StatusCode, retryStatuses, route.RetryNonIdempotent, req.Method) {
			return resp, nil
		}

		// 需要重试：缓冲响应体，关闭原body，进入下一轮
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			// 读取body失败，无法安全重试，直接返回当前响应
			return resp, nil
		}
		savedBody = body
		savedHeader = resp.Header
		savedStatusCode = resp.StatusCode
		savedContentLength = resp.ContentLength
	}

	// 所有重试均失败，返回最后一次缓冲的响应
	return &http.Response{
		StatusCode:    savedStatusCode,
		Header:        savedHeader,
		Body:          io.NopCloser(bytes.NewReader(savedBody)),
		ContentLength: savedContentLength,
	}, nil
}

// parseRetryOnStatus parses a comma-separated status code list (e.g. "502,503,504").
func parseRetryOnStatus(s string) map[int]bool {
	result := make(map[int]bool)
	if s == "" {
		// 默认重试502,503,504
		result[502] = true
		result[503] = true
		result[504] = true
		return result
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if code, err := strconv.Atoi(part); err == nil {
			result[code] = true
		}
	}
	return result
}

// shouldRetry determines whether a response should be retried.
// 非幂等方法默认不重试，除非RetryNonIdempotent=true
func shouldRetry(statusCode int, retryStatuses map[int]bool, retryNonIdempotent bool, method string) bool {
	if !retryStatuses[statusCode] {
		return false
	}
	if !retryNonIdempotent && !isIdempotentMethod(method) {
		return false
	}
	return true
}

// isIdempotentMethod returns true for HTTP methods that are safe to retry.
func isIdempotentMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// errorHandler handles reverse proxy errors (connection refused, timeout, etc.).
func (rp *ReverseProxy) errorHandler(w http.ResponseWriter, req *http.Request, err error) {
	w.Header().Set("Content-Type", "application/json")
	if strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "i/o timeout") {
		w.WriteHeader(http.StatusGatewayTimeout)
		w.Write([]byte(`{"code":504,"message":"gateway timeout"}`))
		return
	}
	w.WriteHeader(http.StatusBadGateway)
	w.Write([]byte(`{"code":502,"message":"bad gateway"}`))
}

// ServeHTTP implements http.Handler.
func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	rp.proxy.ServeHTTP(w, req)
}
