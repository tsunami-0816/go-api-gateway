import axios from 'axios'
import { ElMessage } from 'element-plus'
import { getToken, getOperator, removeToken } from '@/utils/auth'
import router from '@/router'

// Axios 实例：统一 baseURL / 超时 / 拦截器
const service = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '',
  timeout: 15000,
})

// 请求拦截器：注入 Bearer Token 与 X-Operator
service.interceptors.request.use(
  (config) => {
    const token = getToken()
    if (token) {
      config.headers['Authorization'] = `Bearer ${token}`
    }
    const operator = getOperator()
    if (operator) {
      config.headers['X-Operator'] = operator
    }
    return config
  },
  (error) => Promise.reject(error)
)

// 响应拦截器：统一解包 {code,message,data}，处理 401 与业务错误
service.interceptors.response.use(
  (response) => {
    const res = response.data
    // 健康检查等非标准接口直接返回
    if (res && typeof res === 'object' && 'code' in res) {
      if (res.code === 0) {
        return res.data
      }
      // 业务错误
      ElMessage.error(res.message || '请求失败')
      return Promise.reject(new Error(res.message || 'Error'))
    }
    return res
  },
  (error) => {
    const status = error.response?.status
    const msg = error.response?.data?.message || error.message
    if (status === 401) {
      ElMessage.error('登录已过期，请重新登录')
      removeToken()
      router.push('/login')
    } else if (status === 429) {
      ElMessage.warning('请求过于频繁，请稍后再试')
    } else if (status === 503) {
      ElMessage.error('服务暂不可用（熔断开启）')
    } else {
      ElMessage.error(msg || '网络异常')
    }
    return Promise.reject(error)
  }
)

export default service
