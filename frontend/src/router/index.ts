import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import api from '../api'

// Permission denied message (set by guard, consumed by layout)
export let permissionDeniedMsg = ''
export function clearPermissionDeniedMsg() { permissionDeniedMsg = '' }

// Cache validated tenant IDs to avoid repeated API calls
const validTenantIds = new Set<string>()

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/setup',
      name: 'setup',
      component: () => import('../views/Setup.vue'),
      meta: { layout: 'auth', guest: true },
    },
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/Login.vue'),
      meta: { layout: 'auth', guest: true },
    },
    {
      path: '/',
      name: 'tenants',
      component: () => import('../views/Tenants.vue'),
      meta: { requiresAuth: true },
    },
    {
      path: '/:tenantId',
      meta: { requiresAuth: true, validateTenant: true },
      children: [
        {
          path: '',
          name: 'dashboard',
          component: () => import('../views/Dashboard.vue'),
        },
        {
          path: 'channels',
          name: 'channels',
          component: () => import('../views/Channels.vue'),
          meta: { perm: 'channels' },
        },
        {
          path: 'channels/:channelId',
          name: 'channel-detail',
          component: () => import('../views/Channels/ChannelDetail.vue'),
          meta: { perm: 'channels' },
        },
        {
          path: 'messages',
          name: 'messages',
          component: () => import('../views/Messages.vue'),
          meta: { perm: 'messages' },
        },
        {
          path: 'jobs',
          name: 'jobs',
          component: () => import('../views/Jobs/JobList.vue'),
          meta: { perm: 'jobs' },
        },
        {
          path: 'jobs/create',
          name: 'job-create',
          component: () => import('../views/Jobs/JobCreate.vue'),
          meta: { perm: 'jobs', permAction: 'w' },
        },
        {
          path: 'jobs/:jobId',
          name: 'job-detail',
          component: () => import('../views/Jobs/JobDetail.vue'),
          meta: { perm: 'jobs' },
        },
        {
          path: 'jobs/:jobId/edit',
          name: 'job-edit',
          component: () => import('../views/Jobs/JobEdit.vue'),
          meta: { perm: 'jobs', permAction: 'w' },
        },
        {
          path: 'activity-logs',
          name: 'activity-logs',
          component: () => import('../views/ActivityLogs.vue'),
          meta: { perm: 'settings' },
        },
        {
          path: 'cost-logs',
          name: 'cost-logs',
          component: () => import('../views/CostLogs.vue'),
          meta: { perm: 'settings' },
        },
        {
          path: 'notifications',
          name: 'notifications',
          component: () => import('../views/NotificationLogs.vue'),
          meta: { perm: 'jobs' },
        },
        {
          path: 'mcp',
          name: 'mcp',
          component: () => import('../views/MCPConnections.vue'),
          meta: { perm: 'settings' },
        },
        {
          path: 'users',
          name: 'users',
          component: () => import('../views/Users.vue'),
          meta: { perm: 'settings' },
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('../views/Settings.vue'),
          meta: { perm: 'settings' },
        },
      ],
    },
    // Catch-all 404
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('../views/NotFound.vue'),
    },
  ],
})

// Cache setup status to avoid repeated API calls
let setupChecked = false
let needsSetup = false

export function markSetupComplete() {
  needsSetup = false
}

router.beforeEach(async (to) => {
  // Check if initial setup is needed (only once per session)
  if (!setupChecked) {
    try {
      const { data } = await api.get('/setup/status')
      needsSetup = data.needs_setup
    } catch {
      needsSetup = false
    }
    setupChecked = true
  }

  // Redirect to setup if needed
  if (needsSetup && to.name !== 'setup') {
    return { name: 'setup' }
  }
  // Redirect away from setup if already completed
  if (!needsSetup && to.name === 'setup') {
    return { name: 'login' }
  }

  const token = localStorage.getItem('cqa_access_token')
  if (to.meta.requiresAuth && !token) {
    return { name: 'login' }
  }
  if (to.meta.guest && token) {
    return { path: '/' }
  }

  // Validate tenant exists
  if (to.meta.validateTenant && to.params.tenantId) {
    const tid = to.params.tenantId as string
    if (!validTenantIds.has(tid)) {
      try {
        await api.get(`/tenants/${tid}`)
        validTenantIds.add(tid)
      } catch {
        return { name: 'not-found' }
      }
    }

    // Check permission for tenant routes
    const perm = to.meta.perm as string | undefined
    if (perm) {
      const authStore = useAuthStore()
      // Ensure permissions are loaded
      if (!authStore.tenantPerms.role) {
        await authStore.fetchTenantPermissions(tid)
      }
      const action = (to.meta.permAction as string) || 'r'
      const allowed = action === 'w' ? authStore.canEdit(perm) : authStore.canView(perm)
      if (!allowed) {
        permissionDeniedMsg = 'Bạn không có quyền truy cập trang này'
        return { path: `/${tid}` }
      }
    }
  }
})

// Auto-reload when JS chunks are stale after deployment
router.onError((error) => {
  if (error.message.includes('dynamically imported module') || error.message.includes('Failed to fetch')) {
    window.location.reload()
  }
})

// Clear cache on logout
export function clearTenantCache() {
  validTenantIds.clear()
}

export default router
