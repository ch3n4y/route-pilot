import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const routes = [
  { path: '/login', component: () => import('../views/Login.vue'), meta: { public: true } },
  { path: '/setup', component: () => import('../views/Setup.vue'), meta: { public: true } },
  { path: '/', redirect: '/segments' },
  { path: '/segments', component: () => import('../views/Segments.vue'), meta: { title: '网段管理' } },
  { path: '/gateways', component: () => import('../views/Gateways.vue'), meta: { title: '网关管理' } },
  { path: '/matrix', component: () => import('../views/Matrix.vue'), meta: { title: '绑定矩阵' } },
  { path: '/routes', component: () => import('../views/Routes.vue'), meta: { title: '路由状态' } },
  { path: '/settings', component: () => import('../views/Settings.vue'), meta: { title: '设置' } },
]

const router = createRouter({ history: createWebHistory(), routes })

router.beforeEach(async (to) => {
  const a = useAuthStore()
  await a.init()
  if (to.meta.public) {
    if (a.token && to.path === '/login' && !a.needsSetup) return '/segments'
    return true
  }
  if (!a.token) return '/login'
  if (a.needsSetup) return '/setup'
  return true
})

export default router
