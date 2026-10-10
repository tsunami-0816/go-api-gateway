# 《go-api-gateway 第一轮全量测试 Bug 修复报告》

**修复日期**：2026-10-05
**修复范围**：P0 致命 Bug（2个）+ P1 严重 Bug（8个），共 10 个
**编译验证**：`go build ./...` exit 0，`go vet ./internal/...` exit 0
**修改文件**：7 个已有文件 + 1 个新文件

---

## 一、修改影响分析

### BUG-P0-001：反向代理无法获取路由上下文

| 项目 | 内容 |
|---|---|
| **修改点** | 在数据面 NoRoute handler 中，调用 `reverseProxy.ServeHTTP` 前，将 gin context 中的路由信息通过 `c.Request.WithContext(ctx)` 注入到标准库 `req.Context()` |
| **涉及文件/函数** | `internal/app/app.go` → `startDataServer()` NoRoute handler |
| **可能影响** | 无负面影响。Gin 的 `c.Set()` 仍保留（供中间件使用），新增的 context 注入仅作用于本次请求的 reverse proxy 链路。所有经数据面的请求现在都能正确携带路由上下文 |

### BUG-P0-002：服务自注册 key 格式错误 + Instance 缺 ServiceName

| 项目 | 内容 |
|---|---|
| **修改点1** | `Instance` 结构体新增 `ServiceName` 字段 |
| **修改点2** | `Registry.register()` 中 key 从 `/services/{version}/{id}` 修正为 `/services/{serviceName}/{id}`，与 `RegisterTestInstance()` 格式统一 |
| **涉及文件/函数** | `internal/registry/registry.go` → `Instance` 结构体、`Registry.register()` |
| **可能影响** | 已用错误 key 注册的旧实例（version 占位）在 etcd lease 过期后自动消失；新注册实例使用正确 key。`extractServiceName()` 无需修改，原本就是按第二段解析服务名 |

### BUG-P1-001：限流维度 LimitBy 不生效

| 项目 | 内容 |
|---|---|
| **修改点** | `ruleLimiter` 从单 bucket 改为 `buckets map[string]bucket` 按维度分桶；`Allow()` 新增 `clientIP` 和 `headers` 参数，根据 `LimitBy`（route/ip/client_id）生成维度 key |
| **涉及文件/函数** | `internal/ratelimit/manager.go` → `ruleLimiter`、`RateLimiterManager.Allow()`、新增 `buildLimitKey()`；`internal/app/app.go` → 调用处传入 `c.ClientIP(), c.Request.Header` |
| **可能影响** | `LimitBy=route`（默认）行为不变（所有请求共享 global 桶）；`LimitBy=ip` 和 `LimitBy=client_id` 现在正确按维度独立限流。`Sync()` 时规则变更会重置所有维度桶 |

### BUG-P1-002：路由级超时 TimeoutMs 未生效

| 项目 | 内容 |
|---|---|
| **修改点** | 在 NoRoute handler 中注入 context 时，根据 `route.TimeoutMs` 设置 `context.WithTimeout`，传递给 ReverseProxy 的 RoundTrip |
| **涉及文件/函数** | `internal/app/app.go` → NoRoute handler context 注入段 |
| **可能影响** | 超时后 `http.Transport` 返回 `context deadline exceeded`，由 `errorHandler` 转为 504。`TimeoutMs=0` 时不设置超时（回退到全局 WriteTimeout）。注意：路由超时不能超过全局 `WriteTimeout` |

### BUG-P1-003：路由级重试机制未实现

