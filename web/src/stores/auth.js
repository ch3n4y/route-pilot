import { defineStore } from 'pinia'
import http from '../api'

// 应用仅监听 127.0.0.1，无登录流程；该 store 只缓存管理员权限状态。
export const useAuthStore = defineStore('auth', {
  state: () => ({ elevated: false, initialized: false }),
  actions: {
    async init() {
      if (this.initialized) return
      try {
        const me = await http.get('/me')
        this.elevated = me.elevated
      } finally {
        this.initialized = true
      }
    },
  },
})
