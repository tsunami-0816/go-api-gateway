Read "D:\lg\project\go-api-gateway\技术架构设计文档.md":

     1	# 第二轮技术架构设计文档
     2	
     3	| 项目名称 | Go 语言高性能 API 网关与流量治理平台 |
     4	|---|---|
     5	| 文档版本 | v2.0（第二轮架构设计，修正版） |
     6	| 设计日期 | 2026-10-04 |
     7	| 基线依据 | 《项目基线说明书》v1.0 |
     8	| 评审依据 | 《第一轮架构评审报告》v1.0 |
     9	| 文档性质 | 技术架构指导文件，可直接指导开发实施 |
    10	
    11	---
    12	
    13	## 修订说明
    14	
    15	本版基于《第一轮架构评审报告》对第一轮设计进行了全面修正，所有 P0（2 项）、P1（8 项）、P2（10 项）问题均已落实修复。关键修订点：
    16	
    17	| 编号 | 问题 | 本轮修复方案 | 对应章节 |
    18	|---|---|---|---|
    19	| P0-1 | SQLite 驱动与 distroless/cgo 硬冲突 | 替换为纯 Go 驱动 `modernc.org/sqlite`，编译无需 cgo | 1.1 |
    20	| P0-2 | 限流纯 Redis 方案性能不达标 | 正式纳入"本地令牌桶 + Redis 批量预取"双层架构，请求路径无网络往返 | 3.4 |
    21	| P1-1 | 熔断多实例一致性缺失 | 半开探测数通过 Redis 全局计数；状态变更发布事件；滑动窗口本地判断 | 3.5 |
    22	| P1-2 | 逻辑删除唯一索引冲突 | `is_deleted TINYINT` 改为 `deleted_at DATETIME NULL`，利用 NULL 不参与唯一约束 | 4.2 / 4.3 |
    23	| P1-3 | DDL MySQL 语法与 SQLite 矛盾 | DDL 标注为 MySQL 生产参考；本地由 GORM AutoMigrate 建表；`updated_at` 用 GORM 标签 | 4.3 |
    24	| P1-4 | 限流 key 无 TTL 内存泄漏 | Lua 脚本每次执行后 `EXPIRE` 刷新 TTL=1h | 4.5 / 3.4 |
    25	| P1-5 | HPA 自定义指标缺 Prometheus Adapter | 补充 Prometheus Adapter 部署与指标转换规则；指标名对齐基线 | 6.3.2 / 6.3.4 / 7.1 |
    26	| P1-6 | 测试服务未集成 OTel/数据库 | 明确测试服务规格：集成 OTel SDK、连接 SQLite/MySQL、SQL 注释注入 TraceId | 6.2.1 / 附录 C |
    27	| P1-7 | /readyz 检查外部依赖导致不必要摘流 | 拆分为启动就绪（60s 内查依赖）与运行时就绪（仅进程内部状态） | 5.4.6 / 6.3.3 |
    28	| P1-8 | 管理面端口映射矛盾 | 容器内绑 `0.0.0.0:8081`，compose 映射到宿主机回环 `127.0.0.1:8081:8081` | 6.2.3 |
    29	| P2-1 | 指标名与基线不一致 | 重命名为 `gateway_qps_total`、`gateway_request_duration_seconds`、`gateway_error_rate` | 7.1.1 |
    30	| P2-2 | 一致性哈希缺乏设计 | 虚拟节点倍数 150、murmur3、按权重分配虚拟节点、环更新策略 | 2.2.3 / 3.2 |
    31	| P2-3 | 重试机制无细节 | 重试排除失败实例、总耗时不超 timeout、仅幂等方法重试、重试计入熔断统计 | 3.2 |
    32	| P2-4 | OTel 采样策略未定义 | TraceId 100% 生成；本地全量采样；生产默认 10% 采样；5xx 强制采样 | 3.7 |
    33	| P2-5 | 灰度验证单 IP 失真 | 验证方法明确要求 ≥100 个不同 `X-Forwarded-For` | 3.6 / 7.4.2 |
    34	| P2-6 | 静态 Token 无操作人区分 | 支持 token→username 映射 + `X-Operator` Header 兜底 | 5.3.1 |
    35	| P2-7 | 启动初始化顺序未明确 | 明确 9 步启动时序，数据面最后启动 | 6.3.5 |
    36	| P2-8 | etcd 健康检查缺环境变量 | 增加 `ETCDCTL_API=3`，命令带 `--endpoints` | 6.2.3 |
    37	| P2-9 | 熔断滑动窗口与 Redis 关系不明 | 滑动窗口为本地内存（熔断判断依据）；Redis stats 仅做聚合查询，不参与判断 | 3.5 / 4.5 |
    38	| P2-10 | Dockerfile HEALTHCHECK 混用 shell | 改为纯 exec 形式 `CMD ["/app/gateway", "healthcheck"]` | 6.4 |
    39	
    40	---
    41	
    42	## 目录
    43	
    44	1. [技术选型说明](#1-技术选型说明)
    45	2. [整体分层架构](#2-整体分层架构)
    46	3. [核心业务链路](#3-核心业务链路)
    47	4. [数据模型设计](#4-数据模型设计)
    48	5. [接口规范定义](#5-接口规范定义)
    49	6. [部署架构设计](#6-部署架构设计)
    50	7. [监控与运维](#7-监控与运维)
    51	8. [技术风险与应对方案](#8-技术风险与应对方案)
    52	
    53	---
    54	
    55	## 1. 技术选型说明
    56	
    57	### 1.1 选型总览
    58	
    59	| 技术领域 | 选型组件 | 版本 | 许可证 | 选型理由 |
    60	|---|---|---|---|---|
    61	| 编程语言 | Go | 1.22 | BSD-3-Clause | 基线硬约束（TC-01）；goroutine 高并发、单二进制、内存占用低，满足 ≤80MB 目标 |
    62	| Web 框架 | Gin | v1.10.0 | MIT | 基线硬约束（TC-02）；radix tree 路由匹配性能优；中间件机制适配拦截链 |
    63	| 服务注册与发现 | etcd | v3.5.15 | Apache-2.0 | 基线硬约束（TC-03）；强一致 KV、Lease 自动过期、Watch 实时通知 |
    64	| 缓存/分布式计数 | Redis | 7.2.x | BSD-3-Clause | 基线硬约束（TC-04）；Lua 原子操作支持令牌桶预取；低延迟内存存储 |
    65	| 配置存储（本地） | SQLite | 3.45.x（`modernc.org/sqlite`） | BSD-3-Clause | 基线硬约束（TC-05）；**纯 Go 实现，无需 cgo**，可在 `CGO_ENABLED=0` + distroless/static 下编译运行，彻底解决 P0-1 |
    66	| 配置存储（生产） | MySQL | 8.0.x | GPL-2.0（驱动 MIT） | 基线硬约束（TC-05）；生产成熟稳定，通过驱动抽象与 SQLite 无缝切换 |
    67	| 链路追踪 | OpenTelemetry Go SDK | v1.28.0 | Apache-2.0 | 基线硬约束（TC-08）；CNCF 毕业项目，厂商中立，Trace/Metric/Log 三合一 |
    68	| 指标采集 | prometheus/client_golang | v1.19.0 | Apache-2.0 | 基线硬约束（TC-09）；`/metrics` 端点暴露 Counter/Gauge/Histogram |
    69	| 日志 | zap | v1.27.0 | MIT | 零分配结构化日志，高吞吐下不成为瓶颈 |
    70	| 配置管理 | viper | v1.18.2 | MIT | 多源配置加载与热加载 |
    71	| ORM | GORM | v1.25.10 | MIT | 支持 SQLite/MySQL 双驱动，AutoMigrate 自动建表，连接池管理 |
    72	| 容器基础镜像 | distroless/static-debian12 | latest | Apache-2.0 | 无 shell、无 libc 的极简镜像，网关镜像 ≤200MB（RC-05） |
    73	| API 文档 | swaggo/swag | v1.16.3 | MIT | 注释生成 OpenAPI 3.0，与 Gin 集成 |
    74	
    75	### 1.2 关键依赖版本兼容性说明
    76	
    77	#### 1.2.1 Go 1.22 兼容性
    78	
    79	| 组件 | 最低 Go 版本 | 与 Go 1.22 兼容 |
    80	|---|---|---|
    81	| Gin v1.10.0 | 1.18 | ✅ |
    82	| etcd client v3.5.15 | 1.19 | ✅ |
    83	| OpenTelemetry v1.28.0 | 1.21 | ✅ |
    84	| zap v1.27.0 | 1.19 | ✅ |
    85	| GORM v1.25.10 | 1.18 | ✅ |
    86	| modernc.org/sqlite | 1.18 | ✅（纯 Go，无 cgo） |
    87	
    88	#### 1.2.2 modernc.org/sqlite 与 CGO_ENABLED=0 兼容性（P0-1 修复核心）
    89	
    90	`modernc.org/sqlite` 是 SQLite C 源码经 `ccgo` 翻译为纯 Go 的实现，**不依赖 cgo、不依赖系统 libc**。这使得：
    91	
    92	1. 编译：`CGO_ENABLED=0 go build` 可直接通过
    93	2. 运行：可在 `gcr.io/distroless/static-debian12:nonroot`（无 libc）中正常运行
    94	3. 驱动注册：
    95	
    96	```go
    97	import (
    98	    "gorm.io/driver/sqlite"  // GORM 的 SQLite 驱动底层使用 modernc.org/sqlite
    99	    "gorm.io/gorm"
   100	)
   101	// 或显式使用 modernc
   102	import _ "modernc.org/sqlite"
   103	```
   104	
   105	> 注：GORM 官方 `gorm.io/driver/sqlite` v1.5.4+ 底层已支持 `modernc.org/sqlite`，通过 build tag `sqlite_modernc` 或默认纯 Go 路径选择。生产 MySQL 使用 `gorm.io/driver/mysql`。
   106	
   107	#### 1.2.3 SQLite ↔ MySQL 切换兼容性
   108	
   109	通过 GORM 驱动抽象层实现，切换仅修改配置：
   110	
   111	```yaml
   112	# 本地
   113	config_store:
   114	  driver: sqlite
   115	  dsn: "./data/gateway.db"
   116	
   117	# 生产
   118	config_store:
   119	  driver: mysql
   120	  dsn: "user:pwd@tcp(mysql-host:3306)/gateway?charset=utf8mb4&parseTime=True"
   121	```
   122	
   123	**SQL 兼容性约束**（P1-3 修复）：
   124	- 不使用数据库特定函数（`IFNULL`/`NOW()`/`CONCAT`），统一用 `COALESCE` 或 GORM 方法
   125	- 主键使用 UUID（`VARCHAR(36)`），不依赖自增，彻底规避自增语法差异
   126	- 时间字段统一 `time.Time`，由 GORM 映射
   127	- `updated_at` 不使用 `ON UPDATE CURRENT_TIMESTAMP`，改用 GORM `autoUpdateTime` 标签
   128	- **不手写 DDL 初始化**：本地由 `GORM AutoMigrate` 建表；生产首次部署时由 GORM AutoMigrate 或 DBA 审核后的 DDL 脚本建表
   129	- 不使用存储过程、触发器、视图
   130	
   131	#### 1.2.4 etcd 版本兼容性
   132	
   133	- 本地：etcd v3.5.15 单节点，API v3
   134	- 生产：etcd v3.5.x 三节点集群，同大版本 API 完全兼容
   135	- 网关通过 `go.etcd.io/etcd/client/v3` 连接，不依赖特定拓扑
   136	
   137	#### 1.2.5 OpenTelemetry 与 Jaeger 兼容性
   138	
   139	- OTel SDK v1.28.0 通过 OTLP 导出
   140	- Jaeger v1.55+ 原生支持 OTLP
   141	- 本地使用 `stdout` exporter（控制台打印）；生产切换 OTLP exporter 指向 Jaeger
   142	
   143	### 1.3 不引入的技术及原因
   144	
   145	| 技术 | 不引入原因 | 基线依据 |
   146	|---|---|---|
   147	| mattn/go-sqlite3 | 依赖 cgo，与 `CGO_ENABLED=0` + distroless 冲突（P0-1） | TC-01/TC-05 |
   148	| Consul / Nacos / Eureka | 基线指定 etcd | TC-03 |
   149	| MongoDB / PostgreSQL | 基线指定 SQLite/MySQL | TC-05 |
   150	| Zipkin Brave / SkyWalking | 基线指定 OpenTelemetry | TC-08 |
   151	| Echo / Fiber / net/http 裸写 | 基线指定 Gin | TC-02 |
   152	| Kafka / RabbitMQ | 网关同步转发，不做异步削峰 | E-08 |
   153	| WASM / Lua 插件运行时 | 治理逻辑原生 Go 实现 | E-09 |
   154	| OAuth2 / JWT 认证模块 | 不在本次基线范围内 | E-01 |
   155	| WebSocket / gRPC 代理 | 仅支持 HTTP/HTTPS | E-04 |
   156	
   157	---
   158	
   159	## 2. 整体分层架构
   160	
   161	### 2.1 架构分层总览
   162	
   163	系统采用六层架构，自上而下：**接入层 → 应用层 → 服务层 → 计算层 → 存储层**，运维层为横向支撑层贯穿所有层级。
   164	
   165	```
   166	┌─────────────────────────────────────────────────────────────────┐
   167	│                        接入层 (Access Layer)                      │
   168	│  数据面 HTTP Server · 管理面 HTTP Server · 端口隔离 · 连接管理    │
   169	├─────────────────────────────────────────────────────────────────┤
   170	│                        应用层 (Application Layer)                 │
   171	│  路由匹配引擎 · 请求拦截链 · 响应拦截链 · 路径重写 · 反向代理      │
   172	│  管理API控制器                                                    │
   173	├─────────────────────────────────────────────────────────────────┤
   174	│                        服务层 (Service Layer)                     │
   175	│  服务注册 · 服务发现 · 健康检查 · 负载均衡 · 流量治理执行器        │
   176	├─────────────────────────────────────────────────────────────────┤
   177	│                        计算层 (Computation Layer)                 │
   178	│  双层限流引擎 · 熔断状态机(全局协调) · 灰度路由计算 · 滑动窗口     │
   179	│  配置热更新处理器 · 一致性哈希环                                  │
   180	├─────────────────────────────────────────────────────────────────┤
   181	│                        存储层 (Storage Layer)                     │
   182	│  配置存储(SQLite/MySQL) · 服务注册(etcd) · 计数缓存(Redis)        │
   183	├─────────────────────────────────────────────────────────────────┤
   184	│  ┌───────────────────────────────────────────────────────────┐  │
   185	│  │                  运维层 (Operations Layer)                  │  │
   186	│  │  日志(zap) · 指标(Prometheus) · 链路(OTel) · 告警 · 部署   │  │
   187	│  └───────────────────────────────────────────────────────────┘  │
   188	└─────────────────────────────────────────────────────────────────┘
   189	```
   190	
   191	### 2.2 各层职责与组件详解
   192	
   193	#### 2.2.1 接入层（Access Layer）
   194	
   195	| 组件 | 职责 | 技术实现 |
   196	|---|---|---|
   197	| 数据面 HTTP Server | 接收客户端业务请求并转发 | Gin Engine（监听 0.0.0.0:8080） |
   198	| 管理面 HTTP Server | 配置 CRUD 接口 | Gin Engine（监听 0.0.0.0:8081） |
   199	| 指标端点 | Prometheus 指标采集 | `/metrics`（8081） |
   200	| 健康检查端点 | K8s 探针 | `/healthz`、`/readyz`（8081） |
   201	| 连接管理器 | 最大连接数、超时、Keep-Alive | net/http Server 参数 |
   202	
   203	**端口隔离设计**（P1-8 修复）：
   204	
   205	| 端口 | 用途 | 容器内绑定 | 宿主机映射（本地 compose） | 说明 |
   206	|---|---|---|---|---|
   207	| 8080 | 数据面（业务转发） | 0.0.0.0:8080 | `8080:8080` | 对外暴露 |
   208	| 8081 | 管理面（API+指标+健康） | 0.0.0.0:8081 | `127.0.0.1:8081:8081` | 仅宿主机回环可访问，局域网不可达 |
   209	
   210	> 关键决策：容器内管理面绑 `0.0.0.0`（使 Docker 端口映射生效），但通过宿主机回环映射 `127.0.0.1:8081:8081` 保证安全性。生产环境通过 K8s NetworkPolicy 限制访问源。
   211	
   212	#### 2.2.2 应用层（Application Layer）
   213	
   214	| 组件 | 职责 | 技术实现 |
   215	|---|---|---|
   216	| 路由匹配引擎 | Path + Method 匹配规则 | Gin radix tree + 自定义规则缓存 |
   217	| 请求拦截链 | Trace注入→限流→熔断→灰度→路径重写→日志 | Gin Middleware 链 |
   218	| 响应拦截链 | 指标采集→熔断统计→日志→Span结束 | Gin 后置钩子 |
   219	| 路径重写器 | 正则替换 + 前缀截断 | 自定义 Rewriter |
   220	| 反向代理器 | 转发至后端实例 | `httputil.ReverseProxy` 定制 |
   221	| 管理API控制器 | 路由/限流/熔断/灰度 CRUD | Gin Handler + Service |
   222	| 重试执行器 | 失败请求重试（仅幂等方法） | 自定义 RetryHandler（P2-3） |
   223	
   224	**请求处理流水线**：
   225	
   226	```
   227	客户端请求
   228	  ▼
   229	[接入层] 接收解析
   230	  ▼
   231	[应用层-拦截链] Trace注入 → 限流检查(本地双层) → 熔断检查 → 灰度判定 → 路径重写
   232	  ▼
   233	[服务层] 服务发现(本地缓存) → 过滤不健康 → 负载均衡(加权轮询/一致性哈希)
   234	  ▼
   235	[反向代理] 构建请求 → 注入traceparent → 转发后端（含重试）
   236	  ▼
   237	[应用层-响应链] 指标采集 → 熔断统计 → 日志 → Span结束
   238	  ▼
   239	响应返回客户端
   240	```
   241	
   242	#### 2.2.3 服务层（Service Layer）
   243	
   244	| 组件 | 职责 | 技术实现 |
   245	|---|---|---|
   246	| 服务注册器 | 服务启动注册至 etcd | etcd client v3 + Lease（TTL=10s） |
   247	| 服务发现器 | 订阅实例变更 | etcd Watch + 本地 atomic.Value 缓存 |
   248	| 健康检查器 | 主动探测，异常摘除 | 定时 HTTP GET /health |
   249	| 负载均衡器 | 选择目标实例 | 加权轮询 / 一致性哈希（带权） |
   250	| 限流执行器 | 接口级限流 | 调用计算层双层限流引擎 |
   251	| 熔断器 | 下游故障快速失败 | 计算层熔断状态机（全局协调） |
   252	| 灰度路由器 | 按比例/染色路由版本 | 加权随机 / Header匹配 |
   253	
   254	**负载均衡策略设计**（P2-2 修复-一致性哈希）：
   255	
   256	| 策略 | 实现要点 |
   257	|---|---|
   258	| 加权轮询（WRR） | 经典平滑加权轮询（nginx 算法），按权重分配请求 |
   259	| 一致性哈希（CH） | ① 哈希算法：murmur3（32bit）② 虚拟节点倍数：150 ③ 总虚拟节点数 = Σ(实例权重 × 150) ④ 每个实例按权重比例分配虚拟节点 ⑤ 请求 key（IP/指定Header）哈希后在环上顺时针查找最近虚拟节点 ⑥ 实例上下线时重建环（影响约 1/N 的请求） |
   260	
   261	#### 2.2.4 计算层（Computation Layer）
   262	
   263	| 组件 | 职责 | 技术实现 |
   264	|---|---|---|
   265	| 双层限流引擎 | 本地令牌桶/漏桶 + Redis 批量预取 | 见 3.4 节（P0-2 修复） |
   266	| 熔断状态机 | Closed→Open→HalfOpen | 见 3.5 节（P1-1 修复，全局协调） |
   267	| 滑动窗口统计器 | 错误率/慢调用比例 | 本地时间轮+环形缓冲区（熔断判断依据） |
   268	| 灰度路由计算器 | 哈希/随机判定版本 | murmur3 哈希取模 |
   269	| 配置热更新处理器 | 监听变更，原子替换快照 | etcd Watch + atomic.Value |
   270	| 一致性哈希环 | 带权虚拟节点环 | 见 2.2.3 |
   271	
   272	#### 2.2.5 存储层（Storage Layer）
   273	
   274	| 存储组件 | 存储内容 | 访问模式 | 本地 | 生产 |
   275	|---|---|---|---|---|
   276	| SQLite / MySQL | 路由/限流/熔断/灰度配置 | 管理面读写 + 数据面读 | 嵌入式单文件 | MySQL 主从 |
   277	| etcd | 服务实例注册 | 写(注册)+读/Watch(发现) | 单节点 | 三节点集群 |
   278	| Redis | 限流预取令牌/熔断全局状态/灰度计数 | 低频同步读写 + Lua | 单节点 | 主从+哨兵 |
   279	
   280	#### 2.2.6 运维层（Operations Layer）
   281	
   282	| 组件 | 职责 | 技术实现 |
   283	|---|---|---|
   284	| 日志系统 | 结构化日志含 TraceId | zap + 异步写入 |
   285	| 指标系统 | QPS/延迟/错误率等 | prometheus/client_golang |
   286	| 链路追踪 | TraceId 生成透传、Span | OpenTelemetry Go SDK |
   287	| 健康检查 | 存活/就绪探针 | 自定义 HealthIndicator（P1-7 修复） |
   288	| 配置管理 | 多源配置热加载 | viper |
   289	| 部署编排 | Compose / K8s | YAML 清单 |
   290	
   291	---
   292	
   293	## 3. 核心业务链路
   294	
   295	### 3.1 业务链路总览
   296	
   297	| 编号 | 链路名称 | 触发方 | 核心节点 |
   298	|---|---|---|---|
   299	| FL-01 | HTTP 请求转发链路 | 客户端 | 接入→拦截链→路由→服务发现→负载均衡→反向代理(含重试)→响应拦截 |
   300	| FL-02 | 服务注册与发现链路 | 后端服务 | 启动→etcd注册(Lease)→网关Watch→本地缓存→健康检查 |
   301	| FL-03 | 限流执行链路（双层） | 网关数据面 | 请求→本地令牌桶(纳秒级)→后台协程批量预取Redis→降级本地模式 |
   302	| FL-04 | 熔断执行链路（全局协调） | 网关数据面 | 状态检查→本地滑动窗口统计→状态转移→半开全局计数→事件广播 |
   303	| FL-05 | 灰度发布链路 | 网关数据面 | 染色判定→哈希比例计算→版本过滤 |
   304	| FL-06 | 配置热更新链路 | 管理面 | 写入DB→etcd事件→网关Watch→全量拉取→原子替换快照 |
   305	| FL-07 | 全链路追踪链路 | 网关数据面 | TraceId生成→Span→上下文透传→服务继承→DB注释注入→输出 |
   306	
   307	### 3.2 FL-01：HTTP 请求转发链路
   308	
   309	```
   310	客户端              网关数据面(8080)                  后端服务
   311	  │                    │                               │
   312	  │ ① HTTP Request     │                               │
   313	  │───────────────────>│                               │
   314	  │                    │ ② Trace中间件：提取/生成TraceId│
   315	  │                    │ ③ 限流：本地令牌桶扣减(无网络) │
   316	  │                    │    ├ 被限流 → 429             │
   317	  │                    │    └ 通过 → 继续              │
   318	  │                    │ ④ 熔断：读取熔断器状态         │
   319	  │                    │    ├ Open → 503 快速失败      │
   320	  │                    │    └ HalfOpen/Closed → 继续   │
   321	  │                    │ ⑤ 灰度：染色判定+哈希比例      │
   322	  │                    │ ⑥ 路由匹配(radix tree)+重写    │
   323	  │                    │ ⑦ 服务发现(本地缓存,无网络)    │
   324	  │                    │ ⑧ 负载均衡(WRR/CH)选实例       │
   325	  │                    │ ⑨ 反向代理转发(含重试)         │
   326	  │──────────────────────────────────────────────────>│
   327	  │                    │ ⑩ 响应拦截：指标+熔断统计+日志  │
   328	  │ <──────────────────│                               │
   329	```
   330	
   331	**各节点输入/处理/输出**：
   332	
   333	| 节点 | 输入 | 处理逻辑 | 输出 |
   334	|---|---|---|---|
   335	| ② Trace注入 | traceparent Header | 存在则提取上下文，不存在生成新 TraceId；创建 SERVER Span | 上下文注入 Context；TraceId 写响应 Header |
   336	| ③ 限流 | Path+Method+IP | 匹配限流规则；调用本地双层限流引擎（见 FL-03） | 放行/429 |
   337	| ④ 熔断 | 目标服务+路由ID | 读熔断器状态（本地优先，Redis 同步） | 放行/503 |
   338	| ⑤ 灰度 | Header/Cookie/IP | 染色匹配优先，否则哈希比例判定 | 版本标签 stable/canary |
   339	| ⑥ 路由匹配 | Path+Method | radix tree 匹配 → 路径重写 → 目标服务 | 路由配置+重写后Path |
   340	| ⑦ 服务发现 | 服务名+版本 | 本地缓存过滤不健康实例+按版本过滤 | 健康实例列表 |
   341	| ⑧ 负载均衡 | 实例列表+策略 | WRR 或 CH 选择 | 目标实例 Address |
   342	| ⑨ 反向代理 | 原始请求+地址 | 构建请求、注入 traceparent、设置超时、转发；失败按策略重试 | 后端响应 |
   343	| ⑩ 响应拦截 | 响应+上下文 | 记录延迟直方图、状态码计数、熔断滑动窗口、访问日志、结束 Span | 最终响应 |
   344	
   345	**重试机制设计**（P2-3 修复）：
   346	
   347	| 设计项 | 规则 |
   348	|---|---|
   349	| 重试触发 | 路由 `retry_count > 0` 且响应状态码命中 `retry_on_status`（默认 502,503,504）或网络错误 |
   350	| 实例选择 | 重试时**排除已失败实例**，重新从健康列表选择（避免同一实例反复失败） |
   351	| 超时约束 | 重试总耗时不超过路由 `timeout_ms`，超出则放弃重试返回最后一次结果 |
   352	| 幂等限制 | 默认仅对幂等方法（GET/HEAD/OPTIONS/PUT/DELETE）重试；非幂等（POST）不重试，除非路由显式配置 `retry_non_idempotent=true` |
   353	| 熔断统计 | 重试的每次结果（成功/失败）均计入熔断滑动窗口统计 |
   354	| 最大重试 | `retry_count` 默认 0（不重试），最大 3 |
   355	
   356	### 3.3 FL-02：服务注册与发现链路
   357	
   358	**服务注册（后端服务侧）**：
   359	
   360	```
   361	服务启动 → 连接etcd → 创建Lease(TTL=10s) → 注册Key /services/{name}/{id}
   362	  → Value={address,weight,metadata{version},register_time} → 绑定Lease
   363	  → KeepAlive协程(每5s续约) → 正常运行
   364	  → 服务关闭 → 撤销Lease → Key自动过期
   365	```
   366	
   367	**服务发现（网关侧）**：
   368	
   369	```
   370	网关启动 → 连接etcd → 全量拉取 /services/ 前缀构建初始缓存
   371	  → Watch /services/ 前缀(PUT/DELETE事件) → 事件更新本地缓存
   372	  → atomic.Value 原子替换缓存(读无锁) → 健康检查协程(每10s探测/health)
   373	  → 连续2次失败标记Unhealthy剔除 → 连续2次成功恢复Healthy
   374	```
   375	
   376	**关键参数**：Lease TTL=10s；KeepAlive=5s；健康检查间隔=10s；失败/恢复阈值=2次；下线感知≤10s。
   377	
   378	### 3.4 FL-03：限流执行链路（双层架构，P0-2 修复核心）
   379	
   380	**架构原则**：请求路径只操作本地内存（纳秒级），Redis 仅用于多实例间令牌批量预取与全局协调。Redis 调用频率从 10000 次/s 降至每实例 10 次/s。
   381	
   382	**令牌桶双层流程**：
   383	
   384	```
   385	请求到达
   386	  ▼
   387	① 匹配限流规则(route-id, algorithm=token_bucket, rate, capacity)
   388	  ▼
   389	② 操作本地令牌桶(内存, 无网络):
   390	   - 计算自上次补充至今应补充的令牌数 = elapsed × rate
   391	   - tokens = min(capacity, tokens + refill)
   392	   - 若 tokens >= 1: tokens -= 1, 放行
   393	   - 若 tokens < 1: 拒绝, 返回 429
   394	  ▼
   395	③ 后台同步协程(每100ms执行, 与请求路径解耦):
   396	   - 计算本地令牌桶消耗速率
   397	   - 从 Redis 全局桶批量预取令牌到本地:
   398	     * 预取量 = rate × 0.1s × 预取系数(默认2)
   399	     * Lua 脚本原子扣减全局桶并返回实际预取量
   400	     * 若全局桶不足, 预取剩余量
   401	   - 若 Redis 不可用: 本地令牌桶独立运行(降级模式, 精度下降)
   402	  ▼
   403	④ Redis 全局桶补充(每1s执行):
   404	   - 按 rate 向全局桶补充令牌, 不超过 capacity
   405	   - Lua 脚本保证原子性
   406	```
   407	
   408	**漏桶双层流程**：
   409	
   410	```
   411	请求到达
   412	  ▼
   413	① 操作本地漏桶(内存):
   414	   - 漏水: elapsed × rate, current_level = max(0, level - leaked)
   415	   - 若 level < capacity: level += 1, 放行
   416	   - 否则拒绝 429
   417	  ▼
   418	② 后台同步(每100ms): 与 Redis 全局水位对齐(批量同步)
   419	```
   420	
   421	**Redis 批量预取 Lua 脚本伪代码**：
   422	
   423	```lua
   424	-- KEYS[1] = ratelimit:{route-id}:token_bucket:global
   425	-- ARGV[1] = rate, ARGV[2] = capacity, ARGV[3] = prefetch_amount, ARGV[4] = now
   426	local tokens = tonumber(redis.call('HGET', KEYS[1], 'tokens') or 0)
   427	local last_refill = tonumber(redis.call('HGET', KEYS[1], 'last_refill_time') or 0)
   428	local refill = math.floor((tonumber(ARGV[4]) - last_refill) * tonumber(ARGV[1]) / 1000)
   429	tokens = math.min(tonumber(ARGV[2]), tokens + refill)
   430	local fetched = math.min(tokens, tonumber(ARGV[3]))
   431	tokens = tokens - fetched
   432	redis.call('HMSET', KEYS[1], 'tokens', tokens, 'last_refill_time', ARGV[4])
   433	redis.call('EXPIRE', KEYS[1], 3600)  -- P1-4: 刷新TTL=1h
   434	return fetched
   435	```
   436	
   437	**降级策略**：
   438	
   439	| 场景 | 行为 |
   440	|---|---|
   441	| Redis 正常 | 双层模式，本地预取令牌，精度高 |
   442	| Redis 不可用 | 自动切换纯本地模式，限流继续生效（多实例近似），恢复后自动切回 |
   443	| 本地令牌耗尽且 Redis 预取失败 | 按本地剩余令牌放行，不阻塞请求 |
   444	
   445	**响应 Header**：`X-RateLimit-Limit`、`X-RateLimit-Remaining`、`X-RateLimit-Reset`、`Retry-After`
   446	
   447	### 3.5 FL-04：熔断执行链路（全局协调，P1-1 修复）
   448	
   449	**设计原则**：熔断判断基于本地滑动窗口（性能优先）；半开探测请求数通过 Redis 全局计数避免放大；熔断状态变更通过 Redis 发布事件广播同步。
   450	
   451	```
   452	请求到达(目标服务/路由)
   453	  ▼
   454	① 读取熔断器状态: 本地缓存优先(Redis 同步), 三态 CLOSED/OPEN/HALF_OPEN
   455	  │
   456	  ├─ CLOSED(正常)
   457	  │    ▼
   458	  │  ② 放行, 记录到本地滑动窗口(时间轮+环形缓冲区, 10s/10桶)
   459	  │    ▼
   460	  │  ③ 统计: 总请求数、错误率、慢调用比例
   461	  │    ▼
   462	  │  ④ 触发条件(任一满足): 总请求≥min(20) AND (错误率≥50% OR 慢调用比例≥60%)
   463	  │    ├ 满足 → 状态转 OPEN, 写Redis+发布事件, 记录打开时间
   464	  │    └ 不满足 → 保持 CLOSED
   465	  │
   466	  ├─ OPEN(熔断中)
   467	  │    ▼
   468	  │  ⑤ 快速失败 503, Header: X-Circuit-Breaker: open
   469	  │    ▼
   470	  │  ⑥ 到达熔断超时(30s) → 状态转 HALF_OPEN, 写Redis+发布事件
   471	  │
   472	  └─ HALF_OPEN(半开探测)
   473	       ▼
   474	       ⑦ Redis 原子递增全局半开探测计数(INCR circuit:{svc}:{route}:halfopen_count)
   475	          对比阈值 half_open_max_requests(5):
   476	            - count <= 5: 放行探测请求
   477	            - count > 5: 快速失败
   478	       ▼
   479	       ⑧ 探测请求结束统计:
   480	          - 全部成功 → 状态转 CLOSED, 重置统计与全局计数, 发布事件
   481	          - 任一失败 → 状态转 OPEN, 重新计时, 发布事件
   482	```
   483	
   484	**多实例一致性保证**（P1-1 方案 A）：
   485	
   486	| 维度 | 本地 | Redis（全局） |
   487	|---|---|---|
   488	| 滑动窗口统计 | ✅ 本实例独立统计 | ❌ 不参与判断（P2-9 修复） |
   489	| 熔断触发判断 | ✅ 基于本地窗口 | ❌ |
   490	| 半开探测计数 | ❌ | ✅ `INCR` 全局计数，防止探测数放大 |
   491	| 熔断状态读写 | ✅ 本地缓存(读优先) | ✅ 状态变更时 SET + 发布事件 |
   492	| 状态同步 | ✅ 订阅 Redis 事件更新本地 | - |
   493	
   494	**熔断策略参数**：
   495	
   496	| 参数 | 默认值 |
   497	|---|---|
   498	| 滑动窗口大小 | 10s |
   499	| 最小请求数 | 20 |
   500	| 错误率阈值 | 50% |
   501	| 慢调用延迟阈值 | 1s |
   502	| 慢调用比例阈值 | 60% |
   503	| 熔断超时 | 30s |
   504	| 半开探测请求数 | 5（全局） |
   505	
   506	### 3.6 FL-05：灰度发布链路
   507	
   508	```
   509	请求到达 → 检查灰度规则
   510	  ▼
   511	① 染色判定(最高优先级): 遍历 stain_rules
   512	   - Header/Cookie/Query 精确匹配 / UserID IN 列表
   513	   ├ 命中 → 100% 路由灰度版
   514	   └ 未命中 → 继续
   515	  ▼
   516	② 比例判定: hash_key 默认客户端IP, hash_value = murmur3(key) % 100
   517	   ├ hash_value < gray_percent → 灰度版
   518	   └ hash_value >= gray_percent → 稳定版
   519	  ▼
   520	③ 版本标签写入上下文, 服务发现按 Metadata.version 过滤实例
   521	```
   522	
   523	**灰度比例精度验证**（P2-5 修复）：基线 G-05 验证必须使用 ≥100 个不同 `X-Forwarded-For` 值模拟多用户，否则单 IP 哈希导致比例失真。1000 笔请求中灰度版接收量预期 180-220（18%-22%）。
   524	
   525	### 3.7 FL-06：配置热更新链路
   526	
   527	```
   528	管理API修改配置 → Token鉴权+校验 → 写入SQLite/MySQL
   529	  → 记录gw_operation_log + gw_config_version(变更快照)
   530	  → 发布etcd事件 /config/changes/{type}/{ts} (TTL=60s)
   531	  → 网关Watch接收 → 全量拉取对应类型配置 → 构建不可变快照
   532	  → atomic.Value 原子替换(读无锁, 0失败) → 新配置生效(≤5s)
   533	  → 日志记录 config reloaded
   534	```
   535	
   536	**兜底**：每 60s 定时全量同步，即使 Watch 事件丢失也能收敛。
   537	
   538	### 3.8 FL-07：全链路追踪链路
   539	
   540	**采样策略**（P2-4 修复）：
   541	
   542	| 环境 | TraceId 生成 | Span 采样 | 说明 |
   543	|---|---|---|---|
   544	| 本地 | 100% | 全量采样（异步日志输出） | 基线 T-02 控制台打印 |
   545	| 生产 | 100% | 默认 10%（可配置）；5xx 错误强制采样 | 控制开销，错误全保留 |
   546	
   547	> TraceId 必须 100% 生成透传（基线 T-01），采样仅控制 Span 是否上报导出器，不影响 TraceId 透传与日志记录。
   548	
   549	**链路**：网关生成/提取 TraceId → 创建 SERVER Span → 注入 traceparent Header → 后端服务提取创建子 Span → DB 操作时 SQL 注释 `/* trace_id=xxx */` 注入 → Span 结束 → 本地 stdout / 生产 OTLP→Jaeger。
   550	
   551	---
   552	
   553	## 4. 数据模型设计
   554	
   555	### 4.1 数据库设计总览
   556	
   557	配置存储使用 SQLite（本地，GORM AutoMigrate 建表）/ MySQL（生产），核心表：
   558	
   559	| 表名 | 用途 | 模块 |
   560	|---|---|---|
   561	| `gw_route` | 路由配置 | M-01 |
   562	| `gw_rate_limit` | 限流规则 | M-03 |
   563	| `gw_circuit_breaker` | 熔断策略 | M-03 |
   564	| `gw_gray_release` | 灰度规则 | M-03 |
   565	| `gw_service` | 服务元信息 | M-02 |
   566	| `gw_config_version` | 配置变更审计 | 运维 |
   567	| `gw_operation_log` | 操作审计日志 | 运维 |
   568	
   569	> **DDL 说明**（P1-3 修复）：以下 DDL 为 MySQL 生产环境参考，**本地 SQLite 由 GORM AutoMigrate 自动建表，请勿直接执行以下 DDL**。DDL 中的 MySQL 特有语法（ENGINE、CHARSET、COMMENT）仅为生产参考。
   570	
   571	### 4.2 字段规范
   572	
   573	**通用字段规范**（P1-2 修复：逻辑删除改为 `deleted_at`）：
   574	
   575	| 字段名 | 类型 | 说明 | 约束 |
   576	|---|---|---|---|
   577	| `id` | VARCHAR(36) | 主键 UUID v4 | NOT NULL, PRIMARY KEY |
   578	| `created_at` | DATETIME | 创建时间 | NOT NULL, GORM `autoCreateTime` |
   579	| `updated_at` | DATETIME | 更新时间 | NOT NULL, GORM `autoUpdateTime`（**不使用 ON UPDATE CURRENT_TIMESTAMP**） |
   580	| `created_by` | VARCHAR(64) | 创建人 | NOT NULL, DEFAULT 'system' |
   581	| `updated_by` | VARCHAR(64) | 更新人 | NOT NULL, DEFAULT 'system' |
   582	| `deleted_at` | DATETIME NULL | 逻辑删除时间，NULL=未删除 | 唯一索引包含此字段（NULL 不参与唯一约束） |
   583	| `version` | INT | 乐观锁版本号 | NOT NULL, DEFAULT 0 |
   584	
   585	**逻辑删除设计**（P1-2 修复核心）：
   586	- `deleted_at` 为 NULL 表示未删除，有值表示已删除（记录删除时间）
   587	- 唯一索引包含 `deleted_at`：MySQL/SQLite 中 NULL 不参与唯一约束比较
   588	- 因此多条已删除记录（`deleted_at` 各不相同）不会冲突，同一时刻最多一条 `deleted_at=NULL`（未删除记录唯一）
   589	- GORM 模型使用 `gorm.DeletedAt` 类型自动实现
   590	
   591	**命名规范**：表名小写蛇形 `gw_` 前缀；字段小写蛇形；索引 `idx_{表}_{字段}`、唯一 `uk_{表}_{字段}`；时间存 UTC。
   592	
   593	### 4.3 核心表结构设计（MySQL 参考 DDL）
   594	
   595	#### 4.3.1 路由配置表 `gw_route`
   596	
   597	```sql
   598	CREATE TABLE gw_route (
   599	    id              VARCHAR(36)   NOT NULL COMMENT '路由ID, UUID',
   600	    route_name      VARCHAR(128)  NOT NULL COMMENT '路由名称',
   601	    path            VARCHAR(512)  NOT NULL COMMENT '匹配路径, 支持前缀匹配',
   602	    method          VARCHAR(16)   NOT NULL COMMENT 'HTTP方法, GET/POST/*',
   603	    target_service  VARCHAR(128)  NOT NULL COMMENT '目标服务名',
   604	    rewrite_path    VARCHAR(512)  DEFAULT NULL COMMENT '路径重写规则',
   605	    timeout_ms      INT           NOT NULL DEFAULT 5000 COMMENT '后端超时ms',
   606	    retry_count     INT           NOT NULL DEFAULT 0 COMMENT '重试次数',
   607	    retry_on_status VARCHAR(256)  DEFAULT NULL COMMENT '重试状态码逗号分隔',
   608	    retry_non_idempotent TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否允许非幂等重试',
   609	    load_balance    VARCHAR(32)   NOT NULL DEFAULT 'weighted_round_robin',
   610	    hash_header     VARCHAR(64)   DEFAULT NULL COMMENT '一致性哈希Header名',
   611	    priority        INT           NOT NULL DEFAULT 100 COMMENT '优先级越小越高',
   612	    enabled         TINYINT(1)    NOT NULL DEFAULT 1,
   613	    description     VARCHAR(512)  DEFAULT NULL,
   614	    created_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
   615	    updated_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
   616	    created_by      VARCHAR(64)   NOT NULL DEFAULT 'system',
   617	    updated_by      VARCHAR(64)   NOT NULL DEFAULT 'system',
   618	    deleted_at      DATETIME      DEFAULT NULL COMMENT '逻辑删除时间, NULL=未删除',
   619	    version         INT           NOT NULL DEFAULT 0,
   620	    PRIMARY KEY (id),
   621	    UNIQUE KEY uk_route_path_method (path, method, deleted_at),
   622	    KEY idx_route_target_service (target_service),
   623	    KEY idx_route_enabled (enabled)
   624	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='网关路由配置表';
   625	```
   626	
   627	#### 4.3.2 限流规则表 `gw_rate_limit`
   628	
   629	```sql
   630	CREATE TABLE gw_rate_limit (
   631	    id              VARCHAR(36)   NOT NULL,
   632	    rule_name       VARCHAR(128)  NOT NULL,
   633	    route_id        VARCHAR(36)   DEFAULT NULL COMMENT '关联路由ID, 空=全局默认',
   634	    path_pattern    VARCHAR(512)  DEFAULT NULL COMMENT '与route_id二选一',
   635	    method          VARCHAR(16)   DEFAULT '*',
   636	    algorithm       VARCHAR(32)   NOT NULL DEFAULT 'token_bucket' COMMENT 'token_bucket/leaky_bucket',
   637	    rate            INT           NOT NULL COMMENT '速率(次/秒)',
   638	    capacity        INT           NOT NULL COMMENT '容量',
   639	    limit_by        VARCHAR(32)   NOT NULL DEFAULT 'route' COMMENT 'route/ip/client_id',
   640	    header_key      VARCHAR(64)   DEFAULT NULL,
   641	    burst           INT           DEFAULT NULL,
   642	    enabled         TINYINT(1)    NOT NULL DEFAULT 1,
   643	    description     VARCHAR(512)  DEFAULT NULL,
   644	    created_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
   645	    updated_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
   646	    created_by      VARCHAR(64)   NOT NULL DEFAULT 'system',
   647	    updated_by      VARCHAR(64)   NOT NULL DEFAULT 'system',
   648	    deleted_at      DATETIME      DEFAULT NULL,
   649	    version         INT           NOT NULL DEFAULT 0,
   650	    PRIMARY KEY (id),
   651	    KEY idx_rl_route (route_id),
   652	    KEY idx_rl_path (path_pattern, method),
   653	    KEY idx_rl_enabled (enabled)
   654	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='限流规则表';
   655	```
   656	
   657	#### 4.3.3 熔断策略表 `gw_circuit_breaker`
   658	
   659	```sql
   660	CREATE TABLE gw_circuit_breaker (
   661	    id                      VARCHAR(36)   NOT NULL,
   662	    policy_name             VARCHAR(128)  NOT NULL,
   663	    route_id                VARCHAR(36)   DEFAULT NULL,
   664	    target_service          VARCHAR(128)  DEFAULT NULL,
   665	    sliding_window_size_ms  INT           NOT NULL DEFAULT 10000,
   666	    min_request_count       INT           NOT NULL DEFAULT 20,
   667	    error_rate_threshold    DECIMAL(5,2)  NOT NULL DEFAULT 50.00,
   668	    slow_call_duration_ms   INT           NOT NULL DEFAULT 1000,
   669	    slow_call_rate_threshold DECIMAL(5,2) NOT NULL DEFAULT 60.00,
   670	    open_duration_ms        INT           NOT NULL DEFAULT 30000,
   671	    half_open_max_requests  INT           NOT NULL DEFAULT 5 COMMENT '全局半开探测数',
   672	    failure_status_codes    VARCHAR(256)  NOT NULL DEFAULT '500,502,503,504',
   673	    enabled                 TINYINT(1)    NOT NULL DEFAULT 1,
   674	    description             VARCHAR(512)  DEFAULT NULL,
   675	    created_at              DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
   676	    updated_at              DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
   677	    created_by              VARCHAR(64)   NOT NULL DEFAULT 'system',
   678	    updated_by              VARCHAR(64)   NOT NULL DEFAULT 'system',
   679	    deleted_at              DATETIME      DEFAULT NULL,
   680	    version                 INT           NOT NULL DEFAULT 0,
   681	    PRIMARY KEY (id),
   682	    KEY idx_cb_route (route_id),
   683	    KEY idx_cb_service (target_service),
   684	    KEY idx_cb_enabled (enabled)
   685	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='熔断策略表';
   686	```
   687	
   688	#### 4.3.4 灰度发布规则表 `gw_gray_release`
   689	
   690	```sql
   691	CREATE TABLE gw_gray_release (
   692	    id              VARCHAR(36)   NOT NULL,
   693	    rule_name       VARCHAR(128)  NOT NULL,
   694	    route_id        VARCHAR(36)   NOT NULL,
   695	    gray_version    VARCHAR(64)   NOT NULL COMMENT '灰度版本标签',
   696	    stable_version  VARCHAR(64)   NOT NULL DEFAULT 'stable',
   697	    gray_percent    INT           NOT NULL DEFAULT 0 COMMENT '0-100',
   698	    hash_key_type   VARCHAR(32)   NOT NULL DEFAULT 'ip' COMMENT 'ip/header/cookie/user_id',
   699	    hash_key_name   VARCHAR(64)   DEFAULT NULL,
   700	    stain_rules     JSON          DEFAULT NULL COMMENT '染色规则数组',
   701	    enabled         TINYINT(1)    NOT NULL DEFAULT 1,
   702	    description     VARCHAR(512)  DEFAULT NULL,
   703	    created_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
   704	    updated_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
   705	    created_by      VARCHAR(64)   NOT NULL DEFAULT 'system',
   706	    updated_by      VARCHAR(64)   NOT NULL DEFAULT 'system',
   707	    deleted_at      DATETIME      DEFAULT NULL,
   708	    version         INT           NOT NULL DEFAULT 0,
   709	    PRIMARY KEY (id),
   710	    KEY idx_gray_route (route_id),
   711	    KEY idx_gray_enabled (enabled)
   712	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='灰度发布规则表';
   713	```
   714	
   715	#### 4.3.5 服务注册表 `gw_service`
   716	
   717	```sql
   718	CREATE TABLE gw_service (
   719	    id              VARCHAR(36)   NOT NULL,
   720	    service_name    VARCHAR(128)  NOT NULL COMMENT '唯一, 对应etcd注册名',
   721	    display_name    VARCHAR(128)  DEFAULT NULL,
   722	    health_check_path VARCHAR(256) NOT NULL DEFAULT '/health',
   723	    health_check_interval INT      NOT NULL DEFAULT 10,
   724	    health_check_timeout INT       NOT NULL DEFAULT 3,
   725	    unhealthy_threshold INT        NOT NULL DEFAULT 2,
   726	    healthy_threshold INT          NOT NULL DEFAULT 2,
   727	    default_weight  INT           NOT NULL DEFAULT 1,
   728	    description     VARCHAR(512)  DEFAULT NULL,
   729	    created_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
   730	    updated_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
   731	    deleted_at      DATETIME      DEFAULT NULL,
   732	    version         INT           NOT NULL DEFAULT 0,
   733	    PRIMARY KEY (id),
   734	    UNIQUE KEY uk_service_name (service_name, deleted_at)
   735	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='服务元信息表';
   736	```
   737	
   738	#### 4.3.6 配置版本表 `gw_config_version`
   739	
   740	```sql
   741	CREATE TABLE gw_config_version (
   742	    id              BIGINT        NOT NULL AUTO_INCREMENT,
   743	    config_type     VARCHAR(32)   NOT NULL COMMENT 'route/rate_limit/circuit_breaker/gray_release',
   744	    config_id       VARCHAR(36)   NOT NULL,
   745	    action          VARCHAR(16)   NOT NULL COMMENT 'create/update/delete',
   746	    snapshot        JSON          NOT NULL COMMENT '{"before":{}, "after":{}}',
   747	    operator        VARCHAR(64)   NOT NULL COMMENT '操作人(token映射/X-Operator)',
   748	    created_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
   749	    PRIMARY KEY (id),
   750	    KEY idx_version_config (config_type, config_id),
   751	    KEY idx_version_created (created_at)
   752	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='配置变更版本表';
   753	```
   754	
   755	#### 4.3.7 操作日志表 `gw_operation_log`
   756	
   757	```sql
   758	CREATE TABLE gw_operation_log (
   759	    id              BIGINT        NOT NULL AUTO_INCREMENT,
   760	    trace_id        VARCHAR(64)   DEFAULT NULL,
   761	    operator        VARCHAR(64)   NOT NULL,
   762	    operation       VARCHAR(128)  NOT NULL COMMENT '如 create_route',
   763	    resource_type   VARCHAR(32)   NOT NULL,
   764	    resource_id     VARCHAR(36)   DEFAULT NULL,
   765	    request_method  VARCHAR(16)   NOT NULL,
   766	    request_path    VARCHAR(512)  NOT NULL,
   767	    request_body    TEXT          DEFAULT NULL COMMENT '敏感字段脱敏',
   768	    response_code   INT           NOT NULL,
   769	    client_ip       VARCHAR(64)   DEFAULT NULL,
   770	    duration_ms     INT           NOT NULL,
   771	    created_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
   772	    PRIMARY KEY (id),
   773	    KEY idx_oplog_operator (operator),
   774	    KEY idx_oplog_resource (resource_type, resource_id),
   775	    KEY idx_oplog_created (created_at)
   776	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='管理面操作日志表';
   777	```
   778	
   779	### 4.4 etcd 数据模型
   780	
   781	| Key 模式 | 用途 | Value |
   782	|---|---|---|
   783	| `/services/{name}/{instance-id}` | 服务实例注册 | `{address, weight, metadata{version}, register_time}` + Lease(TTL=10s) |
   784	| `/config/changes/{type}/{timestamp}` | 配置变更事件 | `{config_type, action, config_id, updated_at}` TTL=60s |
   785	
   786	### 4.5 Redis 数据模型（P1-4/P2-9 修复）
   787	
   788	| Key 模式 | 类型 | 用途 | TTL |
   789	|---|---|---|---|
   790	| `ratelimit:{route-id}:{alg}:{dim}:global` | Hash | 限流全局桶（tokens/last_refill_time 或 level/last_leak_time） | **每次 Lua 执行后 EXPIRE 刷新为 3600s**（P1-4） |
   791	| `circuit:{service}:{route}:state` | String | 熔断器状态：closed/open/half_open | open 状态带 TTL=open_duration_ms；其余持久但定期清理 |
   792	| `circuit:{service}:{route}:halfopen_count` | String | **全局半开探测计数**（P1-1） | TTL=open_duration_ms |
   793	| `circuit:{service}:{route}:stats` | Hash | 熔断滑动窗口统计（仅多实例聚合查询，**不参与熔断判断**，P2-9） | 滑动窗口自动过期 |
   794	
   795	> 本地限流状态（每实例令牌桶）存在网关进程内存，不入 Redis。熔断滑动窗口统计存在本地内存，作为熔断判断依据；Redis stats key 仅用于管理面聚合查询。
   796	
   797	### 4.6 表关联关系
   798	
   799	```
   800	gw_service ──(service_name)──< gw_route (target_service)
   801	gw_route ──(id)──< gw_rate_limit (route_id)
   802	gw_route ──(id)──< gw_circuit_breaker (route_id)
   803	gw_route ──(id)──< gw_gray_release (route_id)
   804	gw_config_version: config_type+config_id 关联各表变更
   805	gw_operation_log: resource_type+resource_id 关联操作对象
   806	```
   807	
   808	---
   809	
   810	## 5. 接口规范定义
   811	
   812	### 5.1 统一 API 规范
   813	
   814	| 约定项 | 规范 |
   815	|---|---|
   816	| 协议 | HTTP/1.1 + HTTPS（生产） |
   817	| 数据格式 | JSON（`application/json; charset=utf-8`） |
   818	| 编码 | UTF-8 |
   819	| 时间 | ISO 8601 UTC：`2026-10-04T10:00:00Z` |
   820	| 命名 | 路径 kebab-case；字段 snake_case |
   821	| 版本 | URL 前缀 `/api/v1/` |
   822	| 分页 | `page`(从1) + `page_size`(默认20, 最大100) |
   823	
   824	### 5.2 统一响应格式
   825	
   826	**成功**：
   827	```json
   828	{ "code": 0, "message": "success", "data": {...}, "trace_id": "..." }
   829	```
   830	
   831	**分页成功**：
   832	```json
   833	{ "code": 0, "message": "success",
   834	  "data": { "list": [...], "pagination": { "page":1, "page_size":20, "total":156, "total_pages":8 } },
   835	  "trace_id": "..." }
   836	```
   837	
   838	**失败**：
   839	```json
   840	{ "code": 40101, "message": "路由路径已存在", "data": null, "trace_id": "...",
   841	  "errors": [ { "field": "path", "message": "..." } ] }
   842	```
   843	
   844	### 5.3 错误码规范
   845	
   846	```
   847	4  01  01
   848	│  │   └─ 错误序号(01-99)
   849	│  └───── 模块(00通用,01路由,02限流,03熔断,04灰度,05服务,09系统)
   850	└──────── HTTP大类(4客户端,5服务端)
   851	```
   852	
   853	**通用错误码**：0 成功；40000 参数错误；40001 校验失败；40100 未授权；40300 禁止；40400 不存在；40900 冲突；42900 频繁；50000 内部错误；50001 DB 失败；50002 热更新失败；50300 依赖不可用。
   854	
   855	**业务错误码**：40101 路由路径+方法已存在；40102 目标服务不存在；40103 路由不存在；40201 限流算法不支持；40202 rate 必须>0；40301 错误率阈值1-100；40401 灰度比例0-100；40501 服务名已存在。
   856	
   857	### 5.4 鉴权规则
   858	
   859	#### 5.4.1 管理面鉴权（P2-6 修复：操作人区分）
   860	
   861	**请求 Header**：
   862	```
   863	Authorization: Bearer {admin-token}
   864	X-Operator: {username}    # 可选，兜底操作人标识
   865	```
   866	
   867	**Token 配置**（支持多 Token 映射操作人）：
   868	```yaml
   869	admin:
   870	  auth:
   871	    enabled: true
   872	    tokens:
   873	      - token: "${GATEWAY_ADMIN_TOKEN_ALICE}"
   874	        username: "alice"
   875	      - token: "${GATEWAY_ADMIN_TOKEN_BOB}"
   876	        username: "bob"
   877	    # 兼容单一 token 配置
   878	    # token: "${GATEWAY_ADMIN_TOKEN}"
   879	    # default_username: "admin"
   880	```
   881	
   882	**操作人解析优先级**：① Token 映射的 username → ② `X-Operator` Header → ③ default_username。`gw_operation_log.operator` 与 `gw_config_version.operator` 记录解析出的操作人。
   883	
   884	**网络防护**：管理面 8080 端口本地映射到宿主机回环；生产通过 K8s NetworkPolicy 限制运维命名空间访问。
   885	
   886	#### 5.4.2 数据面鉴权
   887	
   888	数据面 8080 不做鉴权（基线 E-01），透传至后端，由后端处理。
   889	
   890	### 5.5 核心接口清单
   891	
   892	**路由管理**：POST/GET(list)/GET(id)/PUT/DELETE/PATCH status `/api/v1/routes[/{id}...]`
   893	
   894	**限流规则**：POST/GET/GET(id)/PUT/DELETE/PATCH status `/api/v1/rate-limits[/{id}...]`
   895	
   896	**熔断策略**：POST/GET/GET(id)/PUT/DELETE `/api/v1/circuit-breakers[/{id}...]`；GET state、POST reset
   897	
   898	**灰度发布**：POST/GET/GET(id)/PUT/DELETE `/api/v1/gray-releases[/{id}...]`；PATCH percent
   899	
   900	**服务管理**：POST/GET/GET(name)/GET instances/PUT(name)/DELETE(name) `/api/v1/services...`
   901	
   902	**系统运维**（5.4.6 节详述）：`/healthz`、`/readyz`、`/metrics`、POST reload、GET config、GET stats
   903	
   904	---
   905	
   906	## 6. 部署架构设计
   907	
   908	### 6.1 环境划分
   909	
   910	| 环境 | 部署方式 | 配置存储 | etcd | 可观测 |
   911	|---|---|---|---|---|
   912	| 本地开发 | Docker Compose | SQLite（嵌入式） | 单节点 | 控制台 + 可选 Jaeger |
   913	| 测试 | Compose / 轻量 K8s | SQLite/MySQL | 单节点 | Jaeger + Prometheus |
   914	| 预发布 | K8s | MySQL | 三节点 | 完整栈 |
   915	| 生产 | K8s | MySQL 主从 | 三节点集群 | 完整栈 + 告警 |
   916	
   917	### 6.2 本地开发部署架构
   918	
   919	#### 6.2.1 测试服务规格（P1-6 修复）
   920	
   921	每个测试服务必须满足：
   922	
   923	| 规格项 | 要求 |
   924	|---|---|
   925	| OTel SDK | 集成 OpenTelemetry Go SDK，提取 `traceparent` 创建服务端 Span |
   926	| 数据库 | 至少 2 个测试服务连接 SQLite/MySQL，执行查询时通过 SQL 注释注入 TraceId：`/* trace_id=xxx */ SELECT ...` |
   927	| 服务注册 | 启动时注册至 etcd，Lease TTL=10s，持续 KeepAlive |
   928	| 健康检查 | 提供 `/health` 端点返回 200 |
   929	| 数量 | ≥ 5 个（基线 T-04） |
   930	
   931	测试服务示例代码结构、依赖清单、Dockerfile 模板见附录 C。
   932	
   933	#### 6.2.2 内存预算
   934	
   935	| 组件 | 内存限制 |
   936	|---|---|
   937	| gateway | 256MB（实际 ≤80MB） |
   938	| etcd | 256MB |
   939	| redis | 128MB |
   940	| 5×测试服务 | 320MB |
   941	| **合计** | **960MB** |
   942	| Jaeger（可选） | 512MB |
   943	
   944	全套环境预计 ≤2GB，远低于基线 5.5GB（D-03）。
   945	
   946	#### 6.2.3 docker-compose.yml 核心配置（P1-8/P2-8 修复）
   947	
   948	```yaml
   949	version: "3.8"
   950	services:
   951	  gateway:
   952	    build: { context: ., dockerfile: Dockerfile }
   953	    container_name: gateway
   954	    ports:
   955	      - "8080:8080"                          # 数据面
   956	      - "127.0.0.1:8081:8081"                # 管理面仅宿主机回环(P1-8)
   957	    environment:
   958	      - GATEWAY_ETCD_ENDPOINTS=etcd:2379
   959	      - GATEWAY_REDIS_ADDR=redis:6379
   960	      - GATEWAY_CONFIG_DRIVER=sqlite
   961	      - GATEWAY_CONFIG_DSN=/data/gateway.db
   962	    volumes: [ "gateway-data:/data" ]
   963	    depends_on:
   964	      etcd: { condition: service_healthy }
   965	      redis: { condition: service_healthy }
   966	    mem_limit: 256m
   967	    restart: unless-stopped
   968	
   969	  etcd:
   970	    image: bitnami/etcd:3.5.15
   971	    environment:
   972	      - ALLOW_NONE_AUTHENTICATION=yes
   973	      - ETCD_ADVERTISE_CLIENT_URLS=http://etcd:2379
   974	      - ETCDCTL_API=3                          # P2-8: etcdctl 使用 API v3
   975	    ports: [ "2379:2379" ]
   976	    mem_limit: 256m
   977	    healthcheck:
   978	      test: ["CMD", "etcdctl", "--endpoints=http://localhost:2379", "endpoint", "health"]
   979	      interval: 5s
   980	      timeout: 3s
   981	      retries: 5
   982	
   983	  redis:
   984	    image: redis:7.2-alpine
   985	    ports: [ "6379:6379" ]
   986	    mem_limit: 128m
   987	    healthcheck:
   988	      test: ["CMD", "redis-cli", "ping"]
   989	      interval: 5s
   990	      timeout: 3s
   991	      retries: 5
   992	
   993	  test-service-1:
   994	    build: ./test-services/service-a
   995	    environment: { SERVICE_NAME: service-a, SERVICE_PORT: 8080, ETCD_ENDPOINTS: etcd:2379 }
   996	    depends_on: [ etcd ]
   997	    mem_limit: 64m
   998	  # test-service-2 ~ 5 类似
   999	
  1000	volumes:
  1001	  gateway-data:
  1002	```
  1003	
  1004	### 6.3 生产部署架构
  1005	
  1006	#### 6.3.1 K8s 拓扑
  1007	
  1008	Ingress/LB → Gateway Deployment（多副本，数据面 8080）→ 后端服务；管理面 Service（8081，NetworkPolicy 限制）；etcd StatefulSet（3 节点）；MySQL 主从；Redis Sentinel；Prometheus+Grafana+AlertManager；Jaeger Collector。
  1009	
  1010	#### 6.3.2 HPA 弹性扩缩容（P1-5 修复）
  1011	
  1012	```yaml
  1013	apiVersion: autoscaling/v2
  1014	kind: HorizontalPodAutoscaler
  1015	metadata: { name: gateway-hpa, namespace: gateway }
  1016	spec:
  1017	  scaleTargetRef: { apiVersion: apps/v1, kind: Deployment, name: gateway }
  1018	  minReplicas: 3
  1019	  maxReplicas: 20
  1020	  metrics:
  1021	    - type: Resource
  1022	      resource: { name: cpu, target: { type: Utilization, averageUtilization: 70 } }
  1023	    - type: Resource
  1024	      resource: { name: memory, target: { type: Utilization, averageUtilization: 80 } }
  1025	    - type: Pods
  1026	      pods:
  1027	        metric: { name: gateway_qps_per_pod }   # 自定义指标
  1028	        target: { type: AverageValue, averageValue: "8000" }
  1029	  behavior:
  1030	    scaleUp: { stabilizationWindowSeconds: 60, policies: [ {type: Percent, value: 50, periodSeconds: 60} ] }
  1031	    scaleDown: { stabilizationWindowSeconds: 300, policies: [ {type: Percent, value: 10, periodSeconds: 120} ] }
  1032	```
  1033	
  1034	#### 6.3.3 健康检查设计（P1-7 修复）
  1035	
  1036	`/healthz`（liveness）：始终返回 200（进程存活即健康）。
  1037	
  1038	`/readyz`（readiness）分两阶段：
  1039	
  1040	| 阶段 | 条件 | 检查内容 |
  1041	|---|---|---|
  1042	| 启动就绪（进程启动后 60s 内） | 依赖初始连接 | 配置加载完成、etcd/Redis 初始连接成功、DB 连接成功 |
  1043	| 运行时就绪（启动 60s 后） | 仅进程内部状态 | 配置快照非空、HTTP Server 正常；**不检查外部依赖实时连通性** |
  1044	
  1045	> 外部依赖（etcd/Redis）短暂故障时通过降级机制继续服务，不触发摘流，避免级联震荡。依赖故障通过告警通知运维处理。
  1046	
  1047	#### 6.3.4 Prometheus Adapter 部署（P1-5 修复）
  1048	
  1049	HPA 自定义指标 `gateway_qps_per_pod` 需通过 Prometheus Adapter 注册到 `custom.metrics.k8s.io`：
  1050	
  1051	```yaml
  1052	apiVersion: apps/v1
  1053	kind: Deployment
  1054	metadata: { name: prometheus-adapter, namespace: monitoring }
  1055	spec:
  1056	  replicas: 1
  1057	  template:
  1058	    spec:
  1059	      containers:
  1060	        - name: prometheus-adapter
  1061	          image: registry.k8s.io/prometheus-adapter/prometheus-adapter:v0.11.2
  1062	          args:
  1063	            - --prometheus-url=http://prometheus.monitoring:9090
  1064	            - --config=/etc/adapter/config.yaml
  1065	          volumeMounts: [ {name: config, mountPath: /etc/adapter} ]
  1066	      volumes:
  1067	        - name: config
  1068	          configMap: { name: prometheus-adapter-config }
  1069	---
  1070	apiVersion: v1
  1071	kind: ConfigMap
  1072	metadata: { name: prometheus-adapter-config, namespace: monitoring }
  1073	data:
  1074	  config.yaml: |
  1075	    rules:
  1076	      - seriesQuery: 'gateway_qps_total{job="gateway"}'
  1077	        resources: { overrides: { pod: { resource: pod } } }
  1078	        name: { matches: "gateway_qps_total", as: "gateway_qps_per_pod" }
  1079	        metricsQuery: 'sum(rate(<<.Series>>{<<.LabelMatchers>>}[1m])) by (<<.GroupBy>>)'
  1080	```
  1081	
  1082	#### 6.3.5 网关启动初始化顺序（P2-7 修复）
  1083	
  1084	```
  1085	① 加载配置文件(config.yaml + 环境变量)
  1086	② 初始化日志(zap)
  1087	③ 连接配置存储(SQLite/MySQL) → AutoMigrate 建表
  1088	④ 全量加载配置快照(路由/限流/熔断/灰度)
  1089	⑤ 连接 etcd → 全量拉取服务实例 → 启动 Watch
  1090	⑥ 连接 Redis → 启动限流同步协程(100ms)
  1091	⑦ 启动健康检查协程
  1092	⑧ 启动管理面 HTTP Server(8081)
  1093	⑨ 启动数据面 HTTP Server(8080)  ← 最后启动
  1094	/readyz 在 ①~⑧ 完成前返回不健康
  1095	```
  1096	
  1097	数据面最后启动确保请求到来时所有依赖就绪。
  1098	
  1099	### 6.4 网关 Docker 镜像构建（P2-10 修复）
  1100	
  1101	```dockerfile
  1102	FROM golang:1.22-alpine AS builder
  1103	WORKDIR /app
  1104	COPY go.mod go.sum ./
  1105	RUN go mod download
  1106	COPY . .
  1107	# P0-1修复: modernc.org/sqlite 为纯Go, CGO_ENABLED=0 可正常编译
  1108	RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  1109	    go build -ldflags="-s -w -X main.version=${VERSION}" \
  1110	    -o /app/gateway ./cmd/gateway
  1111	
  1112	FROM gcr.io/distroless/static-debian12:nonroot
  1113	WORKDIR /app
  1114	COPY --from=builder /app/gateway .
  1115	COPY config/default.yaml /app/config/default.yaml
  1116	USER root
  1117	RUN mkdir -p /data && chown nonroot:nonroot /data
  1118	USER nonroot
  1119	EXPOSE 8080 8081
  1120	# P2-10修复: 纯exec形式, 不混用 shell 的 ||
  1121	HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  1122	  CMD ["/app/gateway", "healthcheck"]
  1123	ENTRYPOINT ["/app/gateway"]
  1124	CMD ["--config", "/app/config/default.yaml"]
  1125	```
  1126	
  1127	镜像大小：distroless 约 2MB + Go 二进制 30-50MB ≤ 100MB，低于基线 200MB（RC-05）。
  1128	
  1129	---
  1130	
  1131	## 7. 监控与运维
  1132	
  1133	### 7.1 监控指标体系
  1134	
  1135	#### 7.1.1 指标分类总览（P2-1 修复：对齐基线指标名）
  1136	
  1137	| 类别 | 指标名称 | 类型 | 说明 | 基线 |
  1138	|---|---|---|---|---|
  1139	| 流量 | `gateway_qps_total` | Counter | 总请求数，按 route/service/method/status 标签 | D-06 |
  1140	| 延迟 | `gateway_request_duration_seconds` | Histogram | 请求延迟分布 | D-06 |
  1141	| 错误 | `gateway_error_rate` | Gauge | 错误率（计算值） | D-06 |
  1142	| 限流 | `gateway_rate_limit_total`/`_passed`/`_rejected` | Counter | 限流统计 | - |
  1143	| 熔断 | `gateway_circuit_breaker_state`(Gauge)/`_open_total`(Counter)/`_rejected_total`(Counter) | - | 熔断状态与次数 | - |
  1144	| 灰度 | `gateway_gray_requests_total` | Counter | 按 route/version 统计 | - |
  1145	| 服务发现 | `gateway_service_instances`(Gauge)/`_discovery_events_total`(Counter) | - | 实例数与事件 | - |
  1146	| 系统 | `gateway_memory_usage_bytes`/`gateway_goroutines`/`gateway_active_connections`/`gateway_config_reload_total`/`gateway_config_reload_duration_seconds` | - | 进程与配置 | P-04 |
  1147	
  1148	> 基线 D-06 要求的 `gateway_qps_total`、`gateway_request_duration_seconds`、`gateway_error_rate` 三个指标已对齐。
  1149	
  1150	#### 7.1.2 核心 PromQL
  1151	
  1152	QPS：`sum(rate(gateway_qps_total[1m])) by (route)`
  1153	P99 延迟：`histogram_quantile(0.99, sum(rate(gateway_request_duration_seconds_bucket[5m])) by (le))`
  1154	错误率：`sum(rate(gateway_qps_total{status=~"5.."}[5m])) / sum(rate(gateway_qps_total[5m])) * 100`
  1155	
  1156	### 7.2 告警规则
  1157	
  1158	| 级别 | 响应 | 示例 |
  1159	|---|---|---|
  1160	| P0 | 5 分钟 | 网关全部不可用、错误率 > 50% |
  1161	| P1 | 15 分钟 | 错误率 > 5%、P99 > 100ms、内存 > 100MB |
  1162	| P2 | 1 小时 | 熔断器打开、限流拒绝率 > 10%、健康实例不足 50% |
  1163	| P3 | 工作日 | 配置热更新失败、实例下线 |
  1164	
  1165	核心规则：`GatewayDown`（up==0 for 1m，P0）；`GatewayHighErrorRate`（>50% for 1m，P0）；`GatewayHighLatency`（P99>0.1s for 5m，P1）；`GatewayHighMemory`（>100MB for 5m，P1）；`CircuitBreakerOpen`（P2）；`RateLimitHighRejection`（>10% for 5m，P2）；`ConfigReloadFailed`（P3）。
  1166	
  1167	### 7.3 日志规范
  1168	
  1169	zap 结构化 JSON，必含字段：timestamp、level、logger、message、trace_id、span_id、client_ip、method、path、route_id、target_service、target_instance、status、latency_ms、backend_latency_ms、rate_limited、circuit_state、gray_version。
  1170	
  1171	**敏感字段脱敏**（CO-02）：password→`******`；token→前4后4；id_card→前6后4；phone→前3后4；email→首字符+域名；cookie/authorization→`[REDACTED]`。
  1172	
  1173	### 7.4 运维流程
  1174	
  1175	**配置变更**：管理API（鉴权）→ 校验 → 写DB（记 operation_log + config_version）→ 发布 etcd 事件 → 网关 Watch → 全量拉取 → 原子替换 → ≤5s 生效 → 验证/回滚。
  1176	
  1177	**灰度发布**（P2-5 修复）：部署 canary 实例 → 创建灰度规则(percent=0) → 逐级 0→5→10→20→50→100，每阶段观察 5-10 分钟 → 全量稳定 30 分钟 → 下线旧版。回滚：percent 调 0。验证使用 ≥100 个不同 `X-Forwarded-For`。
  1178	
  1179	**故障排查**：获取 TraceId → Jaeger 查调用链 → 网关日志过滤 → 监控指标趋势 → 定位根因 → 处置 → 验证记录。
  1180	
  1181	---
  1182	
  1183	## 8. 技术风险与应对方案
  1184	
  1185	### 8.1 核心风险总览
  1186	
  1187	| 编号 | 类别 | 描述 | 等级 | 应对 |
  1188	|---|---|---|---|---|
  1189	| R-01 | 性能 | 单实例 QPS 达不到 10,000 | 高 | 双层限流(本地)、异步日志、连接池、radix tree、对象池 |
  1190	| R-02 | 性能 | 平均延迟超过 2ms | 高 | 本地限流无网络、本地服务发现缓存、GOGC 调优 |
  1191	| R-03 | 性能 | 内存超过 80MB | 中 | 不可变快照、连接池上限、状态 TTL、内存预算 |
  1192	| R-04 | 兼容 | SQLite↔MySQL 切换 | 中 | GORM 抽象、UUID 主键、双驱动 CI 测试、AutoMigrate |
  1193	| R-05 | 可用 | etcd 单节点故障（本地） | 中 | 本地缓存兜底、重连退避、生产三节点 |
  1194	| R-06 | 可用 | Redis 故障 | 中 | 限流本地降级、熔断本地状态、重连、生产 Sentinel |
  1195	| R-07 | 一致 | 热更新多实例不一致 | 中 | 全量拉取+版本校验、60s 兜底同步、atomic.Value |
  1196	| R-08 | 安全 | 管理面未授权变更 | 中 | 端口隔离+Token 鉴权+NetworkPolicy+审计+版本回溯 |
  1197	| R-09 | 扩展 | 路由规则增多性能下降 | 低 | radix tree O(k)、LRU 缓存、按 Method 分桶 |
  1198	| R-10 | 运维 | 灰度比例不精确 | 低 | murmur3 哈希、可选 Redis 计数器、监控告警 |
  1199	| R-11 | 可用 | 熔断多实例不一致（P1-1） | 中 | 半开全局计数、状态事件广播、滑动窗口本地判断 |
  1200	
  1201	### 8.2 性能风险应对（R-01/R-02/R-03）
  1202	
  1203	核心措施已在 FL-03 双层限流落实：请求路径操作本地令牌桶，Redis 仅 100ms 批量预取，消除每请求网络往返。辅以异步日志、连接池（MaxIdleConns=1000）、radix tree、sync.Pool、GOGC/GOMEMLIMIT 调优。
  1204	
  1205	验证：wrk -t4 -c100 -d60s，目标 QPS≥10000、延迟<2ms、内存≤80MB。
  1206	
  1207	### 8.3 兼容性风险应对（R-04）
  1208	
  1209	P0-1 已通过 modernc.org/sqlite 解决 cgo 冲突。SQL 兼容性通过 GORM 抽象、UUID 主键、禁止特定函数、AutoMigrate、CI 双驱动测试保障。
  1210	
  1211	### 8.4 可用性风险应对（R-05/R-06/R-11）
  1212	
  1213	- etcd 故障：本地服务实例缓存兜底，重连指数退避，生产三节点
  1214	- Redis 故障：限流自动切本地模式，熔断本地独立运行，生产 Sentinel
  1215	- 熔断多实例（R-11/P1-1）：半开探测 Redis 全局计数，状态变更发布事件广播，避免阈值与探测数放大
  1216	
  1217	### 8.5 一致性/安全/扩展/运维风险
  1218	
  1219	- 配置一致性（R-07）：全量拉取+版本校验+60s 兜底+atomic.Value，≤60s 收敛
  1220	- 安全（R-08）：端口隔离、Token 鉴权、NetworkPolicy、操作审计、配置版本回溯、管理面限流、定期轮换
  1221	- 扩展（R-09）：radix tree 支持 10000+ 路由无性能下降，匹配 <0.01ms
  1222	- 灰度精度（R-10）：murmur3 分布均匀性≥99%，偏差<2%，监控告警
  1223	
  1224	### 8.6 风险应对总结
  1225	
  1226	| 等级 | 编号 | 策略 | 责任 |
  1227	|---|---|---|---|
  1228	| 高 | R-01,R-02 | 性能优化：双层限流、异步日志、连接池、对象池 | 后端 |
  1229	| 中 | R-03~R-08,R-11 | 降级兜底：本地缓存、内存降级、双驱动测试、熔断全局协调、端口隔离鉴权 | 后端+运维 |
  1230	| 低 | R-09,R-10 | 架构设计：radix tree、均匀哈希、监控告警 | 架构师 |
  1231	
  1232	持续监控：每次发布前性能基准测试；CI 双驱动兼容测试；生产大盘实时展示；每月故障演练（etcd/Redis/热更新）。
  1233	
  1234	---
  1235	
  1236	## 附录 A：架构设计决策记录（ADR）
  1237	
  1238	| 编号 | 决策 | 备选 | 理由 |
  1239	|---|---|---|---|
  1240	| ADR-001 | 限流 Redis+本地内存双层架构 | 纯 Redis / 纯本地 | 纯 Redis 性能不达标，纯本地多实例不精确 |
  1241	| ADR-002 | 配置热更新 etcd Watch+全量拉取 | DB轮询/WebSocket | 实时性好+一致性 |
  1242	| ADR-003 | 服务实例存 etcd 不存 DB | DB/混合 | Lease 自动过期+Watch |
  1243	| ADR-004 | 熔断器本地+Redis 全局协调 | 纯 Redis / 纯本地 | 本地读无延迟，全局协调防放大 |
  1244	| ADR-005 | 主键 UUID | 自增/雪花 | 避免 SQLite/MySQL 自增兼容问题 |
  1245	| ADR-006 | 管理面/数据面端口分离 | 同端口不同路径 | 端口级隔离更安全 |
  1246	| ADR-007 | SQLite 驱动 modernc.org/sqlite | mattn/go-sqlite3 | 纯 Go 无 cgo，兼容 distroless（P0-1） |
  1247	| ADR-008 | 逻辑删除 deleted_at | is_deleted TINYINT | NULL 不参与唯一约束，可反复删除重建（P1-2） |
  1248	
  1249	## 附录 B：术语表
  1250	
  1251	QPS、TraceId、Span、令牌桶、漏桶、熔断器、灰度发布、流量染色、HPA、OTLP。
  1252	
  1253	## 附录 C：测试服务技术规格（P1-6）
  1254	
  1255	每个测试服务：
  1256	1. 集成 `go.opentelemetry.io/otel` SDK，提取 `traceparent` 创建 SERVER Span
  1257	2. 至少 2 个服务连接 SQLite/MySQL，查询时 `/* trace_id=%s */ SELECT ...`
  1258	3. 启动注册 etcd（Lease TTL=10s，KeepAlive）
  1259	4. 提供 `/health` 返回 200
  1260	5. 提供业务端点返回包含 TraceId 的 JSON
  1261	
  1262	依赖：`go.opentelemetry.io/otel`、`go.opentelemetry.io/otel/sdk`、`go.opentelemetry.io/otel/trace`、`go.etcd.io/etcd/client/v3`、`gorm.io/gorm`。
  1263	
  1264	Dockerfile 模板：基于 `golang:1.22-alpine` 构建 + `alpine` 运行，设置 `SERVICE_NAME`、`SERVICE_PORT`、`ETCD_ENDPOINTS` 环境变量。
  1265	
  1266	---
  1267	
  1268	*文档结束*
