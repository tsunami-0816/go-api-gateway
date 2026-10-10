<template>
  <div class="page-container">
    <div class="page-card">
      <div class="page-header">
        <span class="page-title">熔断策略</span>
        <div>
          <el-tooltip content="后端未提供更新接口，修改请删除后重建" placement="top">
            <el-button type="primary" :icon="Plus" @click="openCreate">新建策略</el-button>
          </el-tooltip>
        </div>
      </div>

      <!-- 搜索 -->
      <div class="filter-bar">
        <el-input v-model="query.route_id" placeholder="路由 ID" clearable style="width: 200px" @keyup.enter="handleSearch" />
        <el-input v-model="query.target_service" placeholder="目标服务" clearable style="width: 180px" @keyup.enter="handleSearch" />
        <el-button type="primary" :icon="Search" @click="handleSearch">查询</el-button>
        <el-button :icon="RefreshLeft" @click="resetQuery">重置</el-button>
      </div>

      <!-- 表格 -->
      <el-table :data="list" v-loading="loading" border stripe style="width: 100%">
        <el-table-column prop="policy_name" label="策略名称" min-width="140" show-overflow-tooltip />
        <el-table-column label="作用范围" min-width="180">
          <template #default="{ row }">
            <div v-if="row.route_id">
              <el-tag size="small" type="info">路由</el-tag>
              <span class="mono">{{ row.route_id }}</span>
            </div>
            <div v-else-if="row.target_service">
              <el-tag size="small" type="warning">服务</el-tag>
              <span class="mono">{{ row.target_service }}</span>
            </div>
            <el-tag v-else size="small" type="danger">全局</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="error_rate_threshold" label="错误率阈值(%)" width="130" align="center" />
        <el-table-column prop="slow_call_rate_threshold" label="慢调用比例(%)" width="130" align="center" />
        <el-table-column prop="min_request_count" label="最小请求数" width="110" align="center" />
        <el-table-column prop="open_duration_ms" label="熔断时长(ms)" width="120" align="center" />
        <el-table-column prop="half_open_max_requests" label="半开探测数" width="110" align="center" />
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" size="small">
              {{ row.enabled ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <div class="table-actions">
              <el-button size="small" @click="openDetail(row)">详情</el-button>
              <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-wrap">
        <el-pagination
          v-model:current-page="query.page"
          v-model:page-size="query.page_size"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="loadList"
          @current-change="loadList"
        />
      </div>
    </div>

    <!-- 新建弹窗 -->
    <el-dialog
      v-model="dialogVisible"
      title="新建熔断策略"
      width="680px"
      :close-on-click-modal="false"
      @closed="resetForm"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="140px">
        <el-form-item label="策略名称" prop="policy_name">
          <el-input v-model="form.policy_name" placeholder="如：用户服务熔断" />
        </el-form-item>
        <div class="form-row-2">
          <el-form-item label="绑定路由 ID">
            <el-input v-model="form.route_id" placeholder="与目标服务二选一" />
          </el-form-item>
          <el-form-item label="目标服务">
            <el-input v-model="form.target_service" placeholder="如：user-service" />
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="滑动窗口(ms)">
            <el-input-number v-model="form.sliding_window_size_ms" :min="1000" :max="600000" :step="1000" style="width: 100%" />
          </el-form-item>
          <el-form-item label="最小请求数">
            <el-input-number v-model="form.min_request_count" :min="1" :max="10000" style="width: 100%" />
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="错误率阈值(%)" prop="error_rate_threshold">
            <el-input-number v-model="form.error_rate_threshold" :min="1" :max="100" :step="5" style="width: 100%" />
          </el-form-item>
          <el-form-item label="慢调用阈值(ms)">
            <el-input-number v-model="form.slow_call_duration_ms" :min="10" :max="60000" :step="100" style="width: 100%" />
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="慢调用比例(%)">
            <el-input-number v-model="form.slow_call_rate_threshold" :min="1" :max="100" :step="5" style="width: 100%" />
          </el-form-item>
          <el-form-item label="熔断时长(ms)">
            <el-input-number v-model="form.open_duration_ms" :min="1000" :max="600000" :step="1000" style="width: 100%" />
          </el-form-item>
        </div>
        <el-form-item label="半开最大探测数">
          <el-input-number v-model="form.half_open_max_requests" :min="1" :max="100" style="width: 50%" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">确定</el-button>
      </template>
    </el-dialog>

    <!-- 详情弹窗 -->
    <el-dialog v-model="detailVisible" title="熔断策略详情" width="560px">
      <el-descriptions :column="2" border v-if="detail">
        <el-descriptions-item label="策略名称">{{ detail.policy_name }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="detail.enabled ? 'success' : 'info'" size="small">
            {{ detail.enabled ? '启用' : '禁用' }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="路由 ID">{{ detail.route_id || '-' }}</el-descriptions-item>
        <el-descriptions-item label="目标服务">{{ detail.target_service || '-' }}</el-descriptions-item>
        <el-descriptions-item label="滑动窗口">{{ detail.sliding_window_size_ms }} ms</el-descriptions-item>
        <el-descriptions-item label="最小请求数">{{ detail.min_request_count }}</el-descriptions-item>
        <el-descriptions-item label="错误率阈值">{{ detail.error_rate_threshold }}%</el-descriptions-item>
        <el-descriptions-item label="慢调用阈值">{{ detail.slow_call_duration_ms }} ms</el-descriptions-item>
        <el-descriptions-item label="慢调用比例">{{ detail.slow_call_rate_threshold }}%</el-descriptions-item>
        <el-descriptions-item label="熔断时长">{{ detail.open_duration_ms }} ms</el-descriptions-item>
        <el-descriptions-item label="半开探测数">{{ detail.half_open_max_requests }}</el-descriptions-item>
        <el-descriptions-item label="创建人">{{ detail.created_by }}</el-descriptions-item>
        <el-descriptions-item label="描述" :span="2">{{ detail.description || '-' }}</el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search, RefreshLeft } from '@element-plus/icons-vue'
import {
  listCircuitBreakers,
  createCircuitBreaker,
  deleteCircuitBreaker,
  getCircuitBreaker,
} from '@/api/circuitBreaker'

const loading = ref(false)
const list = ref([])
const total = ref(0)
const query = reactive({ page: 1, page_size: 20, route_id: '', target_service: '' })

const dialogVisible = ref(false)
const detailVisible = ref(false)
const submitting = ref(false)
const formRef = ref(null)
const detail = ref(null)

const defaultForm = () => ({
  policy_name: '',
  route_id: '',
  target_service: '',
  sliding_window_size_ms: 10000,
  min_request_count: 20,
  error_rate_threshold: 50,
  slow_call_duration_ms: 1000,
  slow_call_rate_threshold: 60,
  open_duration_ms: 30000,
  half_open_max_requests: 5,
  enabled: true,
  description: '',
})
const form = reactive(defaultForm())

const rules = {
  policy_name: [{ required: true, message: '请输入策略名称', trigger: 'blur' }],
  error_rate_threshold: [
    { required: true, message: '请输入错误率阈值', trigger: 'blur' },
    { type: 'number', min: 1, max: 100, message: '范围 1-100', trigger: 'blur' },
  ],
}

async function loadList() {
  loading.value = true
  try {
    const res = await listCircuitBreakers({
      page: query.page,
      page_size: query.page_size,
      route_id: query.route_id || undefined,
      target_service: query.target_service || undefined,
    })
    list.value = res?.list || []
    total.value = res?.pagination?.total || 0
  } catch { /* 拦截器已提示 */ } finally {
    loading.value = false
  }
}

function handleSearch() {
  query.page = 1
  loadList()
}
function resetQuery() {
  query.route_id = ''
  query.target_service = ''
  query.page = 1
  loadList()
}

function openCreate() {
  Object.assign(form, defaultForm())
  dialogVisible.value = true
}

function resetForm() {
  formRef.value?.resetFields()
  Object.assign(form, defaultForm())
}

async function handleSubmit() {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    submitting.value = true
    try {
      await createCircuitBreaker({ ...form })
      ElMessage.success('熔断策略创建成功')
      dialogVisible.value = false
      loadList()
    } catch { /* 拦截器已提示 */ } finally {
      submitting.value = false
    }
  })
}

async function handleDelete(row) {
  try {
    await ElMessageBox.confirm(`确定删除熔断策略「${row.policy_name}」吗？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
    await deleteCircuitBreaker(row.id)
    ElMessage.success('删除成功')
    loadList()
  } catch { /* 用户取消 */ }
}

async function openDetail(row) {
  try {
    detail.value = await getCircuitBreaker(row.id)
    detailVisible.value = true
  } catch { /* 拦截器已提示 */ }
}

onMounted(loadList)
</script>

<style scoped>
.mono {
  font-family: monospace;
  font-size: 12px;
  color: #606266;
  margin-left: 6px;
}
.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
