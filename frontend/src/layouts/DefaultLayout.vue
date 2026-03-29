<template>
  <!-- Mobile app bar -->
  <v-app-bar v-if="!mdAndUp" density="compact" color="primary" flat>
    <v-app-bar-nav-icon @click="drawer = !drawer" />
    <v-app-bar-title class="text-body-1 font-weight-bold">Chat Quality Agent</v-app-bar-title>
  </v-app-bar>

  <v-navigation-drawer
    v-model="drawer"
    :rail="mdAndUp && rail"
    :permanent="mdAndUp"
    :temporary="!mdAndUp"
    :color="isDark ? undefined : 'white'"
    class="border-e"
  >
    <!-- Logo -->
    <v-list-item class="px-4 py-3">
      <v-list-item-title v-if="!isRail" class="text-subtitle-2 font-weight-bold text-primary" style="white-space: normal; line-height: 1.3">
        Chat Quality Agent
      </v-list-item-title>
      <v-list-item-title v-else class="text-caption font-weight-bold text-primary text-center">
        CQ
      </v-list-item-title>
      <template #append>
        <v-btn v-if="mdAndUp && !rail" icon="mdi-chevron-left" variant="text" size="small" @click="rail = true" />
      </template>
    </v-list-item>

    <v-divider />

    <!-- Company switcher -->
    <v-list-item v-if="tenantId && !isRail" class="px-4 py-1" density="compact">
      <v-select
        v-model="currentTenantId"
        :items="tenantSelectItems"
        item-title="name"
        item-value="id"
        density="compact"
        variant="outlined"
        hide-details
        class="text-body-2"
        @update:model-value="onTenantSelect"
      />
    </v-list-item>

    <!-- Create Tenant Dialog (from sidebar) -->
    <v-dialog v-model="sidebarCreateDialog" max-width="500">
      <v-card>
        <v-card-title>Thêm công ty</v-card-title>
        <v-card-text>
          <v-text-field v-model="sidebarForm.name" label="Tên công ty" class="mb-4" />
          <v-text-field v-model="sidebarForm.slug" label="Slug" hint="URL-friendly, vd: my-company" persistent-hint />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="sidebarCreateDialog = false">Hủy</v-btn>
          <v-btn color="primary" :loading="sidebarCreating" :disabled="!sidebarForm.name || !sidebarForm.slug" @click="createFromSidebar">Tạo</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
    <v-list-item v-if="tenantId && isRail" class="px-2 py-1" density="compact">
      <v-avatar size="28" color="secondary" style="cursor: pointer" @click="router.push('/')">
        <span class="text-white text-caption">{{ currentTenantInitial }}</span>
      </v-avatar>
    </v-list-item>

    <v-divider v-if="tenantId" class="mb-1" />

    <!-- Nav items -->
    <v-list density="comfortable" nav class="px-2">
      <v-list-item
        v-for="item in navItems"
        :key="item.route"
        :to="item.route"
        :exact="item.exact"
        :prepend-icon="item.icon"
        :title="$t(item.label)"
        rounded="lg"
        color="primary"
        class="mb-1"
        @click="onNavClick"
      />
    </v-list>

    <template #append>
      <v-divider />
      <div class="pa-3">
        <!-- Theme toggle -->
        <v-btn
          :icon="isDark ? 'mdi-weather-sunny' : 'mdi-weather-night'"
          variant="text"
          size="small"
          @click="toggleTheme"
        />

        <!-- Language -->
        <LanguageSwitcher v-if="!isRail" />

        <!-- Docs + Version -->
        <div v-if="!isRail" class="mt-2 d-flex align-center">
          <v-btn size="small" variant="text" href="https://tanviet12.github.io/chat-quality-agent/guide/introduction.html" target="_blank" prepend-icon="mdi-file-document" class="text-none">Docs</v-btn>
          <v-spacer />
          <v-chip size="small" variant="tonal" :color="updateInfo?.has_update ? 'warning' : 'success'" href="https://tanviet12.github.io/chat-quality-agent/changelog.html" target="_blank" style="cursor: pointer;">
            <v-icon start size="10" icon="mdi-circle" />
            {{ updateInfo?.current || 'dev' }}
          </v-chip>
        </div>

        <!-- User info with clickable avatar for profile -->
        <div v-if="!isRail" class="mt-2 d-flex align-center">
          <v-avatar size="32" color="primary" class="mr-2 cursor-pointer" style="cursor: pointer" @click="profileDialog = true">
            <span class="text-white text-caption">{{ userInitials }}</span>
          </v-avatar>
          <div class="text-body-2 text-truncate flex-grow-1 cursor-pointer" style="cursor: pointer" @click="profileDialog = true">
            {{ authStore.user?.name || authStore.user?.email }}
          </div>
          <v-btn icon="mdi-logout" variant="text" size="small" @click="handleLogout" />
        </div>

        <!-- Avatar when rail mode (clickable for profile) -->
        <v-avatar v-if="isRail" size="28" color="primary" class="mt-2" style="cursor: pointer" @click="profileDialog = true">
          <span class="text-white text-caption">{{ userInitials }}</span>
        </v-avatar>

        <!-- Expand rail -->
        <v-btn
          v-if="mdAndUp && rail"
          icon="mdi-chevron-right"
          variant="text"
          size="small"
          class="mt-2"
          @click="rail = false"
        />
      </div>
    </template>
  </v-navigation-drawer>

  <v-main class="bg-background">
    <v-container fluid class="pa-4 pa-md-6">
      <OnboardingWizard />

      <!-- Update notification banner -->
      <v-alert
        v-if="updateInfo && updateInfo.has_update && !isUpdateDismissed"
        type="info"
        variant="tonal"
        class="mb-4"
        closable
        @click:close="dismissUpdate"
      >
        <div class="d-flex align-center flex-wrap">
          <span class="text-body-2">Có phiên bản mới: <a href="https://tanviet12.github.io/chat-quality-agent/changelog.html" target="_blank" class="text-primary font-weight-bold">{{ updateInfo.latest }}</a></span>
          <span class="text-caption text-grey mx-2">|</span>
          <span class="text-caption text-grey">Hiện tại: {{ updateInfo.current }}</span>
          <span class="text-caption text-grey mx-2">|</span>
          <span class="text-caption"><a href="https://tanviet12.github.io/chat-quality-agent/guide/installation.html#tu-%C4%91ong-cap-nhat-tuy-chon" target="_blank" class="text-primary">Cài Watchtower</a> để tự động cập nhật.</span>
        </div>
        <div class="d-flex align-center mt-2 ga-1">
          <span class="text-caption text-grey">Cập nhật thủ công:</span>
          <code class="text-caption pa-1 rounded" style="user-select: all; background: #f5f5f5; color: #333; border: 1px solid #ddd;">cd /opt/cqa && docker compose pull && docker compose up -d</code>
          <v-btn icon="mdi-content-copy" size="x-small" variant="text" color="primary" @click="copyUpdateCmd" />
        </div>
      </v-alert>

      <slot />
    </v-container>
  </v-main>

  <!-- Profile Dialog -->
  <v-dialog v-model="profileDialog" max-width="500" persistent>
    <v-card>
      <v-card-title class="d-flex align-center">
        <v-icon start>mdi-account-circle</v-icon>
        {{ $t('user_profile') }}
        <v-spacer />
        <v-btn icon="mdi-close" variant="text" size="small" @click="profileDialog = false" />
      </v-card-title>

      <v-divider />

      <v-card-text>
        <!-- Display name -->
        <v-text-field
          v-model="profileForm.name"
          :label="$t('display_name')"
          prepend-inner-icon="mdi-account"
          class="mb-3"
        />

        <!-- Email (readonly) -->
        <v-text-field
          :model-value="authStore.user?.email"
          :label="$t('email')"
          prepend-inner-icon="mdi-email"
          readonly
          disabled
          class="mb-3"
        />

        <v-btn color="primary" :loading="savingProfile" @click="saveProfile" class="mb-6">
          {{ $t('save_settings') }}
        </v-btn>

        <v-divider class="mb-4" />

        <div class="text-subtitle-2 font-weight-bold mb-3">
          <v-icon start size="small">mdi-lock</v-icon>
          {{ $t('change_password') }}
        </div>

        <v-text-field
          v-model="passwordForm.currentPassword"
          :label="$t('current_password')"
          type="password"
          prepend-inner-icon="mdi-lock"
          class="mb-2"
        />
        <v-text-field
          v-model="passwordForm.newPassword"
          :label="$t('new_password')"
          type="password"
          prepend-inner-icon="mdi-lock-plus"
          class="mb-2"
        />
        <v-text-field
          v-model="passwordForm.confirmPassword"
          :label="$t('confirm_password')"
          type="password"
          prepend-inner-icon="mdi-lock-check"
          class="mb-3"
        />

        <v-btn color="primary" variant="outlined" :loading="changingPassword" @click="changePassword">
          {{ $t('change_password') }}
        </v-btn>
      </v-card-text>
    </v-card>
  </v-dialog>

  <v-snackbar v-model="snackbar" :color="snackColor" timeout="3000">{{ snackText }}</v-snackbar>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { clearTenantCache, permissionDeniedMsg, clearPermissionDeniedMsg } from '../router'
