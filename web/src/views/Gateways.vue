<template>
  <div>
    <el-card shadow="never" class="table-card">
      <template #header>
        <div class="card-header">
          <div>
            <div class="card-title">网关列表</div>
            <div class="card-subtitle">网段管理页可直接选择这里的启用网关并自动绑定；新增时必须指定本机出口网卡</div>
          </div>
          <div class="actions">
            <el-button type="primary" @click="openCreate">新增网关</el-button>
            <el-button @click="load">刷新</el-button>
          </div>
        </div>
      </template>

      <div v-loading="loading" class="native-table-wrap">
        <table class="gateway-native-table">
          <colgroup>
            <col class="col-gateway" />
            <col class="col-interface" />
            <col class="col-bindings" />
            <col class="col-description" />
            <col class="col-status" />
            <col class="col-actions" />
          </colgroup>
          <thead>
            <tr>
              <th>网关</th>
              <th>出口接口</th>
              <th class="cell-center">已绑定网段</th>
              <th>备注</th>
              <th class="cell-center">状态</th>
              <th class="cell-center">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in items" :key="row.id">
              <td>
                <div class="primary-cell">{{ row.name }}</div>
                <div class="secondary-cell"><code>{{ row.gateway_ip }}</code></div>
              </td>
              <td>
                <span>{{ row.interface || '未指定' }}</span>
                <span v-if="row.ifindex" class="secondary-inline">#{{ row.ifindex }}</span>
              </td>
              <td class="cell-center">
                <el-tag :type="usedCount(row) ? 'primary' : 'info'" effect="plain" round>{{ usedCount(row) }}</el-tag>
              </td>
              <td class="text-ellipsis" :title="row.description">{{ row.description || '—' }}</td>
              <td class="cell-center">
                <el-switch :model-value="row.enabled" inline-prompt active-text="启" inactive-text="停" @change="(value) => toggleEnabled(row, value)" />
              </td>
              <td>
                <div class="operation-cell">
                  <el-button size="small" link type="primary" @click="openEdit(row)">编辑</el-button>
                  <el-button size="small" link type="danger" @click="del(row)">删除</el-button>
                </div>
              </td>
            </tr>
            <tr v-if="!loading && !items.length">
              <td colspan="6" class="empty-cell">暂无网关，请点击“新增网关”开始配置</td>
            </tr>
          </tbody>
        </table>
      </div>
    </el-card>

    <el-dialog v-model="dialog" :title="form.id ? '编辑网关' : '新增网关'" width="520px">
      <el-form label-width="90px">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="网关 IP" required>
          <el-input v-model="form.gateway_ip" placeholder="转发设备 IP（不能填写本机 IP）" />
        </el-form-item>
        <el-form-item label="出口接口" required>
          <el-select
            v-model="form.ifindex"
            placeholder="选择本机出口网卡（必须）"
            style="width: 100%"
            @change="onInterfaceChange"
          >
            <el-option
              v-for="iface in interfaceOptions"
              :key="iface.index"
              :value="iface.index"
              :label="interfaceLabel(iface)"
            />
          </el-select>
          <div v-if="interfacesStore.interfaces.length" class="form-tip">路由将从该网卡发往网关，请选择包含网关 IP 子网的网卡</div>
          <div v-else class="form-tip">未发现本机有 IPv4 的网卡，无法选择出口接口</div>
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
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import http from '../api'
import { useInterfacesStore } from '../stores/interfaces'

const interfacesStore = useInterfacesStore()

const items = ref([])
const loading = ref(false)
const dialog = ref(false)
const saving = ref(false)
const form = ref({ id: 0, name: '', gateway_ip: '', interface: '', ifindex: 0, description: '' })

// 下拉选项：本机接口；编辑旧数据时已选接口可能已不在当前列表，补一个选项避免选择框为空。
const interfaceOptions = computed(() => {
  const opts = interfacesStore.interfaces.map((iface) => ({ ...iface }))
  if (form.value.ifindex && !opts.some((o) => o.index === form.value.ifindex)) {
    opts.unshift({
      index: form.value.ifindex,
      name: form.value.interface || `接口 #${form.value.ifindex}`,
      ips: [],
      subnets: [],
      default_gateway: '',
    })
  }
  return opts
})

function usedCount(row) {
  return Array.isArray(row.used_by) ? row.used_by.length : 0
}

function interfaceLabel(iface) {
  const parts = [iface.name]
  if (iface.subnets && iface.subnets.length) parts.push(iface.subnets.join(', '))
  if (iface.ips && iface.ips.length) parts.push('IP ' + iface.ips.join(', '))
  if (iface.default_gateway) parts.push('默认网关 ' + iface.default_gateway)
  return parts.join(' · ')
}

// 默认出口：优先带默认网关的物理网卡，否则第一张有 IPv4 的网卡。
function defaultInterfaceIndex() {
  const list = interfacesStore.interfaces
  if (!list.length) return 0
  const withGw = list.find((iface) => iface.default_gateway)
  return (withGw || list[0]).index
}

