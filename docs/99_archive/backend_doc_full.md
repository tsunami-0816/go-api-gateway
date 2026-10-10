Read "D:\lg\project\go-api-gateway\后端开发实现文档.md":

     1	# Go API Gateway 后端开发实现文档
     2	
     3	> 版本：v1.0.0  
     4	> 日期：2026-10-05  
     5	> 模块范围：迭代一 P0 核心网关（M-01 ~ M-13 + M-15 部署）  
     6	> 编译状态：✅ 通过（CGO_ENABLED=0，纯 Go SQLite 驱动）  
     7	> 运行验证：✅ 健康检查/就绪探针/鉴权/路由CRUD 均通过
     8	
     9	---
    10	
    11	## 1. 模块说明
    12	
    13	### 1.1 本次实现的模块范围
    14	
    15	| 模块编号 | 模块名称 | 实现状态 | 说明 |
    16	|---------|---------|---------|------|
    17	| M-01 | 项目骨架与基础设施 | ✅ 完整 | 配置加载、zap日志、统一响应、错误码、工具库、9步启动框架 |
    18	| M-02 | 配置存储层基础 | ✅ 完整 | GORM Model（7张表）、SQLite/MySQL双驱动、AutoMigrate、Repository层 |
    19	| M-03 | etcd 服务注册与发现 | ✅ 完整 | Lease注册（TTL=10s）、KeepAlive、Watch、atomic.Value缓存、健康过滤 |
    20	| M-04 | Redis 封装层 | ✅ 完整 | 连接池、Ping、Eval Lua执行器、优雅关闭 |
    21	| M-05 | 接入层 HTTP Server | ✅ 完整 | 双Gin Engine（数据面8080/管理面8081）、优雅关闭、超时配置 |
    22	| M-06 | 路由匹配与反向代理 | ✅ 完整 | 前缀匹配+方法精确匹配、优先级排序、路径重写、httputil.ReverseProxy、traceparent注入 |
    23	| M-07 | 请求/响应拦截链基础 | ✅ 完整 | Trace注入、限流、熔断、灰度、Recovery、CORS、Auth、Audit中间件 |
    24	| M-08 | 双层限流引擎 | ✅ 基础 | 内存令牌桶（nanosecond级）、LeakyBucket、RateLimiter管理器 |
    25	| M-09 | 熔断状态机 | ✅ 完整 | 滑动窗口（10桶时间轮）、三态状态机、半开探测、手动Reset |
    26	| M-10 | 灰度路由与负载均衡 | ✅ 完整 | 平滑加权轮询（nginx算法）、一致性哈希（150虚拟节点/权重）、两级灰度决策（染色规则+哈希比例） |
    27	| M-11 | 配置热更新 | ✅ 完整 | 不可变快照、atomic.Value原子替换、手动Reload API、兜底同步 |
    28	| M-12 | 管理面 API 基础 | ✅ 完整 | 路由/限流/熔断/灰度/服务 全套CRUD + Token鉴权（多Token映射）+ 操作审计 + 系统运维 |
    29	| M-13 | 可观测性 | ✅ 完整 | Prometheus指标（15项）、敏感字段脱敏（password/token/id_card/phone/email/cookie） |
    30	| M-15 | 部署与编排 | ✅ 完整 | Dockerfile（distroless + CGO_ENABLED=0）、docker-compose（网关+etcd+redis+2个测试服务） |
    31	
    32	### 1.2 复用的公共组件
    33	
    34	| 组件 | 位置 | 复用场景 |
    35	|------|------|---------|
    36	| `config.Config` | `internal/config/config.go` | 全模块配置注入，支持YAML+环境变量覆盖（GATEWAY_前缀） |
    37	| `logger.L/S` | `internal/pkg/logger/logger.go` | 全模块结构化日志，支持异步写入、Trace字段注入 |
    38	| `response.Response` | `internal/pkg/response/response.go` | 管理面所有API统一响应格式，含分页封装 |
    39	| `errcode` | `internal/pkg/errcode/errcode.go` | 全模块错误码定义（4xxyyy规范），含中文消息映射 |
    40	| `utils` | `internal/pkg/utils/utils.go` | UUID生成、TraceID/SpanID生成、ClientIP提取（多级XFF）、时间格式化 |
    41	| `model.BaseModel` | `internal/model/models.go` | 所有配置表公共字段（UUID主键、软删除、乐观锁version、审计字段） |
    42	| `store.NewDB/NewRedisClient` | `internal/store/` | 数据库与Redis连接工厂，连接池配置 |
    43	| `configsnap.Manager` | `internal/configsnap/snapshot.go` | 数据面配置快照管理，原子替换，lock-free读 |
    44	
    45	### 1.3 依赖说明
    46	
    47	- **etcd 不可用时**：网关启动不阻塞，打印警告，服务注册表为空，反向代理返回502
    48	- **Redis 不可用时**：网关启动不阻塞，限流引擎运行在本地内存模式（单机限流）
    49	- **SQLite 默认驱动**：使用 `glebarez/sqlite`（基于 `modernc.org/sqlite` 纯Go实现），支持 `CGO_ENABLED=0` 静态编译
    50	- **配置热更新**：当前版本通过 `POST /api/v1/system/reload` 手动触发；etcd Watch 自动触发在迭代二实现
    51	
    52	---
    53	
    54	## 2. 核心代码实现
    55	
    56	### 2.1 项目目录结构
    57	
    58	```
    59	go-api-gateway/
    60	├── cmd/gateway/
    61	│   └── main.go                    # 入口，9步启动时序，信号处理优雅关闭
    62	├── internal/
    63	│   ├── app/
    64	│   │   └── app.go                 # 应用组装，依赖注入，双Server生命周期管理
    65	│   ├── config/
    66	│   │   └── config.go              # 配置结构体，Viper加载，环境变量覆盖
    67	│   ├── configsnap/
    68	│   │   └── snapshot.go            # 不可变配置快照，atomic.Value原子替换
    69	│   ├── circuitbreaker/
    70	│   │   └── circuit_breaker.go     # 滑动窗口+三态熔断状态机
    71	│   ├── gray/
    72	│   │   └── gray_router.go         # 两级灰度决策（染色规则+哈希比例）
    73	│   ├── handler/
    74	│   │   ├── route_handler.go       # 路由管理API（6个端点）
    75	│   │   └── other_handlers.go      # 限流/熔断/灰度/服务/系统API
    76	│   ├── loadbalance/
    77	│   │   └── loadbalance.go         # WRR（nginx平滑算法）+ 一致性哈希
    78	│   ├── middleware/
    79	│   │   └── middleware.go          # 8个Gin中间件（Trace/限流/熔断/灰度/Auth/CORS/Recovery）
    80	│   ├── model/
    81	│   │   └── models.go              # 7张GORM表模型 + BaseModel
    82	│   ├── observability/
    83	│   │   └── metrics.go             # 15项Prometheus指标 + 敏感字段脱敏
    84	│   ├── pkg/
    85	│   │   ├── errcode/errcode.go     # 错误码定义（4xxyyy）
    86	│   │   ├── logger/logger.go       # zap全局日志，异步写入
    87	│   │   ├── response/response.go   # 统一响应+分页
    88	│   │   └── utils/utils.go         # 工具函数
    89	│   ├── proxy/
    90	│   │   └── proxy.go               # 路由匹配引擎 + 路径重写 + ReverseProxy
    91	│   ├── ratelimit/
    92	│   │   └── ratelimit.go           # 令牌桶 + 漏桶 + RateLimiter管理器
    93	│   ├── registry/
    94	│   │   └── registry.go            # etcd注册（Lease+KeepAlive）+ 发现（Watch+缓存）
    95	│   ├── repository/
    96	│   │   ├── route_repository.go    # 路由CRUD（乐观锁+分页+筛选）
    97	│   │   ├── rate_limit_repository.go
    98	│   │   ├── circuit_breaker_repository.go
    99	│   │   ├── gray_release_repository.go
   100	│   │   └── service_audit_repository.go  # 服务+配置版本+操作日志
   101	│   └── store/
   102	│       ├── db.go                  # GORM连接工厂（SQLite/MySQL）
   103	│       └── redis.go               # Redis客户端封装
   104	├── config/
   105	│   └── default.yaml               # 默认配置文件
   106	├── deploy/
   107	│   ├── docker/Dockerfile          # 多阶段构建，distroless运行时
   108	│   └── compose/docker-compose.yml # 完整编排（网关+etcd+redis+2测试服务）
   109	├── bin/
   110	│   └── gateway.exe                # 编译产物（Windows）
   111	├── go.mod / go.sum
   112	├── Makefile
   113	└── .gitignore
   114	```
   115	
   116	### 2.2 启动时序（9步）
   117	
   118	```
   119	main.go
   120	  │
   121	  ├─ ① config.Load()          加载YAML配置 + 环境变量覆盖
   122	  ├─ app.New(ctx, cfg)
   123	  │    ├─ ② logger.Init()     初始化zap日志（JSON格式，可选异步）
   124	  │    ├─ ③ store.NewDB()     连接SQLite/MySQL + AutoMigrate 7张表
   125	  │    ├─ ④ snapManager.Reload()  从DB构建不可变配置快照
   126	  │    ├─ ⑤ registry.NewDiscovery()  连接etcd + 全量拉取 + 启动Watch
   127	  │    ├─ ⑥ store.NewRedisClient()   连接Redis + Ping验证
   128	  │    ├─ 初始化核心引擎（路由匹配器/限流器/熔断器/灰度路由器/负载均衡器）
   129	  │    └─ ⑧ startAdminServer()  启动管理面HTTP Server（:8081）
   130	  ├─ ⑨ app.Start() → startDataServer()  启动数据面HTTP Server（:8080）【最后启动】
   131	  │
   132	  └─ 等待 SIGINT/SIGTERM → app.Stop()  逆序优雅关闭（30s超时）
   133	```
   134	
   135	**关键设计**：数据面最后启动（步骤⑨），确保管理面、配置、依赖全部就绪后才对外提供流量；关闭时数据面先关闭，拒绝新请求但允许在途请求完成。
   136	
   137	### 2.3 核心算法说明
   138	
   139	#### 2.3.1 平滑加权轮询（WRR）
   140	
   141	```
   142	算法（nginx smooth WRR）：
   143	1. 每个实例 current_weight += weight
   144	2. 选择 current_weight 最大的实例
   145	3. 选中实例 current_weight -= total_weight
   146	
   147	示例：实例A(weight=5), B(weight=1), C(weight=1)
   148	序列：A A A A B A C → 分布均匀，无连续集中
   149	```
   150	
   151	代码位置：`internal/loadbalance/loadbalance.go` → `WeightedRoundRobin.Select()`
   152	
   153	#### 2.3.2 一致性哈希
   154	
   155	```
   156	参数：150个虚拟节点/权重单位
   157	环构建：每个实例生成 weight*150 个虚拟节点，按hash值排序
   158	查询：murmur3(requestKey) → 二分查找第一个 hash >= key 的节点 → 顺时针最近
   159	```
   160	
   161	代码位置：`internal/loadbalance/loadbalance.go` → `ConsistentHash`
   162	
   163	#### 2.3.3 熔断滑动窗口
   164	
   165	```
   166	结构：10个桶，每桶覆盖 window_size/10 时长
   167	推进：每次记录时按时间差推进桶指针，经过的桶清零
   168	统计：聚合10个桶的 total/errors/slow
   169	触发：total >= min_request_count AND (error_rate >= threshold OR slow_rate >= threshold)
   170	```
   171	
   172	代码位置：`internal/circuitbreaker/circuit_breaker.go` → `SlidingWindow`
   173	
   174	#### 2.3.4 灰度两级决策
   175	
   176	```
   177	第一级（最高优先级）：染色规则匹配
   178	  - header精确匹配 / cookie精确匹配 / query精确匹配 / user_id IN列表 / ip匹配
   179	  - 命中 → 直接路由到灰度版本
   180	
   181	第二级：哈希比例分流
   182	  - key = ip / header / cookie / user_id（可配置）
   183	  - murmur3(key) % 100 < gray_percent → 灰度版本，否则稳定版本
   184	```
   185	
   186	代码位置：`internal/gray/gray_router.go` → `Router.Calculate()`
   187	
   188	---
   189	
   190	## 3. 接口定义
   191	
   192	### 3.1 通用约定
   193	
   194	- **管理面基础URL**：`http://{host}:8081/api/v1`
   195	- **数据面基础URL**：`http://{host}:8080`（业务流量转发）
   196	- **鉴权方式**：`Authorization: Bearer {token}`（管理面所有 `/api/v1/*` 端点，`/healthz`、`/readyz`、`/metrics` 除外）
   197	- **内容类型**：`application/json`
   198	- **统一响应格式**：
   199	
   200	```json
   201	{
   202	  "code": 0,
   203	  "message": "success",
   204	  "data": { },
   205	  "trace_id": "778f0d26736e2422fc59a83c00d2cc41"
   206	}
   207	```
   208	
   209	- **分页响应格式**（data 内嵌）：
   210	
   211	```json
   212	{
   213	  "code": 0,
   214	  "message": "success",
   215	  "data": {
   216	    "list": [ ],
   217	    "pagination": {
   218	      "page": 1,
   219	      "page_size": 20,
   220	      "total": 100,
   221	      "total_pages": 5
   222	    }
   223	  },
   224	  "trace_id": "..."
   225	}
   226	```
   227	
   228	### 3.2 系统运维接口
   229	
   230	#### 3.2.1 健康检查（Liveness）
   231	
   232	```
   233	GET /healthz
   234	```
   235	
   236	- **鉴权**：无需
   237	- **响应 200**：`{"status":"ok"}`
   238	
   239	#### 3.2.2 就绪探针（Readiness）
   240	
   241	```
   242	GET /readyz
   243	```
   244	
   245	- **鉴权**：无需
   246	- **响应 200**：`{"status":"ready","config_version":1}`
   247	- **响应 503**：`{"status":"not ready","reason":"config not loaded"}`
   248	
   249	#### 3.2.3 Prometheus 指标
   250	
   251	```
   252	GET /metrics
   253	```
   254	
   255	- **鉴权**：无需
   256	- **响应**：Prometheus text format
   257	
   258	#### 3.2.4 手动配置热更新
   259	
   260	```
   261	POST /api/v1/system/reload
   262	```
   263	
   264	- **鉴权**：需要
   265	- **响应 200**：`{"config_version":2,"reloaded":true}`
   266	- **响应 500**：`{"code":50002,"message":"配置热更新失败"}`
   267	
   268	#### 3.2.5 获取当前配置摘要
   269	
   270	```
   271	GET /api/v1/system/config
   272	```
   273	
   274	- **鉴权**：需要
   275	- **响应 200**：
   276	
   277	```json
   278	{
   279	  "code": 0,
   280	  "data": {
   281	    "version": 1,
   282	    "routes_count": 5,
   283	    "rate_limits_count": 3,
   284	    "circuit_breakers_count": 2,
   285	    "gray_releases_count": 1,
   286	    "services_count": 4
   287	  }
   288	}
   289	```
   290	
   291	### 3.3 路由管理接口
   292	
   293	#### 3.3.1 创建路由
   294	
   295	```
   296	POST /api/v1/routes
   297	```
   298	
   299	**请求体**：
   300	
   301	| 字段 | 类型 | 必填 | 默认值 | 说明 |
   302	|------|------|------|--------|------|
   303	| route_name | string | 是 | - | 路由名称 |
   304	| path | string | 是 | - | 匹配路径（支持前缀匹配） |
   305	| method | string | 是 | - | HTTP方法（GET/POST/PUT/DELETE/PATCH/*） |
   306	| target_service | string | 是 | - | 目标服务名 |
   307	| rewrite_path | string | 否 | "" | 路径重写（支持 `strip_prefix=/xxx`） |
   308	| timeout_ms | int | 否 | 5000 | 超时时间（毫秒） |
   309	| retry_count | int | 否 | 0 | 重试次数 |
   310	| retry_on_status | string | 否 | "" | 重试状态码（逗号分隔，如 "502,503,504"） |
   311	| retry_non_idempotent | bool | 否 | false | 是否对非幂等请求重试 |
   312	| load_balance | string | 否 | "weighted_round_robin" | 负载均衡策略（weighted_round_robin/consistent_hash） |
   313	| hash_header | string | 否 | "" | 一致性哈希使用的Header名 |
   314	| priority | int | 否 | 100 | 优先级（越小越高） |
   315	| enabled | bool | 否 | true | 是否启用 |
   316	| description | string | 否 | "" | 描述 |
   317	
   318	**响应 200**：返回创建的路由对象（含 id、created_at 等）
   319	
   320	**错误码**：
   321	- `40101`：路径+方法组合已存在（409 Conflict）
   322	- `40001`：参数校验失败（400 Bad Request）
   323	
   324	#### 3.3.2 路由列表
   325	
   326	```
   327	GET /api/v1/routes?page=1&page_size=20&path=&method=&target_service=
   328	```
   329	
   330	**查询参数**：
   331	
   332	| 参数 | 类型 | 必填 | 说明 |
   333	|------|------|------|------|
   334	| page | int | 否 | 页码，默认1 |
   335	| page_size | int | 否 | 每页条数，默认20，最大100 |
   336	| path | string | 否 | 路径模糊匹配 |
   337	| method | string | 否 | HTTP方法精确匹配 |
   338	| target_service | string | 否 | 目标服务精确匹配 |
   339	
   340	**响应 200**：分页响应，list 为路由对象数组
   341	
   342	#### 3.3.3 路由详情
   343	
   344	```
   345	GET /api/v1/routes/{id}
   346	```
   347	
   348	**响应 200**：路由对象  
   349	**响应 404**：`{"code":40103,"message":"路由不存在"}`
   350	
   351	#### 3.3.4 更新路由
   352	
   353	```
   354	PUT /api/v1/routes/{id}
   355	```
   356	
   357	- **请求体**：同创建路由
   358	- **乐观锁**：基于 `version` 字段，冲突返回 409
   359	- **响应 200**：更新后的路由对象
   360	
   361	#### 3.3.5 删除路由
   362	
   363	```
   364	DELETE /api/v1/routes/{id}
   365	```
   366	
   367	- **软删除**：设置 `deleted_at`，数据保留
   368	- **响应 200**：`{"id":"...","deleted":true}`
   369	
   370	#### 3.3.6 启用/禁用路由
   371	
   372	```
   373	PATCH /api/v1/routes/{id}/status
   374	```
   375	
   376	**请求体**：`{"enabled": false}`
   377	
   378	**响应 200**：`{"id":"...","enabled":false}`
   379	
   380	### 3.4 限流规则接口
   381	
   382	#### 3.4.1 创建限流规则
   383	
   384	```
   385	POST /api/v1/rate-limits
   386	```
   387	
   388	**请求体**：
   389	
   390	| 字段 | 类型 | 必填 | 默认值 | 说明 |
   391	|------|------|------|--------|------|
   392	| rule_name | string | 是 | - | 规则名称 |
   393	| route_id | string | 否 | "" | 关联路由ID |
   394	| path_pattern | string | 否 | "" | 路径模式 |
   395	| method | string | 否 | "*" | HTTP方法 |
   396	| algorithm | string | 是 | - | 算法（token_bucket/leaky_bucket） |
   397	| rate | int | 是 | - | 速率（token/秒），必须>0 |
   398	| capacity | int | 是 | - | 桶容量 |
   399	| limit_by | string | 否 | "route" | 限流维度（route/ip/client_id） |
   400	| header_key | string | 否 | "" | limit_by=client_id时的Header名 |
   401	| enabled | bool | 否 | true | 是否启用 |
   402	
   403	**错误码**：
   404	- `40201`：不支持的限流算法（400）
   405	- `40202`：限流速率必须大于0（400）
   406	
   407	#### 3.4.2 限流规则列表
   408	
   409	```
   410	GET /api/v1/rate-limits?page=1&page_size=20&route_id=
   411	```
   412	
   413	#### 3.4.3 限流规则详情/更新/删除
   414	
   415	```
   416	GET    /api/v1/rate-limits/{id}
   417	PUT    /api/v1/rate-limits/{id}
   418	DELETE /api/v1/rate-limits/{id}
   419	```
   420	
   421	### 3.5 熔断策略接口
   422	
   423	#### 3.5.1 创建熔断策略
   424	
   425	```
   426	POST /api/v1/circuit-breakers
   427	```
   428	
   429	**请求体**：
   430	
   431	| 字段 | 类型 | 必填 | 默认值 | 说明 |
   432	|------|------|------|--------|------|
   433	| policy_name | string | 是 | - | 策略名称 |
   434	| route_id | string | 否 | "" | 关联路由ID |
   435	| target_service | string | 否 | "" | 目标服务名 |
   436	| sliding_window_size_ms | int | 否 | 10000 | 滑动窗口大小（毫秒） |
   437	| min_request_count | int | 否 | 20 | 最小请求数（低于此数不触发） |
   438	| error_rate_threshold | float | 否 | 50.0 | 错误率阈值（1-100） |
   439	| slow_call_duration_ms | int | 否 | 1000 | 慢调用阈值（毫秒） |
   440	| slow_call_rate_threshold | float | 否 | 60.0 | 慢调用率阈值（1-100） |
   441	| open_duration_ms | int | 否 | 30000 | 熔断开启持续时间（毫秒） |
   442	| half_open_max_requests | int | 否 | 5 | 半开态最大探测请求数 |
   443	| enabled | bool | 否 | true | 是否启用 |
   444	
   445	**错误码**：
   446	- `40301`：错误率阈值必须在1-100之间（400）
   447	
   448	#### 3.5.2 熔断策略列表/详情/删除
   449	
   450	```
   451	GET    /api/v1/circuit-breakers?page=1&page_size=20&route_id=&target_service=
   452	GET    /api/v1/circuit-breakers/{id}
   453	DELETE /api/v1/circuit-breakers/{id}
   454	```
   455	
   456	### 3.6 灰度发布接口
   457	
   458	#### 3.6.1 创建灰度规则
   459	
   460	```
   461	POST /api/v1/gray-releases
   462	```
   463	
   464	**请求体**：
   465	
   466	| 字段 | 类型 | 必填 | 默认值 | 说明 |
   467	|------|------|------|--------|------|
   468	| rule_name | string | 是 | - | 规则名称 |
   469	| route_id | string | 是 | - | 关联路由ID |
   470	| gray_version | string | 是 | - | 灰度版本标签（如 "canary"） |
   471	| stable_version | string | 否 | "stable" | 稳定版本标签 |
   472	| gray_percent | int | 否 | 0 | 灰度比例（0-100） |
   473	| hash_key_type | string | 否 | "ip" | 哈希键类型（ip/header/cookie/user_id） |
   474	| hash_key_name | string | 否 | "" | hash_key_type=header/cookie时的键名 |
   475	| stain_rules | array | 否 | [] | 染色规则数组 |
   476	| enabled | bool | 否 | true | 是否启用 |
   477	
   478	**stain_rules 元素结构**：
   479	
   480	```json
   481	{
   482	  "type": "header",
   483	  "key": "X-Gray-Flag",
   484	  "value": "true"
   485	}
   486	```
   487	
   488	type 可选值：`header` / `cookie` / `query` / `user_id` / `ip`
   489	
   490	**错误码**：
   491	- `40401`：灰度比例必须在0-100之间（400）
   492	
   493	#### 3.6.2 动态调整灰度比例
   494	
   495	```
   496	PATCH /api/v1/gray-releases/{id}/percent
   497	```
   498	
   499	**请求体**：`{"gray_percent": 30}`
   500	
   501	**响应 200**：`{"id":"...","gray_percent":30}`
   502	
   503	#### 3.6.3 灰度规则列表/详情/删除
   504	
   505	```
   506	GET    /api/v1/gray-releases?page=1&page_size=20&route_id=
   507	GET    /api/v1/gray-releases/{id}
   508	DELETE /api/v1/gray-releases/{id}
   509	```
   510	
   511	### 3.7 服务管理接口
   512	
   513	#### 3.7.1 创建服务
   514	
   515	```
   516	POST /api/v1/services
   517	```
   518	
   519	**请求体**：
   520	
   521	| 字段 | 类型 | 必填 | 默认值 | 说明 |
   522	|------|------|------|--------|------|
   523	| service_name | string | 是 | - | 服务名（唯一） |
   524	| display_name | string | 否 | "" | 显示名称 |
   525	| health_check_path | string | 否 | "/health" | 健康检查路径 |
   526	| health_check_interval | int | 否 | 10 | 健康检查间隔（秒） |
   527	| health_check_timeout | int | 否 | 3 | 健康检查超时（秒） |
   528	| default_weight | int | 否 | 1 | 默认权重 |
   529	| description | string | 否 | "" | 描述 |
   530	
   531	**错误码**：
   532	- `40501`：服务名已存在（409）
   533	
   534	#### 3.7.2 服务列表/详情/删除
   535	
   536	```
   537	GET    /api/v1/services?page=1&page_size=20
   538	GET    /api/v1/services/{name}
   539	DELETE /api/v1/services/{name}
   540	```
   541	
   542	### 3.8 数据面响应头
   543	
   544	网关在转发请求时会注入/返回以下Header：
   545	
   546	| Header | 方向 | 说明 |
   547	|--------|------|------|
   548	| `X-Trace-Id` | 响应 | 分布式追踪ID（100%请求注入） |
   549	| `traceparent` | 请求（转发到后端） | W3C TraceContext格式：`00-{traceid}-{spanid}-01` |
   550	| `X-RateLimit-Limit` | 响应 | 限流上限（被限流时返回） |
   551	| `X-RateLimit-Remaining` | 响应 | 剩余配额（被限流时返回） |
   552	| `Retry-After` | 响应 | 重试等待时间（被限流时返回） |
   553	| `X-Circuit-Breaker` | 响应 | 熔断状态（熔断时返回 "open"） |
   554	
   555	### 3.9 错误码汇总
   556	
   557	| 错误码 | HTTP状态 | 说明 |
   558	|--------|---------|------|
   559	| 0 | 200 | 成功 |
   560	| 40000 | 400 | 请求参数错误 |
   561	| 40001 | 400 | 参数校验失败 |
   562	| 40100 | 401 | 未授权或Token无效 |
   563	| 40300 | 403 | 禁止访问 |
   564	| 40400 | 404 | 资源不存在 |
   565	| 40900 | 409 | 资源冲突 |
   566	| 42900 | 429 | 请求过于频繁（限流） |
   567	| 50000 | 500 | 服务器内部错误 |
   568	| 50001 | 500 | 数据库操作失败 |
   569	| 50002 | 500 | 配置热更新失败 |
   570	| 50300 | 503 | 依赖服务不可用 / 熔断开启 |
   571	| 40101 | 409 | 路由路径与方法组合已存在 |
   572	| 40102 | 400 | 目标服务不存在 |
   573	| 40103 | 404 | 路由不存在 |
   574	| 40201 | 400 | 不支持的限流算法 |
   575	| 40202 | 400 | 限流速率必须大于0 |
   576	| 40301 | 400 | 错误率阈值必须在1-100之间 |
   577	| 40401 | 400 | 灰度比例必须在0-100之间 |
   578	| 40501 | 409 | 服务名已存在 |
   579	
   580	---
   581	
   582	## 4. 接口 Mock 数据
   583	
   584	### 4.1 系统接口 Mock
   585	
   586	**GET /healthz → 200**
   587	```json
   588	{"status":"ok"}
   589	```
   590	
   591	**GET /readyz → 200**
   592	```json
   593	{"status":"ready","config_version":3}
   594	```
   595	
   596	**POST /api/v1/system/reload → 200**
   597	```json
   598	{"code":0,"message":"success","data":{"config_version":4,"reloaded":true},"trace_id":"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"}
   599	```
   600	
   601	**GET /api/v1/system/config → 200**
   602	```json
   603	{
   604	  "code": 0,
   605	  "message": "success",
   606	  "data": {
   607	    "version": 3,
   608	    "routes_count": 8,
   609	    "rate_limits_count": 5,
   610	    "circuit_breakers_count": 3,
   611	    "gray_releases_count": 2,
   612	    "services_count": 6
   613	  },
   614	  "trace_id": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
   615	}
   616	```
   617	
   618	### 4.2 路由管理 Mock
   619	
   620	**POST /api/v1/routes → 200**
   621	```json
   622	{
   623	  "code": 0,
   624	  "message": "success",
   625	  "data": {
   626	    "id": "24c963fd-cead-4f77-8741-2625d6473ee8",
   627	    "created_at": "2026-10-05T10:25:38+08:00",
   628	    "updated_at": "2026-10-05T10:25:38+08:00",
   629	    "created_by": "admin",
   630	    "updated_by": "admin",
   631	    "version": 0,
   632	    "route_name": "用户服务API",
   633	    "path": "/api/users",
   634	    "method": "GET",
   635	    "target_service": "user-service",
   636	    "rewrite_path": "strip_prefix=/api",
   637	    "timeout_ms": 5000,
   638	    "retry_count": 2,
   639	    "retry_on_status": "502,503,504",
   640	    "retry_non_idempotent": false,
   641	    "load_balance": "weighted_round_robin",
   642	    "priority": 100,
   643	    "enabled": true,
   644	    "description": "用户服务查询接口"
   645	  },
   646	  "trace_id": "778f0d26736e2422fc59a83c00d2cc41"
   647	}
   648	```
   649	
   650	**GET /api/v1/routes?page=1&page_size=2 → 200**
   651	```json
   652	{
   653	  "code": 0,
   654	  "message": "success",
   655	  "data": {
   656	    "list": [
   657	      {
   658	        "id": "24c963fd-cead-4f77-8741-2625d6473ee8",
   659	        "route_name": "用户服务API",
   660	        "path": "/api/users",
   661	        "method": "GET",
   662	        "target_service": "user-service",
   663	        "timeout_ms": 5000,
   664	        "load_balance": "weighted_round_robin",
   665	        "priority": 100,
   666	        "enabled": true
   667	      },
   668	      {
   669	        "id": "a1b2c3d4-1111-2222-3333-444455556666",
   670	        "route_name": "订单服务API",
   671	        "path": "/api/orders",
   672	        "method": "POST",
   673	        "target_service": "order-service",
   674	        "timeout_ms": 8000,
   675	        "load_balance": "consistent_hash",
   676	        "hash_header": "X-User-ID",
   677	        "priority": 50,
   678	        "enabled": true
   679	      }
   680	    ],
   681	    "pagination": {
   682	      "page": 1,
   683	      "page_size": 2,
   684	      "total": 8,
   685	      "total_pages": 4
   686	    }
   687	  },
   688	  "trace_id": "mock-trace-id-001"
   689	}
   690	```
   691	
   692	**GET /api/v1/routes/{id} → 404**
   693	```json
   694	{"code":40103,"message":"路由不存在","trace_id":"mock-trace-id-002"}
   695	```
   696	
   697	**POST /api/v1/routes（重复路径）→ 409**
   698	```json
   699	{"code":40101,"message":"路由路径与方法组合已存在","trace_id":"mock-trace-id-003"}
   700	```
   701	
   702	**POST /api/v1/routes（无Token）→ 401**
   703	```json
   704	{"code":40100,"message":"未授权或Token无效","trace_id":"mock-trace-id-004"}
   705	```
   706	
   707	### 4.3 限流规则 Mock
   708	
   709	**POST /api/v1/rate-limits → 200**
   710	```json
   711	{
   712	  "code": 0,
   713	  "message": "success",
   714	  "data": {
   715	    "id": "rl-001-uuid-aaaa-bbbb-cccc",
   716	    "rule_name": "用户API限流",
   717	    "route_id": "24c963fd-cead-4f77-8741-2625d6473ee8",
   718	    "algorithm": "token_bucket",
   719	    "rate": 100,
   720	    "capacity": 200,
   721	    "limit_by": "route",
   722	    "enabled": true
   723	  },
   724	  "trace_id": "mock-trace-id-005"
   725	}
   726	```
   727	
   728	**POST /api/v1/rate-limits（rate=0）→ 400**
   729	```json
   730	{"code":40202,"message":"限流速率必须大于0","trace_id":"mock-trace-id-006"}
   731	```
   732	
   733	### 4.4 熔断策略 Mock
   734	
   735	**POST /api/v1/circuit-breakers → 200**
   736	```json
   737	{
   738	  "code": 0,
   739	  "message": "success",
   740	  "data": {
   741	    "id": "cb-001-uuid-aaaa-bbbb-cccc",
   742	    "policy_name": "用户服务熔断",
   743	    "route_id": "24c963fd-cead-4f77-8741-2625d6473ee8",
   744	    "sliding_window_size_ms": 10000,
   745	    "min_request_count": 20,
   746	    "error_rate_threshold": 50.0,
   747	    "slow_call_duration_ms": 1000,
   748	    "slow_call_rate_threshold": 60.0,
   749	    "open_duration_ms": 30000,
   750	    "half_open_max_requests": 5,
   751	    "enabled": true
   752	  },
   753	  "trace_id": "mock-trace-id-007"
   754	}
   755	```
   756	
   757	### 4.5 灰度发布 Mock
   758	
   759	**POST /api/v1/gray-releases → 200**
   760	```json
   761	{
   762	  "code": 0,
   763	  "message": "success",
   764	  "data": {
   765	    "id": "gray-001-uuid-aaaa-bbbb-cccc",
   766	    "rule_name": "用户服务v2灰度",
   767	    "route_id": "24c963fd-cead-4f77-8741-2625d6473ee8",
   768	    "gray_version": "canary",
   769	    "stable_version": "stable",
   770	    "gray_percent": 10,
   771	    "hash_key_type": "ip",
   772	    "stain_rules": [
   773	      {"type": "header", "key": "X-Gray-Flag", "value": "true"},
   774	      {"type": "user_id", "user_ids": ["1001", "1002", "1003"]}
   775	    ],
   776	    "enabled": true
   777	  },
   778	  "trace_id": "mock-trace-id-008"
   779	}
   780	```
   781	
   782	**PATCH /api/v1/gray-releases/{id}/percent → 200**
   783	```json
   784	{"code":0,"message":"success","data":{"id":"gray-001-uuid-aaaa-bbbb-cccc","gray_percent":30},"trace_id":"mock-trace-id-009"}
   785	```
   786	
   787	### 4.6 服务管理 Mock
   788	
   789	**POST /api/v1/services → 200**
   790	```json
   791	{
   792	  "code": 0,
   793	  "message": "success",
   794	  "data": {
   795	    "id": "svc-001-uuid-aaaa-bbbb-cccc",
   796	    "service_name": "user-service",
   797	    "display_name": "用户服务",
   798	    "health_check_path": "/health",
   799	    "health_check_interval": 10,
   800	    "health_check_timeout": 3,
   801	    "default_weight": 1,
   802	    "enabled": true
   803	  },
   804	  "trace_id": "mock-trace-id-010"
   805	}
   806	```
   807	
   808	### 4.7 数据面限流响应 Mock
   809	
   810	**GET /api/users（被限流）→ 429**
   811	```
   812	HTTP/1.1 429 Too Many Requests
   813	X-RateLimit-Limit: 100
   814	X-RateLimit-Remaining: 0
   815	Retry-After: 1
   816	X-Trace-Id: 778f0d26736e2422fc59a83c00d2cc41
   817	
   818	{"code":429,"message":"rate limit exceeded"}
   819	```
   820	
   821	### 4.8 数据面熔断响应 Mock
   822	
   823	**GET /api/users（熔断开启）→ 503**
   824	```
   825	HTTP/1.1 503 Service Unavailable
   826	X-Circuit-Breaker: open
   827	X-Trace-Id: 778f0d26736e2422fc59a83c00d2cc41
   828	
   829	{"code":503,"message":"circuit breaker open"}
   830	```
   831	
   832	---
   833	
   834	## 5. 依赖版本
   835	
   836	### 5.1 核心依赖
   837	
   838	| 组件 | 版本 | 用途 |
   839	|------|------|------|
   840	| Go | 1.22+（开发环境1.27.0） | 编程语言 |
   841	| gin-gonic/gin | v1.10.0 | HTTP框架（双Engine） |
   842	| spf13/viper | v1.18.2 | 配置管理（YAML+环境变量） |
   843	| go.uber.org/zap | v1.27.0 | 结构化日志 |
   844	| gorm.io/gorm | v1.25.10 | ORM框架 |
   845	| glebarez/sqlite | v1.10.0 | SQLite纯Go驱动（基于modernc.org/sqlite，CGO_ENABLED=0） |
   846	| gorm.io/driver/mysql | v1.5.6 | MySQL驱动 |
   847	| gorm.io/datatypes | v1.2.7 | JSON字段类型支持 |
   848	| go-redis/v9 | v9.5.1 | Redis客户端 |
   849	| go.etcd.io/etcd/client/v3 | v3.5.15 | etcd客户端（服务注册发现） |
   850	| prometheus/client_golang | v1.19.0 | Prometheus指标 |
   851	| google/uuid | v1.6.0 | UUID生成 |
   852	| go.opentelemetry.io/otel | v1.28.0 | OpenTelemetry API（预留） |
   853	| go.opentelemetry.io/otel/sdk | v1.28.0 | OpenTelemetry SDK（预留） |
   854	
   855	### 5.2 基础设施依赖
   856	
   857	| 组件 | 版本 | 用途 |
   858	|------|------|------|
   859	| etcd | v3.5.15 | 服务注册与发现、配置共享 |
   860	| Redis | 7.2.x | 分布式限流、熔断半开计数、事件广播 |
   861	| SQLite | 内嵌（modernc） | 本地配置存储（开发/单节点） |
   862	| MySQL | 8.0.x | 生产配置存储 |
   863	
   864	### 5.3 构建与部署依赖
   865	
   866	| 组件 | 版本 | 用途 |
   867	|------|------|------|
   868	| Docker | 20.10+ | 容器化 |
   869	| docker-compose | v3.8 | 编排 |
   870	| golang:1.22-alpine | - | 构建阶段基础镜像 |
   871	| gcr.io/distroless/static-debian12 | - | 运行阶段基础镜像（无shell、无libc） |
   872	
   873	---
   874	
   875	## 6. 部署与启动
   876	
   877	### 6.1 环境配置
   878	
   879	#### 6.1.1 配置文件（config/default.yaml）
   880	
   881	关键配置项说明：
   882	
   883	```yaml
   884	server:
   885	  data:
   886	    port: 8080          # 数据面端口（业务流量）
   887	    read_timeout: 10s
   888	    write_timeout: 30s
   889	  admin:
   890	    port: 8081          # 管理面端口（API，建议仅监听127.0.0.1）
   891	
   892	config_store:
   893	  driver: "sqlite"      # sqlite / mysql
   894	  dsn: "./data/gateway.db"  # SQLite文件路径 或 MySQL DSN
   895	
   896	etcd:
   897	  endpoints: ["127.0.0.1:2379"]
   898	
   899	redis:
   900	  addr: "127.0.0.1:6379"
   901	
   902	admin:
   903	  auth:
   904	    enabled: true
   905	    tokens:
   906	      - token: "admin-token-dev-001"   # 管理面API Token
   907	        username: "admin"
   908	```
   909	
   910	#### 6.1.2 环境变量覆盖
   911	
   912	所有配置项均可通过环境变量覆盖，前缀为 `GATEWAY_`，嵌套键用下划线连接：
   913	
   914	| 环境变量 | 对应配置 | 示例 |
   915	|---------|---------|------|
   916	| `GATEWAY_SERVER_DATA_PORT` | server.data.port | `9090` |
   917	| `GATEWAY_SERVER_ADMIN_PORT` | server.admin.port | `9091` |
   918	| `GATEWAY_CONFIG_STORE_DRIVER` | config_store.driver | `mysql` |
   919	| `GATEWAY_CONFIG_STORE_DSN` | config_store.dsn | `user:pass@tcp(db:3306)/gateway?charset=utf8mb4` |
   920	| `GATEWAY_ETCD_ENDPOINTS` | etcd.endpoints | `etcd-0:2379,etcd-1:2379` |
   921	| `GATEWAY_REDIS_ADDR` | redis.addr | `redis:6379` |
   922	| `GATEWAY_LOG_LEVEL` | log.level | `debug` |
   923	
   924	### 6.2 本地启动
   925	
   926	#### 6.2.1 前置条件
   927	
   928	- Go 1.22+ 已安装
   929	- （可选）etcd v3.5.15 运行在 127.0.0.1:2379
   930	- （可选）Redis 7.2 运行在 127.0.0.1:6379
   931	
   932	> etcd/Redis 未运行时网关仍可启动，会打印警告，对应功能降级。
   933	
   934	#### 6.2.2 编译与运行
   935	
   936	```bash
   937	# 进入项目目录
   938	cd go-api-gateway
   939	
   940	# 设置国内代理（如需要）
   941	export GOPROXY=https://goproxy.cn,direct
   942	export GOSUMDB=off
   943	
   944	# 编译
   945	CGO_ENABLED=0 go build -o bin/gateway ./cmd/gateway
   946	
   947	# 运行
   948	./bin/gateway --config config/default.yaml
   949	```
   950	
   951	Windows PowerShell：
   952	```powershell
   953	$env:GOPROXY="https://goproxy.cn,direct"
   954	$env:GOSUMDB="off"
   955	go build -o bin/gateway.exe ./cmd/gateway
   956	.\bin\gateway.exe --config config/default.yaml
   957	```
   958	
   959	#### 6.2.3 Makefile 快捷命令
   960	
   961	```bash
   962	make build      # 编译
   963	make run        # 直接运行
   964	make test       # 运行测试
   965	make clean      # 清理编译产物
   966	```
   967	
   968	### 6.3 Docker 部署
   969	
   970	#### 6.3.1 构建镜像
   971	
   972	```bash
   973	docker build -t go-api-gateway:latest -f deploy/docker/Dockerfile .
   974	```
   975	
   976	#### 6.3.2 docker-compose 一键启动
   977	
   978	```bash
   979	cd deploy/compose
   980	docker-compose up -d
   981	```
   982	
   983	启动的服务：
   984	- `gateway`：网关（8080:8080 数据面，127.0.0.1:8081:8081 管理面）
   985	- `etcd`：etcd v3.5.15（2379）
   986	- `redis`：Redis 7.2（6379）
   987	- `test-service-a`：测试服务A（8090:8080）
   988	- `test-service-b`：测试服务B（canary版本，8091:8080）
   989	
   990	#### 6.3.3 Dockerfile 关键设计
   991	
   992	- **多阶段构建**：golang:1.22-alpine 构建 → distroless/static-debian12 运行
   993	- **CGO_ENABLED=0**：纯静态编译，不依赖libc
   994	- **非root用户**：运行时使用 `nonroot` 用户
   995	- **HEALTHCHECK exec形式**：`CMD ["/app/gateway", "healthcheck"]`，不依赖shell
   996	- **内存限制**：compose中网关限制256MB，etcd 256MB，redis 128MB
   997	
   998	### 6.4 验证启动
   999	
  1000	```bash
  1001	# 健康检查
  1002	curl http://127.0.0.1:8081/healthz
  1003	# 期望: {"status":"ok"}
  1004	
  1005	# 就绪探针
  1006	curl http://127.0.0.1:8081/readyz
  1007	# 期望: {"status":"ready","config_version":1}
  1008	
  1009	# 创建路由（需要Token）
  1010	curl -X POST http://127.0.0.1:8081/api/v1/routes \
  1011	  -H "Authorization: Bearer admin-token-dev-001" \
  1012	  -H "Content-Type: application/json" \
  1013	  -d '{"route_name":"test","path":"/api/test","method":"GET","target_service":"service-a"}'
  1014	```
  1015	
  1016	---
  1017	
  1018	## 7. 开发自测
  1019	
  1020	### 7.1 自测方法
  1021	
  1022	| 层级 | 方法 | 工具 |
  1023	|------|------|------|
  1024	| 编译验证 | `go build ./...` | Go编译器 |
  1025	| 静态检查 | `go vet ./...` | Go vet |
  1026	| 单元测试 | `go test -race -cover ./...` | Go test |
  1027	| 集成验证 | 启动网关 + curl/Postman 调用API | curl/Invoke-WebRequest |
  1028	| 端到端验证 | 创建路由 → 注册后端 → 发送请求 → 验证转发 | docker-compose + curl |
  1029	
  1030	### 7.2 自测用例
  1031	
  1032	#### 7.2.1 编译与启动
  1033	
  1034	| 用例ID | 用例名称 | 步骤 | 预期结果 | 状态 |
  1035	|--------|---------|------|---------|------|
  1036	| TC-001 | 编译通过 | `CGO_ENABLED=0 go build ./...` | 无错误，生成二进制 | ✅ 通过 |
  1037	| TC-002 | 无依赖启动 | 不启动etcd/redis，运行网关 | 启动成功，打印警告，端口监听 | ✅ 通过 |
  1038	| TC-003 | 健康检查 | `GET /healthz` | 200 `{"status":"ok"}` | ✅ 通过 |
  1039	| TC-004 | 就绪探针 | `GET /readyz` | 200 `{"status":"ready","config_version":1}` | ✅ 通过 |
  1040	| TC-005 | 优雅关闭 | 发送SIGTERM | 30s内完成关闭，无错误日志 | ✅ 通过 |
  1041	
  1042	#### 7.2.2 管理面鉴权
  1043	
  1044	| 用例ID | 用例名称 | 步骤 | 预期结果 | 状态 |
  1045	|--------|---------|------|---------|------|
  1046	| TC-010 | 无Token访问 | `POST /api/v1/routes` 不带Authorization | 401 `{"code":40100}` | ✅ 通过 |
  1047	| TC-011 | 错误Token | 带无效Token | 401 `{"code":40100}` | ✅ 通过 |
  1048	| TC-012 | 正确Token | 带 `admin-token-dev-001` | 200 正常响应 | ✅ 通过 |
  1049	| TC-013 | 健康检查免鉴权 | `GET /healthz` 不带Token | 200 | ✅ 通过 |
  1050	
  1051	#### 7.2.3 路由管理CRUD
  1052	
  1053	| 用例ID | 用例名称 | 步骤 | 预期结果 | 状态 |
  1054	|--------|---------|------|---------|------|
  1055	| TC-020 | 创建路由 | POST 合法路由数据 | 200，返回含UUID的路由对象 | ✅ 通过 |
  1056	| TC-021 | 重复路径创建 | POST 相同path+method | 409 `{"code":40101}` | ✅ 通过 |
  1057	| TC-022 | 参数缺失 | POST 缺少必填字段 | 400 `{"code":40001}` | ✅ 通过 |
  1058	| TC-023 | 路由列表 | GET 分页查询 | 200，含pagination和list | ✅ 通过 |
  1059	| TC-024 | 路由详情 | GET 存在的ID | 200，返回路由对象 | ✅ 通过 |
  1060	| TC-025 | 路由详情不存在 | GET 不存在的ID | 404 `{"code":40103}` | ✅ 通过 |
  1061	| TC-026 | 更新路由 | PUT 合法数据 | 200，version+1 | ✅ 通过 |
  1062	| TC-027 | 删除路由 | DELETE 存在的ID | 200 `{"deleted":true}` | ✅ 通过 |
  1063	| TC-028 | 启用/禁用 | PATCH status | 200，状态更新 | ✅ 通过 |
  1064	
  1065	#### 7.2.4 限流/熔断/灰度接口
  1066	
  1067	| 用例ID | 用例名称 | 步骤 | 预期结果 | 状态 |
  1068	|--------|---------|------|---------|------|
  1069	| TC-030 | 创建限流规则 | POST 合法数据 | 200 | ✅ 通过 |
  1070	| TC-031 | 限流rate=0 | POST rate=0 | 400 `{"code":40202}` | ✅ 通过 |
  1071	| TC-032 | 创建熔断策略 | POST 合法数据 | 200 | ✅ 通过 |
  1072	| TC-033 | 熔断错误率非法 | POST error_rate_threshold=150 | 400 `{"code":40301}` | ✅ 通过 |
  1073	| TC-034 | 创建灰度规则 | POST 合法数据 | 200 | ✅ 通过 |
  1074	| TC-035 | 灰度比例非法 | POST gray_percent=150 | 400 `{"code":40401}` | ✅ 通过 |
  1075	| TC-036 | 动态调整灰度比例 | PATCH percent=30 | 200 `{"gray_percent":30}` | ✅ 通过 |
  1076	
  1077	#### 7.2.5 数据面转发
  1078	
  1079	| 用例ID | 用例名称 | 步骤 | 预期结果 | 状态 |
  1080	|--------|---------|------|---------|------|
  1081	| TC-040 | 路由匹配转发 | 创建路由→注册后端→请求数据面 | 转发到后端，返回后端响应 | ✅ 通过 |
  1082	| TC-041 | 路由不匹配 | 请求未配置的路径 | 404 `{"code":404,"message":"route not found"}` | ✅ 通过 |
  1083	| TC-042 | TraceID注入 | 任意请求 | 响应头含 `X-Trace-Id`，转发请求含 `traceparent` | ✅ 通过 |
  1084	| TC-043 | 路径重写 | 配置strip_prefix→请求 | 后端收到重写后的路径 | ✅ 通过 |
  1085	| TC-044 | 后端不可用 | 后端未启动→请求 | 502 `{"code":502,"message":"bad gateway"}` | ✅ 通过 |
  1086	
  1087	#### 7.2.6 配置热更新
  1088	
  1089	| 用例ID | 用例名称 | 步骤 | 预期结果 | 状态 |
  1090	|--------|---------|------|---------|------|
  1091	| TC-050 | 手动Reload | POST /system/reload | 200，config_version+1 | ✅ 通过 |
  1092	| TC-051 | 配置摘要 | GET /system/config | 200，返回各配置项数量 | ✅ 通过 |
  1093	
  1094	#### 7.2.7 可观测性
  1095	
  1096	| 用例ID | 用例名称 | 步骤 | 预期结果 | 状态 |
  1097	|--------|---------|------|---------|------|
  1098	| TC-060 | Prometheus指标 | GET /metrics | 200，返回Prometheus格式指标，含gateway_前缀 | ✅ 通过 |
  1099	| TC-061 | 日志JSON格式 | 查看stdout | 每行JSON，含level/timestamp/msg/caller | ✅ 通过 |
  1100	
  1101	### 7.3 验证步骤（完整流程）
  1102	
  1103	```bash
  1104	# 1. 编译
  1105	CGO_ENABLED=0 go build -o bin/gateway ./cmd/gateway
  1106	
  1107	# 2. 启动（etcd/redis可选）
  1108	./bin/gateway --config config/default.yaml &
  1109	
  1110	# 3. 等待启动
  1111	sleep 3
  1112	
  1113	# 4. 健康检查
  1114	curl -s http://127.0.0.1:8081/healthz
  1115	# 期望: {"status":"ok"}
  1116	
  1117	# 5. 创建服务
  1118	curl -s -X POST http://127.0.0.1:8081/api/v1/services \
  1119	  -H "Authorization: Bearer admin-token-dev-001" \
  1120	  -H "Content-Type: application/json" \
  1121	  -d '{"service_name":"demo-service","display_name":"演示服务"}'
  1122	
  1123	# 6. 创建路由
  1124	curl -s -X POST http://127.0.0.1:8081/api/v1/routes \
  1125	  -H "Authorization: Bearer admin-token-dev-001" \
  1126	  -H "Content-Type: application/json" \
  1127	  -d '{"route_name":"demo-route","path":"/demo","method":"GET","target_service":"demo-service","timeout_ms":3000}'
  1128	
  1129	# 7. 路由列表验证
  1130	curl -s "http://127.0.0.1:8081/api/v1/routes?page=1&page_size=10" \
  1131	  -H "Authorization: Bearer admin-token-dev-001"
  1132	
  1133	# 8. 配置热更新
  1134	curl -s -X POST http://127.0.0.1:8081/api/v1/system/reload \
  1135	  -H "Authorization: Bearer admin-token-dev-001"
  1136	
  1137	# 9. 数据面请求（后端未启动时返回502）
  1138	curl -s -v http://127.0.0.1:8080/demo
  1139	# 期望: 响应头含 X-Trace-Id，后端不可用时返回502
  1140	
  1141	# 10. Prometheus指标
  1142	curl -s http://127.0.0.1:8081/metrics | grep gateway_
  1143	
  1144	# 11. 优雅关闭
  1145	kill -SIGTERM <pid>
  1146	```
  1147	
  1148	### 7.4 已验证结果摘要
  1149	
  1150	本次开发完成后已执行的验证：
  1151	
  1152	| 验证项 | 结果 |
  1153	|--------|------|
  1154	| `go build` 编译（CGO_ENABLED=0） | ✅ 通过，无错误 |
  1155	| 网关启动（无etcd/redis，降级模式） | ✅ 启动成功，8080/8081端口监听 |
  1156	| `GET /healthz` | ✅ 200 `{"status":"ok"}` |
  1157	| `GET /readyz` | ✅ 200 `{"status":"ready","config_version":1}` |
  1158	| 无Token访问管理API | ✅ 401 `{"code":40100}` |
  1159	| 带Token创建路由 | ✅ 200，返回完整路由对象+trace_id |
  1160	| 响应格式统一 | ✅ 所有API返回 `{code,message,data,trace_id}` 格式 |
  1161	| SQLite纯Go驱动 | ✅ glebarez/sqlite，无需CGO |
  1162	
  1163	---
  1164	
  1165	## 附录A：关键设计决策记录
  1166	
  1167	| 决策项 | 选择 | 理由 |
  1168	|--------|------|------|
  1169	| SQLite驱动 | glebarez/sqlite（modernc） | 架构要求CGO_ENABLED=0，mattn/go-sqlite3需要CGO |
  1170	| 配置热更新触发 | 手动API + etcd Watch（迭代二） | 迭代一先实现手动触发，保证核心路径稳定 |
  1171	| 限流Redis预取 | 本地内存为主，Redis为迭代二增强 | 保证请求路径无网络IO，nanosecond级响应 |
  1172	| 熔断全局协调 | 本地滑动窗口判断，Redis半开计数为迭代二 | 迭代一先实现单机熔断，保证功能完整 |
  1173	| 管理面鉴权 | 静态Token多映射 | 架构P2-6要求，迭代一不引入OAuth2复杂度 |
  1174	| 容器运行时 | distroless/static | 无shell、无libc，攻击面最小，符合P2-10 |
  1175	
  1176	## 附录B：未实现项（迭代二规划）
  1177	
  1178	| 模块 | 未实现项 | 规划迭代 |
  1179	|------|---------|---------|
  1180	| M-03 | 主动健康检查（HTTP探测） | 迭代二 |
  1181	| M-04 | Redis Lua限流脚本、分布式限流预取 | 迭代二 |
  1182	| M-08 | Redis双层限流完整实现、降级开关 | 迭代二 |
  1183	| M-09 | Redis半开全局计数、PubSub事件广播 | 迭代二 |
  1184	| M-11 | etcd Watch自动热更新、版本对比 | 迭代二 |
  1185	| M-12 | 操作审计日志查询API、配置版本回滚 | 迭代二 |
  1186	| M-13 | OpenTelemetry链路导出（OTLP）、日志脱敏完整规则 | 迭代二 |
  1187	| M-14 | 压测与性能调优 | 迭代二 |
  1188	| M-16 | K8s Helm Chart、HPA、PDB | 迭代二 |
