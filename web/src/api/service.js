import request from './request'

// 服务管理 API —— 对应后端 /api/v1/services
export function listServices(params) {
  return request.get('/api/v1/services', { params })
}

export function getService(name) {
  return request.get(`/api/v1/services/${name}`)
}

export function createService(data) {
  return request.post('/api/v1/services', data)
}

export function deleteService(name) {
  return request.delete(`/api/v1/services/${name}`)
}
