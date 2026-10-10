<template>
  <!-- 登录页：网关采用静态 Bearer Token 鉴权，此处录入 Token 与操作人 -->
  <div class="login-bg">
    <el-card class="login-card" shadow="always">
      <div class="login-header">
        <el-icon :size="40" color="#409eff"><Promotion /></el-icon>
        <h2>Go API Gateway 管理台</h2>
        <p class="subtitle">高性能 API 网关与流量治理平台</p>
      </div>

      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-position="top"
        @keyup.enter="handleLogin"
      >
        <el-form-item label="访问 Token" prop="token">
          <el-input
            v-model="form.token"
            type="password"
            placeholder="请输入管理员 Token"
            show-password
            size="large"
          >
            <template #prefix><el-icon><Key /></el-icon></template>
          </el-input>
        </el-form-item>

        <el-form-item label="操作人（可选）" prop="operator">
          <el-input
            v-model="form.operator"
            placeholder="默认为 admin，写入审计日志"
            size="large"
          >
            <template #prefix><el-icon><User /></el-icon></template>
          </el-input>
        </el-form-item>

        <el-button
          type="primary"
          size="large"
          class="login-btn"
          :loading="loading"
          @click="handleLogin"
        >
          登 录
        </el-button>
      </el-form>

      <div class="login-tip">
        <el-alert
          type="info"
          :closable="false"
          show-icon
          title="默认开发 Token：admin-token-dev-001"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import { getConfig } from '@/api/system'

const router = useRouter()
const authStore = useAuthStore()

const formRef = ref(null)
const loading = ref(false)

const form = reactive({
  token: '',
  operator: '',
})

const rules = {
  token: [{ required: true, message: '请输入访问 Token', trigger: 'blur' }],
}

// 登录：校验 token 非空后，调用 /system/config 验证 token 有效性
async function handleLogin() {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    loading.value = true
    try {
      // 先暂存 token，让请求拦截器带上它
      authStore.login({ token: form.token.trim(), operator: form.operator.trim() })
      // 用一次真实请求验证 token 是否有效
      await getConfig()
      ElMessage.success('登录成功')
      router.push('/dashboard')
    } catch (err) {
      // 验证失败，清除已暂存的 token
      authStore.logout()
      ElMessage.error('Token 无效或网关不可用，请检查')
    } finally {
      loading.value = false
    }
  })
}
</script>

<style scoped>
.login-card {
  width: 420px;
  border-radius: 10px;
}
.login-header {
  text-align: center;
  margin-bottom: 24px;
}
.login-header h2 {
  margin: 12px 0 4px;
  font-size: 22px;
}
.subtitle {
  color: #909399;
  font-size: 13px;
}
.login-btn {
  width: 100%;
  margin-top: 8px;
}
.login-tip {
  margin-top: 16px;
}
</style>
