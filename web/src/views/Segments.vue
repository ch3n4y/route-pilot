<template>
  <div>
    <div class="toolbar">
      <el-button type="primary" @click="openCreate">新增网段</el-button>
      <el-button
        type="warning"
        :disabled="!selection.length"
        @click="openBatchSwitch"
      >
        批量切换网关 ({{ selection.length }})
      </el-button>
      <el-button @click="load">刷新</el-button>
    </div>

    <el-table :data="items" @selection-change="(s) => (selection = s)" border>
      <el-table-column type="selection" width="40" />
      <el-table-column prop="name" label="名称" min-width="120" />
      <el-table-column prop="cidr" label="网段" min-width="130" />
      <el-table-column prop="netmask" label="掩码" width="120" />
      <el-table-column label="活动网关" min-width="220">
        <template #default="{ row }">
          <el-select
            v-model="row.active_gateway_id"
            placeholder="切换网关"
            size="small"
            :loading="switchingId === row.id"
            @change="(gw) => switchGateway(row, gw)"
          >
            <el-option
              v-for="b in row.bindings"
              :key="b.id"
              :value="b.gateway_id"
              :label="b.gateway_name || '网关#' + b.gateway_id"
            />
          </el-select>
        </template>
      </el-table-column>
      <el-table-column label="候选网关" min-width="140">
        <template #default="{ row }">
          <el-tag v-for="b in row.bindings.filter((x) => !x.is_active)" :key="b.id" size="small" style="margin-right: 4px">
            {{ b.gateway_name || '#' + b.gateway_id }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="description" label="备注" min-width="120" />
      <el-table-column label="启用" width="70" align="center">
        <template #default="{ row }">
          <el-switch :model-value="row.enabled" @change="(v) => toggleEnabled(row, v)" />
        </template>
      </el-table-column>
      <el-table-column label="操作" width="130" fixed="right">
        <template #default="{ row }">
          <el-button size="small" @click="openEdit(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="del(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 新增/编辑 -->
    <el-dialog v-model="dialog" :title="form.id ? '编辑网段' : '新增网段'" width="480px">
      <el-form label-width="80px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="网段" required>
          <el-input v-model="form.cidr" placeholder="如 10.0.0.0/8" />
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

const items = ref([])
const gateways = ref([])
const selection = ref([])
const dialog = ref(false)
const batchDialog = ref(false)
const saving = ref(false)
const batchLoading = ref(false)
const switchingId = ref(0)
const batchGatewayId = ref(null)
const form = ref({ id: 0, name: '', cidr: '', description: '' })

async function load() {
  const r = await http.get('/segments')
  items.value = r.items
  const g = await http.get('/gateways')
  gateways.value = g.items
}

function openCreate() {
  form.value = { id: 0, name: '', cidr: '', description: '' }
  dialog.value = true
}
function openEdit(row) {
  form.value = { id: row.id, name: row.name, cidr: row.cidr, description: row.description }
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
  await ElMessageBox.confirm(`确认删除网段「${row.name}」？`, '提示', { type: 'warning' })
  try {
    await http.delete(`/segments/${row.id}`)
    ElMessage.success('已删除')
    await load()
  } catch (e) {
    ElMessage.error(e.error || '删除失败')
  }
}

async function toggleEnabled(row, v) {
  try {
    await http.put(`/segments/${row.id}`, {
      name: row.name,
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

async function switchGateway(row, gwId) {
  if (!gwId) return
  switchingId.value = row.id
  try {
    const r = await http.post(`/segments/${row.id}/switch`, { gateway_id: gwId })
    ElMessage.success('已切换')
    await load()
  } catch (e) {
    ElMessage.error(e.error || '切换失败')
    await load()
  } finally {
    switchingId.value = 0
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
.toolbar { margin-bottom: 12px; }
</style>
