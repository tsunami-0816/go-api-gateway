// Token 本地存储工具：管理 Bearer Token 与操作人
const TOKEN_KEY = 'gw_admin_token'
const OPERATOR_KEY = 'gw_admin_operator'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || ''
}

export function setToken(token) {
  localStorage.setItem(TOKEN_KEY, token)
}

export function removeToken() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(OPERATOR_KEY)
}

export function getOperator() {
  return localStorage.getItem(OPERATOR_KEY) || ''
}

export function setOperator(operator) {
  localStorage.setItem(OPERATOR_KEY, operator)
}
