# 《go-api-gateway 第一轮全量测试 Bug 清单》

**测试日期**：2026-10-05
**测试版本**：项目基线（联调通过版）
**测试范围**：数据面请求全链路（路由匹配 → 限流 → 熔断 → 灰度 → 负载均衡 → 反向代理）、管理面 CRUD、配置热更新、服务注册发现、可观测性、启动/关闭生命周期

---

## 一、P0 致命 Bug（2 个，必修）

### BUG-P0-001：反向代理无法获取路由上下文，所有请求无法转发到后端

| 项目 | 内容 |
|---|---|
| **严重等级** | P0 致命 |
| **影响模块** | `internal/proxy/proxy.go` `director()` + `internal/app/app.go` 数据面 NoRoute |
| **Bug 描述** | `director()` 函数通过 `req.Context().Value("route_id")`、`req.Context().Value("target_service")` 等从标准库 context 提取路由信息。但 app.go 中通过 Gin 的 `c.Set("route_id", route.ID)` 将值存入 gin.Context 的 keys map，**Gin 不会自动将 c.Set 的值同步到 `req.Context()`**。导致 `targetService` 永远为空字符串，director 在第 152 行 `if targetService == "" { return }` 直接返回，不修改 `req.URL.Scheme/Host`。ReverseProxy 随后将请求代理回网关自身地址（req.Host），形成无限递归直至 WriteTimeout 超时。 |
| **复现步骤** | 1. 启动网关，通过管理 API 创建路由 `path=/api/users, method=GET, target_service=user-svc` 2. 通过 `/api/v1/services/user-svc/instances` 注册测试实例 `address=127.0.0.1:9001` 3. 执行 `curl http://127.0.0.1:8080/api/users` |
| **预期结果** | 请求被转发到 `127.0.0.1:9001`，返回后端响应 |
| **实际结果** | 请求超时返回 504 Gateway Timeout（或 502 Bad Gateway），后端实例收不到任何请求 |
| **影响范围** | **网关核心转发功能完全不可用**，所有经过数据面的业务请求均无法到达后端服务 |

---

### BUG-P0-002：服务自注册 key 格式错误，生产自注册实例无法被发现

| 项目 | 内容 |
|---|---|
| **严重等级** | P0 致命 |
| **影响模块** | `internal/registry/registry.go` `Registry.register()` + `Discovery` |
| **Bug 描述** | 两处注册逻辑的 etcd key 格式不一致：① `Registry.register()` 第 71 行 key = `/services/{version}/{id}`，将 **version** 放在了服务名位置；② `Discovery.RegisterTestInstance()` 第 294 行 key = `/services/{serviceName}/{id}`，格式正确。`extractServiceName()` 从 key 第二段提取服务名，导致自注册实例的服务名被错误解析为 version（如 "stable"）。此外 `Instance` 结构体**根本没有 ServiceName 字段**，自注册时无法记录实例归属哪个服务。`GetInstances("user-svc", "stable")` 在 cache 中查找 `cache["user-svc"]`，但实际数据存在 `cache["stable"]` 下，永远返回空。 |
| **复现步骤** | 1. 后端服务通过 `registry.NewRegistry()` 自注册（address=127.0.0.1:9001, version=stable） 2. 网关启动并连接 etcd 3. 创建路由 target_service=user-svc 4. 发送请求到网关 |
| **预期结果** | 网关发现 user-svc 的实例，请求转发到 127.0.0.1:9001 |
| **实际结果** | `GetInstances("user-svc", "stable")` 返回 nil，director 中 `len(instances)==0` 直接 return，请求无法转发 |
| **影响范围** | **生产环境所有通过 Registry 自注册的后端服务完全无法被网关发现**，数据面全部返回 502/504。仅管理 API 的 RegisterTestInstance（key 格式正确）可用，但该接口仅用于测试且实例 lease 只有 60s 无 keepalive |

---

## 二、P1 严重 Bug（8 个，必修）

### BUG-P1-001：限流维度 LimitBy 完全不生效，按 IP/ClientID 限流退化为全局限流

