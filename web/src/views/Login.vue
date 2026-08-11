<template>
  <div class="auth-page">
    <el-card class="auth-card">
      <h2>路由管理</h2>
      <el-form @submit.prevent="submit">
        <el-form-item>
          <el-input
            v-model="password"
            type="password"
            show-password
            placeholder="管理员密码"
            @keyup.enter="submit"
          />
        </el-form-item>
        <el-button type="primary" style="width: 100%" :loading="loading" @click="submit">
          登录
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
const loading = ref(false)

async function submit() {
  if (!password.value) return
  loading.value = true
  try {
    await authStore.login(password.value)
    router.push('/segments')
  } catch (e) {
    ElMessage.error(e.error || '登录失败')
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
.auth-card h2 { margin: 0 0 24px; }
</style>
