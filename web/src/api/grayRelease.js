import request from './request'

// 灰度发布 API —— 对应后端 /api/v1/gray-releases
export function listGrayReleases(params) {
  return request.get('/api/v1/gray-releases', { params })
}

export function getGrayRelease(id) {
  return request.get(`/api/v1/gray-releases/${id}`)
}

export function createGrayRelease(data) {
  return request.post('/api/v1/gray-releases', data)
}

export function deleteGrayRelease(id) {
  return request.delete(`/api/v1/gray-releases/${id}`)
}

// 运行时调整灰度比例（0-100）
export function patchGrayPercent(id, grayPercent) {
  return request.patch(`/api/v1/gray-releases/${id}/percent`, { gray_percent: grayPercent })
}
