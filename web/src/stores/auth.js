import { defineStore } from 'pinia'
import { getToken, setToken, removeToken, getOperator, setOperator } from '@/utils/auth'
import { getConfig } from '@/api/system'

// 认证状态管理：token / operator / 登录登出 / 获取当前配置快照
export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: getToken(),
    operator: getOperator(),
    configSnapshot: null,
  }),
  getters: {
    isLoggedIn: (state) => !!state.token,
  },
  actions: {
    // 登录：保存 token 与操作人（网关为静态 Token 鉴权，无独立登录接口）
    login({ token, operator }) {
      this.token = token
      this.operator = operator || 'admin'
      setToken(token)
      setOperator(this.operator)
    },
    logout() {
      this.token = ''
      this.operator = ''
      this.configSnapshot = null
      removeToken()
    },
    // 拉取当前配置快照，用于 Dashboard 展示
    async fetchConfig() {
      this.configSnapshot = await getConfig()
      return this.configSnapshot
    },
  },
})
