# Go API Gateway 管理台前端

基于 Vue 3 + Vite + Element Plus 的网关管理台，对接 Go API Gateway 管理面 API（端口 8081）。

## 功能页面

| 页面 | 路由 | 功能 |
|---|---|---|
| 登录 | /login | Token 鉴权 + 操作人录入 |
| 运行概览 | /dashboard | 健康状态、配置统计、资源分布饼图、热更新 |
| 路由管理 | /routes | 路由 CRUD、启用/禁用、搜索分页 |
| 限流规则 | /rate-limits | 限流规则 CRUD、令牌桶/漏桶 |
| 熔断策略 | /circuit-breakers | 熔断策略创建/删除/详情 |
| 灰度发布 | /gray-releases | 灰度规则 CRUD、运行时调比例 |
| 服务管理 | /services | 服务注册/删除/详情 |

## 快速开始

```bash
# 安装依赖
npm install

# 启动开发服务器（默认 http://localhost:5173，代理 /api 到 127.0.0.1:8081）
npm run dev

# 生产构建
npm run build

# 预览构建产物
npm run preview
```

## 环境变量

| 变量 | 说明 | 默认 |
|---|---|---|
| VITE_API_BASE_URL | API 基础路径，开发环境留空走 Vite proxy | 空 |
| VITE_APP_TITLE | 页面标题 | Go API Gateway 管理台 |

## 技术栈

- Vue 3（Composition API）
- Vite 5
- Element Plus（按需引入 + 中文语言包）
- Vue Router 4
- Pinia
- Axios
- ECharts + vue-echarts

## 默认 Token

开发环境默认 Token：`admin-token-dev-001`（对应 config/default.yaml）。
