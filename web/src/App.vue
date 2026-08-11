<template>
  <router-view v-if="isPublic" />
  <el-container v-else class="layout">
    <el-aside width="200px" class="aside">
      <div class="brand">路由管理</div>
      <el-menu :default-active="$route.path" router>
        <el-menu-item index="/segments">网段管理</el-menu-item>
        <el-menu-item index="/gateways">网关管理</el-menu-item>
        <el-menu-item index="/matrix">绑定矩阵</el-menu-item>
        <el-menu-item index="/routes">路由状态</el-menu-item>
        <el-menu-item index="/settings">设置</el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="header">
        <el-alert
          v-if="authStore.token && !authStore.elevated"
          type="warning"
          :closable="false"
          show-icon
          style="margin-right: 12px"
        >
          <span>当前为只读模式，无法修改路由。</span>
          <el-button size="small" style="margin-left: 12px" @click="relaunchElevated">以管理员重启</el-button>
        </el-alert>
        <span v-else style="color: #909399">已提权</span>
        <div style="flex: 1"></div>
        <span style="color: #606266; margin-right: 16px">{{ title }}</span>
        <el-button link @click="logout">退出登录</el-button>
      </el-header>
      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from './stores/auth'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const isPublic = computed(() => route.meta.public)
const title = computed(() => route.meta.title || '')

async function logout() {
  await authStore.logout()
  router.push('/login')
}

// 通过 /api/me 拿到当前提权状态后刷新页面重进，或提示手动右键管理员运行。
function relaunchElevated() {
  // 让用户手动以管理员重启（浏览器无法直接触发 UAC）。
  ElMessage.info('请关闭本窗口，右键 RouteManager.exe 选择"以管理员身份运行"')
}
</script>

<style>
body { margin: 0; font-family: 'Microsoft YaHei', sans-serif; }
.layout { height: 100vh; }
.aside { background: #001529; }
.aside .brand {
  color: #fff; font-size: 18px; font-weight: 600;
  padding: 18px 20px; text-align: center;
}
.aside .el-menu { border-right: none; background: transparent; }
.aside .el-menu-item { color: rgba(255,255,255,0.75); }
.aside .el-menu-item.is-active { color: #409eff; background: rgba(255,255,255,0.08); }
.header {
  display: flex; align-items: center;
  border-bottom: 1px solid #ebeef5;
  background: #fff;
}
.main { background: #f5f7fa; }
</style>