import { useTheme, useDisplay } from 'vuetify'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '../stores/auth'
import LanguageSwitcher from '../components/LanguageSwitcher.vue'
import OnboardingWizard from '../components/OnboardingWizard.vue'
import api from '../api'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const theme = useTheme()
const { mdAndUp } = useDisplay()
const { t } = useI18n()

const drawer = ref(mdAndUp.value)
const rail = ref(false)
const isDark = computed(() => theme.global.current.value.dark)
const isRail = computed(() => mdAndUp.value && rail.value)

const tenantId = computed(() => route.params.tenantId as string)

// Tenant switcher
interface TenantItem { id: string; name: string; slug: string }
const tenants = ref<TenantItem[]>([])
const currentTenantId = ref('')

const currentTenantInitial = computed(() => {
  const t = tenants.value.find(t => t.id === currentTenantId.value)
  return t ? t.name.charAt(0).toUpperCase() : '?'
})

// Show permission denied toast when redirected by router guard
watch(() => route.path, () => {
  if (permissionDeniedMsg) {
    showSnack(permissionDeniedMsg, 'error')
    clearPermissionDeniedMsg()
  }
})

watch(tenantId, async (id) => {
  if (id) {
    currentTenantId.value = id
    authStore.fetchTenantPermissions(id)
    if (!tenants.value.length) {
      try {
        const { data } = await api.get('/tenants')
        tenants.value = data
      } catch { /* ignore */ }
    }
  }
}, { immediate: true })