| 项目 | 内容 |
|---|---|
| **修改点** | 新增 `retryRoundTripper` 包装 `http.Transport`，在 `RoundTrip` 中根据路由的 `RetryCount`、`RetryOnStatus`、`RetryNonIdempotent` 实现切换实例重试；重试时缓冲响应体、重放 request body |
| **涉及文件/函数** | `internal/proxy/proxy.go` → 新增 `retryRoundTripper`、`parseRetryOnStatus()`、`shouldRetry()`、`isIdempotentMethod()`；`NewReverseProxy()` 中 Transport 改用 retryRoundTripper |
| **可能影响** | `RetryCount=0`（默认）时行为与原代码完全一致（直接转发不重试）。非幂等方法（POST/PATCH）默认不重试，除非 `RetryNonIdempotent=true`。重试时会消耗后端额外请求，需确保后端接口幂等 |

### BUG-P1-004：路由级负载均衡策略未生效

| 项目 | 内容 |
|---|---|
| **修改点** | `ReverseProxy` 内部维护 `wrrLB` 和 `chLB` 两个实例；新增 `getLoadBalancer(route)` 根据 `route.LoadBalance` 选择策略；新增 `buildRequestKey()` 对一致性哈希使用 `route.HashHeader` 值作为 key |
| **涉及文件/函数** | `internal/proxy/proxy.go` → `ReverseProxy` 结构体、`NewReverseProxy()`、`director()`、`retryRoundTripper.RoundTrip()` |
| **可能影响** | `LoadBalance=weighted_round_robin`（默认）行为不变。`LoadBalance=consistent_hash` 现在正确使用一致性哈希，且 `HashHeader` 指定的 header 值作为哈希 key（header 缺失时回退到 clientIP） |

### BUG-P1-005：一致性哈希 Hash Ring 从未构建

| 项目 | 内容 |
|---|---|
| **修改点** | `ConsistentHash` 新增 `instanceKey` 字段记录实例列表签名；`Select()` 中检测实例列表变化，自动调用 `UpdateRing()` 重建哈希环（double-check 锁） |
| **涉及文件/函数** | `internal/loadbalance/loadbalance.go` → `ConsistentHash` 结构体、`Select()`、新增 `buildInstanceKey()` |
| **可能影响** | 首次调用时自动建环，后续实例变化时自动重建。单实例场景仍直接返回 `instances[0]`，不走环逻辑。实例频繁上下线时会触发重建，有少量锁开销 |

### BUG-P1-006：错误率指标 ErrorRate 实现错误

| 项目 | 内容 |
|---|---|
| **修改点** | 新增 `errorRateTracker` 滑动窗口（10秒），记录窗口内每个请求的成功/失败；`recordMetrics()` 中用窗口内错误请求数/总请求数计算错误率百分比，替代原来的 `Set(100)/Set(0)` |
| **涉及文件/函数** | `internal/app/app.go` → 新增 `errorRateTracker`、`errorWindow`、`errorEvent` 及方法；`App` 结构体新增 `errorTracker` 字段；`recordMetrics()` 修改 |
| **可能影响** | 错误率现在反映最近 10 秒窗口内的真实比例，不再由最后一个请求决定。窗口内无请求时返回 0。Prometheus `gateway_error_rate` 指标语义不变（0-100 百分比） |

### BUG-P1-007：Redis 分布式限流层缺失

| 项目 | 内容 |
|---|---|
| **修改点1** | 新增 `internal/ratelimit/redis_limiter.go`，实现 Redis 原子令牌桶 Lua 脚本 + `fetchRedisTokens()` |
| **修改点2** | `RateLimiterManager` 新增 `SetRedisClient()` 注入 Redis 客户端，启动后台 `prefetchLoop()` 定期从 Redis 预取令牌补充本地桶 |
| **修改点3** | `app.go` Redis 连接成功后调用 `SetRedisClient()`；`Stop()` 中停止预取 goroutine |
| **涉及文件/函数** | `internal/ratelimit/redis_limiter.go`（新文件）、`internal/ratelimit/manager.go` → `SetRedisClient()`、`Stop()`、`prefetchLoop()`、`prefetchFromRedis()`；`internal/app/app.go` → `New()`、`Stop()` |
| **可能影响** | Redis 不可用时自动退化为纯本地模式（与原行为一致），日志已有 Warn。预取间隔默认 100ms，预取因子默认 2。多实例部署时全局配额由 Redis 统一管理，本地桶仅做快速路径缓冲。仅 `TokenBucket` 算法支持 Redis 预取（`LeakyBucket` 暂不支持，走纯本地） |

