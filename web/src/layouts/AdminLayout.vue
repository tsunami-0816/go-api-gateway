<template>
  <!-- 管理台整体布局：左侧导航 + 顶部栏 + 主内容区 -->
  <el-container class="layout-root">
    <!-- 侧边栏 -->
    <el-aside :width="collapse ? '64px' : '220px'" class="layout-aside">
      <div class="logo">
        <el-icon :size="24"><Promotion /></el-icon>
        <span v-show="!collapse" class="logo-text">Gateway Admin</span>
      </div>
      <el-menu
        :default-active="activeMenu"
        :collapse="collapse"
        :collapse-transition="false"
        router
        background-color="#001529"
        text-color="rgba(255,255,255,0.85)"
        active-text-color="#409eff"
      >
        <el-menu-item
          v-for="item in menuItems"
          :key="item.path"
          :index="item.path"
        >
          <el-icon><component :is="item.icon" /></el-icon>
          <template #title>{{ item.title }}</template>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <el-container>
      <!-- 顶部栏 -->
      <el-header class="layout-header">
        <div class="header-left">
          <el-icon class="collapse-btn" @click="collapse = !collapse">
            <Fold v-if="!collapse" />
            <Expand v-else />
          </el-icon>
          <el-breadcrumb separator="/">
            <el-breadcrumb-item :to="{ path: '/dashboard' }">首页</el-breadcrumb-item>
            <el-breadcrumb-item>{{ currentTitle }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right">
          <el-tag type="success" size="small" effect="plain">
            {{ authStore.operator || 'admin' }}
          </el-tag>
          <el-button text @click="handleLogout">
            <el-icon><SwitchButton /></el-icon>
            退出
          </el-button>
        </div>
      </el-header>

      <!-- 主内容区 -->
      <el-main class="layout-main">
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const collapse = ref(false)

// 菜单项与路由 meta 对齐
const menuItems = [
  { path: '/dashboard', title: '概览', icon: 'Odometer' },
  { path: '/routes', title: '路由管理', icon: 'Connection' },
  { path: '/rate-limits', title: '限流规则', icon: 'Timer' },
  { path: '/circuit-breakers', title: '熔断策略', icon: 'Warning' },
  { path: '/gray-releases', title: '灰度发布', icon: 'Share' },
  { path: '/services', title: '服务管理', icon: 'Box' },
]

const activeMenu = computed(() => route.path)
const currentTitle = computed(() => route.meta.title || '')

// 退出登录：确认后清除 token 并跳转
async function handleLogout() {
  try {
    await ElMessageBox.confirm('确定要退出登录吗？', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
    authStore.logout()
    router.push('/login')
  } catch {
    /* 用户取消 */
  }
}
</script>

<style scoped>
.layout-root {
  height: 100vh;
}
.layout-aside {
  background: #001529;
  transition: width 0.2s;
  overflow: hidden;
}
.logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: #fff;
  font-size: 18px;
  font-weight: 700;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}
.logo-text {
  white-space: nowrap;
}
.layout-header {
  background: #fff;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid #e4e7ed;
  padding: 0 20px;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}
.collapse-btn {
  font-size: 20px;
  cursor: pointer;
  color: #606266;
}
.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}
.layout-main {
  background: var(--gw-bg);
  padding: 0;
  overflow-y: auto;
}
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
:deep(.el-menu) {
  border-right: none;
}
</style>
