import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'

const http = axios.create({ baseURL: '/api' })

http.interceptors.request.use((cfg) => {
  const t = localStorage.getItem('token')
  if (t) cfg.headers.Authorization = `Bearer ${t}`
  return cfg
})

http.interceptors.response.use(
  (r) => r.data,
  (err) => {
    const data = err.response?.data || { error: '网络错误' }
    if (err.response?.status === 401) {
      localStorage.removeItem('token')
      if (router.currentRoute.value.path !== '/login' && router.currentRoute.value.path !== '/setup') {
        ElMessage.error(data.error || '登录已过期')
        router.push('/login')
      }
    }
    return Promise.reject(data)
  }
)

export default http