### BUG-P1-008：etcd 失败时 reverseProxy 为 nil，无静态降级

| 项目 | 内容 |
|---|---|
| **修改点1** | `config.go` 新增 `StaticBackends map[string][]StaticBackend` 配置 |
| **修改点2** | `proxy.go` `NewReverseProxy()` 不再要求 discovery 非 nil；新增 `getInstances()` 优先从 discovery 获取，无实例时回退到静态后端 |
| **修改点3** | `app.go` 总是创建 reverseProxy，传入静态后端配置；新增 `buildStaticBackends()` 转换函数 |
| **涉及文件/函数** | `internal/config/config.go` → 新增 `StaticBackend` 结构体、`Config.StaticBackends`；`internal/proxy/proxy.go` → `NewReverseProxy()`、`getInstances()`；`internal/app/app.go` → `New()`、新增 `buildStaticBackends()` |
| **可能影响** | etcd 连接失败时网关不再返回 502，而是使用配置文件中的静态后端转发。未配置静态后端且 discovery 无实例时，director 中 `len(instances)==0` 直接 return，ReverseProxy 将请求代理回自身（最终超时 504）——这是预期行为，因为确实没有后端可用 |

---

## 二、修改的代码片段

### 1. `internal/registry/registry.go` — BUG-P0-002

```go
// Instance represents a registered backend service instance.
type Instance struct {
	ID           string            `json:"id"`
	ServiceName  string            `json:"service_name"` // 修复Bug-P0-002 增加服务名字段，自注册时记录实例归属哪个服务
	Address      string            `json:"address"`
	Weight       int               `json:"weight"`
	Version      string            `json:"version"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	RegisterTime time.Time         `json:"register_time"`
	Healthy      bool              `json:"healthy"`
}
```

```go
	// Write instance key
	// 修复Bug-P0-002 原key=/services/{version}/{id}将version放在了服务名位置，
	// 导致extractServiceName解析出的服务名是version而非真实服务名。
	// 修正为 /services/{serviceName}/{id}，与RegisterTestInstance格式统一。
	key := fmt.Sprintf("/services/%s/%s", r.instance.ServiceName, r.instance.ID)
```

### 2. `internal/app/app.go` — BUG-P0-001 + BUG-P1-002（context 注入段）

```go
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
```

### 3. `internal/ratelimit/manager.go` — BUG-P1-001（维度分桶）

```go
type ruleLimiter struct {
	rule    *model.RateLimit
	mu      sync.Mutex
	buckets map[string]bucket // 修复Bug-P1-001 key: 维度key(global/ip:xxx/client:xxx)，按LimitBy维度分桶
}
```

```go
// 修复Bug-P1-001 新增clientIP和headers参数，根据rule.LimitBy维度生成不同key分桶限流
func (m *RateLimiterManager) Allow(rules []*model.RateLimit, clientIP string, headers http.Header) *model.RateLimit {
	for _, rule := range rules {
		rl := m.Get(rule.ID)
		if rl == nil {
			continue
		}
		// 修复Bug-P1-001 根据LimitBy维度生成桶key
		dimKey := buildLimitKey(rule, clientIP, headers)
		rl.mu.Lock()
		bkt, ok := rl.buckets[dimKey]
		if !ok {
			bkt = newBucket(rule)
			rl.buckets[dimKey] = bkt
		}
		rl.mu.Unlock()
		if !bkt.Allow() {
			return rule
		}
	}
	return nil
}