function onInterfaceChange(idx) {
  const found = interfacesStore.interfaces.find((iface) => iface.index === idx)
  form.value.interface = found ? found.name : (form.value.interface || '')
}

async function load() {
  loading.value = true
  try {
    const r = await http.get('/gateways')
    items.value = r.items
  } catch (e) {
    ElMessage.error(e.error || '加载失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  // 网卡列表由应用启动时预取；若尚未返回，后台补拉一次，弹窗即时可用。
  if (!interfacesStore.loaded) interfacesStore.load()
  const defIdx = defaultInterfaceIndex()
  const def = interfacesStore.interfaces.find((iface) => iface.index === defIdx)
  form.value = {
    id: 0,
    name: '',
    gateway_ip: '',
    interface: def ? def.name : '',
    ifindex: defIdx,
    description: '',
  }
  dialog.value = true
}

function openEdit(row) {
  form.value = {
    id: row.id,
    name: row.name,
    gateway_ip: row.gateway_ip,
    interface: row.interface,
    ifindex: row.ifindex,
    description: row.description,
  }
  dialog.value = true
}

async function save() {
  if (!form.value.name.trim()) {
    ElMessage.warning('请填写名称')
    return
  }
  if (!form.value.gateway_ip.trim()) {
    ElMessage.warning('请填写网关 IP')
    return
  }
  if (!form.value.ifindex) {
    ElMessage.warning('请选择出口接口')
    return
  }
  if (!gatewayOnSelectedInterface(form.value.gateway_ip, form.value.ifindex)) {
    ElMessage.warning('网关 IP 不在所选出口接口的子网内，请选择能直达该网关的网卡')
    return
  }
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

// 所选接口子网必须包含网关 IP（Windows 下一跳要求 on-link）；无法判断时不拦截。
function gatewayOnSelectedInterface(ip, idx) {
  const iface = interfacesStore.interfaces.find((i) => i.index === idx)
  if (!iface || !iface.subnets || !iface.subnets.length) return true
  return iface.subnets.some((cidr) => ipInCidr(ip, cidr))
}

function ipInCidr(ip, cidr) {
  const [network, prefix] = cidr.split('/')
  const toInt = (s) => s.split('.').reduce((acc, o) => ((acc << 8) + Number(o)) >>> 0, 0)
  const bits = prefix ? Number(prefix) : 32
  const mask = bits === 0 ? 0 : (0xffffffff << (32 - bits)) >>> 0
  return (toInt(ip) & mask) === (toInt(network) & mask)
}

async function del(row) {
  try {
    await ElMessageBox.confirm(`确认删除网关「${row.name}」？`, '提示', { type: 'warning' })
    await http.delete(`/gateways/${row.id}`)
    ElMessage.success('已删除')
    await load()
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(e.error || '删除失败')
  }
}

async function toggleEnabled(row, v) {
  try {
    await http.put(`/gateways/${row.id}`, {
      name: row.name,
      gateway_ip: row.gateway_ip,
      interface: row.interface,
      ifindex: row.ifindex,
      description: row.description,
      enabled: v,
    })
    ElMessage.success(v ? '已启用' : '已禁用')
    await load()
  } catch (e) {
    ElMessage.error(e.error || '操作失败')
  }
}

onMounted(() => {
  load()
  if (!interfacesStore.loaded) interfacesStore.load()
})
</script>

<style scoped>
.table-card { border-radius: 10px; }
.card-header, .actions { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
.card-title, .primary-cell { color: #303133; font-weight: 600; }
.card-title { font-size: 16px; }
.card-subtitle, .secondary-cell { margin-top: 4px; color: #909399; font-size: 12px; }
.secondary-inline { margin-left: 6px; color: #909399; font-size: 12px; }
.form-tip { color: #909399; font-size: 12px; line-height: 1.5; margin-top: 2px; }
.native-table-wrap { width: 100%; overflow-x: auto; min-height: 120px; }
.gateway-native-table { width: 100%; min-width: 1000px; border-collapse: collapse; table-layout: fixed; color: #606266; }
.gateway-native-table th { height: 54px; padding: 0 16px; border-bottom: 1px solid #ebeef5; color: #909399; font-weight: 600; text-align: left; }
.gateway-native-table td { height: 72px; padding: 0 16px; border-bottom: 1px solid #ebeef5; vertical-align: middle; }
.gateway-native-table tbody tr:nth-child(even) { background: #fafafa; }
.gateway-native-table tbody tr:hover { background: #f5f7fa; }
.col-gateway { width: 22%; }
.col-interface { width: 19%; }
.col-bindings { width: 15%; }
.col-description { width: 24%; }
.col-status { width: 9%; }
.col-actions { width: 11%; }
.cell-center { text-align: center !important; }
.text-ellipsis { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.empty-cell { height: 120px !important; color: #909399; text-align: center; }
.operation-cell { display: flex; align-items: center; justify-content: center; white-space: nowrap; }
code { font-family: Consolas, monospace; }
</style>
