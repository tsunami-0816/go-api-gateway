<template>
  <div class="page-container">
    <div class="page-card">
      <!-- 页头 -->
      <div class="page-header">
        <span class="page-title">路由管理</span>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建路由</el-button>
      </div>

      <!-- 搜索筛选 -->
      <div class="filter-bar">
        <el-input
          v-model="query.path"
          placeholder="路径模糊匹配"
          clearable
          style="width: 200px"
          @keyup.enter="loadList"
        />
        <el-select v-model="query.method" placeholder="HTTP 方法" clearable style="width: 140px">
          <el-option v-for="m in methods" :key="m" :label="m" :value="m" />
        </el-select>
        <el-input
          v-model="query.target_service"
          placeholder="目标服务"
          clearable
          style="width: 180px"
          @keyup.enter="loadList"
        />
        <el-button type="primary" :icon="Search" @click="handleSearch">查询</el-button>
        <el-button :icon="RefreshLeft" @click="resetQuery">重置</el-button>
      </div>

      <!-- 数据表格 -->
      <el-table :data="list" v-loading="loading" border stripe style="width: 100%">
        <el-table-column prop="route_name" label="路由名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="path" label="路径" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ row.method }}</el-tag>
            <span class="path-text">{{ row.path }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="target_service" label="目标服务" min-width="140" show-overflow-tooltip />
        <el-table-column prop="load_balance" label="负载均衡" width="150">
          <template #default="{ row }">
            {{ lbMap[row.load_balance] || row.load_balance }}
          </template>
        </el-table-column>
        <el-table-column prop="timeout_ms" label="超时(ms)" width="100" align="center" />
        <el-table-column prop="priority" label="优先级" width="90" align="center" />
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              :loading="row._statusLoading"
              @change="(val) => handleStatusChange(row, val)"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right">
          <template #default="{ row }">
            <div class="table-actions">
              <el-button size="small" @click="openEdit(row)">编辑</el-button>
              <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <div class="pagination-wrap">
        <el-pagination
          v-model:current-page="query.page"
          v-model:page-size="query.page_size"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="loadList"
          @current-change="loadList"
        />
      </div>
    </div>

    <!-- 新建/编辑弹窗 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑路由' : '新建路由'"
      width="720px"
      :close-on-click-modal="false"
      @closed="resetForm"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px">
        <div class="form-row-2">
          <el-form-item label="路由名称" prop="route_name">
            <el-input v-model="form.route_name" placeholder="如：用户服务路由" />
          </el-form-item>
          <el-form-item label="目标服务" prop="target_service">
            <el-input v-model="form.target_service" placeholder="如：user-service" />
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="请求路径" prop="path">
            <el-input v-model="form.path" placeholder="如：/api/users/*" />
          </el-form-item>
          <el-form-item label="HTTP 方法" prop="method">
            <el-select v-model="form.method" style="width: 100%">
              <el-option v-for="m in methods" :key="m" :label="m" :value="m" />
            </el-select>
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="重写路径">
            <el-input v-model="form.rewrite_path" placeholder="留空则不重写" />
          </el-form-item>
          <el-form-item label="超时(ms)">
            <el-input-number v-model="form.timeout_ms" :min="100" :max="60000" :step="500" style="width: 100%" />
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="重试次数">
            <el-input-number v-model="form.retry_count" :min="0" :max="5" style="width: 100%" />
          </el-form-item>
          <el-form-item label="重试状态码">
            <el-input v-model="form.retry_on_status" placeholder="如：500,502,503" />
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="负载均衡">
            <el-select v-model="form.load_balance" style="width: 100%">
              <el-option label="加权轮询 (WRR)" value="weighted_round_robin" />
              <el-option label="一致性哈希 (CH)" value="consistent_hash" />
            </el-select>
          </el-form-item>
          <el-form-item label="哈希 Header">
            <el-input v-model="form.hash_header" placeholder="CH 模式生效，如：X-User-Id" />
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="优先级">
            <el-input-number v-model="form.priority" :min="1" :max="9999" style="width: 100%" />
          </el-form-item>
          <el-form-item label="非幂等重试">
            <el-switch v-model="form.retry_non_idempotent" />
            <span class="form-tip">默认仅幂等方法重试</span>
          </el-form-item>
        </div>
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="2" placeholder="可选" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search, RefreshLeft } from '@element-plus/icons-vue'
import {
  listRoutes,
  createRoute,
  updateRoute,
  deleteRoute,
  patchRouteStatus,
} from '@/api/route'

