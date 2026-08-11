<template>
  <div>
    <div class="toolbar">
      <el-button :loading="syncing" @click="manualSync">手动同步</el-button>
      <el-button @click="loadActual">刷新实际路由</el-button>
      <el-tag v-if="status.syncing" type="warning" style="margin-left: 8px">同步中…</el-tag>
      <span v-else style="color: #909399; font-size: 13px; margin-left: 8px">
        上次同步：{{ status.last_sync_at ? new Date(status.last_sync_at).toLocaleString() : '—' }}
      </span>
    </div>

    <div class="summary">
      <el-tag type="success">正常 {{ summary.ok }}</el-tag>
      <el-tag type="warning">缺失 {{ summary.missing }}</el-tag>
      <el-tag type="danger">冲突 {{ summary.conflict }}</el-tag>
      <el-tag type="danger">错误 {{ summary.error }}</el-tag>
    </div>

    <el-table :data="status.entries" border style="margin-top: 12px">
      <el-table-column prop="segment_name" label="网段名" min-width="120" />
      <el-table-column prop="cidr" label="网段" min-width="130" />
      <el-table-column prop="gateway_ip" label="目标网关" min-width="130" />
      <el-table-column label="状态" width="90" align="center">
        <template #default="{ row }">
          <el-tag :type="tagType(row.status)" size="small">{{ statusText(row.status) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="error" label="信息" min-width="180" show-overflow-tooltip />
    </el-table>

    <el-collapse style="margin-top: 16px">
      <el-collapse-item title="系统实际路由表（活动）">
        <el-table :data="actual.active" size="small" border max-height="320">
          <el-table-column prop="Cidr" label="网段" width="150" />
          <el-table-column prop="Gateway" label="网关" width="130" />
          <el-table-column prop="Interface" label="接口" min-width="140" />
          <el-table-column prop="Metric" label="跃点数" width="80" align="center" />
        </el-table>
      </el-collapse-item>
      <el-collapse-item title="系统实际路由表（持久）">
        <el-table :data="actual.persistent" size="small" border max-height="320">
          <el-table-column prop="Cidr" label="网段" width="150" />
          <el-table-column prop="Gateway" label="网关" width="130" />
          <el-table-column prop="Metric" label="跃点数" width="80" align="center" />
        </el-table>
      </el-collapse-item>
    </el-collapse>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import http from '../api'

const status = ref({ syncing: false, last_sync_at: '', entries: [], summary: {} })
const actual = ref({ active: [], persistent: [] })
const syncing = ref(false)
let timer = null

const summary = computed(() => status.value.summary || {})

function tagType(s) {
  if (s === 'OK') return 'success'
  if (s === 'MISSING') return 'warning'
  if (s === 'CONFLICT' || s === 'ERROR') return 'danger'
  return 'info'
}
function statusText(s) {
  return { OK: '正常', MISSING: '缺失', CONFLICT: '冲突', ERROR: '错误' }[s] || s
}

async function loadStatus() {
  try {
    status.value = await http.get('/routes/status')
  } catch (e) {
    /* 轮询静默失败 */
  }
}

async function loadActual() {
  try {
    actual.value = await http.get('/routes/actual')
  } catch (e) {
    ElMessage.error(e.error || '读取失败')
  }
}

async function manualSync() {
  syncing.value = true
  try {
    status.value = await http.post('/routes/sync')
    ElMessage.success('同步完成')
  } catch (e) {
    ElMessage.error(e.error || '同步失败')
  } finally {
    syncing.value = false
  }
}

onMounted(() => {
  loadStatus()
  loadActual()
  timer = setInterval(loadStatus, 2000)
})
onBeforeUnmount(() => clearInterval(timer))
</script>

<style scoped>
.toolbar { margin-bottom: 4px; }
.summary { margin-top: 12px; }
.summary .el-tag { margin-right: 8px; }
</style>