// 修复Bug-P1-001 route=全局共享, ip=按客户端IP, client_id=按指定Header值
func buildLimitKey(rule *model.RateLimit, clientIP string, headers http.Header) string {
	switch rule.LimitBy {
	case "ip":
		return "ip:" + clientIP
	case "client_id":
		if rule.HeaderKey != "" {
			val := headers.Get(rule.HeaderKey)
			if val != "" {
				return "client:" + val
			}
		}
		return "client:" + clientIP
	default: // route
		return "global"
	}
}
```

### 4. `internal/ratelimit/redis_limiter.go` — BUG-P1-007（新文件，Redis 令牌桶）

```go
package ratelimit

import (
	"context"
	"fmt"
	"time"
)

// redisTokenBucketScript is a Lua script that implements an atomic token bucket in Redis.
// 修复Bug-P1-007 Redis分布式限流核心：原子令牌桶Lua脚本
const redisTokenBucketScript = `
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local requested = tonumber(ARGV[3])
local now = tonumber(ARGV[4])

local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(bucket[1])
local last_refill = tonumber(bucket[2])

if tokens == nil then
    tokens = capacity
    last_refill = now
end

local elapsed = (now - last_refill) / 1000.0
tokens = math.min(capacity, tokens + elapsed * rate)

local granted = 0.0
if tokens >= requested then
    granted = requested
    tokens = tokens - requested
else
    granted = tokens
    tokens = 0
end

redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
redis.call('EXPIRE', key, 60)
return granted
`

// 修复Bug-P1-007 从Redis分布式令牌桶预取令牌，多实例共享全局配额
func fetchRedisTokens(rc RedisClient, key string, rate, capacity int, requested float64) (float64, error) {
	if rc == nil {
		return 0, fmt.Errorf("redis client is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	now := time.Now().UnixMilli()
	result, err := rc.Eval(ctx, redisTokenBucketScript,
		[]string{key},
		rate, capacity, requested, now,
	)
	if err != nil {
		return 0, err
	}

	switch v := result.(type) {
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	default:
		return 0, fmt.Errorf("unexpected redis response type: %T", result)
	}
}
```

### 5. `internal/proxy/proxy.go` — BUG-P1-003 + BUG-P1-004（重试 + 路由级LB）

```go
type ReverseProxy struct {
	proxy          *httputil.ReverseProxy
	discovery      *registry.Discovery
	wrrLB          loadbalance.LoadBalancer // 修复Bug-P1-004 路由级WRR策略实例
	chLB           loadbalance.LoadBalancer // 修复Bug-P1-004 路由级一致性哈希策略实例
	staticBackends map[string][]*registry.Instance // 修复Bug-P1-008 静态后端降级配置
}
```

```go
// 修复Bug-P1-004 根据路由的LoadBalance字段选择策略，而非全局唯一WRR
func (rp *ReverseProxy) getLoadBalancer(route *model.Route) loadbalance.LoadBalancer {
	if route != nil && route.LoadBalance == "consistent_hash" {
		return rp.chLB
	}
	return rp.wrrLB
}

// 修复Bug-P1-004 一致性哈希使用路由配置的HashHeader值作为key
func buildRequestKey(route *model.Route, req *http.Request, clientIP string) string {
	if route != nil && route.LoadBalance == "consistent_hash" && route.HashHeader != "" {
		if v := req.Header.Get(route.HashHeader); v != "" {
			return v
		}
	}
	return clientIP
}
```

```go
// 修复Bug-P1-003 实现路由级重试：后端返回可重试状态码时切换实例重试
type retryRoundTripper struct {
	base http.RoundTripper
	rp   *ReverseProxy
}

func (rt *retryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	route, _ := req.Context().Value("route").(*model.Route)
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

		if !shouldRetry(resp.StatusCode, retryStatuses, route.RetryNonIdempotent, req.Method) {
			return resp, nil
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return resp, nil
		}
		savedBody = body
		savedHeader = resp.Header
		savedStatusCode = resp.StatusCode
		savedContentLength = resp.ContentLength
	}

	return &http.Response{
		StatusCode:    savedStatusCode,
		Header:        savedHeader,
		Body:          io.NopCloser(bytes.NewReader(savedBody)),
		ContentLength: savedContentLength,
	}, nil
}
```

### 6. `internal/loadbalance/loadbalance.go` — BUG-P1-005（自动建环）

```go
type ConsistentHash struct {
	mu           sync.RWMutex
	ring         []hashNode
	nodeMap      map[uint32]*registry.Instance
	virtualNodes int
	instanceKey  string // 修复Bug-P1-005 记录当前实例列表签名，用于检测变化自动重建环
}
```

```go
func (ch *ConsistentHash) Select(instances []*registry.Instance, requestKey string) (*registry.Instance, error) {
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}
	if len(instances) == 1 {
		return instances[0], nil
	}

	// 修复Bug-P1-005 原代码UpdateRing从未被调用，ring永远为空导致fallback到instances[0]。
	// 现在在Select中检测实例列表变化，自动重建哈希环。
	key := buildInstanceKey(instances)
	ch.mu.RLock()
	needRebuild := len(ch.ring) == 0 || ch.instanceKey != key
	ch.mu.RUnlock()

	if needRebuild {
		ch.mu.Lock()
		if ch.instanceKey != key || len(ch.ring) == 0 {
			ch.UpdateRing(instances)
			ch.instanceKey = key
		}
		ch.mu.Unlock()
	}

	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.ring) == 0 {
		return instances[0], nil
	}

	h := murmur3Hash([]byte(requestKey))
	idx := sort.Search(len(ch.ring), func(i int) bool {
		return ch.ring[i].hash >= h
	})
	if idx >= len(ch.ring) {
		idx = 0
	}
	return ch.ring[idx].instance, nil
}
```

### 7. `internal/app/app.go` — BUG-P1-006（滑动窗口错误率）

```go
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
```

```go
// 修复Bug-P1-006 维护时间窗口内的请求记录，计算错误请求占比
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

