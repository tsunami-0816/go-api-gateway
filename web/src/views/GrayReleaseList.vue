<template>
  <div class="page-container">
    <div class="page-card">
      <div class="page-header">
        <span class="page-title">灰度发布</span>
        <el-button type="primary" :icon="Plus" @click="openCreate">新建灰度规则</el-button>
      </div>

      <!-- 搜索 -->
      <div class="filter-bar">
        <el-input v-model="query.route_id" placeholder="路由 ID" clearable style="width: 220px" @keyup.enter="handleSearch" />
        <el-button type="primary" :icon="Search" @click="handleSearch">查询</el-button>
        <el-button :icon="RefreshLeft" @click="resetQuery">重置</el-button>
      </div>

      <!-- 表格 -->
      <el-table :data="list" v-loading="loading" border stripe style="width: 100%">
        <el-table-column prop="rule_name" label="规则名称" min-width="140" show-overflow-tooltip />
        <el-table-column prop="route_id" label="绑定路由" min-width="200">
          <template #default="{ row }">
            <span class="mono">{{ row.route_id }}</span>
          </template>
        </el-table-column>
        <el-table-column label="版本" min-width="160">
          <template #default="{ row }">
            <div>
              <el-tag size="small" type="success">稳定: {{ row.stable_version }}</el-tag>
            </div>
            <div style="margin-top: 4px">
              <el-tag size="small" type="warning">灰度: {{ row.gray_version }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="灰度比例" width="200">
          <template #default="{ row }">
            <div class="percent-cell">
              <el-progress
                :percentage="row.gray_percent"
                :stroke-width="14"
                :color="percentColor(row.gray_percent)"
              />
              <el-button
                size="small"
                text
                type="primary"
                @click="openPercent(row)"
              >调整</el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="hash_key_type" label="哈希维度" width="100" align="center">
          <template #default="{ row }">
            {{ hashKeyMap[row.hash_key_type] || row.hash_key_type }}
          </template>
        </el-table-column>
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
      title="新建灰度规则"
      width="640px"
      :close-on-click-modal="false"
      @closed="resetForm"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px">
        <el-form-item label="规则名称" prop="rule_name">
          <el-input v-model="form.rule_name" placeholder="如：用户服务 v2 灰度" />
        </el-form-item>
        <div class="form-row-2">
          <el-form-item label="绑定路由 ID" prop="route_id">
            <el-input v-model="form.route_id" placeholder="路由 UUID" />
          </el-form-item>
          <el-form-item label="灰度版本" prop="gray_version">
            <el-input v-model="form.gray_version" placeholder="如：v2" />
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="稳定版本">
            <el-input v-model="form.stable_version" placeholder="默认 stable" />
          </el-form-item>
          <el-form-item label="灰度比例(%)">
            <el-slider v-model="form.gray_percent" :min="0" :max="100" show-input />
          </el-form-item>
        </div>
        <div class="form-row-2">
          <el-form-item label="哈希维度">
            <el-select v-model="form.hash_key_type" style="width: 100%">
              <el-option label="客户端 IP" value="ip" />
              <el-option label="Header" value="header" />
              <el-option label="Cookie" value="cookie" />
              <el-option label="用户 ID" value="user_id" />
            </el-select>
          </el-form-item>
          <el-form-item label="哈希 Key 名">
            <el-input v-model="form.hash_key_name" placeholder="header/cookie 维度时指定" />
          </el-form-item>
        </div>
        <el-form-item label="染色规则 (JSON)">
          <el-input
            v-model="form.stain_rules_text"
            type="textarea"
            :rows="3"
            placeholder='[{"type":"header","key":"X-Gray","value":"true"}]'
          />
          <div class="form-tip">可选，JSON 数组；命中染色规则的请求 100% 走灰度版本</div>
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

    <!-- 调整灰度比例弹窗 -->
    <el-dialog v-model="percentVisible" title="调整灰度比例" width="480px">
      <div v-if="percentRow" class="percent-dialog">
        <p>规则：<strong>{{ percentRow.rule_name }}</strong></p>
        <p>当前灰度版本：<el-tag size="small" type="warning">{{ percentRow.gray_version }}</el-tag></p>
        <el-slider
          v-model="percentValue"
          :min="0"
          :max="100"
          :step="5"
          show-input
          style="margin: 24px 0"
        />
        <el-alert
          v-if="percentValue === 100"
          type="warning"
          :closable="false"
          title="100% 表示全量切到灰度版本，请确认"
          show-icon
        />
      </div>
      <template #footer>
        <el-button @click="percentVisible = false">取消</el-button>
        <el-button type="primary" :loading="percentSubmitting" @click="submitPercent">应用</el-button>
      </template>
    </el-dialog>

    <!-- 详情弹窗 -->
    <el-dialog v-model="detailVisible" title="灰度规则详情" width="560px">
      <el-descriptions :column="2" border v-if="detail">
        <el-descriptions-item label="规则名称">{{ detail.rule_name }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="detail.enabled ? 'success' : 'info'" size="small">
            {{ detail.enabled ? '启用' : '禁用' }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="路由 ID" :span="2">
          <span class="mono">{{ detail.route_id }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="灰度版本">{{ detail.gray_version }}</el-descriptions-item>
        <el-descriptions-item label="稳定版本">{{ detail.stable_version }}</el-descriptions-item>
        <el-descriptions-item label="灰度比例">{{ detail.gray_percent }}%</el-descriptions-item>
        <el-descriptions-item label="哈希维度">
          {{ hashKeyMap[detail.hash_key_type] || detail.hash_key_type }}
        </el-descriptions-item>
        <el-descriptions-item label="哈希 Key">{{ detail.hash_key_name || '-' }}</el-descriptions-item>
        <el-descriptions-item label="创建人">{{ detail.created_by }}</el-descriptions-item>
        <el-descriptions-item label="染色规则" :span="2">
          <pre class="json-pre">{{ detail.stain_rules || '[]' }}</pre>
        </el-descriptions-item>
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
  listGrayReleases,
  createGrayRelease,
  deleteGrayRelease,
  getGrayRelease,
  patchGrayPercent,
} from '@/api/grayRelease'

const hashKeyMap = {
  ip: 'IP',
  header: 'Header',
  cookie: 'Cookie',
  user_id: '用户ID',
}

const loading = ref(false)
const list = ref([])
const total = ref(0)
const query = reactive({ page: 1, page_size: 20, route_id: '' })

const dialogVisible = ref(false)
const detailVisible = ref(false)
const percentVisible = ref(false)
const submitting = ref(false)
const percentSubmitting = ref(false)
const formRef = ref(null)
const detail = ref(null)
const percentRow = ref(null)
const percentValue = ref(0)

const defaultForm = () => ({
  rule_name: '',
  route_id: '',
  gray_version: '',
  stable_version: 'stable',
  gray_percent: 10,
  hash_key_type: 'ip',
  hash_key_name: '',
  stain_rules_text: '',
  enabled: true,
  description: '',
})
const form = reactive(defaultForm())

const rules = {
  rule_name: [{ required: true, message: '请输入规则名称', trigger: 'blur' }],
  route_id: [{ required: true, message: '请输入路由 ID', trigger: 'blur' }],
  gray_version: [{ required: true, message: '请输入灰度版本', trigger: 'blur' }],
}

// 比例颜色：低绿、中橙、高红
function percentColor(p) {
  if (p <= 30) return '#67c23a'
  if (p <= 70) return '#e6a23c'
  return '#f56c6c'
}

async function loadList() {
  loading.value = true
  try {
    const res = await listGrayReleases({
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
      // 染色规则文本转 JSON 对象
      let stainRules = []
      if (form.stain_rules_text.trim()) {
        stainRules = JSON.parse(form.stain_rules_text)
      }
      const payload = {
        rule_name: form.rule_name,
        route_id: form.route_id,
        gray_version: form.gray_version,
        stable_version: form.stable_version,
        gray_percent: form.gray_percent,
        hash_key_type: form.hash_key_type,
        hash_key_name: form.hash_key_name,
        stain_rules: stainRules,
        enabled: form.enabled,
        description: form.description,
      }
      await createGrayRelease(payload)
      ElMessage.success('灰度规则创建成功')
      dialogVisible.value = false
      loadList()
    } catch (err) {
      if (err instanceof SyntaxError) {
        ElMessage.error('染色规则 JSON 格式错误')
      }
      /* 其他错误拦截器已提示 */
    } finally {
      submitting.value = false
    }
  })
}

async function handleDelete(row) {
  try {
    await ElMessageBox.confirm(`确定删除灰度规则「${row.rule_name}」吗？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
    await deleteGrayRelease(row.id)
    ElMessage.success('删除成功')
    loadList()
  } catch { /* 用户取消 */ }
}

async function openDetail(row) {
  try {
    detail.value = await getGrayRelease(row.id)
    detailVisible.value = true
  } catch { /* 拦截器已提示 */ }
}

// 打开比例调整弹窗
function openPercent(row) {
  percentRow.value = row
  percentValue.value = row.gray_percent
  percentVisible.value = true
}

// 提交比例调整（运行时 PATCH）
async function submitPercent() {
  if (!percentRow.value) return
  percentSubmitting.value = true
  try {
    await patchGrayPercent(percentRow.value.id, percentValue.value)
    ElMessage.success(`灰度比例已调整为 ${percentValue.value}%`)
    percentRow.value.gray_percent = percentValue.value
    percentVisible.value = false
  } catch { /* 拦截器已提示 */ } finally {
    percentSubmitting.value = false
  }
}

onMounted(loadList)
</script>

<style scoped>
.mono {
  font-family: monospace;
  font-size: 12px;
  color: #606266;
}
.percent-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}
.percent-cell .el-progress {
  flex: 1;
}
.pagination-wrap {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}
.json-pre {
  margin: 0;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
  background: #f5f7fa;
  padding: 8px;
  border-radius: 4px;
}
.percent-dialog p {
  margin: 8px 0;
}
</style>
