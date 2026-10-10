Read "D:\lg\project\go-api-gateway\任务拆解与开发规范.md":

     1	# 任务拆解与开发规范
     2	
     3	| 项目名称 | Go 语言高性能 API 网关与流量治理平台 |
     4	|---|---|
     5	| 文档版本 | v1.0 |
     6	| 编制日期 | 2026-10-04 |
     7	| 依据文档 | 《第二轮技术架构设计文档》v2.0 |
     8	| 文档性质 | 开发执行指导文件，可直接分配任务 |
     9	
    10	---
    11	
    12	## 目录
    13	
    14	1. [模块划分总览](#1-模块划分总览)
    15	2. [各模块任务拆解](#2-各模块任务拆解)
    16	3. [优先级与依赖关系](#3-优先级与依赖关系)
    17	4. [开发规范](#4-开发规范)
    18	5. [验收标准](#5-验收标准)
    19	
    20	---
    21	
    22	## 1. 模块划分总览
    23	
    24	按第二轮架构六层模型，拆分为 **16 个独立开发模块**，每个模块职责单一、交付物明确、可并行开发。
    25	
    26	| 模块编号 | 模块名称 | 所属层级 | 核心职责 | 交付物 | 依赖模块 |
    27	|---|---|---|---|---|---|
    28	| M-01 | 项目骨架与基础设施 | 横向支撑 | 目录结构、配置管理、日志、统一响应、启动时序 | 可编译运行的空壳工程 + 基础库 | 无 |
    29	| M-02 | 配置存储层 | 存储层 | GORM 模型、SQLite/MySQL 双驱动、AutoMigrate、Repository | 7 张表 Model + Repository 接口 + 单元测试 | M-01 |
    30	| M-03 | etcd 服务注册与发现 | 存储层 | etcd 客户端、服务注册(Lease)、服务发现(Watch+缓存)、健康检查 | Registry + Discovery + HealthChecker | M-01 |
    31	| M-04 | Redis 封装层 | 存储层 | Redis 客户端、连接池、Lua 脚本执行器 | RedisClient + LuaExecutor | M-01 |
    32	| M-05 | 接入层 HTTP Server | 接入层 | 数据面/管理面双 Gin Engine、端口隔离、连接管理、健康探针 | 双 Server 启动 + /healthz + /readyz | M-01 |
    33	| M-06 | 路由匹配与反向代理 | 应用层 | radix tree 路由匹配、路径重写、ReverseProxy 定制、重试执行器 | Router + Rewriter + Proxy + RetryHandler | M-02, M-03 |
    34	| M-07 | 请求/响应拦截链 | 应用层 | Trace 注入、限流、熔断、灰度中间件编排、响应后置处理 | Middleware 链 + 响应拦截器 | M-05, M-08, M-09, M-10 |
    35	| M-08 | 双层限流引擎 | 计算层 | 本地令牌桶/漏桶、Redis 批量预取、全局桶 Lua、降级策略 | RateLimiter + 同步协程 + Lua 脚本 | M-04 |
    36	| M-09 | 熔断状态机(全局协调) | 计算层 | 三态状态机、本地滑动窗口、Redis 半开计数、状态事件广播 | CircuitBreaker + SlidingWindow | M-04 |
    37	| M-10 | 灰度路由与负载均衡 | 计算层 | 灰度计算器(染色+哈希比例)、一致性哈希环、加权轮询 | GrayRouter + ConsistentHash + WRR | M-03 |
    38	| M-11 | 配置热更新 | 计算层 | etcd Watch 配置变更、全量拉取、不可变快照、atomic.Value 替换 | ConfigWatcher + SnapshotManager | M-02, M-03 |
    39	| M-12 | 管理面 API | 应用层 | 路由/限流/熔断/灰度/服务 CRUD、Token 鉴权、操作审计 | 全套 REST API + 鉴权中间件 + 审计 | M-02, M-05, M-11 |
    40	| M-13 | 可观测性 | 运维层 | Prometheus 指标、OTel 链路追踪、zap 结构化日志(脱敏) | Metrics + Tracer + Logger 集成 | M-01 |
    41	| M-14 | 测试服务集群 | 测试支撑 | 5 个测试服务(OTel+DB+etcd 注册+/health) | test-services/ 目录 + Dockerfile | M-03 |
    42	| M-15 | 部署与编排 | 运维层 | Dockerfile、docker-compose、K8s 清单、HPA、Prometheus Adapter | deploy/ 目录全套 YAML | M-05, M-13 |
    43	| M-16 | 性能测试与集成验收 | 验收 | wrk 基准、灰度验证、双驱动 CI、端到端集成测试 | 测试脚本 + 性能报告 | 全部 |
    44	
    45	---
    46	
    47	## 2. 各模块任务拆解
    48	
    49	### M-01 项目骨架与基础设施
    50	
    51	> **目标**：搭建可编译、可运行的 Go 工程骨架，提供所有模块依赖的基础库。
    52	
    53	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
    54	|---|---|---|---|
    55	| T-0101 | 初始化 Go Module 与目录结构 | 创建 `go.mod`(Go 1.22)，按 `cmd/`、`internal/`、`pkg/`、`config/`、`deploy/`、`test-services/` 组织目录；编写 `.gitignore`、`Makefile`(build/test/lint/run) | 0.5d |
    56	| T-0102 | 配置管理(viper) | 集成 viper，支持 `config/default.yaml` + 环境变量覆盖(`GATEWAY_` 前缀)；定义 Config 结构体(etcd/redis/db/server/admin/log/trace 各节)；支持热加载监听 | 1d |
    57	| T-0103 | 日志系统(zap) | 集成 zap 结构化 JSON 日志；定义 Logger 接口(Info/Warn/Error/Debug)；支持日志级别配置、异步写入、TraceId/SpanId 字段注入；提供全局 `logger.L` 单例 | 1d |
    58	| T-0104 | 统一响应与错误码 | 定义 `Response{code,message,data,trace_id,errors}` 结构体；定义错误码常量(通用+各模块业务码，按 4xxyyy 规范)；实现 `Success()`/`Fail()`/`PageSuccess()` 辅助函数；Gin 错误恢复中间件 | 1d |
    59	| T-0105 | 启动初始化框架 | 实现 9 步启动时序(配置→日志→DB→配置快照→etcd→Redis→健康检查→管理面→数据面)；定义 `App` 结构体持有所有依赖；实现优雅关闭(signal 监听，逆序关闭)；数据面最后启动 | 1.5d |
    60	| T-0106 | 通用工具库 | UUID 生成(v4)、时间工具(UTC)、IP 提取( X-Forwarded-For/RemoteAddr)、JSON 工具、HTTP 客户端封装、重试工具(指数退避)、slice/map 工具函数 | 1d |
    61	
    62	---
    63	
    64	### M-02 配置存储层
    65	
    66	> **目标**：基于 GORM 实现 7 张配置表的 Model 定义与 Repository 层，支持 SQLite/MySQL 双驱动切换。
    67	
    68	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
    69	|---|---|---|---|
    70	| T-0201 | GORM 集成与双驱动 | 集成 GORM v1.25.10；实现 `gorm.io/driver/sqlite`(modernc 纯 Go) 与 `gorm.io/driver/mysql` 驱动工厂；连接池配置(MaxOpenConns=100/MaxIdleConns=20/ConnMaxLifetime)；`CGO_ENABLED=0` 编译验证 | 1d |
    71	| T-0202 | 核心表 Model 定义 | 定义 7 个 GORM Model：`Route`、`RateLimit`、`CircuitBreaker`、`GrayRelease`、`Service`、`ConfigVersion`、`OperationLog`；通用字段嵌入 `BaseModel`(id UUID/created_at/updated_at/created_by/updated_by/deleted_at gorm.DeletedAt/version)；表名 `gw_` 前缀；JSON 字段(stain_rules/snapshot)用 `datatypes.JSON` | 1.5d |
    72	| T-0203 | AutoMigrate 与建表验证 | 实现启动时 `db.AutoMigrate()` 自动建表；SQLite 本地建表验证；MySQL 生产 DDL 脚本导出(参考)；验证 `deleted_at` 唯一索引(NULL 不参与约束)；验证 UUID 主键不依赖自增 | 0.5d |
    73	| T-0204 | Route Repository | 实现路由 CRUD：Create/Update/Delete(逻辑删除)/GetByID/List(分页+条件筛选)/GetByPathMethod(精确匹配)/ListByTargetService/UpdateStatus；含乐观锁(version 递增) | 1d |
    74	| T-0205 | RateLimit Repository | 限流规则 CRUD：Create/Update/Delete/GetByID/List/GetByRouteID/GetByPathMethod/ListEnabled；支持 route_id 与 path_pattern 二选一查询 | 0.5d |
    75	| T-0206 | CircuitBreaker Repository | 熔断策略 CRUD：Create/Update/Delete/GetByID/List/GetByRouteID/GetByTargetService/ListEnabled | 0.5d |
    76	| T-0207 | GrayRelease Repository | 灰度规则 CRUD：Create/Update/Delete/GetByID/List/GetByRouteID/UpdatePercent/ListEnabled | 0.5d |
    77	| T-0208 | Service Repository | 服务元信息 CRUD：Create/Update/Delete/GetByName/List/GetWithHealthConfig | 0.5d |
    78	| T-0209 | 审计 Repository | `ConfigVersion` 写入(create/update/delete 快照 before/after)；`OperationLog` 写入(操作人/操作/资源/请求/响应/耗时)；查询接口(按资源/操作人/时间范围分页) | 1d |
    79	| T-0210 | 存储层单元测试 | 各 Repository 单测(SQLite 内存库)；覆盖 CRUD、逻辑删除、唯一约束冲突、乐观锁冲突、分页；测试覆盖率 ≥80% | 1.5d |
    80	
    81	---
    82	
    83	### M-03 etcd 服务注册与发现
    84	
    85	> **目标**：实现后端服务向 etcd 注册、网关通过 Watch 实时发现实例、主动健康检查摘除异常实例。
    86	
    87	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
    88	|---|---|---|---|
    89	| T-0301 | etcd 客户端封装 | 集成 `go.etcd.io/etcd/client/v3`；实现连接管理(多 endpoint、重连指数退避、拨号超时)；定义 `EtcdClient` 接口(Put/Get/Delete/Watch/Grant/KeepAlive/Revoke) | 1d |
    90	| T-0302 | 服务注册器(Registry) | 后端服务侧注册：创建 Lease(TTL=10s) → 写入 Key `/services/{name}/{instance-id}` → Value(JSON: address/weight/metadata.version/register_time) → 绑定 Lease → KeepAlive 协程(每 5s 续约) → 关闭时撤销 Lease | 1d |
    91	| T-0303 | 服务发现器(Discovery) | 网关侧：启动全量拉取 `/services/` 前缀构建初始缓存 → Watch 前缀(PUT/DELETE 事件) → 事件增量更新 → `atomic.Value` 原子替换缓存(读无锁) → 提供 `GetInstances(serviceName, version)` 接口 | 1.5d |
    92	| T-0304 | 健康检查器(HealthChecker) | 定时 HTTP GET 实例 `/health`(间隔=服务配置 health_check_interval，默认 10s；超时=3s)；连续 2 次失败标记 Unhealthy 并从可用列表剔除；连续 2 次成功恢复 Healthy；不健康实例仍保留在缓存(标记状态) | 1d |
    93	| T-0305 | 实例缓存与过滤 | 实现实例列表数据结构(按 serviceName → version → []Instance)；过滤接口：按版本过滤、按健康状态过滤、按权重排序；实例变更事件回调(通知负载均衡器重建环) | 0.5d |
    94	| T-0306 | etcd 降级与重连 | etcd 不可用时使用本地缓存兜底(继续服务)；重连指数退避(1s→2s→4s→最大 30s)；重连成功后全量拉取重建缓存；下线感知 ≤10s 验证 | 0.5d |
    95	
    96	---
    97	
    98	### M-04 Redis 封装层
    99	
   100	> **目标**：提供 Redis 客户端连接管理与 Lua 脚本原子执行能力，供限流/熔断模块使用。
   101	
   102	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
   103	|---|---|---|---|
   104	| T-0401 | Redis 客户端封装 | 集成 `github.com/redis/go-redis/v9`；连接池配置(MaxIdleConns=1000/PoolSize=50/MinIdleConns=10)；定义 `RedisClient` 接口(Get/Set/Incr/Eval/Del/Expire/Ping/HGet/HSet)；重连退避 | 0.5d |
   105	| T-0402 | Lua 脚本执行器 | 实现 `LuaScript` 加载与缓存(脚本 SHA 复用)；限流预取脚本、全局桶补充脚本、熔断半开计数脚本的定义与参数封装；保证原子性(Eval 执行)；每次执行后 EXPIRE 刷新 TTL=3600s | 1d |
   106	| T-0403 | Redis 降级策略 | Redis 不可用时自动降级(调用方感知 error)；熔断/限流模块的降级回调接口定义；健康检查(Ping)与状态上报 | 0.5d |
   107	
   108	---
   109	
   110	### M-05 接入层 HTTP Server
   111	
   112	> **目标**：实现数据面(8080)与管理面(8081)双 Gin Engine 隔离启动，含健康检查端点。
   113	
   114	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
   115	|---|---|---|---|
   116	| T-0501 | 数据面 Gin Engine | 创建数据面 Gin Engine(监听 `0.0.0.0:8080`)；net/http Server 参数(ReadTimeout=10s/WriteTimeout=30s/IdleTimeout=120s/MaxHeaderBytes=1MB)；NoRoute 处理器(404 统一响应)；Recovery 中间件 | 0.5d |
   117	| T-0502 | 管理面 Gin Engine | 创建管理面 Gin Engine(监听 `0.0.0.0:8081`)；容器内绑 0.0.0.0 使端口映射生效；compose 映射到宿主机回环 `127.0.0.1:8081:8081`；管理面路由组 `/api/v1/` | 0.5d |
   118	| T-0503 | 健康检查端点 | `/healthz`(liveness)：始终返回 200；`/readyz`(readiness)两阶段：启动 60s 内检查(配置加载+etcd/Redis/DB 初始连接)，运行时仅检查(配置快照非空+HTTP Server 正常，不检查外部依赖实时连通性) | 1d |
   119	| T-0504 | 双 Server 启动与优雅关闭 | 管理面先启动(⑧)，数据面最后启动(⑨)；各自独立 goroutine 监听；signal 触发时逆序关闭(数据面→管理面→依赖)；关闭超时 30s | 0.5d |
   120	| T-0505 | 连接管理器 | 最大连接数限制(通过 net/http LimitListener 或 semaphore)；Keep-Alive 配置；活跃连接数指标上报；连接数超限返回 503 | 0.5d |
   121	
   122	---
   123	
   124	### M-06 路由匹配与反向代理
   125	
   126	> **目标**：实现基于 radix tree 的路由匹配、路径重写、定制化反向代理与重试执行器。
   127	
   128	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
   129	|---|---|---|---|
   130	| T-0601 | 路由匹配引擎 | 基于 Gin radix tree 构建路由匹配表(从配置快照加载)；支持 Path 前缀匹配 + Method 精确匹配(GET/POST/*)；优先级排序(priority 越小越高)；匹配结果返回 Route 配置(含 target_service/rewrite_path/timeout_ms/retry/load_balance) | 1d |
   131	| T-0602 | 路径重写器 | 支持前缀截断(`strip_prefix`)、正则替换(`rewrite_path` 规则)；重写后 Path 写入请求；保留原始 Path 到上下文(供日志/指标) | 0.5d |
   132	| T-0603 | 反向代理器 | 基于 `httputil.ReverseProxy` 定制：Director 构建目标 URL(从负载均衡器获取实例地址)；注入 `traceparent` Header；设置路由级超时(`timeout_ms`)；Transport 连接池复用；错误处理(连接失败/超时) | 1.5d |
   133	| T-0604 | 重试执行器 | 触发条件：`retry_count > 0` 且状态码命中 `retry_on_status`(默认 502,503,504)或网络错误；重试时排除已失败实例重新选择；总耗时不超过 `timeout_ms`；仅幂等方法重试(GET/HEAD/OPTIONS/PUT/DELETE)，POST 需 `retry_non_idempotent=true`；最大重试 3 次；每次结果计入熔断统计 | 1.5d |
   134	| T-0605 | 路由配置热更新适配 | 路由表从 ConfigSnapshot 读取；配置变更时重建 radix tree(原子替换)；匹配无锁读 | 0.5d |
   135	
   136	---
   137	
   138	### M-07 请求/响应拦截链
   139	
   140	> **目标**：编排数据面请求处理流水线(Trace→限流→熔断→灰度→路由→代理)与响应后置处理。
   141	
   142	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
   143	|---|---|---|---|
   144	| T-0701 | 拦截链框架 | 定义 Gin Middleware 执行顺序：Trace注入 → 限流检查 → 熔断检查 → 灰度判定 → 路由匹配+重写 → 反向代理(handler)；响应后置：指标采集 → 熔断统计 → 日志 → Span结束；通过 `c.Set()` 传递上下文(route_id/target_service/start_time 等) | 1d |
   145	| T-0702 | Trace 注入中间件 | 提取 `traceparent` Header(存在则继承，不存在生成新 TraceId)；创建 SERVER Span；TraceId 写入响应 Header；上下文注入 Context；100% 生成 TraceId(基线 T-01) | 1d |
   146	| T-0703 | 限流中间件 | 匹配限流规则(route_id/path_pattern+method)；调用双层限流引擎 `Allow()`；被限流返回 429 + `X-RateLimit-*` Header + `Retry-After`；限流结果写入上下文(供日志/指标) | 0.5d |
   147	| T-0704 | 熔断中间件 | 读取熔断器状态(目标服务+路由ID)；Open 状态返回 503 + `X-Circuit-Breaker: open`；HalfOpen 走全局计数判定；Closed 放行；熔断状态写入上下文 | 0.5d |
   148	| T-0705 | 灰度中间件 | 检查灰度规则(按 route_id)；染色判定(Header/Cookie/Query 精确匹配/UserID 列表)优先；未命中则哈希比例判定(murmur3(key)%100 < gray_percent)；版本标签(stable/canary)写入上下文，服务发现按 version 过滤 | 1d |
   149	| T-0706 | 响应拦截器 | 后置处理：记录延迟直方图(`gateway_request_duration_seconds`)、状态码计数(`gateway_qps_total`)、熔断滑动窗口记录(成功/失败/慢调用)、访问日志(zap 含全字段)、结束 Span；后端延迟单独记录 | 1d |
   150	
   151	---
   152	
   153	### M-08 双层限流引擎
   154	
   155	> **目标**：实现"本地令牌桶 + Redis 批量预取"双层限流，请求路径无网络往返(P0-2 核心)。
   156	
   157	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
   158	|---|---|---|---|
   159	| T-0801 | 本地令牌桶 | 内存令牌桶实现：`tokens`(当前令牌数)、`rate`(补充速率/s)、`capacity`(容量)、`last_refill_time`；`Allow()` 方法：计算 elapsed×rate 补充 → min(capacity, tokens+refill) → tokens≥1 则扣减放行，否则拒绝；线程安全(sync.Mutex 或 atomic) | 1d |
   160	| T-0802 | 本地漏桶 | 内存漏桶实现：`level`(当前水位)、`rate`(漏水速率/s)、`capacity`(容量)；`Allow()`：漏水 elapsed×rate → max(0, level-leaked) → level<capacity 则 level+=1 放行，否则拒绝 | 0.5d |
   161	| T-0803 | Redis 批量预取协程 | 后台协程每 100ms 执行：计算本地消耗速率 → 从 Redis 全局桶批量预取(预取量=rate×0.1s×预取系数默认2) → Lua 脚本原子扣减全局桶返回实际预取量 → 补充到本地令牌桶；全局桶不足时预取剩余量 | 1.5d |
   162	| T-0804 | 全局桶补充 Lua 脚本 | 每 1s 执行：按 rate 向 Redis 全局桶补充令牌(不超过 capacity)；Lua 脚本保证原子性；Key 格式 `ratelimit:{route-id}:{alg}:{dim}:global`(Hash: tokens/last_refill_time)；每次执行后 EXPIRE 刷新 TTL=3600s(P1-4) | 1d |
   163	| T-0805 | 限流规则匹配与维度 | 支持 `limit_by` 维度：route(按路由)、ip(按客户端IP)、client_id(按 Header)；限流 Key 构建：`{route-id}:{alg}:{dim-value}`；规则匹配优先级：route_id 精确 > path_pattern 匹配 > 全局默认 | 1d |
   164	| T-0806 | 降级策略 | Redis 正常：双层模式；Redis 不可用：自动切纯本地模式(本地令牌桶独立运行，多实例近似)，恢复后自动切回；本地令牌耗尽且 Redis 预取失败：按本地剩余令牌放行，不阻塞请求；降级状态指标上报 | 0.5d |
   165	| T-0807 | 限流响应 Header | 响应写入：`X-RateLimit-Limit`(容量)、`X-RateLimit-Remaining`(剩余令牌)、`X-RateLimit-Reset`(重置时间)、`Retry-After`(重试等待秒数)；429 响应体统一格式 | 0.5d |
   166	| T-0808 | 限流引擎单元测试 | 令牌桶/漏桶单测(速率/容量/并发)；预取协程模拟测试；降级切换测试；Lua 脚本集成测试(Redis 容器)；覆盖率 ≥85% | 1.5d |
   167	
   168	---
   169	
   170	### M-09 熔断状态机(全局协调)
   171	
   172	> **目标**：实现 Closed→Open→HalfOpen 三态熔断，本地滑动窗口判断 + Redis 全局半开计数 + 状态事件广播(P1-1 核心)。
   173	
   174	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
   175	|---|---|---|---|
   176	| T-0901 | 本地滑动窗口统计器 | 时间轮+环形缓冲区实现：窗口大小 10s/10 桶(每桶 1s)；记录每桶总请求数、错误数、慢调用数(延迟>slow_call_duration_ms)；`Record(success, duration)` 方法；统计接口：总请求数、错误率、慢调用比例；线程安全 | 1.5d |
   177	| T-0902 | 熔断三态状态机 | 状态定义 CLOSED/OPEN/HALF_OPEN；CLOSED：基于滑动窗口判断触发(总请求≥min_request_count 默认20 AND (错误率≥50% OR 慢调用比例≥60%)) → 转 OPEN；OPEN：快速失败 503，到达 open_duration_ms(默认30s) → 转 HALF_OPEN；HALF_OPEN：探测请求全部成功 → CLOSED，任一失败 → OPEN | 1.5d |
   178	| T-0903 | Redis 半开全局计数 | HALF_OPEN 状态时：`INCR circuit:{service}:{route}:halfopen_count` 全局原子递增；对比阈值 half_open_max_requests(默认5)：count≤5 放行探测，count>5 快速失败；Key TTL=open_duration_ms；状态转 CLOSED/OPEN 时重置计数 | 1d |
   179	| T-0904 | 状态变更事件广播 | 状态变更时：SET `circuit:{service}:{route}:state` 到 Redis(open 状态带 TTL) + PUBLISH 事件到 channel；其他网关节点 SUBSCRIBE 接收事件 → 更新本地熔断器状态；本地缓存读优先，Redis 同步 | 1d |
   180	| T-0905 | 熔断策略参数化 | 支持按 route_id 或 target_service 配置熔断策略；参数：sliding_window_size_ms/min_request_count/error_rate_threshold/slow_call_duration_ms/slow_call_rate_threshold/open_duration_ms/half_open_max_requests/failure_status_codes；默认值与架构文档一致 | 0.5d |
   181	| T-0906 | Redis stats 聚合(仅查询) | `circuit:{service}:{route}:stats`(Hash) 记录聚合统计(仅管理面查询用，不参与熔断判断 P2-9)；定期同步本地窗口统计到 Redis；管理面查询接口读取 | 0.5d |
   182	| T-0907 | 熔断引擎单元测试 | 滑动窗口单测(桶滚动/统计精度/并发)；状态机单测(三态转移/触发条件/边界)；半开全局计数集成测试(Redis)；事件广播集成测试；覆盖率 ≥85% | 1.5d |
   183	
   184	---
   185	
   186	### M-10 灰度路由与负载均衡
   187	
   188	> **目标**：实现灰度发布路由计算(染色+哈希比例)与两种负载均衡策略(加权轮询/一致性哈希)。
   189	
   190	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
   191	|---|---|---|---|
   192	| T-1001 | 加权轮询(WRR) | 经典平滑加权轮询(nginx 算法)：每个实例 current_weight += weight，选最大者，current_weight -= total_weight；按权重分配请求；实例列表变更时重置状态 | 0.5d |
   193	| T-1002 | 一致性哈希环 | murmur3(32bit) 哈希算法；虚拟节点倍数 150；总虚拟节点数 = Σ(实例权重×150)；每个实例按权重比例分配虚拟节点；环用有序数组(sort.Search 二分查找)；请求 key(IP/指定Header)哈希后顺时针查找最近虚拟节点；实例上下线时重建环(影响约 1/N 请求) | 1.5d |
   194	| T-1003 | 负载均衡器接口 | 定义 `LoadBalancer` 接口(`Select(instances, request) (instance, error)`)；工厂方法按路由配置 `load_balance` 字段创建 WRR 或 CH 实例；CH 模式下 `hash_header` 配置指定哈希 Key Header(默认客户端 IP) | 0.5d |
   195	| T-1004 | 灰度路由计算器 | 染色判定(最高优先级)：遍历 stain_rules(JSON 数组)，支持 Header/Cookie/Query 精确匹配、UserID IN 列表；命中 → 100% 路由灰度版；未命中 → 比例判定：hash_key 默认客户端 IP，`murmur3(key) % 100 < gray_percent` → 灰度版，否则稳定版；版本标签写入上下文 | 1d |
   196	| T-1005 | 灰度规则热更新 | 灰度规则从 ConfigSnapshot 读取；配置变更时原子替换；gray_percent 运行时调整(PATCH 接口)立即生效 | 0.5d |
   197	| T-1006 | 负载均衡单元测试 | WRR 单测(权重分配精度/平滑性)；CH 单测(虚拟节点分布均匀性/节点增减影响范围/哈希命中)；灰度计算器单测(染色优先级/比例精度边界)；覆盖率 ≥85% | 1d |
   198	
   199	---
   200	
   201	### M-11 配置热更新
   202	
   203	> **目标**：实现管理面修改配置后 ≤5s 生效的热更新链路，etcd Watch + 全量拉取 + atomic.Value 原子替换。
   204	
   205	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
   206	|---|---|---|---|
   207	| T-1101 | 不可变配置快照 | 定义 `ConfigSnapshot` 结构体(路由表/限流规则/熔断策略/灰度规则/服务元信息，均为只读副本)；快照构建方法(从 DB 全量加载)；版本号标记；快照不可变(外部只能读，修改通过构建新快照替换) | 1d |
   208	| T-1102 | etcd Watch 配置变更 | 管理面写 DB 后发布 etcd 事件 Key `/config/changes/{type}/{timestamp}`(TTL=60s)；网关 Watch 该前缀接收事件；事件触发对应类型配置的全量拉取 | 1d |
   209	| T-1103 | 全量拉取与原子替换 | 收到变更事件 → 从 DB 全量拉取对应类型配置 → 构建新快照(或增量更新快照) → `atomic.Value.Store()` 原子替换(读无锁，0 失败)；新配置生效 ≤5s；日志记录 `config reloaded` | 1d |
   210	| T-1104 | 60s 兜底同步 | 每 60s 定时全量同步所有配置(即使 Watch 事件丢失也能收敛)；对比版本号，无变更则跳过；兜底同步与 Watch 触发的同步互斥(避免重复构建) | 0.5d |
   211	| T-1105 | 配置热更新管理接口 | POST `/api/v1/system/reload`(手动触发全量重载)；GET `/api/v1/system/config`(查看当前快照版本与摘要)；热更新失败返回 50002 错误码 | 0.5d |
   212	
   213	---
   214	
   215	### M-12 管理面 API
   216	
   217	> **目标**：实现全套管理面 REST API，含 Token 鉴权、操作人解析、操作审计。
   218	
   219	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
   220	|---|---|---|---|
   221	| T-1201 | Token 鉴权中间件 | 管理面请求 Header `Authorization: Bearer {token}`；支持多 Token 配置(tokens 列表，每个 token 映射 username)；兼容单一 token 配置 + default_username；Token 无效返回 40100；管理面 8081 端口网络隔离(回环映射/NetworkPolicy) | 1d |
   222	| T-1202 | 操作人解析 | 解析优先级：① Token 映射的 username → ② `X-Operator` Header → ③ default_username；操作人写入 `gw_operation_log.operator` 与 `gw_config_version.operator`；上下文传递操作人 | 0.5d |
   223	| T-1203 | 路由管理 API | POST 创建 / GET 列表(分页+筛选) / GET ID / PUT 更新 / DELETE 逻辑删除 / PATCH 启用禁用；路径+方法唯一校验(40101)；目标服务存在校验(40102)；创建/更新后触发热更新事件 | 1.5d |
   224	| T-1204 | 限流规则 API | POST / GET(list) / GET(id) / PUT / DELETE / PATCH status；算法校验(token_bucket/leaky_bucket，40201)；rate>0 校验(40202)；关联路由或 path_pattern 二选一 | 1d |
   225	| T-1205 | 熔断策略 API | POST / GET(list) / GET(id) / PUT / DELETE；错误率阈值 1-100 校验(40301)；GET `/circuit-breakers/{id}/state` 查看当前熔断状态；POST `/circuit-breakers/{id}/reset` 手动重置熔断器(转 CLOSED) | 1d |
   226	| T-1206 | 灰度发布 API | POST / GET(list) / GET(id) / PUT / DELETE；灰度比例 0-100 校验(40401)；PATCH `/gray-releases/{id}/percent` 运行时调整比例；stain_rules JSON 校验 | 1d |
   227	| T-1207 | 服务管理 API | POST / GET(list) / GET(name) / PUT(name) / DELETE(name)；服务名唯一校验(40501)；GET `/services/{name}/instances` 查看当前注册实例列表(从 etcd 缓存读取，含健康状态/地址/权重/版本) | 1d |
   228	| T-1208 | 操作审计 API | GET `/api/v1/operation-logs`(分页查询，按操作人/资源类型/时间范围筛选)；GET `/api/v1/config-versions`(配置变更历史，按 config_type/config_id 查询，含 before/after 快照)；敏感字段脱敏展示 | 0.5d |
   229	| T-1209 | 操作日志拦截器 | Gin 中间件自动记录管理面操作：trace_id/operator/operation/resource_type/resource_id/request_method/request_path/request_body(脱敏)/response_code/client_ip/duration_ms；异步写入 DB(不阻塞请求) | 0.5d |
   230	| T-1210 | 管理面 API 集成测试 | 各 CRUD 接口集成测试(SQLite+etcd mock)；鉴权测试(有效/无效/缺失 Token)；操作人解析测试；审计日志验证；参数校验与错误码测试 | 1.5d |
   231	
   232	---
   233	
   234	### M-13 可观测性
   235	
   236	> **目标**：实现 Prometheus 指标、OpenTelemetry 链路追踪、zap 结构化日志(含脱敏)三大可观测能力。
   237	
   238	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
   239	|---|---|---|---|
   240	| T-1301 | Prometheus 指标定义 | 定义全部指标(对齐基线 P2-1)：`gateway_qps_total`(Counter, route/service/method/status 标签)、`gateway_request_duration_seconds`(Histogram)、`gateway_error_rate`(Gauge)、限流 `gateway_rate_limit_total/passed/rejected`、熔断 `gateway_circuit_breaker_state/open_total/rejected_total`、灰度 `gateway_gray_requests_total`、服务发现 `gateway_service_instances/discovery_events_total`、系统 `gateway_memory_usage_bytes/goroutines/active_connections/config_reload_total/config_reload_duration_seconds` | 1d |
   241	| T-1302 | /metrics 端点与指标采集 | 管理面 8081 暴露 `/metrics`(promhttp.Handler)；请求路径中埋点(响应拦截器记录 QPS/延迟/状态码)；限流/熔断/灰度模块各自埋点；系统指标(goroutine/内存/活跃连接)定时采集 | 1d |
   242	| T-1303 | OpenTelemetry 链路追踪 | 集成 OTel Go SDK v1.28.0；TraceProvider 配置：本地 stdout exporter(全量采样)，生产 OTLP exporter(默认 10% 采样，5xx 强制采样 P2-4)；TraceId 100% 生成透传(基线 T-01)；网关创建 SERVER Span；反向代理注入 `traceparent` Header 到后端；Propagator 配置(W3C TraceContext) | 1.5d |
   243	| T-1304 | zap 结构化日志 | 访问日志必含字段：timestamp/level/logger/message/trace_id/span_id/client_ip/method/path/route_id/target_service/target_instance/status/latency_ms/backend_latency_ms/rate_limited/circuit_state/gray_version；异步写入(不阻塞请求路径)；日志级别可配置；JSON 格式 | 1d |
   244	| T-1305 | 敏感字段脱敏 | 脱敏规则(CO-02)：password→`******`；token→前4后4；id_card→前6后4；phone→前3后4；email→首字符+域名；cookie/authorization→`[REDACTED]`；日志中间件与操作日志统一应用脱敏；脱敏工具函数单测 | 0.5d |
   245	| T-1306 | 可观测性集成验证 | 本地启动后 `/metrics` 返回全部指标；TraceId 在日志/响应 Header/后端 Span 中一致传递；Jaeger(可选)能看到完整调用链；脱敏验证(发送含敏感字段请求，检查日志) | 0.5d |
   246	
   247	---
   248	
   249	### M-14 测试服务集群
   250	
   251	> **目标**：构建 5 个测试服务，集成 OTel SDK、数据库、etcd 注册，用于端到端验证(P1-6)。
   252	
   253	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
   254	|---|---|---|---|
   255	| T-1401 | 测试服务基础框架 | 公共库：etcd 注册封装(Lease TTL=10s + KeepAlive)、OTel SDK 初始化(提取 traceparent 创建 SERVER Span)、`/health` 端点(返回 200)、业务端点模板(返回含 TraceId 的 JSON)；可复用基础包 | 1d |
   256	| T-1402 | 测试服务 A/B(带数据库) | service-a、service-b：连接 SQLite/MySQL；业务端点执行查询时 SQL 注释注入 TraceId：`/* trace_id=xxx */ SELECT ...`；返回查询结果 + TraceId；不同端口/服务名 | 1d |
   257	| T-1403 | 测试服务 C/D/E(纯HTTP) | service-c、service-d、service-e：纯 HTTP 服务，提供 `/echo`(回显请求)、`/slow`(模拟慢调用，可配置延迟)、`/error`(模拟 500 错误，可配置概率)、`/health`；用于熔断/限流/灰度测试 | 1d |
   258	| T-1404 | 测试服务 Dockerfile 与配置 | 每个服务独立 Dockerfile(golang:1.22-alpine 构建 + alpine 运行)；环境变量 `SERVICE_NAME`/`SERVICE_PORT`/`ETCD_ENDPOINTS`；docker-compose 中定义 5 个服务(各 mem_limit=64m) | 0.5d |
   259	| T-1405 | 测试服务多版本支持 | service-a 支持 v1/v2 两个版本(通过环境变量 `SERVICE_VERSION`，返回不同响应体标识版本)；用于灰度发布验证(canary 版本 vs stable 版本) | 0.5d |
   260	
   261	---
   262	
   263	### M-15 部署与编排
   264	
   265	> **目标**：提供本地 Docker Compose 一键启动与生产 K8s 部署清单。
   266	
   267	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
   268	|---|---|---|---|
   269	| T-1501 | 网关 Dockerfile | 多阶段构建：golang:1.22-alpine builder(`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w"`) → distroless/static-debian12:nonroot 运行；`/data` 目录创建并 chown nonroot；HEALTHCHECK 纯 exec 形式 `CMD ["/app/gateway", "healthcheck"]`(P2-10)；EXPOSE 8080 8081；镜像 ≤100MB | 1d |
   270	| T-1502 | docker-compose.yml | 服务定义：gateway(端口 8080:8080 + 127.0.0.1:8081:8081，mem_limit=256m，depends_on etcd/redis healthy)、etcd(bitnami/etcd:3.5.15，ETCDCTL_API=3，healthcheck etcdctl endpoint health)、redis(7.2-alpine，healthcheck ping)、5×测试服务；volumes gateway-data；全套环境变量配置 | 1d |
   271	| T-1503 | 配置文件模板 | `config/default.yaml`：完整配置注释(etcd endpoints/redis addr/config_store driver+dsn/server 端口/admin auth tokens/log level/trace sampler)；环境变量覆盖示例；生产配置模板 `config/production.yaml`(MySQL DSN/OTLP endpoint/采样率) | 0.5d |
   272	| T-1504 | K8s 部署清单 | Gateway Deployment(多副本，资源 limits requests)、数据面 Service(8080，LoadBalancer/NodePort)、管理面 Service(8081，ClusterIP + NetworkPolicy 限制运维命名空间)、ConfigMap(配置文件)、Secret(Token/DB 密码)、etcd StatefulSet(3 节点)、Redis Sentinel | 1.5d |
   273	| T-1505 | HPA 与 Prometheus Adapter | HPA v2：minReplicas=3/maxReplicas=20，CPU 70%/内存 80%/自定义指标 `gateway_qps_per_pod` AverageValue=8000；扩缩容行为(scaleUp 60s 稳定窗/50%，scaleDown 300s/10%)；Prometheus Adapter Deployment + ConfigMap(seriesQuery `gateway_qps_total` → `gateway_qps_per_pod`，metricsQuery sum(rate)) | 1d |
   274	| T-1506 | 告警规则 | Prometheus AlertManager 规则：`GatewayDown`(up==0 for 1m，P0)、`GatewayHighErrorRate`(>50% for 1m，P0)、`GatewayHighLatency`(P99>0.1s for 5m，P1)、`GatewayHighMemory`(>100MB for 5m，P1)、`CircuitBreakerOpen`(P2)、`RateLimitHighRejection`(>10% for 5m，P2)、`ConfigReloadFailed`(P3)；告警分级响应时间 | 0.5d |
   275	| T-1507 | 本地一键启动验证 | `docker-compose up -d` 一键启动全套环境(gateway+etcd+redis+5测试服务)；验证 gateway /healthz 返回 200；验证测试服务注册到 etcd；验证通过 gateway 转发请求到后端成功；内存 ≤2GB | 0.5d |
   276	
   277	---
   278	
   279	### M-16 性能测试与集成验收
   280	
   281	> **目标**：验证系统满足基线性能指标(QPS≥10000/延迟<2ms/内存≤80MB)与功能正确性。
   282	
   283	| 任务ID | 任务名称 | 功能点描述 | 预估工时 |
   284	|---|---|---|---|
   285	| T-1601 | wrk 性能基准测试 | 测试脚本：`wrk -t4 -c100 -d60s` 压测数据面；指标采集：QPS/平均延迟/P99/P999/内存占用；目标：QPS≥10000、平均延迟<2ms、内存≤80MB；多轮测试取稳定值；输出性能报告 | 1d |
   286	| T-1602 | 限流功能验证 | 配置 rate=100/s 路由；wrk 压测验证 QPS 被限制在 ~100；验证 429 响应与 `X-RateLimit-*` Header；多实例限流精度验证(3 实例总 QPS≈配置值)；Redis 故障降级验证(限流继续生效) | 0.5d |
   287	| T-1603 | 熔断功能验证 | 后端模拟 100% 错误；验证 20 请求后熔断器打开(后续请求 503 + X-Circuit-Breaker: open)；30s 后半开探测；探测成功恢复 CLOSED，失败重新 OPEN；多实例半开探测总数 ≤5(全局计数)；状态变更事件广播验证 | 0.5d |
   288	| T-1604 | 灰度发布验证 | 部署 service-a v1(stable)+v2(canary)；配置灰度规则 gray_percent=20；使用 ≥100 个不同 `X-Forwarded-For` 发送 1000 请求(P2-5)；验证 canary 接收量 180-220(18%-22%)；染色规则验证(特定 Header 100% 路由 canary)；percent 运行时调整立即生效 | 0.5d |
   289	| T-1605 | 配置热更新验证 | 通过管理面 API 创建/修改/删除路由；验证 ≤5s 后数据面新配置生效；etcd Watch 事件丢失模拟(停止 etcd 再恢复)→ 60s 兜底同步收敛；多实例配置一致性验证(同时修改，所有实例 ≤60s 收敛) | 0.5d |
   290	| T-1606 | 服务发现与健康检查验证 | 启动/停止测试服务实例；验证网关 ≤10s 感知实例上下线；健康检查：实例 /health 返回 500 → 连续 2 次失败后摘除；恢复后连续 2 次成功重新加入；etcd 故障时本地缓存兜底 | 0.5d |
   291	| T-1607 | 全链路追踪验证 | 请求经过网关→后端→DB；验证 TraceId 在网关日志/响应 Header/后端日志/DB SQL 注释中一致；Jaeger 查看完整调用链(网关 Span + 后端 Span)；5xx 错误强制采样验证 | 0.5d |
   292	| T-1608 | 双驱动兼容 CI | GitHub Actions / 本地脚本：SQLite 模式跑全部单元测试 + 集成测试；MySQL 模式跑全部测试；验证切换 driver 配置后功能一致；`CGO_ENABLED=0` 编译验证 | 0.5d |
   293	| T-1609 | 端到端集成测试 | 编写 e2e 测试套件：完整请求转发链路(含限流/熔断/灰度/重试)；管理面全流程(创建路由→配置限流→发请求验证→修改配置→验证热更新→删除)；故障注入(etcd/Redis/后端故障)验证降级 | 1.5d |
   294	
   295	---
   296	
   297	## 3. 优先级与依赖关系
   298	
   299	### 3.1 优先级分级定义
   300	
   301	| 级别 | 定义 | 交付要求 |
   302	|---|---|---|
   303	| **P0** | 核心必做，没有则系统无法运行或不满足基线硬约束 | 第一迭代必须完成，阻塞后续所有开发 |
   304	| **P1** | 重要功能，核心治理能力与可观测性，影响系统完整性 | 第二迭代完成，P0 完成后可并行 |
   305	| **P2** | 优化增强，生产级运维能力与非核心特性 | 第三迭代完成，不阻塞核心功能验证 |
   306	
   307	### 3.2 任务优先级总表
   308	
   309	#### P0 任务（核心必做，共 22 项）
   310	
   311	| 任务ID | 任务名称 | 所属模块 | 依赖 |
   312	|---|---|---|---|
   313	| T-0101 | 初始化 Go Module 与目录结构 | M-01 | 无 |
   314	| T-0102 | 配置管理(viper) | M-01 | T-0101 |
   315	| T-0103 | 日志系统(zap) | M-01 | T-0101 |
   316	| T-0104 | 统一响应与错误码 | M-01 | T-0101 |
   317	| T-0105 | 启动初始化框架 | M-01 | T-0102,T-0103 |
   318	| T-0201 | GORM 集成与双驱动 | M-02 | T-0102 |
   319	| T-0202 | 核心表 Model 定义 | M-02 | T-0201 |
   320	| T-0203 | AutoMigrate 与建表验证 | M-02 | T-0202 |
   321	| T-0204 | Route Repository | M-02 | T-0203 |
   322	| T-0301 | etcd 客户端封装 | M-03 | T-0102 |
   323	| T-0303 | 服务发现器(Discovery) | M-03 | T-0301 |
   324	| T-0401 | Redis 客户端封装 | M-04 | T-0102 |
   325	| T-0501 | 数据面 Gin Engine | M-05 | T-0104 |
   326	| T-0502 | 管理面 Gin Engine | M-05 | T-0104 |
   327	| T-0503 | 健康检查端点 | M-05 | T-0501,T-0502 |
   328	| T-0601 | 路由匹配引擎 | M-06 | T-0204 |
   329	| T-0603 | 反向代理器 | M-06 | T-0303,T-1001 |
   330	| T-0701 | 拦截链框架 | M-07 | T-0501 |
   331	| T-0702 | Trace 注入中间件 | M-07 | T-0701 |
   332	| T-1001 | 加权轮询(WRR) | M-10 | T-0303 |
   333	| T-1501 | 网关 Dockerfile | M-15 | T-0105 |
   334	| T-1502 | docker-compose.yml | M-15 | T-1501 |
   335	
   336	#### P1 任务（重要功能，共 33 项）
   337	
   338	| 任务ID | 任务名称 | 所属模块 | 依赖 |
   339	|---|---|---|---|
   340	| T-0106 | 通用工具库 | M-01 | T-0101 |
   341	| T-0205 | RateLimit Repository | M-02 | T-0203 |
   342	| T-0206 | CircuitBreaker Repository | M-02 | T-0203 |
   343	| T-0207 | GrayRelease Repository | M-02 | T-0203 |
   344	| T-0208 | Service Repository | M-02 | T-0203 |
   345	| T-0209 | 审计 Repository | M-02 | T-0203 |
   346	| T-0210 | 存储层单元测试 | M-02 | T-0204~T-0209 |
   347	| T-0302 | 服务注册器(Registry) | M-03 | T-0301 |
   348	| T-0304 | 健康检查器 | M-03 | T-0303 |
   349	| T-0305 | 实例缓存与过滤 | M-03 | T-0303 |
   350	| T-0306 | etcd 降级与重连 | M-03 | T-0303 |
   351	| T-0402 | Lua 脚本执行器 | M-04 | T-0401 |
   352	| T-0403 | Redis 降级策略 | M-04 | T-0401 |
   353	| T-0504 | 双 Server 启动与优雅关闭 | M-05 | T-0503,T-0105 |
   354	| T-0505 | 连接管理器 | M-05 | T-0501 |
   355	| T-0602 | 路径重写器 | M-06 | T-0601 |
   356	| T-0604 | 重试执行器 | M-06 | T-0603 |
   357	| T-0605 | 路由配置热更新适配 | M-06 | T-0601,T-1103 |
   358	| T-0703 | 限流中间件 | M-07 | T-0701,T-0805 |
   359	| T-0704 | 熔断中间件 | M-07 | T-0701,T-0902 |
   360	| T-0705 | 灰度中间件 | M-07 | T-0701,T-1004 |
   361	| T-0706 | 响应拦截器 | M-07 | T-0701,T-1301 |
   362	| T-0801 | 本地令牌桶 | M-08 | T-0401 |
   363	| T-0803 | Redis 批量预取协程 | M-08 | T-0801,T-0402 |
   364	| T-0804 | 全局桶补充 Lua 脚本 | M-08 | T-0402 |
   365	| T-0805 | 限流规则匹配与维度 | M-08 | T-0801,T-0205 |
   366	| T-0901 | 本地滑动窗口统计器 | M-09 | T-0401 |
   367	| T-0902 | 熔断三态状态机 | M-09 | T-0901 |
   368	| T-0903 | Redis 半开全局计数 | M-09 | T-0902,T-0402 |
   369	| T-0904 | 状态变更事件广播 | M-09 | T-0902 |
   370	| T-0905 | 熔断策略参数化 | M-09 | T-0902,T-0206 |
   371	| T-1002 | 一致性哈希环 | M-10 | T-0303 |
   372	| T-1003 | 负载均衡器接口 | M-10 | T-1001,T-1002 |
   373	| T-1004 | 灰度路由计算器 | M-10 | T-0207 |
   374	| T-1101 | 不可变配置快照 | M-11 | T-0204 |
   375	| T-1102 | etcd Watch 配置变更 | M-11 | T-0301,T-1101 |
   376	| T-1103 | 全量拉取与原子替换 | M-11 | T-1101,T-1102 |
   377	| T-1104 | 60s 兜底同步 | M-11 | T-1103 |
   378	| T-1201 | Token 鉴权中间件 | M-12 | T-0502 |
   379	| T-1202 | 操作人解析 | M-12 | T-1201 |
   380	| T-1203 | 路由管理 API | M-12 | T-0204,T-1201,T-1102 |
   381	| T-1209 | 操作日志拦截器 | M-12 | T-0209,T-1202 |
   382	| T-1301 | Prometheus 指标定义 | M-13 | T-0103 |
   383	| T-1302 | /metrics 端点与指标采集 | M-13 | T-1301,T-0502 |
   384	| T-1303 | OpenTelemetry 链路追踪 | M-13 | T-0103 |
   385	| T-1304 | zap 结构化日志 | M-13 | T-0103,T-1303 |
   386	| T-1305 | 敏感字段脱敏 | M-13 | T-1304 |
   387	| T-1401 | 测试服务基础框架 | M-14 | T-0302 |
   388	| T-1402 | 测试服务 A/B(带数据库) | M-14 | T-1401 |
   389	| T-1403 | 测试服务 C/D/E(纯HTTP) | M-14 | T-1401 |
   390	| T-1404 | 测试服务 Dockerfile 与配置 | M-14 | T-1402,T-1403 |
   391	| T-1503 | 配置文件模板 | M-15 | T-0102 |
   392	| T-1507 | 本地一键启动验证 | M-15 | T-1502,T-1404 |
   393	
   394	#### P2 任务（优化增强，共 17 项）
   395	
   396	| 任务ID | 任务名称 | 所属模块 | 依赖 |
   397	|---|---|---|---|
   398	| T-0802 | 本地漏桶 | M-08 | T-0801 |
   399	| T-0806 | 降级策略 | M-08 | T-0803,T-0403 |
   400	| T-0807 | 限流响应 Header | M-08 | T-0801 |
   401	| T-0808 | 限流引擎单元测试 | M-08 | T-0801~T-0807 |
   402	| T-0906 | Redis stats 聚合(仅查询) | M-09 | T-0902 |
   403	| T-0907 | 熔断引擎单元测试 | M-09 | T-0901~T-0906 |
   404	| T-1005 | 灰度规则热更新 | M-10 | T-1004,T-1103 |
   405	| T-1006 | 负载均衡单元测试 | M-10 | T-1003,T-1004 |
   406	| T-1105 | 配置热更新管理接口 | M-11 | T-1103 |
   407	| T-1204 | 限流规则 API | M-12 | T-0205,T-1201 |
   408	| T-1205 | 熔断策略 API | M-12 | T-0206,T-1201,T-0902 |
   409	| T-1206 | 灰度发布 API | M-12 | T-0207,T-1201 |
   410	| T-1207 | 服务管理 API | M-12 | T-0208,T-1201,T-0303 |
   411	| T-1208 | 操作审计 API | M-12 | T-0209,T-1201 |
   412	| T-1210 | 管理面 API 集成测试 | M-12 | T-1203~T-1209 |
   413	| T-1306 | 可观测性集成验证 | M-13 | T-1301~T-1305 |
   414	| T-1405 | 测试服务多版本支持 | M-14 | T-1402 |
   415	| T-1504 | K8s 部署清单 | M-15 | T-1503 |
   416	| T-1505 | HPA 与 Prometheus Adapter | M-15 | T-1504,T-1301 |
   417	| T-1506 | 告警规则 | M-15 | T-1301 |
   418	| T-1601~T-1609 | 性能测试与集成验收(9项) | M-16 | 全部 P0/P1 |
   419	
   420	### 3.3 开发迭代计划
   421	
   422	```
   423	迭代一（P0，约 2 周）：可运行的最小网关
   424	├── 第1-3天：M-01 项目骨架（T-0101~T-0105）
   425	├── 第3-5天：M-02 配置存储基础（T-0201~T-0204）+ M-03 etcd（T-0301,T-0303）+ M-04 Redis（T-0401）
   426	├── 第5-7天：M-05 接入层（T-0501~T-0503）+ M-10 WRR（T-1001）
   427	├── 第7-10天：M-06 路由+代理（T-0601,T-0603）+ M-07 拦截链基础（T-0701,T-0702）
   428	└── 第10-14天：M-15 Docker（T-1501,T-1502）+ 联调验证（请求可转发）
   429	
   430	迭代二（P1，约 3 周）：核心治理能力
   431	├── 第1-5天：M-08 双层限流（T-0801,T-0803~T-0805）+ M-09 熔断（T-0901~T-0905）
   432	├── 第3-7天：M-10 一致性哈希+灰度（T-1002~T-1004）+ M-07 治理中间件（T-0703~T-0706）
   433	├── 第5-9天：M-11 配置热更新（T-1101~T-1104）+ M-06 重写+重试（T-0602,T-0604,T-0605）
   434	├── 第7-12天：M-12 管理面 API（T-1201~T-1203,T-1209）+ M-02 剩余 Repository（T-0205~T-0210）
   435	├── 第10-15天：M-13 可观测性（T-1301~T-1305）+ M-03 健康检查+降级（T-0304~T-0306）
   436	├── 第12-18天：M-14 测试服务（T-1401~T-1404）+ M-05 优雅关闭+连接管理（T-0504,T-0505）
   437	└── 第18-21天：M-15 本地一键启动（T-1503,T-1507）+ 全链路联调
   438	
   439	迭代三（P2，约 2 周）：生产级增强与验收
   440	├── 第1-4天：M-08/M-09/M-10 单元测试（T-0808,T-0907,T-1006）+ 漏桶/降级/Header（T-0802,T-0806,T-0807）
   441	├── 第3-7天：M-12 剩余 API（T-1204~T-1208,T-1210）+ M-11 管理接口（T-1105）
   442	├── 第5-9天：M-15 K8s+HPA+告警（T-1504~T-1506）+ M-14 多版本（T-1405）+ M-13 验证（T-1306）
   443	└── 第7-14天：M-16 性能测试与集成验收（T-1601~T-1609）+ 性能优化 + 文档收尾
   444	```
   445	
   446	### 3.4 关键依赖路径（关键路径）
   447	
   448	```
   449	T-0101 → T-0102 → T-0201 → T-0202 → T-0203 → T-0204 → T-0601 → T-0603 → T-0701 → T-0702
   450	                                                                              ↓
   451	T-0101 → T-0103 → T-1301 → T-1302 → T-0706（响应拦截依赖指标）
   452	                                        ↓
   453	T-0801 → T-0805 → T-0703（限流中间件）
   454	T-0901 → T-0902 → T-0704（熔断中间件）
   455	T-1004 → T-0705（灰度中间件）
   456	                                        ↓
   457	T-1101 → T-1102 → T-1103 → T-1203（路由API触发热更新）
   458	                                        ↓
   459	T-1501 → T-1502 → T-1507（本地可运行）
   460	                                        ↓
   461	T-1601~T-1609（最终验收）
   462	```
   463	
   464	---
   465	
   466	## 4. 开发规范
   467	
   468	### 4.1 代码规范
   469	
   470	#### 4.1.1 语言与版本
   471	
   472	| 项 | 规范 |
   473	|---|---|
   474	| Go 版本 | 1.22（基线 TC-01 硬约束） |
   475	| 编译模式 | `CGO_ENABLED=0`（P0-1，modernc.org/sqlite 纯 Go） |
   476	| 构建目标 | `GOOS=linux GOARCH=amd64`（容器） |
   477	| 代码格式化 | `gofmt -s` + `goimports`（CI 强制检查） |
   478	| Lint | `golangci-lint`（启用 errcheck/gosimple/govet/ineffassign/staticcheck/typecheck） |
   479	| 依赖管理 | Go Modules，禁止 `vendor/` 提交，`go.sum` 必须提交 |
   480	
   481	#### 4.1.2 项目结构规范
   482	
   483	```
   484	go-api-gateway/
   485	├── cmd/
   486	│   └── gateway/
   487	│       └── main.go              # 入口，仅做初始化调用
   488	├── internal/                     # 私有包，不对外暴露
   489	│   ├── app/                     # 应用组装与启动时序
   490	│   ├── config/                  # 配置结构体与加载
   491	│   ├── handler/                 # Gin Handler（管理面 API）
   492	│   ├── middleware/              # Gin 中间件
   493	│   ├── model/                   # GORM Model 定义
   494	│   ├── repository/              # 数据访问层
   495	│   ├── service/                 # 业务逻辑层
   496	│   ├── proxy/                   # 反向代理与重试
   497	│   ├── ratelimit/               # 限流引擎
   498	│   ├── circuitbreaker/          # 熔断引擎
   499	│   ├── loadbalance/             # 负载均衡(WRR/CH)
   500	│   ├── gray/                    # 灰度路由
   501	│   ├── registry/                # etcd 注册与发现
   502	│   ├── store/                   # Redis/etcd/DB 客户端封装
   503	│   ├── observability/           # 指标/链路/日志
   504	│   └── pkg/                     # 内部通用工具
   505	├── pkg/                          # 可对外暴露的公共包(如有)
   506	├── config/
   507	│   ├── default.yaml             # 默认配置
   508	│   └── production.yaml          # 生产配置模板
   509	├── deploy/
   510	│   ├── docker/                  # Dockerfile
   511	│   ├── compose/                 # docker-compose.yml
   512	│   └── k8s/                     # K8s 清单
   513	├── test-services/               # 测试服务集群
   514	├── docs/                        # 文档
   515	├── scripts/                     # 构建/测试/部署脚本
   516	├── go.mod
   517	├── go.sum
   518	├── Makefile
   519	└── .golangci.yml
   520	```
   521	
   522	#### 4.1.3 分层调用规范
   523	
   524	```
   525	Handler(HTTP) → Service(业务逻辑) → Repository(数据访问) → DB/Redis/etcd
   526	     ↓                ↓                    ↓
   527	  中间件           领域模型              Model
   528	```
   529	
   530	- **Handler 层**：只做参数绑定、校验、调用 Service、组装响应；不写业务逻辑
   531	- **Service 层**：核心业务逻辑，事务边界，调用多个 Repository 或外部客户端
   532	- **Repository 层**：纯数据访问，GORM/Redis/etcd 操作，不包含业务逻辑
   533	- **禁止跨层调用**：Handler 不得直接调用 Repository；Repository 不得调用 Service
   534	- **依赖注入**：通过构造函数注入依赖，不使用全局变量（除 logger.L 等基础单例）
   535	
   536	#### 4.1.4 错误处理规范
   537	
   538	```go
   539	// 1. 错误必须处理，禁止 _ 忽略（除非明确注释原因）
   540	result, err := doSomething()
   541	if err != nil {
   542	    return fmt.Errorf("failed to do something: %w", err)  // %w 包装错误
   543	}
   544	
   545	// 2. Service 层返回 domain error，Handler 层映射为 API 错误码
   546	var ErrRouteNotFound = errors.New("route not found")
   547	
   548	// 3. 不使用 panic 处理业务错误（仅初始化失败可用 panic）
   549	// 4. goroutine 中的错误必须通过 channel 或日志上报，禁止静默丢失
   550	```
   551	
   552	#### 4.1.5 并发规范
   553	
   554	```go
   555	// 1. 共享状态用 sync.Mutex / sync.RWMutex / atomic / sync.Once / sync.Pool
   556	// 2. 配置快照用 atomic.Value（读无锁）
   557	var configSnapshot atomic.Value  // 存储 *ConfigSnapshot
   558	
   559	// 3. goroutine 必须有明确的生命周期管理（context 取消 / WaitGroup / stop channel）
   560	ctx, cancel := context.WithCancel(context.Background())
   561	defer cancel()
   562	go worker(ctx)
   563	
   564	// 4. 禁止 goroutine 泄漏：启动 goroutine 必须能被优雅关闭
   565	// 5. channel 发送方负责关闭，接收方不关闭 channel
   566	```
   567	
   568	#### 4.1.6 性能规范
   569	
   570	- 请求路径禁止同步 I/O（DB/Redis/etcd 网络调用），必须走本地缓存/内存
   571	- 热路径禁止 `fmt.Sprintf` 拼接字符串（用 `strings.Builder` 或预分配）
   572	- 热路径禁止反射（`reflect`）
   573	- 使用 `sync.Pool` 复用高频分配对象（如 `http.Request` 包装、缓冲区）
   574	- 连接池上限必须配置（DB/Redis/HTTP Client）
   575	- 日志异步写入，禁止请求路径同步写磁盘
   576	- `GOGC`/`GOMEMLIMIT` 可配置，默认 `GOGC=100`
   577	
   578	### 4.2 命名规范
   579	
   580	#### 4.2.1 包命名
   581	
   582	| 规则 | 示例 |
   583	|---|---|
   584	| 全小写，无下划线 | `ratelimit`、`circuitbreaker`、`loadbalance` |
   585	| 简洁且见名知意 | `registry`（不用 `service_registry`） |
   586	| 不与标准库重名 | 不用 `config` 作为包名时注意与标准库区分，用 `appconfig` |
   587	| 测试包 `_test` 后缀 | `ratelimit_test` |
   588	
   589	#### 4.2.2 文件命名
   590	
   591	| 类型 | 规范 | 示例 |
   592	|---|---|---|
   593	| Go 源文件 | 小写蛇形 | `rate_limiter.go`、`token_bucket.go` |
   594	| 测试文件 | `_test.go` 后缀 | `rate_limiter_test.go` |
   595	| 配置文件 | 小写蛇形 | `default.yaml` |
   596	| 部署文件 | 小写连字符 | `deployment.yaml`、`docker-compose.yml` |
   597	| 文档 | 中文标题或英文蛇形 | `任务拆解与开发规范.md` |
   598	
   599	#### 4.2.3 结构体与接口命名
   600	
   601	```go
   602	// 结构体：大驼峰，名词
   603	type TokenBucket struct { ... }
   604	type CircuitBreaker struct { ... }
   605	type RouteRepository struct { ... }
   606	
   607	// 接口：大驼峰，名词或以 -er 结尾（行为接口）
   608	type Loader interface { ... }
   609	type Discovery interface { ... }
   610	type HealthChecker interface { ... }
   611	
   612	// 接口方法：大驼峰，动词或动词短语
   613	type Repository interface {
   614	    Create(ctx context.Context, m *Model) error
   615	    GetByID(ctx context.Context, id string) (*Model, error)
   616	    List(ctx context.Context, filter Filter) ([]*Model, int64, error)
   617	    Update(ctx context.Context, m *Model) error
   618	    Delete(ctx context.Context, id string) error
   619	}
   620	```
   621	
   622	#### 4.2.4 变量与常量命名
   623	
   624	```go
   625	// 局部变量：小驼峰
   626	var routeConfig *Route
   627	var maxRetries int
   628	
   629	// 常量：大驼峰（Go 惯例，不用全大写+下划线）
   630	const DefaultRate = 100
   631	const MaxRetryCount = 3
   632	
   633	// 错误变量：Err 前缀
   634	var ErrRouteNotFound = errors.New("route not found")
   635	var ErrRateLimitExceeded = errors.New("rate limit exceeded")
   636	
   637	// 布尔变量：is/has/can/should 前缀
   638	var isEnabled bool
   639	var hasFailed bool
   640	
   641	// 接收者：1-2 个字母，对应类型首字母
   642	func (rb *TokenBucket) Allow() bool { ... }
   643	func (cb *CircuitBreaker) Record(success bool, duration time.Duration) { ... }
   644	```
   645	
   646	#### 4.2.5 数据库命名
   647	
   648	| 项 | 规范 | 示例 |
   649	|---|---|---|
   650	| 表名 | 小写蛇形，`gw_` 前缀 | `gw_route`、`gw_rate_limit` |
   651	| 字段名 | 小写蛇形 | `route_name`、`target_service`、`deleted_at` |
   652	| 主键 | `id`，UUID v4，`VARCHAR(36)` | - |
   653	| 普通索引 | `idx_{表}_{字段}` | `idx_route_target_service` |
   654	| 唯一索引 | `uk_{表}_{字段}` | `uk_route_path_method` |
   655	| 时间字段 | UTC 存储，`DATETIME` | `created_at`、`updated_at` |
   656	| 逻辑删除 | `deleted_at DATETIME NULL` | NULL=未删除 |
   657	
   658	#### 4.2.6 Redis Key 命名
   659	
   660	| 类型 | 格式 | 示例 |
   661	|---|---|---|
   662	| 限流全局桶 | `ratelimit:{route-id}:{alg}:{dim}:global` | `ratelimit:abc-123:token_bucket:route:global` |
   663	| 熔断状态 | `circuit:{service}:{route}:state` | `circuit:user-service:abc-123:state` |
   664	| 熔断半开计数 | `circuit:{service}:{route}:halfopen_count` | `circuit:user-service:abc-123:halfopen_count` |
   665	| 熔断统计 | `circuit:{service}:{route}:stats` | `circuit:user-service:abc-123:stats` |
   666	| 冒号分隔，全小写 | - | - |
   667	
   668	#### 4.2.7 etcd Key 命名
   669	
   670	| 类型 | 格式 |
   671	|---|---|
   672	| 服务实例 | `/services/{service-name}/{instance-id}` |
   673	| 配置变更事件 | `/config/changes/{config-type}/{timestamp}` |
   674	
   675	#### 4.2.8 API 命名
   676	
   677	| 项 | 规范 | 示例 |
   678	|---|---|---|
   679	| 路径 | kebab-case，复数名词 | `/api/v1/routes`、`/api/v1/rate-limits` |
   680	| 版本 | URL 前缀 | `/api/v1/` |
   681	| 方法 | RESTful | GET/POST/PUT/DELETE/PATCH |
   682	| 字段 | snake_case | `route_name`、`target_service` |
   683	| 分页参数 | `page`(从1) + `page_size`(默认20,最大100) | - |
   684	
   685	#### 4.2.9 指标命名
   686	
   687	| 项 | 规范 | 示例 |
   688	|---|---|---|
   689	| 前缀 | `gateway_` | `gateway_qps_total` |
   690	| 单位 | 后缀标明单位 | `_seconds`、`_bytes`、`_total` |
   691	| Counter | `_total` 后缀 | `gateway_qps_total` |
   692	| Gauge | 无后缀 | `gateway_error_rate` |
   693	| Histogram | 无后缀(自动生成 _bucket/_sum/_count) | `gateway_request_duration_seconds` |
   694	| 标签 | 小写蛇形 | `route`、`service`、`method`、`status` |
   695	
   696	### 4.3 提交规范
   697	
   698	#### 4.3.1 Commit Message 格式
   699	
   700	采用 **Conventional Commits** 规范：
   701	
   702	```
   703	<type>(<scope>): <subject>
   704	
   705	<body>
   706	
   707	<footer>
   708	```
   709	
   710	**type（必填）**：
   711	
   712	| type | 说明 |
   713	|---|---|
   714	| `feat` | 新功能 |
   715	| `fix` | Bug 修复 |
   716	| `docs` | 文档变更 |
   717	| `style` | 代码格式(不影响功能，如空格/分号) |
   718	| `refactor` | 重构(既不是新功能也不是修bug) |
   719	| `perf` | 性能优化 |
   720	| `test` | 测试相关 |
   721	| `chore` | 构建/工具/依赖变更 |
   722	| `ci` | CI 配置变更 |
   723	
   724	**scope（可选）**：模块名，如 `ratelimit`、`circuitbreaker`、`registry`、`api`、`deploy`
   725	
   726	**subject（必填）**：简短描述，不超过 50 字符，祈使句，不加句号
   727	
   728	**body（可选）**：详细描述，每行不超过 72 字符，说明动机与对比
   729	
   730	**footer（可选）**：Breaking Changes 标记 / 关联 Issue
   731	
   732	#### 4.3.2 提交示例
   733	
   734	```
   735	feat(ratelimit): implement local token bucket with Redis batch prefetch
   736	
   737	- Add in-memory token bucket (rate/capacity/last_refill_time)
   738	- Add background goroutine to prefetch tokens from Redis every 100ms
   739	- Add Lua script for atomic global bucket refill with TTL refresh
   740	- Support degrade to local-only mode when Redis is unavailable
   741	
   742	Refs: #12
   743	```
   744	
   745	```
   746	fix(circuitbreaker): fix half-open probe count not reset on state transition
   747	
   748	The half_open_count key in Redis was not deleted when transitioning
   749	from HALF_OPEN to CLOSED, causing subsequent open cycles to start
   750	with a non-zero counter.
   751	
   752	Fixes: #45
   753	```
   754	
   755	#### 4.3.3 提交纪律
   756	
   757	- **原子提交**：一个 commit 只做一件事，禁止混合多个不相关变更
   758	- **可编译提交**：每个 commit 必须能通过编译和基本测试（禁止提交半成品）
   759	- **提交频率**：每完成一个任务(T-xxxx)的一个可验证子功能即提交
   760	- **禁止**：`git add .` 盲目全量提交；提交密码/Token/密钥；提交大文件(>10MB)
   761	- **分支策略**：`main`(保护分支，仅 PR 合入) → `develop`(集成分支) → `feature/T-xxxx-描述`(功能分支)
   762	- **PR 要求**：标题同 commit 格式；描述包含变更点/测试方式/截图(如有)；至少 1 人 review；CI 通过后合入
   763	
   764	### 4.4 注释规范
   765	
   766	#### 4.4.1 包注释
   767	
   768	每个包的 `doc.go` 或首个文件顶部必须有包注释：
   769	
   770	```go
   771	// Package ratelimit implements a two-layer rate limiting engine.
   772	//
   773	// The first layer is an in-memory token bucket (nanosecond-level, no network I/O),
   774	// the second layer is Redis for multi-instance token batch prefetch.
   775	// Request path only operates on local memory; Redis sync runs in background goroutine.
   776	package ratelimit
   777	```
   778	
   779	#### 4.4.2 导出符号注释
   780	
   781	所有导出的类型/函数/方法/常量/变量必须有注释，注释以符号名开头：
   782	
   783	```go
   784	// TokenBucket implements a thread-safe in-memory token bucket algorithm.
   785	type TokenBucket struct { ... }
   786	
   787	// Allow attempts to consume one token from the bucket.
   788	// Returns true if a token was available (request allowed), false otherwise.
   789	func (tb *TokenBucket) Allow() bool { ... }
   790	
   791	// DefaultRate is the default refill rate in tokens per second.
   792	const DefaultRate = 100
   793	```
   794	
   795	#### 4.4.3 函数注释规范
   796	
   797	```go
   798	// NewCircuitBreaker creates a new CircuitBreaker with the given configuration.
   799	//
   800	// Parameters:
   801	//   - name: unique identifier for this breaker (service:route format)
   802	//   - cfg: breaker configuration (thresholds, durations, etc.)
   803	//   - redisClient: Redis client for global half-open count and state sync
   804	//
   805	// Returns:
   806	//   - *CircuitBreaker: initialized breaker in CLOSED state
   807	//   - error: configuration validation error
   808	//
   809	// Example:
   810	//
   811	//	cb, err := NewCircuitBreaker("user-svc:route-1", cfg, redis)
   812	func NewCircuitBreaker(name string, cfg Config, redisClient RedisClient) (*CircuitBreaker, error) { ... }
   813	```
   814	
   815	#### 4.4.4 行内注释
   816	
   817	```go
   818	// 复杂逻辑前注释说明意图（不是复述代码）
   819	// Calculate refill amount since last access, capped at capacity
   820	elapsed := time.Since(tb.lastRefill).Seconds()
   821	refill := elapsed * tb.rate
   822	tb.tokens = min(tb.capacity, tb.tokens+refill)  // 不超过容量
   823	
   824	// TODO/FIXME 标记必须带责任人与日期
   825	// TODO(alice, 2026-10-10): optimize with lock-free ring buffer
   826	// FIXME(bob, 2026-10-15): race condition under high concurrency, see issue #78
   827	```
   828	
   829	#### 4.4.5 API 文档注释（swaggo）
   830	
   831	管理面 Handler 使用 swaggo 注释生成 OpenAPI 文档：
   832	
   833	```go
   834	// CreateRoute godoc
   835	// @Summary      创建路由
   836	// @Description  创建新的网关路由配置
   837	// @Tags         routes
   838	// @Accept       json
   839	// @Produce      json
   840	// @Param        request  body      CreateRouteRequest  true  "路由配置"
   841	// @Success      200      {object}  Response{data=RouteResponse}
   842	// @Failure      400      {object}  Response
   843	// @Failure      409      {object}  Response  "路径+方法已存在"
   844	// @Router       /api/v1/routes [post]
   845	// @Security     BearerAuth
   846	func (h *RouteHandler) CreateRoute(c *gin.Context) { ... }
   847	```
   848	
   849	#### 4.4.6 注释纪律
   850	
   851	- **禁止**：无意义注释（`i++ // i 加 1`）、注释掉的代码（直接删除）、与代码不符的过时注释
   852	- **必须**：复杂算法注释、非显而易见的业务规则注释、并发安全注意事项、性能敏感路径标注、降级/容错逻辑注释
   853	- **中文注释**：业务逻辑与架构设计用中文注释；代码技术术语用英文；团队统一中文为主
   854	
   855	---
   856	
   857	## 5. 验收标准
   858	
   859	### 5.1 通用验收标准（所有任务必须满足）
   860	
   861	| 类别 | 验收项 | 标准 |
   862	|---|---|---|
   863	| **功能** | 功能完整性 | 任务描述的所有功能点全部实现，无遗漏 |
   864	| **功能** | 边界处理 | 空输入/异常输入/并发场景/超阈值场景均有处理 |
   865	| **代码** | 编译通过 | `CGO_ENABLED=0 go build ./...` 无错误无警告 |
   866	| **代码** | 格式检查 | `gofmt -l` 无输出；`goimports -l` 无输出 |
   867	| **代码** | Lint 通过 | `golangci-lint run` 无 error 级别问题 |
   868	| **代码** | 命名规范 | 符合 4.2 节命名规范，CI 检查通过 |
   869	| **测试** | 单元测试 | 核心逻辑模块测试覆盖率 ≥80%，治理引擎 ≥85% |
   870	| **测试** | 测试通过 | `go test ./...` 全部通过，无 flaky test |
   871	| **测试** | 并发测试 | 涉及共享状态的模块必须有并发测试(`-race` 通过) |
   872	| **文档** | 注释完整 | 导出符号 100% 有注释；复杂逻辑有行内注释 |
   873	| **文档** | API 文档 | 管理面接口有 swaggo 注释，可生成 OpenAPI 文档 |
   874	| **安全** | 无硬编码密钥 | Token/密码不硬编码，通过环境变量/配置文件注入 |
   875	| **安全** | 敏感字段脱敏 | 日志/操作日志中 password/token/cookie 等已脱敏 |
   876	| **性能** | 无请求路径同步 I/O | 数据面请求路径不直接访问 DB/Redis/etcd |
   877	| **性能** | 无 goroutine 泄漏 | 所有启动的 goroutine 有明确的关闭机制 |
   878	
   879	### 5.2 各模块专项验收标准
   880	
   881	#### M-01 项目骨架与基础设施
   882	
   883	| 任务ID | 验收标准 |
   884	|---|---|
   885	| T-0101 | `go mod init` 成功；目录结构符合 4.1.2；`Makefile` 支持 build/test/lint/run；`.gitignore` 忽略二进制/IDE/临时文件 |
   886	| T-0102 | `config/default.yaml` 加载成功；环境变量 `GATEWAY_` 前缀覆盖生效；Config 结构体包含所有配置节；viper 热加载监听文件变更 |
   887	| T-0103 | zap 输出 JSON 格式；含 timestamp/level/logger/message 字段；TraceId/SpanId 字段可注入；日志级别可通过配置切换；异步写入不阻塞 |
   888	| T-0104 | `Response` 结构体含 code/message/data/trace_id/errors；成功/失败/分页三种辅助函数；错误码常量按 4xxyyy 规范定义；Recovery 中间件捕获 panic 返回 50000 |
   889	| T-0105 | 9 步启动时序按顺序执行；数据面最后启动；SIGTERM/SIGINT 触发优雅关闭；关闭顺序逆序；各步骤失败有明确错误日志 |
   890	| T-0106 | UUID v4 生成；IP 提取支持 XFF 多级；HTTP 客户端带超时+连接池；重试工具指数退避+最大次数 |
   891	
   892	#### M-02 配置存储层
   893	
   894	| 任务ID | 验收标准 |
   895	|---|---|
   896	| T-0201 | SQLite(modernc 纯 Go)与 MySQL 双驱动可通过配置切换；`CGO_ENABLED=0` 编译通过；连接池参数生效 |
   897	| T-0202 | 7 个 Model 全部定义；BaseModel 通用字段嵌入；`gorm.DeletedAt` 逻辑删除；JSON 字段用 datatypes.JSON；表名 `gw_` 前缀 |
   898	| T-0203 | `AutoMigrate` 自动建表成功；SQLite 建表验证；`deleted_at` 唯一索引验证(多条已删除不冲突)；UUID 主键不依赖自增 |
   899	| T-0204 | 路由 CRUD 全部可用；逻辑删除后查询不返回；路径+方法唯一约束冲突返回错误；乐观锁 version 递增；分页查询正确 |
   900	| T-0205~T-0208 | 各 Repository CRUD 可用；条件筛选正确；启用状态过滤正确 |
   901	| T-0209 | ConfigVersion 记录 before/after 快照；OperationLog 记录完整操作信息；查询分页+筛选正确 |
   902	| T-0210 | 全部 Repository 单测通过(SQLite 内存库)；覆盖率 ≥80%；唯一约束/乐观锁/逻辑删除场景覆盖 |
   903	
   904	#### M-03 etcd 服务注册与发现
   905	
   906	| 任务ID | 验收标准 |
   907	|---|---|
   908	| T-0301 | etcd 多 endpoint 连接成功；重连退避生效；Put/Get/Delete/Watch/Grant/KeepAlive 接口可用 |
   909	| T-0302 | 服务启动后 etcd 中出现 `/services/{name}/{id}` Key；Lease TTL=10s；KeepAlive 每 5s 续约；服务关闭后 Key 自动过期删除 |
   910	| T-0303 | 网关启动全量拉取构建缓存；Watch 事件实时更新；`atomic.Value` 原子替换；`GetInstances(name, version)` 返回正确列表；读无锁 |
   911	| T-0304 | 健康检查间隔=配置值(默认10s)；超时=3s；连续 2 次失败摘除；连续 2 次成功恢复；摘除后请求不路由到该实例 |
   912	| T-0305 | 实例按 service→version 分组；版本过滤正确；健康状态过滤正确；实例变更触发回调 |
   913	| T-0306 | etcd 停止后网关使用本地缓存继续服务；etcd 恢复后自动重连+全量重建；下线感知 ≤10s |
   914	
   915	#### M-04 Redis 封装层
   916	
   917	| 任务ID | 验收标准 |
   918	|---|---|
   919	| T-0401 | Redis 连接成功；连接池参数生效；Ping 健康检查；Get/Set/Incr/Eval/Del/Expire/HGet/HSet 接口可用 |
   920	| T-0402 | Lua 脚本加载+SHA 缓存；限流预取/全局桶补充/半开计数脚本可执行；原子性验证；每次执行后 EXPIRE 刷新 TTL=3600s |
   921	| T-0403 | Redis 不可用时调用返回 error；降级回调接口定义；健康状态上报 |
   922	
   923	#### M-05 接入层 HTTP Server
   924	
   925	| 任务ID | 验收标准 |
   926	|---|---|
   927	| T-0501 | 数据面 8080 启动可访问；net/http Server 超时参数生效；NoRoute 返回 404 统一响应；Recovery 捕获 panic |
   928	| T-0502 | 管理面 8081 启动可访问；容器内 0.0.0.0 绑定；compose 回环映射后局域网不可达；`/api/v1/` 路由组 |
   929	| T-0503 | `/healthz` 始终 200；`/readyz` 启动 60s 内检查依赖(全成功才 200)；运行时仅检查内部状态(不检查外部依赖)；依赖短暂故障时 /readyz 仍返回 200 |
   930	| T-0504 | 管理面先启动、数据面后启动；signal 触发优雅关闭；关闭超时 30s；进行中请求完成后关闭 |
   931	| T-0505 | 最大连接数限制生效；超限返回 503；活跃连接数指标上报 |
   932	
   933	#### M-06 路由匹配与反向代理
   934	
   935	| 任务ID | 验收标准 |
   936	|---|---|
   937	| T-0601 | radix tree 路由匹配正确；Path 前缀匹配+Method 精确匹配；优先级排序正确；匹配返回完整 Route 配置 |
   938	| T-0602 | 前缀截断正确；正则替换正确；重写后 Path 写入请求；原始 Path 保留在上下文 |
   939	| T-0603 | 请求成功转发到后端实例；traceparent Header 注入；路由级超时生效(超时返回 504)；连接池复用；后端不可达返回 502 |
   940	| T-0604 | 重试触发条件正确(状态码/网络错误)；重试排除已失败实例；总耗时不超 timeout_ms；仅幂等方法重试(除非显式配置)；最大 3 次；重试结果计入熔断统计 |
   941	| T-0605 | 路由配置变更后 radix tree 原子重建；新请求使用新配置；匹配无锁读 |
   942	
   943	#### M-07 请求/响应拦截链
   944	
   945	| 任务ID | 验收标准 |
   946	|---|---|
   947	| T-0701 | 中间件执行顺序正确(Trace→限流→熔断→灰度→路由→代理)；上下文传递正确(route_id/target_service/start_time)；响应后置处理顺序正确 |
   948	| T-0702 | 入站 traceparent 存在则继承；不存在则生成新 TraceId；TraceId 写入响应 Header；SERVER Span 创建；100% 生成 TraceId |
   949	| T-0703 | 限流规则匹配正确；被限流返回 429+Header；限流结果写入上下文 |
   950	| T-0704 | Open 状态返回 503+X-Circuit-Breaker: open；HalfOpen 全局计数判定；Closed 放行；状态写入上下文 |
   951	| T-0705 | 染色规则命中优先；未命中走哈希比例；版本标签写入上下文；服务发现按 version 过滤 |
   952	| T-0706 | 延迟直方图记录正确；状态码计数正确；熔断滑动窗口记录正确；访问日志含全字段；Span 正确结束 |
   953	
   954	#### M-08 双层限流引擎
   955	
   956	| 任务ID | 验收标准 |
   957	|---|---|
   958	| T-0801 | 令牌桶速率/容量正确；并发安全(`-race` 通过)；Allow() 准确率 ≥99%；边界(rate=0/capacity=0)处理正确 |
   959	| T-0802 | 漏桶速率/容量正确；漏水逻辑正确；并发安全 |
   960	| T-0803 | 预取协程每 100ms 执行；预取量计算正确(rate×0.1s×2)；Lua 原子扣减；全局桶不足时预取剩余量；本地令牌桶得到补充 |
   961	| T-0804 | 全局桶每 1s 补充；不超过 capacity；Lua 原子性；Key TTL 每次刷新 3600s；Key 格式正确 |
   962	| T-0805 | limit_by=route/ip/client_id 三维度正确；Key 构建正确；规则匹配优先级(route_id > path_pattern > 全局) |
   963	| T-0806 | Redis 正常→双层模式；Redis 停止→自动切本地模式(限流继续)；Redis 恢复→自动切回；本地耗尽+Redis 失败→按剩余放行不阻塞；降级状态有指标 |
   964	| T-0807 | 429 响应含 X-RateLimit-Limit/Remaining/Reset/Retry-After；值正确 |
   965	| T-0808 | 单元测试覆盖率 ≥85%；并发测试通过；Redis 集成测试通过；降级切换测试通过 |
   966	
   967	#### M-09 熔断状态机(全局协调)
   968	
   969	| 任务ID | 验收标准 |
   970	|---|---|
   971	| T-0901 | 滑动窗口 10s/10 桶正确；桶滚动正确；总请求/错误率/慢调用比例统计准确；并发安全；Record() 性能无锁或细粒度锁 |
   972	| T-0902 | 三态转移正确；CLOSED 触发条件(≥20 请求 AND 错误率≥50% OR 慢调用≥60%)正确；OPEN 30s 后转 HALF_OPEN；HALF_OPEN 全成功→CLOSED，任一失败→OPEN |
   973	| T-0903 | HALF_OPEN 时 Redis INCR 全局计数；count≤5 放行，>5 快速失败；多实例总探测数 ≤5；Key TTL=open_duration_ms；状态转移时计数重置 |
   974	| T-0904 | 状态变更 SET Redis + PUBLISH 事件；其他节点 SUBSCRIBE 接收并更新本地；本地缓存读优先；事件广播延迟 ≤1s |
   975	| T-0905 | 按 route_id/target_service 配置策略；所有参数可配置；默认值与架构文档一致 |
   976	| T-0906 | Redis stats Key 记录聚合统计；仅管理面查询用；不参与熔断判断(P2-9) |
   977	| T-0907 | 单元测试覆盖率 ≥85%；滑动窗口精度测试；状态机边界测试；Redis 集成测试(半开计数/事件广播) |
   978	
   979	#### M-10 灰度路由与负载均衡
   980	
   981	| 任务ID | 验收标准 |
   982	|---|---|
   983	| T-1001 | WRR 权重分配精度偏差 <1%；平滑性(无连续同一实例)；实例变更时重置 |
   984	| T-1002 | murmur3 哈希正确；虚拟节点 150 倍；按权重分配虚拟节点；环有序数组+二分查找；节点增减影响约 1/N 请求；分布均匀性 ≥99% |
   985	| T-1003 | LoadBalancer 接口定义；工厂按配置创建 WRR/CH；CH 模式 hash_header 配置生效 |
   986	| T-1004 | 染色判定(Header/Cookie/Query/UserID)正确且优先级最高；未命中走哈希比例(murmur3%100)；gray_percent=0 全稳定，=100 全灰度；版本标签写入上下文 |
   987	| T-1005 | 灰度规则从快照读取；配置变更原子替换；percent 运行时调整立即生效 |
   988	| T-1006 | 单元测试覆盖率 ≥85%；WRR 精度测试；CH 分布/影响范围测试；灰度比例边界测试 |
   989	
   990	#### M-11 配置热更新
   991	
   992	| 任务ID | 验收标准 |
   993	|---|---|
   994	| T-1101 | ConfigSnapshot 包含所有配置类型；不可变(外部只能读)；快照构建从 DB 全量加载；版本号标记 |
   995	| T-1102 | 管理面写 DB 后发布 etcd 事件；网关 Watch 接收；事件触发对应类型全量拉取；事件 Key TTL=60s |
   996	| T-1103 | 全量拉取→构建新快照→atomic.Value 原子替换；读无锁；新配置 ≤5s 生效；日志记录 config reloaded |
   997	| T-1104 | 每 60s 定时全量同步；版本号对比无变更跳过；与 Watch 触发互斥；Watch 丢失后 ≤60s 收敛 |
   998	| T-1105 | POST /reload 手动触发；GET /config 查看快照版本；热更新失败返回 50002 |
   999	
  1000	#### M-12 管理面 API
  1001	
  1002	| 任务ID | 验收标准 |
  1003	|---|---|
  1004	| T-1201 | Bearer Token 鉴权；多 Token 映射 username；无效 Token 返回 40100；缺失返回 40100；管理面端口网络隔离 |
  1005	| T-1202 | 操作人解析优先级正确(Token映射 > X-Operator > default)；操作人写入审计日志 |
  1006	| T-1203 | 路由 CRUD 全部可用；唯一约束校验(40101)；目标服务校验(40102)；启用/禁用；创建/更新后触发热更新 |
  1007	| T-1204~T-1207 | 限流/熔断/灰度/服务 CRUD 可用；参数校验正确(错误码)；熔断状态查询/reset；灰度 percent 调整；服务实例查询 |
  1008	| T-1208 | 操作日志/配置版本查询可用；分页+筛选；敏感字段脱敏 |
  1009	| T-1209 | 管理面操作自动记录日志；异步写入不阻塞；字段完整(trace_id/operator/operation/resource/request/response/耗时) |
  1010	| T-1210 | 集成测试全部通过；鉴权/参数校验/错误码/审计验证 |
  1011	
  1012	#### M-13 可观测性
  1013	
  1014	| 任务ID | 验收标准 |
  1015	|---|---|
  1016	| T-1301 | 全部指标定义且名称对齐基线(gateway_qps_total/duration_seconds/error_rate)；标签正确；Counter/Gauge/Histogram 类型正确 |
  1017	| T-1302 | /metrics 端点可访问并返回全部指标；请求路径埋点正确；各治理模块埋点正确；系统指标定时采集 |
  1018	| T-1303 | TraceId 100% 生成；本地 stdout 全量采样；生产 OTLP 10% 采样+5xx 强制；网关 SERVER Span；traceparent 注入后端；W3C Propagator |
  1019	| T-1304 | 访问日志 JSON 格式；必含字段全部存在；异步写入；日志级别可配置 |
  1020	| T-1305 | password/token/id_card/phone/email/cookie/authorization 脱敏规则正确；日志与操作日志统一应用；脱敏单测通过 |
  1021	| T-1306 | /metrics 返回全部指标；TraceId 全链路一致；Jaeger 可见完整调用链；脱敏验证通过 |
  1022	
  1023	#### M-14 测试服务集群
  1024	
  1025	| 任务ID | 验收标准 |
  1026	|---|---|
  1027	| T-1401 | 公共库可复用；etcd 注册(Lease+KeepAlive)；OTel SDK 提取 traceparent 创建 Span；/health 返回 200；业务端点返回含 TraceId JSON |
  1028	| T-1402 | service-a/b 连接 DB；SQL 注释注入 TraceId(`/* trace_id=xxx */ SELECT`)；返回查询结果+TraceId |
  1029	| T-1403 | service-c/d/e 提供 /echo(回显)、/slow(可配置延迟)、/error(可配置错误概率)、/health；用于熔断/限流测试 |
  1030	| T-1404 | 每个服务独立 Dockerfile；环境变量配置；compose 中 5 服务定义正确；各 mem_limit=64m |
  1031	| T-1405 | service-a 支持 v1/v2(环境变量切换)；返回不同标识；用于灰度验证 |
  1032	
  1033	#### M-15 部署与编排
  1034	
  1035	| 任务ID | 验收标准 |
  1036	|---|---|
  1037	| T-1501 | 多阶段构建成功；CGO_ENABLED=0 编译；distroless 运行；HEALTHCHECK 纯 exec；镜像 ≤100MB；nonroot 用户运行 |
  1038	| T-1502 | compose 定义 gateway+etcd+redis+5测试服务；端口映射正确(8080 对外/8081 回环)；健康检查正确；mem_limit 配置；depends_on 条件正确 |
  1039	| T-1503 | default.yaml 完整注释；环境变量覆盖示例；production.yaml 模板(MySQL/OTLP/采样率) |
  1040	| T-1504 | K8s Deployment/Service/ConfigMap/Secret/StatefulSet 完整；管理面 NetworkPolicy 限制；资源 limits/requests |
  1041	| T-1505 | HPA v2 配置正确(CPU/内存/自定义指标)；Prometheus Adapter 部署+ConfigMap；指标转换规则正确(seriesQuery/metricsQuery) |
  1042	| T-1506 | 告警规则 7 条全部定义；分级(P0/P1/P2/P3)正确；PromQL 表达式正确 |
  1043	| T-1507 | `docker-compose up -d` 一键启动；/healthz 200；测试服务注册到 etcd；通过 gateway 转发请求成功；总内存 ≤2GB |
  1044	
  1045	#### M-16 性能测试与集成验收
  1046	
  1047	| 任务ID | 验收标准 |
  1048	|---|---|
  1049	| T-1601 | wrk 压测 QPS≥10000；平均延迟<2ms；P99<10ms；内存≤80MB；多轮稳定 |
  1050	| T-1602 | rate=100/s 时 QPS 限制在 ~100(偏差<5%)；429+Header 正确；3 实例总 QPS≈配置值；Redis 故障降级限流继续 |
  1051	| T-1603 | 100% 错误时 ≥20 请求后熔断打开；后续 503+Header；30s 后半开；多实例半开总探测 ≤5；状态事件广播 |
  1052	| T-1604 | ≥100 不同 XFF 发 1000 请求；canary 接收 180-220(18%-22%)；染色 100% 命中；percent 调整立即生效 |
  1053	| T-1605 | 管理面修改后 ≤5s 生效；Watch 丢失后 60s 兜底收敛；多实例 ≤60s 一致 |
  1054	| T-1606 | 实例上下线 ≤10s 感知；健康检查摘除/恢复正确；etcd 故障本地缓存兜底 |
  1055	| T-1607 | TraceId 网关日志/响应Header/后端日志/DB注释一致；Jaeger 完整调用链；5xx 强制采样 |
  1056	| T-1608 | SQLite 模式全部测试通过；MySQL 模式全部测试通过；切换 driver 功能一致；CGO_ENABLED=0 编译 |
  1057	| T-1609 | e2e 套件覆盖完整链路；管理面全流程；故障注入(etcd/Redis/后端)降级验证；全部通过 |
  1058	
  1059	### 5.3 整体交付验收标准
  1060	
  1061	| 维度 | 验收项 | 基线/目标 |
  1062	|---|---|---|
  1063	| **功能完整** | 16 模块全部交付 | 所有 P0/P1 任务完成，P2 ≥90% |
  1064	| **性能** | 单实例 QPS | ≥10,000 (D-01) |
  1065	| **性能** | 平均延迟 | <2ms (D-02) |
  1066	| **性能** | 内存占用 | ≤80MB (D-04/P-04) |
  1067	| **性能** | 镜像大小 | ≤200MB (RC-05)，目标 ≤100MB |
  1068	| **可用性** | 实例下线感知 | ≤10s |
  1069	| **可用性** | 配置热更新生效 | ≤5s |
  1070	| **可用性** | 配置一致性收敛 | ≤60s |
  1071	| **兼容性** | SQLite/MySQL 双驱动 | 切换配置即可，功能一致 |
  1072	| **兼容性** | CGO_ENABLED=0 | 编译通过，distroless 运行 |
  1073	| **可观测** | 三大基线指标 | gateway_qps_total/duration_seconds/error_rate (D-06) |
  1074	| **可观测** | TraceId 透传 | 100% 生成，全链路一致 (T-01) |
  1075	| **安全** | 管理面鉴权 | Token 鉴权 + 端口隔离 + 操作审计 |
  1076	| **安全** | 敏感字段脱敏 | 日志/审计中 password/token/cookie 脱敏 (CO-02) |
  1077	| **测试** | 单元测试覆盖率 | 核心模块 ≥80%，治理引擎 ≥85% |
  1078	| **测试** | 集成测试 | 管理面 API + 端到端链路全部通过 |
  1079	| **部署** | 本地一键启动 | docker-compose up 全套环境 ≤2GB (D-03) |
  1080	| **文档** | 代码注释 | 导出符号 100% 注释 |
  1081	| **文档** | API 文档 | OpenAPI 3.0 可生成 |
  1082	
  1083	---
  1084	
  1085	*文档结束*
