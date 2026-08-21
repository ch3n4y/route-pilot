<template>
  <div>
    <el-card>
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
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import http from '../api'

const info = ref({})

async function load() {
  try {
    info.value = await http.get('/settings')
  } catch (e) {
    ElMessage.error(e.error || '设置加载失败')
  }
}

onMounted(load)
</script>