const methods = ['GET', 'POST', 'PUT', 'DELETE', 'PATCH', 'HEAD', 'OPTIONS']
const lbMap = {
  weighted_round_robin: '加权轮询',
  consistent_hash: '一致性哈希',
}

// 列表与查询状态
const loading = ref(false)
const list = ref([])
const total = ref(0)
const query = reactive({
  page: 1,
  page_size: 20,
  path: '',
  method: '',
  target_service: '',
})

// 弹窗与表单
const dialogVisible = ref(false)
const isEdit = ref(false)
const submitting = ref(false)
const formRef = ref(null)
const editId = ref('')

const defaultForm = () => ({
  route_name: '',
  path: '',
  method: 'GET',
  target_service: '',
  rewrite_path: '',
  timeout_ms: 5000,
  retry_count: 0,
  retry_on_status: '',
  retry_non_idempotent: false,
  load_balance: 'weighted_round_robin',
  hash_header: '',
  priority: 100,
  enabled: true,
  description: '',
})
const form = reactive(defaultForm())

const rules = {
  route_name: [{ required: true, message: '请输入路由名称', trigger: 'blur' }],
  path: [{ required: true, message: '请输入请求路径', trigger: 'blur' }],
  method: [{ required: true, message: '请选择 HTTP 方法', trigger: 'change' }],
  target_service: [{ required: true, message: '请输入目标服务', trigger: 'blur' }],
}

// 加载列表
async function loadList() {
  loading.value = true
  try {
    const res = await listRoutes({
      page: query.page,
      page_size: query.page_size,
      path: query.path || undefined,
      method: query.method || undefined,
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
  query.path = ''
  query.method = ''
  query.target_service = ''
  query.page = 1
  loadList()
}

// 新建
function openCreate() {
  isEdit.value = false
  editId.value = ''
  Object.assign(form, defaultForm())
  dialogVisible.value = true
}

// 编辑
function openEdit(row) {
  isEdit.value = true
  editId.value = row.id
  Object.assign(form, {
    route_name: row.route_name,
    path: row.path,
    method: row.method,
    target_service: row.target_service,
    rewrite_path: row.rewrite_path || '',
    timeout_ms: row.timeout_ms || 5000,
    retry_count: row.retry_count || 0,
    retry_on_status: row.retry_on_status || '',
    retry_non_idempotent: !!row.retry_non_idempotent,
    load_balance: row.load_balance || 'weighted_round_robin',
    hash_header: row.hash_header || '',
    priority: row.priority || 100,
    enabled: !!row.enabled,
    description: row.description || '',
  })
  dialogVisible.value = true
}

function resetForm() {
  formRef.value?.resetFields()
  Object.assign(form, defaultForm())
}

// 提交（新建或更新）
async function handleSubmit() {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    submitting.value = true
    try {
      const payload = { ...form }
      if (isEdit.value) {
        await updateRoute(editId.value, payload)
        ElMessage.success('路由更新成功')
      } else {
        await createRoute(payload)
        ElMessage.success('路由创建成功')
      }
      dialogVisible.value = false
      loadList()
    } catch { /* 拦截器已提示 */ } finally {
      submitting.value = false
    }
  })
}

// 删除
async function handleDelete(row) {
  try {
    await ElMessageBox.confirm(`确定删除路由「${row.route_name}」吗？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
    await deleteRoute(row.id)
    ElMessage.success('删除成功')
    loadList()
  } catch { /* 用户取消或接口失败 */ }
}

// 启用/禁用
async function handleStatusChange(row, val) {
  row._statusLoading = true
  try {
    await patchRouteStatus(row.id, val)
    ElMessage.success(val ? '已启用' : '已禁用')
    row.enabled = val
  } catch {
    row.enabled = !val // 回滚
  } finally {
    row._statusLoading = false
  }
}

onMounted(loadList)
</script>

<style scoped>
.path-text {
  margin-left: 8px;
  font-family: monospace;
  color: #606266;
}
.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
.form-tip {
  margin-left: 8px;
  font-size: 12px;
  color: #909399;
}
</style>
