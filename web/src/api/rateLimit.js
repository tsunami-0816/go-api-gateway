import request from './request'

// 限流规则 API —— 对应后端 /api/v1/rate-limits
export function listRateLimits(params) {
  return request.get('/api/v1/rate-limits', { params })
}

export function getRateLimit(id) {
  return request.get(`/api/v1/rate-limits/${id}`)
}

export function createRateLimit(data) {
  return request.post('/api/v1/rate-limits', data)
}

export function updateRateLimit(id, data) {
  return request.put(`/api/v1/rate-limits/${id}`, data)
}

export function deleteRateLimit(id) {
  return request.delete(`/api/v1/rate-limits/${id}`)
}
