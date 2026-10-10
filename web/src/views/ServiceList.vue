<template>
  <div class="page-container">
    <div class="page-card">
      <div class="page-header">
        <span class="page-title">服务管理</span>
        <el-button type="primary" :icon="Plus" @click="openCreate">注册服务</el-button>
      </div>

      <!-- 表格 -->
      <el-table :data="list" v-loading="loading" border stripe style="width: 100%">
        <el-table-column prop="service_name" label="服务名称" min-width="160">
          <template #default="{ row }">
            <span class="mono">{{ row.service_name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="display_name" label="显示名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="health_check_path" label="健康检查路径" width="140">
          <template #default="{ row }">
            <span class="mono">{{ row.health_check_path }}</span>
          </template>
        </el-table-column>
        <el-table-column label="健康检查" width="160" align="center">
          <template #default="{ row }">
            <span>{{ row.health_check_interval }}s / {{ row.health_check_timeout }}s</span>
          </template>
        </el-table-column>
        <el-table-column prop="default_weight" label="默认权重" width="100" align="center" />
        <el-table-column prop="unhealthy_threshold" label="不健康阈值" width="100" align="center" />
        <el-table-column prop="healthy_threshold" label="恢复阈值" width="100" align="center" />
        <el-table-column label="操作" width="160" fixed="right">
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
      title="注册服务"
      width="600px"
      :close-on-click-modal="false"
      @closed="resetForm"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="130px">
        <el-form-item label="服务名称" prop="service_name">
          <el-input v-model="form.service_name" placeholder="如：user-service（唯一）" />
        </el-form-item>
        <el-form-item label="显示名称">
          <el-input v-model="form.display_name" placeholder="如：用户服务" />
        </el-form-item>
        <div class="form-row-2">
          <el-form-item label="健康检查路径">
            <el-input v-model="form.health_check_path" placeholder="默认 /health" />
          </el-form-item>
          <el-form-item label="默认权重">
            <el-input-number v-model="form.default_weight" :min="1" :max="100" style="width: 100%" />
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="检查间隔(s)">
            <el-input-number v-model="form.health_check_interval" :min="1" :max="300" style="width: 100%" />
          </el-form-item>
          <el-form-item label="检查超时(s)">
            <el-input-number v-model="form.health_check_timeout" :min="1" :max="60" style="width: 100%" />
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="不健康阈值">
            <el-input-number v-model="form.unhealthy_threshold" :min="1" :max="10" style="width: 100%" />
          </el-form-item>
          <el-form-item label="恢复阈值">
            <el-input-number v-model="form.healthy_threshold" :min="1" :max="10" style="width: 100%" />
          </el-form-item>
        </div>
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
    <el-dialog v-model="detailVisible" title="服务详情" width="560px">
      <el-descriptions :column="2" border v-if="detail">
        <el-descriptions-item label="服务名称">
          <span class="mono">{{ detail.service_name }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="显示名称">{{ detail.display_name || '-' }}</el-descriptions-item>
        <el-descriptions-item label="健康检查路径">
          <span class="mono">{{ detail.health_check_path }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="默认权重">{{ detail.default_weight }}</el-descriptions-item>
        <el-descriptions-item label="检查间隔">{{ detail.health_check_interval }}s</el-descriptions-item>
        <el-descriptions-item label="检查超时">{{ detail.health_check_timeout }}s</el-descriptions-item>
        <el-descriptions-item label="不健康阈值">{{ detail.unhealthy_threshold }}</el-descriptions-item>
        <el-descriptions-item label="恢复阈值">{{ detail.healthy_threshold }}</el-descriptions-item>
        <el-descriptions-item label="创建人">{{ detail.created_by }}</el-descriptions-item>
        <el-descriptions-item label="更新时间">{{ formatTime(detail.updated_at) }}</el-descriptions-item>
        <el-descriptions-item label="描述" :span="2">{{ detail.description || '-' }}</el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import {
  listServices,
  createService,
  deleteService,
  getService,
} from '@/api/service'

const loading = ref(false)
const list = ref([])
const total = ref(0)
const query = reactive({ page: 1, page_size: 20 })

const dialogVisible = ref(false)
const detailVisible = ref(false)
const submitting = ref(false)
const formRef = ref(null)
const detail = ref(null)

const defaultForm = () => ({
  service_name: '',
  display_name: '',
  health_check_path: '/health',
  health_check_interval: 10,
  health_check_timeout: 3,
  unhealthy_threshold: 2,
  healthy_threshold: 2,
  default_weight: 1,
  description: '',
})
const form = reactive(defaultForm())

const rules = {
  service_name: [{ required: true, message: '请输入服务名称', trigger: 'blur' }],
}

function formatTime(t) {
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN')
}

async function loadList() {
  loading.value = true
  try {
    const res = await listServices({
      page: query.page,
      page_size: query.page_size,
    })
    list.value = res?.list || []
    total.value = res?.pagination?.total || 0
  } catch { /* 拦截器已提示 */ } finally {
    loading.value = false
  }
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
      await createService({ ...form })
      ElMessage.success('服务注册成功')
      dialogVisible.value = false
      loadList()
    } catch { /* 拦截器已提示 */ } finally {
      submitting.value = false
    }
  })
}

async function handleDelete(row) {
  try {
    await ElMessageBox.confirm(
      `确定删除服务「${row.service_name}」吗？关联的路由将无法转发。`,
      '删除确认',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' }
    )
    await deleteService(row.service_name)
    ElMessage.success('删除成功')
    loadList()
  } catch { /* 用户取消 */ }
}

async function openDetail(row) {
  try {
    detail.value = await getService(row.service_name)
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
}
.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
