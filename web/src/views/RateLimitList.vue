<template>
  <div class="page-container">
    <div class="page-card">
      <div class="page-header">
        <span class="page-title">限流规则</span>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建规则</el-button>
      </div>

      <!-- 搜索 -->
      <div class="filter-bar">
        <el-input
          v-model="query.route_id"
          placeholder="路由 ID"
          clearable
          style="width: 220px"
          @keyup.enter="handleSearch"
        />
        <el-button type="primary" :icon="Search" @click="handleSearch">查询</el-button>
        <el-button :icon="RefreshLeft" @click="resetQuery">重置</el-button>
      </div>

      <!-- 表格 -->
      <el-table :data="list" v-loading="loading" border stripe style="width: 100%">
        <el-table-column prop="rule_name" label="规则名称" min-width="140" show-overflow-tooltip />
        <el-table-column label="匹配范围" min-width="200">
          <template #default="{ row }">
            <div v-if="row.route_id">
              <el-tag size="small" type="info">路由</el-tag>
              <span class="mono">{{ row.route_id }}</span>
            </div>
            <div v-else-if="row.path_pattern">
              <el-tag size="small" type="warning">{{ row.method || '*' }}</el-tag>
              <span class="mono">{{ row.path_pattern }}</span>
            </div>
            <el-tag v-else size="small" type="danger">全局</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="algorithm" label="算法" width="120" align="center">
          <template #default="{ row }">
            <el-tag :type="row.algorithm === 'token_bucket' ? 'success' : 'warning'" size="small">
              {{ row.algorithm === 'token_bucket' ? '令牌桶' : '漏桶' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="rate" label="速率(QPS)" width="110" align="center" />
        <el-table-column prop="capacity" label="桶容量" width="100" align="center" />
        <el-table-column prop="limit_by" label="限流维度" width="110" align="center">
          <template #default="{ row }">
            {{ limitByMap[row.limit_by] || row.limit_by }}
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" size="small">
              {{ row.enabled ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <div class="table-actions">
              <el-button size="small" @click="openEdit(row)">编辑</el-button>
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

    <!-- 新建/编辑弹窗 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEdit ? '编辑限流规则' : '新建限流规则'"
      width="640px"
      :close-on-click-modal="false"
      @closed="resetForm"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px">
        <el-form-item label="规则名称" prop="rule_name">
          <el-input v-model="form.rule_name" placeholder="如：用户接口限流" />
        </el-form-item>
        <div class="form-row-2">
          <el-form-item label="绑定路由 ID">
            <el-input v-model="form.route_id" placeholder="留空则按路径匹配" />
          </el-form-item>
          <el-form-item label="路径匹配">
            <el-input v-model="form.path_pattern" placeholder="如：/api/users/*" />
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="HTTP 方法">
            <el-select v-model="form.method" style="width: 100%">
              <el-option label="全部 (*)" value="*" />
              <el-option v-for="m in methods" :key="m" :label="m" :value="m" />
            </el-select>
          </el-form-item>
          <el-form-item label="限流算法" prop="algorithm">
            <el-select v-model="form.algorithm" style="width: 100%">
              <el-option label="令牌桶 (token_bucket)" value="token_bucket" />
              <el-option label="漏桶 (leaky_bucket)" value="leaky_bucket" />
            </el-select>
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="速率(QPS)" prop="rate">
            <el-input-number v-model="form.rate" :min="1" :max="1000000" style="width: 100%" />
          </el-form-item>
          <el-form-item label="桶容量" prop="capacity">
            <el-input-number v-model="form.capacity" :min="1" :max="1000000" style="width: 100%" />
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="限流维度">
            <el-select v-model="form.limit_by" style="width: 100%">
              <el-option label="按路由" value="route" />
              <el-option label="按 IP" value="ip" />
              <el-option label="按 Client-ID" value="client_id" />
            </el-select>
          </el-form-item>
          <el-form-item label="Header Key">
            <el-input v-model="form.header_key" placeholder="client_id 维度时指定 Header" />
          </el-form-item>
        </div>
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
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search, RefreshLeft } from '@element-plus/icons-vue'
import {
  listRateLimits,
  createRateLimit,
  updateRateLimit,
  deleteRateLimit,
} from '@/api/rateLimit'

const methods = ['GET', 'POST', 'PUT', 'DELETE', 'PATCH']
const limitByMap = { route: '按路由', ip: '按 IP', client_id: '按Client' }

const loading = ref(false)
const list = ref([])
const total = ref(0)
const query = reactive({ page: 1, page_size: 20, route_id: '' })

const dialogVisible = ref(false)
const isEdit = ref(false)
const submitting = ref(false)
const formRef = ref(null)
const editId = ref('')

const defaultForm = () => ({
  rule_name: '',
  route_id: '',
  path_pattern: '',
  method: '*',
  algorithm: 'token_bucket',
  rate: 100,
  capacity: 200,
  limit_by: 'route',
  header_key: '',
  enabled: true,
  description: '',
})
const form = reactive(defaultForm())

const rules = {
  rule_name: [{ required: true, message: '请输入规则名称', trigger: 'blur' }],
  algorithm: [{ required: true, message: '请选择算法', trigger: 'change' }],
  rate: [{ required: true, message: '请输入速率', trigger: 'blur' }],
  capacity: [{ required: true, message: '请输入桶容量', trigger: 'blur' }],
}

async function loadList() {
  loading.value = true
  try {
    const res = await listRateLimits({
      page: query.page,
      page_size: query.page_size,
      route_id: query.route_id || undefined,
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
  query.page = 1
  loadList()
}

function openCreate() {
  isEdit.value = false
  editId.value = ''
  Object.assign(form, defaultForm())
  dialogVisible.value = true
}

function openEdit(row) {
  isEdit.value = true
  editId.value = row.id
  Object.assign(form, {
    rule_name: row.rule_name,
    route_id: row.route_id || '',
    path_pattern: row.path_pattern || '',
    method: row.method || '*',
    algorithm: row.algorithm || 'token_bucket',
    rate: row.rate || 100,
    capacity: row.capacity || 200,
    limit_by: row.limit_by || 'route',
    header_key: row.header_key || '',
    enabled: !!row.enabled,
    description: row.description || '',
  })
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
      const payload = { ...form }
      if (isEdit.value) {
        await updateRateLimit(editId.value, payload)
        ElMessage.success('限流规则更新成功')
      } else {
        await createRateLimit(payload)
        ElMessage.success('限流规则创建成功')
      }
      dialogVisible.value = false
      loadList()
    } catch { /* 拦截器已提示 */ } finally {
      submitting.value = false
    }
  })
}

async function handleDelete(row) {
  try {
    await ElMessageBox.confirm(`确定删除限流规则「${row.rule_name}」吗？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
    await deleteRateLimit(row.id)
    ElMessage.success('删除成功')
    loadList()
  } catch { /* 用户取消 */ }
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
