<template>
  <div class="page-stack">
    <el-card shadow="never" class="table-card">
      <template #header>
        <div class="card-header">
          <div>
            <div class="card-title">网段列表</div>
            <div class="card-subtitle">管理目标网段，并为网段选择当前活动网关</div>
          </div>
          <div class="actions">
            <el-button type="primary" @click="openCreate">新增网段</el-button>
            <el-button type="warning" :disabled="!selection.length || !auth.elevated" @click="openBatchSwitch">
              批量切换（{{ selection.length }}）
            </el-button>
            <el-button @click="load">刷新</el-button>
          </div>
        </div>
      </template>

      <el-table
        v-loading="loading"
        :data="items"
        empty-text="暂无网段，请点击“新增网段”开始配置"
        stripe
        row-key="id"
        table-layout="fixed"
        @selection-change="(rows) => (selection = rows)"
      >
        <el-table-column type="selection" width="46" />
        <el-table-column label="网段" min-width="190" fixed="left">
          <template #default="{ row }">
            <div class="primary-cell"><code>{{ row.cidr }}</code></div>
          </template>
        </el-table-column>
        <el-table-column label="跃点" width="76" align="center">
          <template #default="{ row }"><code>{{ row.metric }}</code></template>
        </el-table-column>
        <el-table-column label="已选网关" min-width="240">
          <template #default="{ row }">
            <el-select
              :model-value="selectedGatewayIDs(row)"
              multiple
              collapse-tags
              collapse-tags-tooltip
              placeholder="选择一个或多个网关"
              size="small"
              style="width: 100%"
              :disabled="!auth.elevated || !row.enabled"
              @change="(gatewayIDs) => setGatewaySelection(row, gatewayIDs)"
            >
              <el-option
                v-for="gateway in gateways"
                :key="gateway.id"
                :value="gateway.id"
                :label="`${gateway.name}（${gateway.gateway_ip}）`"
              />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column label="优先级（可拖拽）" min-width="220">
          <template #default="{ row }">
            <div class="priority-list">
              <div
                v-for="(binding, index) in enabledBindings(row)"
                :key="binding.id"
                class="priority-item"
                draggable="true"
                @dragstart="startDrag(row.id, binding.id)"
                @dragover.prevent
                @drop="dropBinding(row, binding.id)"
              ><span class="drag-handle">⠿</span><el-tag size="small" type="success">{{ index + 1 }}. {{ binding.gateway_name || `#${binding.gateway_id}` }}</el-tag></div>
              <span v-if="!enabledBindings(row).length" class="muted">未选择网关</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="备注" min-width="150" show-overflow-tooltip>
          <template #default="{ row }"><span>{{ row.description || '—' }}</span></template>
        </el-table-column>
        <el-table-column label="状态" width="86" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" inline-prompt active-text="启" inactive-text="停" @change="(value) => toggleEnabled(row, value)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="142" fixed="right" align="center">
          <template #default="{ row }">
            <el-button size="small" link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" link type="danger" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 新增/编辑 -->
    <el-dialog v-model="dialog" :title="form.id ? '编辑网段' : '新增网段'" width="480px">
      <el-form label-width="80px">
        <el-form-item label="网段" required>
      <el-input v-model="form.cidr" placeholder="如 192.168.0.0/16 或 192.168.27.10" />
      <div class="form-tip">单个 IPv4 地址会自动保存为 /32 主机路由；支持与现有网段重叠/嵌套</div>
        </el-form-item>
        <el-form-item label="跃点">
          <el-input-number v-model="form.metric" :min="1" :max="9999" />
          <div class="form-tip">被更具体的网段覆盖时，本段跃点会自动 +1 提升（如 19.25.0.0/16 下加 19.25.22.0/24，/16 变 2）</div>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.description" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <!-- 批量切换 -->
    <el-dialog v-model="batchDialog" title="批量切换网关" width="420px">
      <p>已选 {{ selection.length }} 个网段，切换到网关：</p>
      <el-select v-model="batchGatewayId" placeholder="选择目标网关" style="width: 100%">
        <el-option v-for="g in gateways" :key="g.id" :value="g.id" :label="g.name + ' (' + g.gateway_ip + ')'" />
      </el-select>
      <template #footer>
        <el-button @click="batchDialog = false">取消</el-button>
        <el-button type="warning" :loading="batchLoading" @click="doBatchSwitch">确认切换</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import http from '../api'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const items = ref([])
const loading = ref(false)
const gateways = ref([])
const selection = ref([])
const dialog = ref(false)
const batchDialog = ref(false)
const saving = ref(false)
const batchLoading = ref(false)
const batchGatewayId = ref(null)
const dragging = ref(null)
const form = ref({ id: 0, cidr: '', metric: 1, description: '' })

