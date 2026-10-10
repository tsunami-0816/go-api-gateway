import { createRouter, createWebHistory } from 'vue-router'
import { getToken } from '@/utils/auth'

// 路由表：登录页独立，其余页面挂在 AdminLayout 下
const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { title: '登录', public: true },
  },
  {
    path: '/',
    component: () => import('@/layouts/AdminLayout.vue'),
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('@/views/Dashboard.vue'),
        meta: { title: '概览', icon: 'Odometer' },
      },
      {
        path: 'routes',
        name: 'RouteList',
        component: () => import('@/views/RouteList.vue'),
        meta: { title: '路由管理', icon: 'Connection' },
      },
      {
        path: 'rate-limits',
        name: 'RateLimitList',
        component: () => import('@/views/RateLimitList.vue'),
        meta: { title: '限流规则', icon: 'Timer' },
      },
      {
        path: 'circuit-breakers',
        name: 'CircuitBreakerList',
        component: () => import('@/views/CircuitBreakerList.vue'),
        meta: { title: '熔断策略', icon: 'Warning' },
      },
      {
        path: 'gray-releases',
        name: 'GrayReleaseList',
        component: () => import('@/views/GrayReleaseList.vue'),
        meta: { title: '灰度发布', icon: 'Share' },
      },
      {
        path: 'services',
        name: 'ServiceList',
        component: () => import('@/views/ServiceList.vue'),
        meta: { title: '服务管理', icon: 'Box' },
      },
    ],
  },
  { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// 全局前置守卫：未登录跳转登录页
router.beforeEach((to, from, next) => {
  document.title = to.meta.title
    ? `${to.meta.title} - Go API Gateway`
    : 'Go API Gateway 管理台'
  if (to.meta.public) {
    next()
    return
  }
  if (!getToken()) {
    next('/login')
    return
  }
  next()
})

export default router
