import request from './request'

// 熔断策略 API —— 对应后端 /api/v1/circuit-breakers
export function listCircuitBreakers(params) {
  return request.get('/api/v1/circuit-breakers', { params })
}

export function getCircuitBreaker(id) {
  return request.get(`/api/v1/circuit-breakers/${id}`)
}

export function createCircuitBreaker(data) {
  return request.post('/api/v1/circuit-breakers', data)
}

export function deleteCircuitBreaker(id) {
  return request.delete(`/api/v1/circuit-breakers/${id}`)
}