const CREATE_TENANT_ID = '__create__'
const MANAGE_TENANT_ID = '__manage__'

const tenantSelectItems = computed(() => {
  const items = [...tenants.value]
  if (authStore.user?.is_admin) {
    items.push({ id: CREATE_TENANT_ID, name: '+ Thêm công ty', slug: '' })
  }
  items.push({ id: MANAGE_TENANT_ID, name: 'Quản lý công ty', slug: '' })
  return items
})

function onTenantSelect(newId: string) {
  if (newId === CREATE_TENANT_ID) {
    currentTenantId.value = tenantId.value
    sidebarForm.value = { name: '', slug: '' }
    sidebarCreateDialog.value = true
    return
  }
  if (newId === MANAGE_TENANT_ID) {
    currentTenantId.value = tenantId.value
    router.push('/')
    return
  }
  if (newId && newId !== tenantId.value) {
    router.push(`/${newId}`)
  }
}

const sidebarCreateDialog = ref(false)
const sidebarCreating = ref(false)
const sidebarForm = ref({ name: '', slug: '' })

watch(() => sidebarForm.value.name, (name) => {
  sidebarForm.value.slug = name
    .toLowerCase()
    .normalize('NFD').replace(/[\u0300-\u036f]/g, '')
    .replace(/đ/g, 'd').replace(/Đ/g, 'd')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
})

async function createFromSidebar() {
  sidebarCreating.value = true
  try {
    const { data } = await api.post('/tenants', { name: sidebarForm.value.name, slug: sidebarForm.value.slug })
    tenants.value.push(data)
    sidebarCreateDialog.value = false
    router.push(`/${data.id}`)
  } catch { /* ignore */ } finally {
    sidebarCreating.value = false
  }
}

// Profile dialog
const profileDialog = ref(false)
const savingProfile = ref(false)
const changingPassword = ref(false)
const snackbar = ref(false)
const snackText = ref('')
const snackColor = ref('success')

const profileForm = ref({ name: '' })
const passwordForm = ref({ currentPassword: '', newPassword: '', confirmPassword: '' })