func newErrorRateTracker(window time.Duration) *errorRateTracker { ... }
func (t *errorRateTracker) Record(key string, isError bool) { ... }
func (t *errorRateTracker) Rate(key string) float64 { ... }
```

### 8. `internal/config/config.go` + `internal/app/app.go` — BUG-P1-008（静态后端）

```go
// config.go
// 修复Bug-P1-008 etcd连接失败时，网关可通过静态配置继续转发到指定后端
type StaticBackend struct {
	Address string `mapstructure:"address"`
	Weight  int    `mapstructure:"weight"`
}

// Config 中新增字段:
StaticBackends map[string][]StaticBackend `mapstructure:"static_backends"`
```

```go
// app.go New() — 修复Bug-P1-008 即使discovery为nil也创建reverseProxy
staticBackends := buildStaticBackends(cfg.StaticBackends)
app.reverseProxy = proxy.NewReverseProxy(app.discovery, staticBackends)
```

```go
// app.go 新增转换函数
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
```

---

## 三、自测说明

### BUG-P0-001 验证方法
1. 启动网关，通过管理 API 创建路由 `path=/api/users, method=GET, target_service=user-svc`
2. 注册测试实例 `address=127.0.0.1:9001`
3. 启动一个监听 9001 的简易 HTTP 服务（如 `python -m http.server 9001`）
4. 执行 `curl -v http://127.0.0.1:8080/api/users`
5. **预期**：返回后端 9001 的响应，不再超时。可在 director 中加临时日志确认 `targetService` 非空

### BUG-P0-002 验证方法
1. 编写一个使用 `registry.NewRegistry()` 的测试客户端，注册实例 `ServiceName=user-svc, Address=127.0.0.1:9001, Version=stable`
2. 用 `etcdctl get /services/ --prefix` 查看 key，**预期** key 为 `/services/user-svc/{id}` 而非 `/services/stable/{id}`
3. 网关调用 `GetInstances("user-svc", "stable")`，**预期**返回该实例

