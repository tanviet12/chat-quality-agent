import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import api from '../api'

interface User {
  id: string
  email: string
  name: string
  is_admin: boolean
  language: string
}

interface TenantPermissions {
  role: string
  permissions: Record<string, string> // { channels: "rw", messages: "r", jobs: "", settings: "" }
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const accessToken = ref(localStorage.getItem('cqa_access_token') || '')
  const isAuthenticated = computed(() => !!accessToken.value)
  const tenantPerms = ref<TenantPermissions>({ role: '', permissions: {} })

  function canView(resource: string): boolean {
    const role = tenantPerms.value.role
    if (role === 'owner' || role === 'admin') return true
    const perm = tenantPerms.value.permissions[resource] || ''
    return perm.includes('r')
  }

  function canEdit(resource: string): boolean {
    const role = tenantPerms.value.role
    if (role === 'owner' || role === 'admin') return true
    const perm = tenantPerms.value.permissions[resource] || ''
    return perm.includes('w')
  }

  async function fetchTenantPermissions(tenantId: string) {
    try {
      const { data } = await api.get(`/tenants/${tenantId}/me`)
      let perms: Record<string, string> = {}
      if (data.permissions) {
        try { perms = JSON.parse(data.permissions) } catch { perms = {} }
      }
      tenantPerms.value = { role: data.role || '', permissions: perms }
    } catch {
      tenantPerms.value = { role: '', permissions: {} }
    }
  }

  async function login(email: string, password: string) {
    const { data } = await api.post('/auth/login', { email, password })
    accessToken.value = data.access_token
    localStorage.setItem('cqa_access_token', data.access_token)
    // Refresh token is now set as HttpOnly cookie by backend
    await fetchProfile()
  }

  async function register(name: string, email: string, password: string) {
    const { data } = await api.post('/auth/register', { name, email, password })
    accessToken.value = data.access_token
    localStorage.setItem('cqa_access_token', data.access_token)
    await fetchProfile()
  }

  async function fetchProfile() {
    const { data } = await api.get('/profile')
    user.value = data
  }

  async function updateProfile(name: string) {
    const { data } = await api.put('/profile', { name })
    if (user.value) {
      user.value.name = data.name || name
    }
  }

  async function logout() {
    try { await api.post('/auth/logout') } catch { /* ignore */ }
    user.value = null
    accessToken.value = ''
    localStorage.removeItem('cqa_access_token')
    localStorage.removeItem('cqa_refresh_token') // cleanup legacy
  }

  return { user, accessToken, isAuthenticated, tenantPerms, canView, canEdit, fetchTenantPermissions, login, register, fetchProfile, updateProfile, logout }
})