// Version update check
const updateInfo = ref<any>(null)
const isUpdateDismissed = computed(() => {
  if (!updateInfo.value?.latest) return true
  return localStorage.getItem('cqa_dismissed_version') === updateInfo.value.latest
})
function dismissUpdate() {
  if (updateInfo.value?.latest) localStorage.setItem('cqa_dismissed_version', updateInfo.value.latest)
}
function copyUpdateCmd() {
  navigator.clipboard.writeText('cd /opt/cqa && docker compose pull && docker compose up -d')
}
onMounted(async () => {
  // Check cached version info (max 1 hour)
  const cached = localStorage.getItem('cqa_version_check')
  if (cached) {
    try {
      const { data, ts } = JSON.parse(cached)
      if (Date.now() - ts < 3600000) { updateInfo.value = data; return }
    } catch { /* ignore */ }
  }
  try {
    const { data } = await api.get('/version/check')
    updateInfo.value = data
    localStorage.setItem('cqa_version_check', JSON.stringify({ data, ts: Date.now() }))
  } catch { /* ignore */ }
})

// Load profile data when dialog opens
watch(profileDialog, (val) => {
  if (val && authStore.user) {
    profileForm.value.name = authStore.user.name || ''
    passwordForm.value = { currentPassword: '', newPassword: '', confirmPassword: '' }
  }
})

const navItems = computed(() => {
  if (!tenantId.value) return []
  const base = `/${tenantId.value}`
  const all = [
    { icon: 'mdi-view-dashboard', label: 'nav_home', route: base, exact: true, perm: null },
    { icon: 'mdi-connection', label: 'nav_channels', route: `${base}/channels`, exact: false, perm: 'channels' },
    { icon: 'mdi-forum', label: 'nav_messages', route: `${base}/messages`, exact: false, perm: 'messages' },
    { icon: 'mdi-robot', label: 'nav_jobs', route: `${base}/jobs`, exact: false, perm: 'jobs' },
    { icon: 'mdi-text-box-search', label: 'activity_logs', route: `${base}/activity-logs`, exact: false, perm: 'settings' },
    { icon: 'mdi-currency-usd', label: 'cost_logs', route: `${base}/cost-logs`, exact: false, perm: 'settings' },
    { icon: 'mdi-bell-ring', label: 'nav_notification_logs', route: `${base}/notifications`, exact: false, perm: 'jobs' },
    { icon: 'mdi-api', label: 'nav_mcp', route: `${base}/mcp`, exact: false, perm: 'settings' },
    { icon: 'mdi-account-group', label: 'nav_users', route: `${base}/users`, exact: false, perm: 'settings' },
    { icon: 'mdi-cog', label: 'nav_settings', route: `${base}/settings`, exact: false, perm: 'settings' },
  ]
  return all.filter(item => !item.perm || authStore.canView(item.perm))
})

const userInitials = computed(() => {
  const name = authStore.user?.name || authStore.user?.email || '?'
  return name.charAt(0).toUpperCase()
})

function toggleTheme() {
  theme.global.name.value = isDark.value ? 'light' : 'dark'
}

function onNavClick() {
  // Close drawer on mobile after navigation
  if (!mdAndUp.value) {
    drawer.value = false
  }
}

async function handleLogout() {
  await authStore.logout()
  clearTenantCache()
  window.location.href = '/login'
}

function showSnack(text: string, color: string) {
  snackText.value = text
  snackColor.value = color
  snackbar.value = true
}

async function saveProfile() {
  if (!profileForm.value.name.trim()) {
    showSnack(t('validation_required'), 'error')
    return
  }
  savingProfile.value = true
  try {
    await authStore.updateProfile(profileForm.value.name.trim())
    showSnack(t('profile_updated'), 'success')
  } catch (err: any) {
    showSnack(err.response?.data?.error || t('error'), 'error')
  } finally {
    savingProfile.value = false
  }
}

async function changePassword() {
  if (passwordForm.value.newPassword !== passwordForm.value.confirmPassword) {
    showSnack(t('password_mismatch') || 'Passwords do not match', 'error')
    return
  }
  if (passwordForm.value.newPassword.length < 6) {
    showSnack(t('password_too_short') || 'Password must be at least 6 characters', 'error')
    return
  }
  changingPassword.value = true
  try {
    await api.put('/profile/password', {
      current_password: passwordForm.value.currentPassword,
      new_password: passwordForm.value.newPassword,
    })
    showSnack(t('success'), 'success')
    passwordForm.value = { currentPassword: '', newPassword: '', confirmPassword: '' }
  } catch (err: any) {
    const error = err.response?.data?.error
    if (error === 'wrong_current_password') {
      showSnack(t('wrong_password') || 'Wrong current password', 'error')
    } else {
      showSnack(error || t('error'), 'error')
    }
  } finally {
    changingPassword.value = false
  }
}
</script>
