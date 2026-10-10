# Go API Gateway — 高性能 API 网关与流量治理平台

基于 Go 1.22 + Gin 实现的轻量级 API 网关，提供动态路由、服务发现、限流熔断、灰度发布、全链路追踪等核心能力。

## 技术栈

| 层级 | 技术 |
|---|---|
| 语言 | Go 1.22 |
| Web 框架 | Gin |
| 服务发现 | etcd（本地单节点 / 生产可切集群） |
| 缓存/计数 | Redis |
| 配置存储 | SQLite（本地）/ MySQL（生产） |
| ORM | GORM |
| 链路追踪 | OpenTelemetry |
| 指标采集 | Prometheus |
| 日志 | Zap |
| 配置管理 | Viper |
| 前端 | Vue 3 + Vite（`web/` 目录） |

## 快速开始

### 环境要求

- Go 1.22+
- Node.js 18+（前端）
- Docker Desktop（可选，用于容器化部署）

### 一键启动（Windows）

```bat
scripts\01-检查环境.bat    # 检查环境
scripts\02-安装依赖.bat    # 安装依赖
scripts\06-一键启动全部.bat # 启动全部服务
```

启动后访问：
- 管理后台：http://localhost:5173 （Token: `admin-token-dev-001`）
- 网关数据面：http://localhost:8080
- 网关管理面：http://localhost:8081

### Docker 启动

```bat
scripts\09-Docker一键启动.bat
```

或手动：

```bash
docker compose -f deploy/compose/docker-compose.yml up -d
```

### 手动构建

```bash
make build          # 编译网关到 bin/
make run            # 本地运行
make test           # 运行单元测试
make docker-build   # 构建 Docker 镜像
```

## 目录结构

```
go-api-gateway/
├── bin/                  # 编译产物（gitignore）
├── cmd/gateway/          # 程序入口
├── config/               # 配置文件（default.yaml / local-demo.yaml）
├── data/                 # 运行时 SQLite 数据（gitignore）
├── deploy/
│   ├── compose/          # Docker Compose 编排
│   ├── docker/           # Dockerfile
│   └── k8s/              # Kubernetes 部署清单
├── docs/
│   ├── 01_architecture/  # 架构设计、基线、评审报告
│   ├── 02_development/   # 开发规范、实现文档、快速开始
│   ├── 03_testing/       # 回归测试报告、Bug 清单
│   ├── 04_integration/   # 联调验证报告
│   ├── 05_api/           # OpenAPI / Swagger 接口文档
│   └── 99_archive/       # 历史草稿归档
├── internal/             # 核心业务代码
│   ├── app/              # 应用初始化与组装
│   ├── proxy/            # 反向代理核心
│   ├── ratelimit/        # 限流（令牌桶 + 漏桶）
│   ├── circuitbreaker/   # 熔断器
│   ├── gray/             # 灰度发布与流量染色
│   ├── registry/         # 服务注册与发现（etcd）
│   ├── loadbalance/      # 负载均衡（加权轮询 + 一致性哈希）
│   ├── middleware/       # 中间件链
│   ├── handler/          # 管理面 HTTP Handler
│   ├── model/            # 数据模型
│   ├── repository/       # 数据访问层
│   ├── store/            # 配置存储抽象
│   ├── config/           # 配置加载
│   ├── configsnap/       # 配置快照热更新
│   ├── observability/    # 可观测性（Trace / Metrics）
│   └── pkg/              # 内部公共包（errcode / logger / response / utils）
├── scripts/              # Windows 启动与运维脚本
├── test-services/        # 测试用上游模拟服务
├── web/                  # 管理后台前端（Vue 3 + Vite）
├── .gitignore
├── go.mod / go.sum
├── Makefile
└── README.md
```

## 脚本说明

| 脚本 | 用途 |
|---|---|
| `01-检查环境.bat` | 检查 Go / Node.js / Docker 环境 |
| `02-安装依赖.bat` | 安装 Go 模块和前端 npm 依赖 |
| `03-启动网关后端.bat` | 启动网关后端（端口 8080/8081） |
| `04-启动管理前端.bat` | 启动管理前端（端口 5173） |
| `05-启动测试后端.bat` | 启动测试上游服务（端口 9001） |
| `06-一键启动全部.bat` | 依次启动测试后端 + 网关 + 前端 |
| `07-停止所有服务.bat` | 停止所有相关进程 |
| `08-演示主流程.bat` | 演示网关核心功能流程 |
| `09-Docker一键启动.bat` | Docker Compose 一键启动 |
| `10-Docker停止.bat` | Docker Compose 停止 |

## 核心能力

- **动态路由**：路径 + Method 匹配，路径重写，请求/响应拦截
- **服务发现**：基于 etcd 的自动注册与上下线感知，主动健康检查
- **负载均衡**：加权轮询、一致性哈希（虚拟节点 150，murmur3）
- **限流**：令牌桶 + 漏桶双算法，接口级粒度，本地令牌桶 + Redis 批量预取双层架构
- **熔断**：错误率 / 慢调用触发，半开探测自动恢复，多实例状态同步
- **灰度发布**：按流量比例灰度，基于 Header / 用户标识的流量染色
- **全链路追踪**：OpenTelemetry TraceId 透传（网关→服务→数据库），控制台输出 + 可选 Jaeger
- **配置热更新**：路由 / 限流 / 熔断配置 ≤5s 生效，无需重启
- **可观测性**：Prometheus 指标（QPS / 延迟 / 错误率）

## 性能指标（基线）

| 指标 | 阈值 |
|---|---|
| 单实例 QPS | ≥ 10,000 |
| 平均延迟 | < 2 ms |
| 错误率 | < 0.01% |
| 网关内存（运行时） | ≤ 80 MB |
| 启动时间 | ≤ 5s |

## 文档索引

- 架构设计：[`docs/01_architecture/技术架构设计文档.md`](docs/01_architecture/技术架构设计文档.md)
- 项目基线：[`docs/01_architecture/项目基线说明书.md`](docs/01_architecture/项目基线说明书.md)
- 快速开始：[`docs/02_development/快速开始指南.md`](docs/02_development/快速开始指南.md)
- 后端实现：[`docs/02_development/后端开发实现文档.md`](docs/02_development/后端开发实现文档.md)
- 前端实现：[`docs/02_development/前端开发实现文档.md`](docs/02_development/前端开发实现文档.md)
- 测试报告：[`docs/03_testing/`](docs/03_testing/)
- 联调报告：[`docs/04_integration/`](docs/04_integration/)