| 项目 | 内容 |
|---|---|
| **严重等级** | P1 严重 |
| **影响模块** | `internal/ratelimit/manager.go` `Allow()` |
| **Bug 描述** | `RateLimit` 模型定义了 `LimitBy` 字段（route/ip/client_id）和 `HeaderKey` 字段，`RateLimiter` 结构体也实现了按 key 分桶的逻辑。但 `RateLimiterManager.Allow(rules)` 遍历规则时直接调用 `rl.bucket.Allow()`，**完全没有根据 LimitBy 维度和请求信息生成不同 key**。所有请求共享同一个 bucket，导致按 IP 限流变成了全局限流，单个用户的流量会耗尽整个路由的配额。 |
| **复现步骤** | 1. 创建限流规则：route_id=xxx, limit_by=ip, rate=10, capacity=10 2. 客户端 A（IP=1.1.1.1）以 5 QPS 持续请求 3. 客户端 B（IP=2.2.2.2）同时发起请求 |
| **预期结果** | 每个 IP 独立享有 10 QPS 配额，A 和 B 均不被限流 |
| **实际结果** | A 和 B 共享同一个 10 QPS 桶，总 QPS 超过 10 后双方均被限流 |
| **影响范围** | 多租户/多用户场景下限流保护失效，单个恶意用户可打满全局限流配额，影响所有用户；按 Header/ClientID 维度的限流同样无效 |

---

### BUG-P1-002：路由级超时 TimeoutMs 未生效，慢请求长期占用连接

| 项目 | 内容 |
|---|---|
| **严重等级** | P1 严重 |
| **影响模块** | `internal/proxy/proxy.go` `director()` + `internal/app/app.go` |
| **Bug 描述** | `Route` 模型定义了 `TimeoutMs` 字段（默认 5000ms）。`director()` 第 180-183 行注释称"Route timeout is handled by the caller via context; skip here to avoid double timeout"，但 app.go 的 NoRoute handler 中**从未为每个请求设置带超时的 context**。ReverseProxy 的 `http.Transport` 也未设置 `ResponseHeaderTimeout`，仅设置了连接池参数。唯一的超时是数据面 server 的 `WriteTimeout`（全局配置），无法按路由粒度控制。 |
| **复现步骤** | 1. 创建路由 timeout_ms=1000，后端服务 sleep 5s 后返回 2. 发送请求到该路由 |
| **预期结果** | 1s 后网关返回 504 Gateway Timeout |
| **实际结果** | 请求挂起直到全局 WriteTimeout（或后端 5s 后返回），路由级超时配置被忽略 |
| **影响范围** | 后端服务慢查询或故障时，请求无法及时超时返回，连接被长时间占用，高并发下可导致网关连接耗尽、服务雪崩 |

---

### BUG-P1-003：路由级重试机制完全未实现

| 项目 | 内容 |
|---|---|
| **严重等级** | P1 严重 |
| **影响模块** | `internal/model/models.go` Route + `internal/proxy/proxy.go` |
| **Bug 描述** | `Route` 模型定义了 `RetryCount`、`RetryOnStatus`、`RetryNonIdempotent` 三个重试相关字段，管理 API 也支持配置这些字段。但 `ReverseProxy` 基于 `httputil.ReverseProxy`，**默认不重试**，代码中也没有任何重试逻辑（无自定义 RoundTripper、无重试包装）。配置了重试的路由在后端实例返回 502/503/504 时直接失败，不会切换实例重试。 |
| **复现步骤** | 1. 注册两个后端实例 A（正常）和 B（返回 503） 2. 创建路由 retry_count=2, retry_on_status="502,503" 3. 当负载均衡选中 B 时发送请求 |
| **预期结果** | 网关收到 503 后自动重试到实例 A，最终返回 200 |
| **实际结果** | 直接返回 503，不重试 |
| **影响范围** | 后端实例临时故障或网络抖动时请求直接失败，无法通过重试提高可用性；配置了重试参数的用户会误以为重试已生效 |

---

### BUG-P1-004：路由级负载均衡策略 LoadBalance 未生效，所有路由强制使用全局 WRR

