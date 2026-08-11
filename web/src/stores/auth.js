import { defineStore } from 'pinia'
import http from '../api'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('token') || '',
    needsSetup: false,
    elevated: false,
    passwordSet: false,
  }),
  actions: {
    async init() {
      const st = await http.get('/setup/status')
      this.needsSetup = st.needs_setup
      if (this.token) {
        try {
          const me = await http.get('/me')
          this.elevated = me.elevated
          this.passwordSet = me.password_set
        } catch (e) {
          // token 失效时拦截器已清理
        }
      }
    },
    async login(pw) {
      const r = await http.post('/login', { password: pw })
      this.token = r.token
      localStorage.setItem('token', r.token)
    },
    async setup(pw) {
      const r = await http.post('/setup', { password: pw })
      this.token = r.token
      this.needsSetup = false
      localStorage.setItem('token', r.token)
    },
    async logout() {
      try {
        await http.post('/logout')
      } catch (e) {
        /* ignore */
      }
      this.token = ''
      localStorage.removeItem('token')
    },
  },
})
