import request from './request'

// 系统操作 API —— 对应后端 /api/v1/system 与健康探针
export function getConfig() {
  return request.get('/api/v1/system/config')
}

export function reloadConfig() {
  return request.post('/api/v1/system/reload')
}

export function healthz() {
  return request.get('/healthz')
}

export function readyz() {
  return request.get('/readyz')
}
