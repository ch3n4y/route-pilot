import axios from 'axios'

const http = axios.create({ baseURL: '/api', timeout: 15000 })

http.interceptors.response.use(
  (response) => response.data,
  (err) => Promise.reject(err.response?.data || {
    error: err.code === 'ECONNABORTED' ? '请求超时，请检查服务状态' : '无法连接服务，请稍后重试',
  }),
)

export default http