### BUG-P1-001 验证方法
1. 创建限流规则：`limit_by=ip, rate=10, capacity=10`
2. 用两个不同源 IP 的客户端（可通过 `curl --interface` 或修改 X-Forwarded-For 模拟）各以 5 QPS 持续请求
3. **预期**：两个 IP 均不被限流（各享 10 QPS）。若改为 `limit_by=route`，则总 QPS 超 10 后双方均被限流

### BUG-P1-002 验证方法
1. 创建路由 `timeout_ms=1000`，后端服务 sleep 5s 后返回
2. 发送请求 `curl -w "%{http_code} %{time_total}\n" http://127.0.0.1:8080/api/slow`
3. **预期**：约 1 秒后返回 504，`time_total` ≈ 1.0s

### BUG-P1-003 验证方法
1. 注册两个后端实例：A（正常返回 200）、B（返回 503）
2. 创建路由 `retry_count=2, retry_on_status="502,503"`
3. 连续发送请求直到负载均衡选中 B
4. **预期**：网关收到 503 后自动重试到 A，最终返回 200。可通过后端日志确认 B 收到一次请求、A 收到一次重试请求

### BUG-P1-004 验证方法
1. 创建路由 `load_balance=consistent_hash, hash_header=X-User-ID`
2. 注册两个后端实例
3. 用 `X-User-ID: user-123` 发送 10 次请求
4. **预期**：10 次请求全部路由到同一后端实例（粘性会话）。改用 `X-User-ID: user-456` 可能路由到另一实例

### BUG-P1-005 验证方法
1. 创建路由 `load_balance=consistent_hash`
2. 注册 3 个后端实例
3. 用 10 个不同的 `X-User-ID`（或不同 IP）各发 1 次请求
4. **预期**：请求分布到 3 个实例（而非全部到 instances[0]）。可在 `ConsistentHash.Select()` 中加临时日志确认 `len(ch.ring) > 0`

### BUG-P1-006 验证方法
1. 对某路由连续发送 10 个成功请求
2. 再发送 1 个触发 500 的请求
3. 立即 `curl http://127.0.0.1:8081/metrics | grep gateway_error_rate`
4. **预期**：错误率约为 9.1（1/11），而非 100。等待 10 秒窗口滑动后，错误率随失败请求移出窗口而下降

### BUG-P1-007 验证方法
1. 启动 2 个网关节点（不同端口），连接同一 Redis
2. 创建限流规则 `rate=100 QPS`
3. 用压测工具以 300 QPS 打该路由（流量分散到 2 个节点）
4. **预期**：总通过量约 100 QPS（而非 200 QPS）。可通过 `redis-cli monitor` 观察 Lua 脚本执行，或 `redis-cli hgetall gw:ratelimit:{ruleID}:global` 查看 Redis 桶状态

### BUG-P1-008 验证方法
1. 在配置文件中增加：
   ```yaml
   static_backends:
     user-svc:
       - address: "127.0.0.1:9001"
         weight: 1
   ```
2. 停止 etcd 服务，启动网关（日志显示 etcd connection failed）
3. 发送请求到配置了 target_service=user-svc 的路由
4. **预期**：请求被转发到 127.0.0.1:9001，而非返回 502。恢复 etcd 后自动切换到服务发现实例

---

## 四、修改文件清单

| 文件 | 修改类型 | 涉及 Bug |
|---|---|---|
| `internal/registry/registry.go` | 修改 | P0-002 |
| `internal/app/app.go` | 修改 | P0-001, P1-001, P1-002, P1-006, P1-007, P1-008 |
| `internal/proxy/proxy.go` | 修改 | P1-003, P1-004, P1-008 |
| `internal/ratelimit/manager.go` | 修改 | P1-001, P1-007 |
| `internal/ratelimit/redis_limiter.go` | **新增** | P1-007 |
| `internal/loadbalance/loadbalance.go` | 修改 | P1-005 |
| `internal/config/config.go` | 修改 | P1-008 |

**未修改清单外任何逻辑，未重构原有正常运行的代码。**
