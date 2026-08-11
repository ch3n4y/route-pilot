<template>
  <div>
    <div class="toolbar">
      <el-button @click="load">刷新</el-button>
      <span style="color: #909399; font-size: 13px">
        ● 活动网关　○ 候选网关　＋ 添加为候选
      </span>
    </div>

    <el-table :data="segments" border size="small" :table-layout="'auto'">
      <el-table-column prop="name" label="IP 段" min-width="150" fixed />
      <el-table-column prop="cidr" label="网段" min-width="130" fixed />
      <el-table-column
        v-for="g in gateways"
        :key="g.id"
        :label="g.name"
        align="center"
        min-width="100"
      >
        <template #default="{ row }">
          <span v-if="cell(row, g.id) === 'active'" class="dot dot-active">●</span>
          <span v-else-if="cell(row, g.id) === 'candidate'" class="dot dot-candidate">○</span>
          <el-button
            v-if="cell(row, g.id) === 'active' || cell(row, g.id) === 'candidate'"
            size="small"
            text
            type="primary"
            @click="setActive(row, g.id)"
          >
            设为活动
          </el-button>
          <el-button v-else size="small" circle text @click="addCandidate(row, g.id)">＋</el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import http from '../api'

const segments = ref([])
const gateways = ref([])

async function load() {
  const [s, g] = await Promise.all([http.get('/segments'), http.get('/gateways')])
  segments.value = s.items
  gateways.value = g.items
}

function cell(row, gwId) {
  const b = row.bindings.find((x) => x.gateway_id === gwId)
  if (!b) return null
  return b.is_active ? 'active' : 'candidate'
}

async function addCandidate(row, gwId) {
  try {
    await http.post('/bindings', { segment_id: row.id, gateway_id: gwId })
    ElMessage.success(`已将「${row.name}」加入网关候选`)
    await load()
  } catch (e) {
    ElMessage.error(e.error || '添加失败')
  }
}

async function setActive(row, gwId) {
  try {
    await http.post('/bindings/set-active', { segment_id: row.id, gateway_id: gwId })
    ElMessage.success(`已将「${row.name}」切换到活动网关`)
    await load()
  } catch (e) {
    ElMessage.error(e.error || '切换失败')
  }
}

onMounted(load)
</script>

<style scoped>
.toolbar { margin-bottom: 12px; }
.dot { margin-right: 6px; }
.dot-active { color: #67c23a; font-weight: 700; }
.dot-candidate { color: #e6a23c; }
</style>
