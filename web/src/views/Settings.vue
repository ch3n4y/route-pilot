<template>
  <div>
    <el-card style="margin-bottom: 16px">
      <template #header>系统信息</template>
      <el-descriptions :column="2" border>
        <el-descriptions-item label="版本">{{ info.version }}</el-descriptions-item>
        <el-descriptions-item label="权限状态">
          <el-tag :type="info.elevated ? 'success' : 'danger'" size="small">
            {{ info.elevated ? '管理员' : '只读' }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="监听">{{ info.host }}:{{ info.port }}</el-descriptions-item>
        <el-descriptions-item label="数据目录">{{ info.data_dir }}</el-descriptions-item>
        <el-descriptions-item label="变更自动同步">
          {{ info.sync_on_change === '1' ? '开启' : '关闭' }}
        </el-descriptions-item>
      </el-descriptions>
    </el-card>

    <el-card>
      <template #header>修改密码</template>
      <el-form label-width="90px" style="max-width: 420px">
        <el-form-item label="旧密码">
          <el-input v-model="pw.old_password" type="password" show-password />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="pw.new_password" type="password" show-password />
        </el-form-item>
        <el-button type="primary" :loading="saving" @click="changePassword">保存</el-button>
      </el-form>
    </el-card>

    <el-card>
      <template #header>程序</template>
      <el-button type="danger" plain @click="shutdownApp">退出程序</el-button>
      <span style="color: #909399; font-size: 13px; margin-left: 12px">
        停止 HTTP 服务并退出（也可用系统托盘图标退出）
      </span>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import http from '../api'

const info = ref({})
const saving = ref(false)
const pw = ref({ old_password: '', new_password: '' })

async function load() {
  info.value = await http.get('/settings')
}

async function changePassword() {
  if (!pw.value.old_password || !pw.value.new_password) {
    ElMessage.warning('请填写完整')
    return
  }
  saving.value = true
  try {
    await http.put('/settings/password', pw.value)
    ElMessage.success('密码已修改')
    pw.value = { old_password: '', new_password: '' }
  } catch (e) {
    ElMessage.error(e.error || '修改失败')
  } finally {
    saving.value = false
  }
}

async function shutdownApp() {
  await ElMessageBox.confirm('确认退出程序？服务将停止。', '提示', { type: 'warning' })
  try {
    await http.post('/system/shutdown')
    ElMessage.success('正在退出…')
  } catch (e) {
    /* 服务已停止，请求必然失败 */
  }
}

onMounted(load)
</script>