| 项目 | 内容 |
|---|---|
| **严重等级** | P1 严重 |
| **影响模块** | `internal/app/app.go` `New()` + `internal/proxy/proxy.go` |
| **Bug 描述** | `Route` 模型定义了 `LoadBalance` 字段（weighted_round_robin / consistent_hash）和 `HashHeader` 字段。但 app.go 第 135 行创建了**全局唯一**的 `loadbalance.NewLoadBalancer("weighted_round_robin")`，ReverseProxy 直接使用这个全局实例。director 中调用 `rp.loadBalancer.Select(instances, clientIP)` 时**没有根据当前路由的 LoadBalance 字段选择策略**，也没有使用路由的 HashHeader 字段作为一致性哈希的 key。所有路由无论配置什么策略都走全局 WRR。 |
| **复现步骤** | 1. 创建路由 load_balance=consistent_hash, hash_header=X-User-ID 2. 注册两个后端实例 3. 用不同 X-User-ID 的请求多次访问 |
| **预期结果** | 相同 X-User-ID 的请求始终路由到同一后端实例 |
| **实际结果** | 所有请求按 WRR 轮询分发，一致性哈希不生效 |
| **影响范围** | 需要会话保持（粘性会话）的场景无法工作，缓存命中率下降；需要按特定 Header 做一致性哈希的业务（如用户态路由）完全失效 |

---

### BUG-P1-005：一致性哈希负载均衡的 Hash Ring 从未构建，永远 fallback 到第一个实例

| 项目 | 内容 |
|---|---|
| **严重等级** | P1 严重 |
| **影响模块** | `internal/loadbalance/loadbalance.go` `ConsistentHash` |
| **Bug 描述** | `ConsistentHash` 结构体定义了 `UpdateRing()` 方法用于根据实例列表构建哈希环，但该方法**在整个项目中从未被调用**。`Select()` 方法第 155-158 行检查 `if len(ch.ring) == 0` 时 fallback 到 `instances[0]`。即使修复了 P1-004 让路由能选择 consistent_hash 策略，由于 ring 永远为空，每次都固定返回第一个实例，既不是轮询也不是哈希。 |
| **复现步骤** | （假设 P1-004 已修复）1. 创建路由 load_balance=consistent_hash 2. 注册 3 个后端实例 3. 连续发送 10 个不同 key 的请求 |
| **预期结果** | 按一致性哈希分布到 3 个实例 |
| **实际结果** | 10 个请求全部路由到 instances[0] |
| **影响范围** | 一致性哈希策略完全不可用，退化为固定单实例路由，其余实例永远接收不到流量 |

---

### BUG-P1-006：错误率指标 ErrorRate 实现错误，最后一个请求决定错误率

| 项目 | 内容 |
|---|---|
| **严重等级** | P1 严重 |
| **影响模块** | `internal/app/app.go` `recordMetrics()` |
| **Bug 描述** | `gateway_error_rate` 定义为 `GaugeVec`（仪表盘型指标）。`recordMetrics()` 第 381-385 行每次请求都执行：若 status>=500 则 `Set(100)`，否则 `Set(0)`。这意味着错误率**完全由最后一个到达的请求决定**，而不是时间窗口内错误请求占总请求的比例。例如 1000 个请求中 999 个成功、最后 1 个失败，错误率显示为 100%；反之 999 个失败、最后 1 个成功，错误率显示为 0%。 |
| **复现步骤** | 1. 对某路由连续发送 10 个成功请求 2. 再发送 1 个失败请求（触发 500） 3. 查看 `/metrics` 中 `gateway_error_rate` |
| **预期结果** | 错误率约为 9.1%（1/11） |
| **实际结果** | 错误率为 100（被最后一个失败请求 Set 为 100） |
| **影响范围** | 监控告警完全不可信，无法基于错误率指标做自动扩容、熔断联动或告警决策；运维人员会被错误数据误导 |

---

### BUG-P1-007：两层限流架构中 Redis 分布式层完全缺失，多实例部署时限流放大 N 倍

