import request from './request'

// 路由管理 API —— 对应后端 /api/v1/routes
export function listRoutes(params) {
  return request.get('/api/v1/routes', { params })
}

export function getRoute(id) {
  return request.get(`/api/v1/routes/${id}`)
}

export function createRoute(data) {
  return request.post('/api/v1/routes', data)
}

export function updateRoute(id, data) {
  return request.put(`/api/v1/routes/${id}`, data)
}

export function deleteRoute(id) {
  return request.delete(`/api/v1/routes/${id}`)
}

export function patchRouteStatus(id, enabled) {
  return request.patch(`/api/v1/routes/${id}/status`, { enabled })
}
