import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const routes = [
  { path: '/', redirect: '/segments' },
  { path: '/segments', component: () => import('../views/Segments.vue'), meta: { title: '网段管理' } },
  { path: '/gateways', component: () => import('../views/Gateways.vue'), meta: { title: '网关管理' } },
  { path: '/routes', component: () => import('../views/Routes.vue'), meta: { title: '路由状态' } },
  { path: '/settings', component: () => import('../views/Settings.vue'), meta: { title: '设置' } },
  { path: '/:pathMatch(.*)*', redirect: '/segments' },
]

const router = createRouter({ history: createWebHistory(), routes })

router.beforeEach(async () => {
  await useAuthStore().init()
  return true
})

export default router