| 项目 | 内容 |
|---|---|
| **严重等级** | P1 严重 |
| **影响模块** | `internal/config/config.go` RateLimitConfig + `internal/ratelimit/` 全部 + `internal/app/app.go` |
| **Bug 描述** | 架构设计为"本地内存 TokenBucket + Redis 批量预取"两层限流，配置中定义了 `RateLimitConfig`（PrefetchInterval、PrefetchFactor、GlobalRefillInterval、KeyTTL）。但实际代码中：① `RateLimiterManager` 只使用本地内存 bucket，**没有任何 Redis 预取/同步逻辑**；② app.go 中创建了 `redisClient` 但**从未注入到限流模块**；③ `ratelimit` 包中没有任何 Redis 相关代码。多实例部署时，每个实例独立维护本地桶，总限流倍数 = 实例数。 |
| **复现步骤** | 1. 部署 3 个网关节点 2. 配置某路由限流 rate=100 QPS 3. 用压测工具以 300 QPS 打该路由 |
| **预期结果** | 总通过量约 100 QPS，超出部分被限流 |
| **实际结果** | 每个节点独立通过 100 QPS，总通过量约 300 QPS，限流完全失效 |
| **影响范围** | 多实例生产部署时全局 QPS 限流不精确，后端可能被超过预期 N 倍的流量打垮；Redis 连接虽然建立但对限流毫无作用 |

---

### BUG-P1-008：etcd 连接失败时 reverseProxy 为 nil，无静态后端降级，网关完全不可用

| 项目 | 内容 |
|---|---|
| **严重等级** | P1 严重 |
| **影响模块** | `internal/app/app.go` `New()` + 数据面 NoRoute |
| **Bug 描述** | app.go 第 136-138 行：`if app.discovery != nil { app.reverseProxy = proxy.NewReverseProxy(...) }`。etcd 连接失败时 `discovery` 为 nil（仅打 Warn 日志不中断启动），导致 `reverseProxy` 也为 nil。数据面 NoRoute handler 第 335-339 行：`if a.reverseProxy != nil { ... } else { c.JSON(502, "no backend available") }`。**没有静态配置后端地址的降级方案**，etcd 短暂抖动或维护期间网关所有请求直接返回 502。 |
| **复现步骤** | 1. 停止 etcd 服务 2. 启动网关（日志显示 etcd connection failed, running with empty service registry） 3. 发送任意业务请求 |
| **预期结果** | 至少支持通过配置文件静态指定后端地址，或返回更明确的降级提示；etcd 恢复后自动恢复 |
| **实际结果** | 所有请求返回 502 `{"code":502,"message":"no backend available"}`，网关形同虚设 |
| **影响范围** | etcd 是网关的单点依赖，其短暂故障即可导致整个网关集群不可用；无降级能力违反高可用设计原则 |

---

## 三、待优化建议（P2 + P3，本轮不修复，留到下版本）

### P2 一般问题（11 项）

| 编号 | 问题描述 | 影响 |
|---|---|---|
| P2-01 | 熔断策略管理 API 缺少 PUT 更新接口，只有 Create/List/Get/Delete | 修改熔断策略需删了重建，操作不便且丢失历史 |
| P2-02 | 灰度规则管理 API 缺少完整 PUT 更新接口，只有 PatchPercent | 灰度规则的版本号、染色规则等无法整体更新 |
| P2-03 | 限流规则 Update 时未从 existing 记录保留 Version 字段，乐观锁不生效 | 并发更新限流规则时可能产生丢失更新 |
| P2-04 | CreateRoute 中所有数据库错误（含连接失败）统一返回 409 "路径与方法组合已存在" | 错误码误导，排障困难 |
| P2-05 | 路由匹配仅支持前缀匹配，不支持 RESTful 路径参数（如 `/users/:id`）和正则 | 微服务常见的 RESTful 路由无法精确匹配 |
| P2-06 | 路径重写仅支持 `strip_prefix=` 和直接替换，不支持正则捕获组（如 `$1`） | 复杂路径转换场景无法满足 |
| P2-07 | 熔断器 HalfOpen 状态下，若探针请求未达到 HalfOpenMaxRequests 就长时间无新请求，熔断器永久停留在 half-open | 状态机不收敛，影响状态监控 |
| P2-08 | 配置热更新通过 `asyncReload` 异步执行，管理 API 返回成功时配置可能尚未生效 | 管理操作与实际生效之间存在时间差，可能导致测试误判 |
| P2-09 | CORS 中间件 `Access-Control-Allow-Origin` 硬编码为 `"*"`，且未处理 `Allow-Credentials` | 生产环境存在安全风险，无法支持带凭据的跨域请求 |
| P2-10 | 后端服务健康检查仅依赖 etcd lease 过期，无主动 HTTP 探活机制 | 实例进程假死但 lease 未过期时，流量仍会转发到故障实例 |
| P2-11 | `SlidingWindow.Record()` 方法中 slow 统计的 if 块为空（第 107-109 行），虽 CircuitBreaker 使用的是 RecordWithSlowThreshold，但该公开方法行为错误 | 其他调用方使用 Record() 时慢调用统计永远为 0 |

