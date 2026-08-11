<template>
  <div class="auth-page">
    <el-card class="auth-card">
      <h2>首次使用</h2>
      <p style="color: #909399">设置管理员登录密码</p>
      <el-form @submit.prevent="submit">
        <el-form-item>
          <el-input v-model="password" type="password" show-password placeholder="设置密码" />
        </el-form-item>
        <el-form-item>
          <el-input v-model="confirm" type="password" show-password placeholder="确认密码" />
        </el-form-item>
        <el-button type="primary" style="width: 100%" :loading="loading" @click="submit">
          完成设置
        </el-button>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const authStore = useAuthStore()
const password = ref('')
const confirm = ref('')
const loading = ref(false)

async function submit() {
  if (!password.value || password.value.length < 6) {
    ElMessage.warning('密码至少 6 位')
    return
  }
  if (password.value !== confirm.value) {
    ElMessage.warning('两次密码不一致')
    return
  }
  loading.value = true
  try {
    await authStore.setup(password.value)
    router.push('/segments')
  } catch (e) {
    ElMessage.error(e.error || '设置失败')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.auth-page {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f0f2f5;
}
.auth-card { width: 360px; text-align: center; }
.auth-card h2 { margin: 0 0 8px; }
</style>
