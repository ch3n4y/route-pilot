import { defineStore } from 'pinia'
import http from '../api'

// 本机网卡列表全局缓存：应用启动时预取一次，网关表单下拉即时可用，
// 避免每次打开弹窗都触发 PowerShell 探测导致界面空白等待。
export const useInterfacesStore = defineStore('interfaces', {
  state: () => ({
    interfaces: [],
    loaded: false,
  }),
  actions: {
    async load() {
      try {
        const r = await http.get('/network/interfaces')
        this.interfaces = r.interfaces
        this.loaded = true
      } catch (e) {
        this.loaded = false
      }
    },
  },
})