async function load() {
  loading.value = true
  try {
    const [r, g] = await Promise.all([http.get('/segments'), http.get('/gateways')])
    items.value = r.items
    gateways.value = g.items.filter((x) => x.enabled)
  } catch (e) {
    ElMessage.error(e.error || '加载失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  form.value = { id: 0, cidr: '', metric: 1, description: '' }
  dialog.value = true
}
function openEdit(row) {
  form.value = { id: row.id, cidr: row.cidr, metric: row.metric || 1, description: row.description }
  dialog.value = true
}

async function save() {
  saving.value = true
  try {
    if (form.value.id) {
      await http.put(`/segments/${form.value.id}`, form.value)
    } else {
      await http.post('/segments', form.value)
    }
    dialog.value = false
    ElMessage.success('已保存')
    await load()
  } catch (e) {
    ElMessage.error(e.error || '保存失败')
  } finally {
    saving.value = false
  }
}

async function del(row) {
  try {
  await ElMessageBox.confirm(`确认删除网段「${row.cidr}」？`, '提示', { type: 'warning' })
    await http.delete(`/segments/${row.id}`)
    ElMessage.success('已删除')
    await load()
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(e.error || '删除失败')
  }
}

async function toggleEnabled(row, v) {
  try {
    await http.put(`/segments/${row.id}`, {
      cidr: row.cidr,
      description: row.description,
      enabled: v,
    })
    ElMessage.success(v ? '已启用' : '已禁用')
    await load()
  } catch (e) {
    ElMessage.error(e.error || '操作失败')
  }
}

function enabledBindings(row) {
  return [...row.bindings].filter((binding) => binding.enabled).sort((a, b) => a.position - b.position || a.id - b.id)
}

function selectedGatewayIDs(row) {
  return enabledBindings(row).map((binding) => binding.gateway_id)
}

async function setGatewaySelection(row, gatewayIDs) {
  const selected = new Set(gatewayIDs)
  try {
    for (const binding of row.bindings) {
      const enabled = selected.has(binding.gateway_id)
      if (binding.enabled !== enabled) await http.put(`/bindings/${binding.id}`, { enabled })
      selected.delete(binding.gateway_id)
    }
    let position = enabledBindings(row).length
    for (const gatewayID of selected) {
      await http.post('/bindings', { segment_id: row.id, gateway_id: gatewayID, enabled: true, position: position++ })
    }
    await http.post('/routes/sync')
    ElMessage.success('网关选择已保存')
    await load()
  } catch (e) {
    ElMessage.error(e.error || '保存网关选择失败')
    await load()
  }
}

function startDrag(segmentID, bindingID) {
  dragging.value = { segmentID, bindingID }
}

async function dropBinding(row, targetBindingID) {
  if (!dragging.value || dragging.value.segmentID !== row.id || dragging.value.bindingID === targetBindingID) return
  const ordered = enabledBindings(row)
  const from = ordered.findIndex((binding) => binding.id === dragging.value.bindingID)
  const to = ordered.findIndex((binding) => binding.id === targetBindingID)
  if (from < 0 || to < 0) return
  const [moved] = ordered.splice(from, 1)
  ordered.splice(to, 0, moved)
  dragging.value = null
  try {
    await Promise.all(ordered.map((binding, index) => http.put(`/bindings/${binding.id}`, { position: index })))
    await http.post('/routes/sync')
    ElMessage.success('优先级已更新')
    await load()
  } catch (e) {
    ElMessage.error(e.error || '优先级更新失败')
    await load()
  }
}

function openBatchSwitch() {
  batchGatewayId.value = null
  batchDialog.value = true
}

async function doBatchSwitch() {
  if (!batchGatewayId.value) {
    ElMessage.warning('请选择目标网关')
    return
  }
  batchLoading.value = true
  try {
    const r = await http.post('/segments/batch-switch', {
      segment_ids: selection.value.map((s) => s.id),
      gateway_id: batchGatewayId.value,
    })
    const failed = r.results.filter((x) => !x.ok)
    if (failed.length) {
      ElMessage.warning(`${r.results.length - failed.length} 成功，${failed.length} 失败：` + failed[0].error)
    } else {
      ElMessage.success(`已切换 ${r.results.length} 个网段`)
    }
    batchDialog.value = false
    await load()
  } catch (e) {
    ElMessage.error(e.error || '批量切换失败')
  } finally {
    batchLoading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.table-card { border-radius: 10px; }
.card-header, .actions { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
.card-title, .primary-cell { color: #303133; font-weight: 600; }
.card-title { font-size: 16px; }
.card-subtitle, .secondary-cell, .muted { margin-top: 4px; color: #909399; font-size: 12px; }
.priority-list { display: flex; gap: 6px; flex-wrap: wrap; }
.priority-item { cursor: grab; user-select: none; }
.drag-handle { color: #909399; margin-right: 3px; }
.form-tip { color: #909399; font-size: 12px; line-height: 1.5; }
code { font-family: Consolas, monospace; }
</style>
