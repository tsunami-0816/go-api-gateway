<template>
  <div class="page-container">
    <!-- 顶部操作栏 -->
    <div class="page-header">
      <span class="page-title">运行概览</span>
      <div>
        <el-button :icon="Refresh" @click="loadAll">刷新</el-button>
        <el-button type="primary" :icon="Lightning" :loading="reloading" @click="handleReload">
          触发热更新
        </el-button>
      </div>
    </div>

    <!-- 健康状态 -->
    <el-row :gutter="16" class="mb16">
      <el-col :span="12">
        <div class="stat-card">
          <div class="stat-label">数据面 /healthz</div>
          <div class="stat-value" :class="healthStatus === 'ok' ? 'text-success' : 'text-danger'">
            {{ healthStatus === 'ok' ? '正常' : '异常' }}
          </div>
          <el-tag :type="healthStatus === 'ok' ? 'success' : 'danger'" size="small">
            {{ healthStatus === 'ok' ? '200 OK' : '不可达' }}
          </el-tag>
        </div>
      </el-col>
      <el-col :span="12">
        <div class="stat-card">
          <div class="stat-label">管理面 /readyz</div>
          <div class="stat-value" :class="readyStatus === 'ready' ? 'text-success' : 'text-warning'">
            {{ readyStatus === 'ready' ? '就绪' : '未就绪' }}
          </div>
          <el-tag :type="readyStatus === 'ready' ? 'success' : 'warning'" size="small">
            配置版本 #{{ configSnapshot?.version ?? '-' }}
          </el-tag>
        </div>
      </el-col>
    </el-row>

    <!-- 配置统计卡片 -->
    <el-row :gutter="16" class="mb16">
      <el-col :span="8" v-for="item in statCards" :key="item.label">
        <div class="stat-card">
          <div class="stat-label">{{ item.label }}</div>
          <div class="stat-value">{{ item.value }}</div>
          <el-progress :percentage="item.percent" :stroke-width="6" :show-text="false" />
        </div>
      </el-col>
    </el-row>

    <!-- 配置分布 + 快捷入口 -->
    <el-row :gutter="16">
      <el-col :span="14">
        <div class="page-card">
          <div class="page-header">
            <span class="page-title">配置资源分布</span>
          </div>
          <v-chart class="chart" :option="chartOption" autoresize />
        </div>
      </el-col>
      <el-col :span="10">
        <div class="page-card">
          <div class="page-header">
            <span class="page-title">快捷入口</span>
          </div>
          <div class="quick-links">
            <el-button
              v-for="link in quickLinks"
              :key="link.path"
              size="large"
              @click="$router.push(link.path)"
            >
              <el-icon><component :is="link.icon" /></el-icon>
              {{ link.title }}
            </el-button>
          </div>
        </div>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { PieChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { ElMessage } from 'element-plus'
import { Refresh, Lightning } from '@element-plus/icons-vue'
import { getConfig, reloadConfig, healthz, readyz } from '@/api/system'

use([CanvasRenderer, PieChart, TooltipComponent, LegendComponent])

const configSnapshot = ref(null)
const healthStatus = ref('unknown')
const readyStatus = ref('unknown')
const reloading = ref(false)

// 统计卡片：基于配置快照各资源计数
const statCards = computed(() => {
  const s = configSnapshot.value || {}
  const total =
    (s.routes_count || 0) +
    (s.rate_limits_count || 0) +
    (s.circuit_breakers_count || 0) +
    (s.gray_releases_count || 0) +
    (s.services_count || 0)
  const pct = (n) => (total ? Math.round((n / total) * 100) : 0)
  return [
    { label: '路由规则', value: s.routes_count ?? 0, percent: pct(s.routes_count) },
    { label: '限流规则', value: s.rate_limits_count ?? 0, percent: pct(s.rate_limits_count) },
    { label: '熔断策略', value: s.circuit_breakers_count ?? 0, percent: pct(s.circuit_breakers_count) },
    { label: '灰度规则', value: s.gray_releases_count ?? 0, percent: pct(s.gray_releases_count) },
    { label: '后端服务', value: s.services_count ?? 0, percent: pct(s.services_count) },
    { label: '配置版本', value: s.version ?? 0, percent: 100 },
  ]
})

// 饼图配置
const chartOption = computed(() => ({
  tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
  legend: { bottom: 0 },
  series: [
    {
      type: 'pie',
      radius: ['40%', '70%'],
      avoidLabelOverlap: true,
      itemStyle: { borderRadius: 6, borderColor: '#fff', borderWidth: 2 },
      label: { show: true, formatter: '{b}\n{c}' },
      data: [
        { name: '路由规则', value: configSnapshot.value?.routes_count ?? 0 },
        { name: '限流规则', value: configSnapshot.value?.rate_limits_count ?? 0 },
        { name: '熔断策略', value: configSnapshot.value?.circuit_breakers_count ?? 0 },
        { name: '灰度规则', value: configSnapshot.value?.gray_releases_count ?? 0 },
        { name: '后端服务', value: configSnapshot.value?.services_count ?? 0 },
      ],
    },
  ],
}))

const quickLinks = [
  { path: '/routes', title: '路由管理', icon: 'Connection' },
  { path: '/rate-limits', title: '限流规则', icon: 'Timer' },
  { path: '/circuit-breakers', title: '熔断策略', icon: 'Warning' },
  { path: '/gray-releases', title: '灰度发布', icon: 'Share' },
  { path: '/services', title: '服务管理', icon: 'Box' },
]

async function loadAll() {
  try {
    configSnapshot.value = await getConfig()
  } catch { /* 拦截器已提示 */ }
  try {
    const h = await healthz()
    healthStatus.value = h?.status || 'ok'
  } catch {
    healthStatus.value = 'error'
  }
  try {
    const r = await readyz()
    readyStatus.value = r?.status || 'ready'
  } catch {
    readyStatus.value = 'not_ready'
  }
}

// 手动触发热更新
async function handleReload() {
  reloading.value = true
  try {
    const res = await reloadConfig()
    ElMessage.success(`热更新成功，当前版本 #${res?.config_version ?? '-'}`)
    await loadAll()
  } catch { /* 拦截器已提示 */ } finally {
    reloading.value = false
  }
}

onMounted(loadAll)
</script>

<style scoped>
.mb16 {
  margin-bottom: 16px;
}
.text-success {
  color: #67c23a;
}
.text-warning {
  color: #e6a23c;
}
.text-danger {
  color: #f56c6c;
}
.chart {
  height: 320px;
}
.quick-links {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}
.quick-links .el-button {
  justify-content: flex-start;
  height: 56px;
}
</style>