### P3 轻微问题（7 项）

| 编号 | 问题描述 |
|---|---|
| P3-01 | `middleware.go` 中 `RateLimitMiddleware`、`CircuitBreakerMiddleware`、`GrayMiddleware` 定义后从未被注册使用（死代码），实际逻辑写在 app.go NoRoute 中 |
| P3-02 | `ratelimit.go` 中 `RateLimiter` 结构体（按 key 分桶版）定义后从未被使用，manager 使用的是 `ruleLimiter` |
| P3-03 | `loadbalance.go` 中 `murmur3Hash` 函数实际使用 FNV-1a 算法，函数名和注释与实现不符 |
| P3-04 | `config.go` `ShutdownTimeout()` 硬编码返回 30s，未从配置文件读取 |
| P3-05 | `main.go` `runHealthcheck()` 硬编码配置路径 `"config/default.yaml"`，未使用 `-config` 命令行参数，Docker 中自定义配置路径时 healthcheck 失效 |
| P3-06 | `configsnap.go` 中加载 services 时分页硬编码 `pageSize=10000`，超大规模时会截断 |
| P3-07 | 部分日志调用 `logger.Info("...", nil)` 传入 nil fields，可能产生空指针或冗余日志 |

---

## 四、未覆盖场景与边界

| 类别 | 未覆盖场景 |
|---|---|
| **协议与长连接** | WebSocket 代理、gRPC/HTTP2 代理、SSE 流式响应、CONNECT 方法 |
| **内容处理** | 请求/响应 Body 改写、响应脱敏、请求压缩/解压、大文件流式上传下载 |
| **安全防护** | IP 黑白名单、请求签名校验与防重放、SQL 注入/XSS 检测、WAF 规则、管理 API 操作审计日志写入（OperationLog 表已建但无写入逻辑） |
| **高可用** | 配置变更灰度发布（先部分实例生效）、配置版本回滚（ConfigVersion 表已建但无回滚接口）、多可用区实例亲和性、后端连接耗尽时的快速失败 |
| **可观测性** | 分布式链路追踪实际导出（TraceConfig 定义了 OTLP 但未集成 exporter）、访问日志结构化输出、慢请求日志采样 |
| **多租户** | 租户级隔离、租户级限流配额、租户路由优先级抢占 |
| **缓存** | 后端 304 响应处理、网关级响应缓存、缓存键设计与失效 |
| **优雅关闭** | 进行中请求的强制超时处理、关闭期间新请求的拒绝策略、连接 draining 状态码 |
| **边界输入** | 超长 URL/Header、畸形 HTTP 请求、超大 Body、URL 编码绕过、路径穿越（`../`）、Host 头注入 |
| **运维** | 管理 API 的批量导入导出、配置 diff 对比、运行时动态修改日志级别、pprof 性能分析端点 |

---

## 五、本轮测试总结

| 指标 | 数值 |
|---|---|
| P0 致命 Bug | **2 个**（核心转发链路完全不可用） |
| P1 严重 Bug | **8 个**（限流/超时/重试/负载均衡/监控/高可用均有缺陷） |
| P2 一般问题 | 11 个（下版本处理） |
| P3 轻微问题 | 7 个（下版本处理） |
| 未覆盖场景 | 10 大类 |

**核心结论**：当前基线版本存在 **2 个 P0 阻断性 Bug**，导致数据面核心转发功能在真实生产场景（服务自注册 + 多实例）下完全不可用。建议本轮优先修复 P0-001（gin context 同步到 request context）和 P0-002（注册 key 格式统一 + Instance 增加 ServiceName 字段），修复后再进行第二轮回归验证。P1 级 Bug 中，P1-001（限流维度）、P1-002（路由超时）、P1-007（Redis 分布式限流）对生产稳定性影响最大，建议与 P0 同步修复。
