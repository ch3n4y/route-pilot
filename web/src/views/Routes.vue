<template>
  <div class="page-stack">
    <div class="stat-grid">
      <div class="stat-card stat-ok"><strong>{{ summary.ok || 0 }}</strong><span>正常</span></div>
      <div class="stat-card stat-missing"><strong>{{ summary.missing || 0 }}</strong><span>缺失</span></div>
      <div class="stat-card stat-conflict"><strong>{{ summary.conflict || 0 }}</strong><span>冲突</span></div>
      <div class="stat-card stat-error"><strong>{{ summary.error || 0 }}</strong><span>错误</span></div>
    </div>

    <el-card shadow="never" class="table-card">
      <template #header>
        <div class="card-header">
          <div>
            <div class="card-title">期望路由状态</div>
            <div class="card-subtitle">冲突处理会删除该网段已有路由，并按目标网关重新创建</div>
          </div>
          <div class="actions">
            <el-button
              v-if="conflicts.length"
              type="danger"
              :disabled="!auth.elevated"
              :loading="resolving"
              @click="resolveConflicts(conflicts)"
            >
              一键解决全部冲突（{{ conflicts.length }}）
            </el-button>
            <el-button :disabled="!auth.elevated" :loading="syncing" @click="manualSync">手动同步</el-button>
            <el-button @click="refreshAll">刷新</el-button>
          </div>
        </div>
      </template>

      <div class="sync-meta">
        <el-tag v-if="status.syncing" type="warning">同步中…</el-tag>
        <span v-else>上次同步：{{ formatTime(status.last_sync_at) }}</span>
      </div>

      <el-table
        v-loading="loading"
        :data="status.entries"
        empty-text="暂无期望路由，请先配置活动网关"
        stripe
        row-key="binding_id"
        table-layout="fixed"
      >
        <el-table-column prop="segment_name" label="网段" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
      <div class="primary-cell"><code>{{ row.cidr }}</code></div>
          </template>
        </el-table-column>
        <el-table-column prop="gateway_ip" label="目标网关" min-width="140">
          <template #default="{ row }"><code>{{ row.gateway_ip }}</code></template>
        </el-table-column>
        <el-table-column label="有效跃点" width="84" align="center">
          <template #default="{ row }"><code>{{ row.metric }}</code></template>
        </el-table-column>
        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="tagType(row.status)" effect="light" round>{{ statusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="error" label="诊断信息" min-width="240" show-overflow-tooltip>
          <template #default="{ row }">
            <span :class="row.error ? 'error-text' : 'muted'">{{ row.error || '—' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="126" fixed="right" align="center">
          <template #default="{ row }">
            <el-button
              v-if="row.status === 'CONFLICT'"
              type="danger"
              size="small"
              plain
              :disabled="!auth.elevated"
              :loading="resolvingId === row.segment_id"
              @click="resolveConflicts([row])"
            >解决冲突</el-button>
            <span v-else class="muted">无需操作</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card shadow="never" class="table-card">
      <template #header>
        <div class="card-header">
          <div>
            <div class="card-title">系统路由表</div>
            <div class="card-subtitle">用于核对 Windows 当前活动路由和持久路由</div>
          </div>
          <div class="actions">
            <el-button
              type="danger"
              plain
              :disabled="!auth.elevated || !actual.persistent.length"
              :loading="clearingPersistent"
              @click="clearPersistent"
            >
              清空永久路由
            </el-button>
            <el-button @click="loadActual">刷新实际路由</el-button>
          </div>
        </div>
      </template>
      <el-tabs>
        <el-tab-pane :label="`活动路由（${actual.active.length}）`">
      <el-table :data="actual.active" stripe size="small" max-height="360" empty-text="暂无活动路由" table-layout="fixed">
      <el-table-column prop="cidr" label="目标网段" min-width="180" />
      <el-table-column prop="gateway" label="下一跳" min-width="150" />
      <el-table-column prop="interface" label="接口" min-width="160" />
      <el-table-column prop="metric" label="跃点数" width="90" align="center" />
      <el-table-column label="操作" width="100" align="center">
        <template #default="{ row }">
        <el-button link type="danger" :disabled="!auth.elevated" :loading="deletingKey === routeKey(row, 'active')" @click="deleteRoute(row, 'active')">删除</el-button>
        </template>
      </el-table-column>
      </el-table>
        </el-tab-pane>
        <el-tab-pane :label="`持久路由（${actual.persistent.length}）`">
      <el-table :data="actual.persistent" stripe size="small" max-height="360" empty-text="暂无持久路由" table-layout="fixed">
      <el-table-column prop="cidr" label="目标网段" min-width="180" />
      <el-table-column prop="gateway" label="下一跳" min-width="150" />
      <el-table-column prop="metric" label="跃点数" width="90" align="center" />
      <el-table-column label="操作" width="100" align="center">
        <template #default="{ row }">
        <el-button link type="danger" :disabled="!auth.elevated" :loading="deletingKey === routeKey(row, 'persistent')" @click="deleteRoute(row, 'persistent')">删除</el-button>
        </template>
      </el-table-column>
      </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import http from '../api'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const status = ref({ syncing: false, last_sync_at: '', entries: [], summary: {} })
const actual = ref({ active: [], persistent: [] })
const loading = ref(false)
const syncing = ref(false)
const resolving = ref(false)
const resolvingId = ref(0)
const deletingKey = ref('')
const clearingPersistent = ref(false)
let timer = null

const summary = computed(() => status.value.summary || {})
const conflicts = computed(() => (status.value.entries || []).filter((row) => row.status === 'CONFLICT'))

function tagType(value) {
  return { OK: 'success', MISSING: 'warning', CONFLICT: 'danger', MISMATCH: 'warning', ERROR: 'danger' }[value] || 'info'
}

function statusText(value) {
  return { OK: '正常', MISSING: '缺失', CONFLICT: '冲突', MISMATCH: '跃点不符', ERROR: '错误' }[value] || value
}

function formatTime(value) {
  if (!value) return '尚未同步'
  const date = new Date(value)
  return date.getFullYear() <= 1 ? '尚未同步' : date.toLocaleString()
}

async function loadStatus(silent = true) {
  if (!silent) loading.value = true
  try {
    status.value = await http.get('/routes/status')
  } catch (e) {
    if (!silent) ElMessage.error(e.error || '状态加载失败')
  } finally {
    loading.value = false
  }
}

async function loadActual() {
  try {
    actual.value = await http.get('/routes/actual')
  } catch (e) {
    ElMessage.error(e.error || '实际路由读取失败')
  }
}

async function refreshAll() {
  await Promise.all([loadStatus(false), loadActual()])
}

async function manualSync() {
  syncing.value = true
  try {
    status.value = await http.post('/routes/sync')
    await loadActual()
    ElMessage.success('同步完成')
  } catch (e) {
    ElMessage.error(e.error || '同步失败')
  } finally {
    syncing.value = false
  }
}

function routeKey(row, store) {
  return `${store}:${row.cidr}:${row.gateway}`
}

async function deleteRoute(row, store) {
  const storeText = store === 'active' ? '活动路由' : '持久路由'
  try {
    await ElMessageBox.confirm(
    `确认删除${storeText}？\n目标网段：${row.cidr}\n下一跳：${row.gateway}\n此操作不会删除数据库中的网段配置。`,
    `删除${storeText}`,
    { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消', dangerouslyUseHTMLString: false },
    )
  } catch (e) {
    return
  }
  deletingKey.value = routeKey(row, store)
  try {
    await http.post('/routes/delete', { cidr: row.cidr, gateway: row.gateway, store })
    await loadActual()
    ElMessage.success(`${storeText}已删除`)
  } catch (e) {
    ElMessage.error(e.error || '删除路由失败')
  } finally {
    deletingKey.value = ''
  }
}

async function clearPersistent() {
  try {
    await ElMessageBox.confirm(
      '将删除本应用创建、以及当前网段命中的所有永久路由（系统接口路由不受影响）。\n仍在使用的网段会立即以活动路由方式重建。是否继续？',
      '清空永久路由',
      { type: 'warning', confirmButtonText: '确认清空', cancelButtonText: '取消' },
    )
  } catch (e) {
    return
  }
  clearingPersistent.value = true
  try {
    const result = await http.post('/routes/clear-persistent')
    status.value = result.status
    await loadActual()
    const failed = result.failures || []
    if (failed.length) {
      ElMessage.warning(`已清空 ${result.cleared} 条，${failed.length} 条失败：${failed[0].error}`)
    } else {
      ElMessage.success(`已清空 ${result.cleared} 条永久路由`)
    }
  } catch (e) {
    ElMessage.error(e.error || '清空失败')
  } finally {
    clearingPersistent.value = false
  }
}

async function resolveConflicts(rows) {
  const names = rows.map((row) => row.cidr).join('、')
  try {
    await ElMessageBox.confirm(
      `将删除「${names}」已有的冲突路由，并按目标网关重新创建。是否继续？`,
      '解决路由冲突',
      { type: 'warning', confirmButtonText: '删除并重建', cancelButtonText: '取消' },
    )
  } catch (e) {
    return
  }

  resolving.value = rows.length > 1
  resolvingId.value = rows.length === 1 ? rows[0].segment_id : 0
  try {
    const result = await http.post('/routes/resolve-conflicts', {
      segment_ids: rows.map((row) => row.segment_id),
    })
    status.value = result.status
    await loadActual()
    const failed = result.results.filter((item) => !item.ok)
    if (failed.length) {
      ElMessage.warning(`${result.results.length - failed.length} 条已解决，${failed.length} 条失败：${failed[0].error}`)
    } else {
      ElMessage.success(`已解决 ${result.results.length} 条路由冲突`)
    }
  } catch (e) {
    ElMessage.error(e.error || '冲突处理失败')
  } finally {
    resolving.value = false
    resolvingId.value = 0
  }
}

onMounted(() => {
  refreshAll()
  timer = setInterval(() => loadStatus(true), 3000)
})
onBeforeUnmount(() => clearInterval(timer))
</script>

<style scoped>
.page-stack { display: grid; gap: 16px; }
.stat-grid { display: grid; grid-template-columns: repeat(4, minmax(120px, 1fr)); gap: 12px; }
.stat-card { display: flex; align-items: baseline; gap: 10px; padding: 16px 18px; border-radius: 10px; background: #fff; border: 1px solid #e4e7ed; }
.stat-card strong { font-size: 28px; line-height: 1; }
.stat-card span { color: #606266; }
.stat-ok strong { color: #67c23a; }
.stat-missing strong { color: #e6a23c; }
.stat-conflict strong, .stat-error strong { color: #f56c6c; }
.table-card { border-radius: 10px; }
.card-header, .actions { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
.card-title { font-size: 16px; font-weight: 600; color: #303133; }
.card-subtitle, .sync-meta, .secondary-cell, .muted { margin-top: 4px; color: #909399; font-size: 12px; }
.sync-meta { margin: 0 0 12px; }
.primary-cell { color: #303133; font-weight: 500; }
.error-text { color: #f56c6c; }
code { color: #303133; font-family: Consolas, monospace; }
@media (max-width: 720px) {
  .stat-grid { grid-template-columns: repeat(2, 1fr); }
  .actions { width: 100%; justify-content: flex-start; }
}
</style>
