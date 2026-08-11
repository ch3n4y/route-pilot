<template>
  <div>
    <div class="toolbar">
      <el-button type="primary" @click="openCreate">新增网关</el-button>
      <el-button @click="load">刷新</el-button>
    </div>

    <el-table :data="items" border>
      <el-table-column prop="name" label="名称" min-width="120" />
      <el-table-column prop="gateway_ip" label="网关 IP" min-width="130" />
      <el-table-column prop="interface" label="接口" min-width="140" />
      <el-table-column prop="metric" label="跃点数" width="80" align="center" />
      <el-table-column label="服务网段数" width="110" align="center">
        <template #default="{ row }">{{ row.used_by.length }}</template>
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

    <el-dialog v-model="dialog" :title="form.id ? '编辑网关' : '新增网关'" width="520px">
      <el-form label-width="90px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="网关 IP" required>
          <el-input v-model="form.gateway_ip" placeholder="局域网内转发设备 IP，如 192.168.1.2">
            <template #append>
              <el-button @click="loadSuggestions">本机子网建议</el-button>
            </template>
          </el-input>
        </el-form-item>
        <el-form-item v-if="suggestions.length" label="候选">
          <el-select v-model="pickedSuggestion" placeholder="从本机子网中选择" @change="applySuggestion">
            <el-option
              v-for="(iface, i) in suggestions"
              :key="i"
              :value="i"
              :label="iface.name + ' · ' + iface.ips.join(', ')"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="接口">
          <el-input v-model="form.interface" placeholder="可选，适配器名" />
        </el-form-item>
        <el-form-item label="跃点数">
          <el-input-number v-model="form.metric" :min="1" :max="9999" />
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
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import http from '../api'

const items = ref([])
const dialog = ref(false)
const saving = ref(false)
const suggestions = ref([])
const pickedSuggestion = ref(null)
const form = ref({ id: 0, name: '', gateway_ip: '', interface: '', metric: 1, description: '' })

async function load() {
  const r = await http.get('/gateways')
  items.value = r.items
}

async function loadSuggestions() {
  try {
    const r = await http.get('/network/interfaces')
    suggestions.value = r.interfaces
    if (!r.interfaces.length) ElMessage.info('未发现本机有 IPv4 的网卡')
  } catch (e) {
    ElMessage.error(e.error || '探测失败')
  }
}

function applySuggestion(idx) {
  const s = suggestions.value[idx]
  if (s.ips[0]) form.value.gateway_ip = s.ips[0]
  if (s.name) form.value.interface = s.name
}

function openCreate() {
  form.value = { id: 0, name: '', gateway_ip: '', interface: '', metric: 1, description: '' }
  suggestions.value = []
  dialog.value = true
}

function openEdit(row) {
  form.value = {
    id: row.id,
    name: row.name,
    gateway_ip: row.gateway_ip,
    interface: row.interface,
    metric: row.metric,
    description: row.description,
  }
  suggestions.value = []
  dialog.value = true
}

async function save() {
  saving.value = true
  try {
    if (form.value.id) {
      await http.put(`/gateways/${form.value.id}`, form.value)
    } else {
      await http.post('/gateways', form.value)
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
  await ElMessageBox.confirm(`确认删除网关「${row.name}」？`, '提示', { type: 'warning' })
  try {
    await http.delete(`/gateways/${row.id}`)
    ElMessage.success('已删除')
    await load()
  } catch (e) {
    ElMessage.error(e.error || '删除失败')
  }
}

async function toggleEnabled(row, v) {
  try {
    await http.put(`/gateways/${row.id}`, {
      name: row.name,
      gateway_ip: row.gateway_ip,
      interface: row.interface,
      metric: row.metric,
      description: row.description,
      enabled: v,
    })
    ElMessage.success(v ? '已启用' : '已禁用')
    await load()
  } catch (e) {
    ElMessage.error(e.error || '操作失败')
  }
}

onMounted(load)
</script>

<style scoped>
.toolbar { margin-bottom: 12px; }
</style>
