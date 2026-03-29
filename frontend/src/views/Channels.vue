<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-6">
      <h1 class="text-h5 font-weight-bold">{{ $t('channels') }}</h1>
      <v-btn v-if="authStore.canEdit('channels')" color="primary" prepend-icon="mdi-plus" @click="showDialog = true">
        {{ $t('connect_channel') }}
      </v-btn>
    </div>

    <v-row>
      <v-col v-for="ch in channelStore.channels" :key="ch.id" cols="12" sm="6" md="4">
        <v-card class="pa-4" style="cursor: pointer" @click="router.push(`/${tenantId}/channels/${ch.id}`)">
          <div class="d-flex align-center mb-3">
            <v-icon :color="ch.channel_type === 'zalo_oa' ? 'blue' : 'indigo'" size="32" class="mr-3">
              {{ ch.channel_type === 'zalo_oa' ? 'mdi-message-text' : 'mdi-facebook-messenger' }}
            </v-icon>
            <div class="flex-grow-1">
              <div class="text-subtitle-1 font-weight-bold">{{ ch.name }}</div>
              <v-chip size="x-small" :color="ch.channel_type === 'zalo_oa' ? 'blue' : 'indigo'" variant="tonal">
                {{ ch.channel_type === 'zalo_oa' ? $t('channel_zalo') : $t('channel_facebook') }}
              </v-chip>
              <div v-if="ch.channel_type === 'zalo_oa' && ch.external_id" class="text-caption text-grey mt-1" style="white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">
                OA: {{ ch.external_id }}
              </div>
            </div>
            <div class="text-right">
              <div class="text-h6 font-weight-bold">{{ ch.conversation_count || 0 }}</div>
              <div class="text-caption text-grey">cuộc chat</div>
            </div>
          </div>

          <div class="d-flex align-center justify-space-between mb-2">
            <span class="text-caption text-grey">{{ $t('status') }}</span>
            <v-chip size="x-small" :color="ch.is_active ? 'success' : 'grey'" variant="tonal">
              {{ ch.is_active ? $t('active') : $t('inactive') }}
            </v-chip>
          </div>
          <div class="d-flex align-center justify-space-between mb-2">
            <span class="text-caption text-grey">{{ $t('sync_status') }}</span>
            <v-chip size="x-small" :color="syncColor(ch.last_sync_status)" variant="tonal">
              {{ ch.last_sync_status || '—' }}
            </v-chip>
          </div>
          <div class="d-flex align-center justify-space-between mb-3">
            <span class="text-caption text-grey">{{ $t('last_sync') }}</span>
            <span class="text-body-2">{{ ch.last_sync_at ? new Date(ch.last_sync_at).toLocaleString() : '—' }}</span>
          </div>

          <v-divider class="mb-3" />
          <div class="d-flex ga-2 flex-wrap" @click.stop>
            <v-btn size="small" variant="tonal" color="primary" prepend-icon="mdi-sync" :loading="syncing === ch.id" @click="syncNow(ch.id)">
              {{ $t('sync_now') }}
            </v-btn>
            <v-btn v-if="ch.last_sync_status === 'error'" size="small" variant="tonal" color="warning" prepend-icon="mdi-link-variant" :loading="reauthing === ch.id" @click="reauthChannel(ch.id)">
              Kết nối lại
            </v-btn>
            <v-btn size="small" variant="text" color="primary" @click="testConn(ch.id)">
              {{ $t('test_connection') }}
            </v-btn>
            <v-spacer />
            <v-btn icon="mdi-pencil" size="small" variant="text" color="primary" @click="openEdit(ch)" />
            <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="remove(ch.id)" />
          </div>
        </v-card>
      </v-col>
    </v-row>

    <div v-if="!channelStore.channels.length" class="text-center mt-12 pa-8">
      <v-icon size="64" color="grey-lighten-1" class="mb-4">mdi-chat-plus</v-icon>
      <div class="text-h6 text-grey-darken-1 mb-2">Chưa có kênh chat nào</div>
      <div class="text-body-2 text-grey mb-4" style="max-width: 500px; margin: 0 auto;">
        Kết nối kênh chat Facebook, Zalo OA để hệ thống đồng bộ tin nhắn và AI phân tích chất lượng CSKH.
      </div>
      <v-btn color="primary" prepend-icon="mdi-plus" @click="showDialog = true">Kết nối kênh</v-btn>
    </div>

    <!-- Connect Channel Dialog -->
    <v-dialog v-model="showDialog" max-width="560">
      <v-card class="pa-6">
        <v-card-title>{{ $t('connect_channel') }}</v-card-title>
        <v-select
          v-model="newChannel.channel_type"
          :label="$t('channel_type')"
          :items="[{ title: $t('channel_zalo'), value: 'zalo_oa' }, { title: $t('channel_facebook'), value: 'facebook' }]"
          class="mb-3"
        />
        <v-text-field v-model="newChannel.name" :label="$t('channel_name')" class="mb-3" />

        <!-- Zalo OA -->
        <template v-if="newChannel.channel_type === 'zalo_oa'">
          <v-btn variant="tonal" color="info" prepend-icon="mdi-book-open-variant" href="https://tanviet12.github.io/chat-quality-agent/usage/channels.html#zalo-oa" target="_blank" class="mb-3">
            Hướng dẫn lấy App ID và Secret Key
          </v-btn>
          <v-text-field v-model="newChannel.creds.app_id" :label="$t('zalo_app_id')" density="compact" class="mb-2" hint="Lấy từ Cài đặt ứng dụng trên Zalo Developers" persistent-hint />
          <v-text-field v-model="newChannel.creds.app_secret" :label="$t('zalo_app_secret')" type="password" density="compact" class="mb-2" />
          <div class="text-caption text-grey-darken-1 mb-2">
            <v-icon size="14" class="mr-1">mdi-information-outline</v-icon>
            Nếu ứng dụng Zalo có nhiều OA, bước tiếp theo sẽ mở trang Zalo để chọn OA — hãy chọn <b>đúng OA</b> tương ứng với kênh này.
          </div>
        </template>

        <!-- Facebook -->
        <template v-else>
          <v-btn variant="tonal" color="info" prepend-icon="mdi-book-open-variant" href="https://tanviet12.github.io/chat-quality-agent/usage/facebook.html" target="_blank" class="mb-3">
            Hướng dẫn kết nối Facebook Fanpage
          </v-btn>
          <v-text-field v-model="newChannel.creds.page_id" :label="$t('fb_page_id')" density="compact" class="mb-2" hint="Page ID từ Cài đặt trang Facebook" persistent-hint />
          <v-text-field v-model="newChannel.creds.access_token" :label="$t('fb_access_token')" density="compact" class="mb-2" hint="Page Access Token (nên dùng long-lived token)" persistent-hint />
        </template>

        <!-- Sync settings -->
        <v-divider class="my-3" />
        <v-select
          v-model="newChannel.sync_interval"
          :items="syncIntervalOptions"
          label="Chu kỳ đồng bộ"
          density="compact"
          class="mb-3"
          hint="Khoảng thời gian giữa mỗi lần tự động đồng bộ tin nhắn"
          persistent-hint
        />
        <v-alert v-if="newChannel.sync_interval <= 5" type="warning" variant="tonal" density="compact" class="mb-3">
          Đồng bộ quá thường xuyên có thể bị giới hạn bởi API của nền tảng.
        </v-alert>
        <v-switch
          v-model="newChannel.sync_files"
          label="Lưu trữ file/ảnh từ cuộc chat"
          hint="Tải và lưu file, ảnh từ cuộc chat lên server. Tăng dung lượng lưu trữ."
          persistent-hint
          density="compact"
          color="primary"
        />

        <v-card-actions class="mt-4 px-0">
          <v-spacer />
          <v-btn variant="text" @click="showDialog = false">{{ $t('cancel') }}</v-btn>
          <v-btn
            v-if="newChannel.channel_type === 'zalo_oa'"
            color="blue"
            :loading="creating"
            :disabled="!newChannel.name || !newChannel.creds.app_id || !newChannel.creds.app_secret"
            @click="createAndAuthZalo"
          >
            {{ $t('zalo_authorize') }}
          </v-btn>
          <v-btn
            v-else
            color="indigo"
            :loading="creating"
            :disabled="!newChannel.name || !newChannel.creds.page_id || !newChannel.creds.access_token"
            @click="createFacebook"
          >
            {{ $t('create') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Edit Channel Dialog -->
    <v-dialog v-model="editDialog" max-width="500">
      <v-card class="pa-6">
        <v-card-title>Sửa kênh chat</v-card-title>
        <v-text-field v-model="editForm.name" label="Tên kênh" class="mb-3" />
        <v-switch v-model="editForm.is_active" label="Hoạt động" color="primary" density="compact" class="mb-3" />
        <v-select
          v-model="editForm.sync_interval"
          :items="syncIntervalOptions"
          label="Chu kỳ đồng bộ"
          density="compact"
          class="mb-3"
          hint="Khoảng thời gian giữa mỗi lần tự động đồng bộ tin nhắn"
          persistent-hint
        />
        <v-alert v-if="editForm.sync_interval <= 5" type="warning" variant="tonal" density="compact" class="mb-3">
          Đồng bộ quá thường xuyên có thể bị giới hạn bởi API của nền tảng (Facebook/Zalo).
        </v-alert>
        <v-switch v-model="editForm.sync_files" label="Lưu trữ file/ảnh từ cuộc chat" color="primary" density="compact" hint="Tải và lưu file, ảnh từ cuộc chat lên server." persistent-hint />
        <v-card-actions class="mt-4 px-0">
          <v-spacer />
          <v-btn variant="text" @click="editDialog = false">{{ $t('cancel') }}</v-btn>
          <v-btn color="primary" :loading="savingEdit" @click="saveEdit">{{ $t('save_settings') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Snackbar -->
    <v-snackbar v-model="snackbar" :color="snackColor" timeout="3000">{{ snackText }}</v-snackbar>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, reactive, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useChannelStore } from '../stores/channels'
import { useAuthStore } from '../stores/auth'
import api from '../api'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const channelStore = useChannelStore()
const authStore = useAuthStore()
const tenantId = computed(() => route.params.tenantId as string)

const showDialog = ref(false)
const creating = ref(false)
const syncing = ref('')
const reauthing = ref('')
const snackbar = ref(false)
const snackText = ref('')
const snackColor = ref('success')

const newChannel = reactive({
  channel_type: 'zalo_oa',
  name: '',
  creds: {} as Record<string, string>,
  sync_files: false,
  sync_interval: 15,
})

onMounted(() => {
  channelStore.fetchChannels(tenantId.value)

  // Check if returning from OAuth callback
  const params = new URLSearchParams(window.location.search)
  if (params.get('zalo_auth') === 'success') {
    showSnack(t('zalo_auth_success'), 'success')
    channelStore.fetchChannels(tenantId.value)
    window.history.replaceState({}, '', window.location.pathname)
  } else if (params.get('zalo_auth') === 'error') {
    showSnack(params.get('message') || t('zalo_auth_failed'), 'error')
    window.history.replaceState({}, '', window.location.pathname)
  }
})

async function createAndAuthZalo() {
  creating.value = true
  try {
    // Step 1: Create channel with app_id + app_secret (no tokens yet)
    const created = await channelStore.createChannel(tenantId.value, {
      channel_type: newChannel.channel_type,
      name: newChannel.name,
      credentials: {
        app_id: newChannel.creds.app_id,
        app_secret: newChannel.creds.app_secret,
      },
      metadata: JSON.stringify({ sync_files: newChannel.sync_files, sync_interval: newChannel.sync_interval }),
    })
    showDialog.value = false

    // Step 2: Use backend reauth API to get signed OAuth URL (with HMAC state)
    const channelId = created?.id || created?.data?.id
    if (channelId && newChannel.channel_type === 'zalo_oa') {
      try {
        const { data: reauthData } = await api.post(`/tenants/${tenantId.value}/channels/${channelId}/reauth`)
        const redirectUrl = reauthData?.redirect_url
        if (redirectUrl) {
          window.location.href = redirectUrl
          return
        }
      } catch {
        // Fallback: redirect to channel detail
        router.push(`/${tenantId.value}/channels/${channelId}`)
        return
      }
    }
    showSnack(t('success'), 'success')
    channelStore.fetchChannels(tenantId.value)
    if (channelId) {
      router.push(`/${tenantId.value}/channels/${channelId}`)
    }

    newChannel.name = ''
    newChannel.creds = {}
  } catch {
    showSnack(t('error'), 'error')
  } finally {
    creating.value = false
  }
}

async function createFacebook() {
  creating.value = true
  try {
    await channelStore.createChannel(tenantId.value, {
      channel_type: newChannel.channel_type,
      name: newChannel.name,
      credentials: {
        page_id: newChannel.creds.page_id,
        access_token: newChannel.creds.access_token,
      },
      metadata: JSON.stringify({ sync_files: newChannel.sync_files, sync_interval: newChannel.sync_interval }),
    })
    showDialog.value = false
    newChannel.name = ''
    newChannel.creds = {}
    showSnack(t('success'), 'success')
    await channelStore.fetchChannels(tenantId.value)
  } catch {
    showSnack(t('error'), 'error')
  } finally {
    creating.value = false
  }
}


async function syncNow(channelId: string) {
  syncing.value = channelId
  try {
    const data = await channelStore.syncChannel(tenantId.value, channelId)
    const msg = data?.conversations_synced != null
      ? `${t('sync_now')}: ${data.conversations_synced} conversations, ${data.messages_synced} messages`
      : t('success')
    showSnack(msg, 'success')
    await channelStore.fetchChannels(tenantId.value)
  } catch (e: any) {
    showSnack(e?.response?.data?.error || t('error'), 'error')
  } finally {
    syncing.value = ''
  }
}

async function reauthChannel(channelId: string) {
  reauthing.value = channelId
  try {
    const { data } = await api.post(`/tenants/${tenantId.value}/channels/${channelId}/reauth`)
    if (data.redirect_url) {
      window.location.href = data.redirect_url
    }
  } catch (e: any) {
    showSnack(e?.response?.data?.error || 'Lỗi kết nối lại', 'error')
  } finally {
    reauthing.value = ''
  }
}

async function testConn(channelId: string) {
  try {
    await channelStore.testConnection(tenantId.value, channelId)
    showSnack(t('connection_ok'), 'success')
  } catch {
    showSnack(t('connection_failed'), 'error')
  }
}

async function remove(channelId: string) {
  if (confirm('Delete this channel?')) {
    await channelStore.deleteChannel(tenantId.value, channelId)
  }
}

function syncColor(status: string) {
  if (status === 'success') return 'success'
  if (status === 'error') return 'error'
  return 'grey'
}

function showSnack(text: string, color: string) {
  snackText.value = text
  snackColor.value = color
  snackbar.value = true
}



// Edit channel
const editDialog = ref(false)
const savingEdit = ref(false)
const editChannelId = ref('')
const editForm = reactive({ name: '', is_active: true, sync_files: false, sync_interval: 15 })
const syncIntervalOptions = [
  { title: 'Mỗi 1 phút', value: 1 },
  { title: 'Mỗi 5 phút', value: 5 },
  { title: 'Mỗi 10 phút', value: 10 },
  { title: 'Mỗi 15 phút (mặc định)', value: 15 },
  { title: 'Mỗi 30 phút', value: 30 },
  { title: 'Mỗi 1 giờ', value: 60 },
  { title: 'Mỗi 6 giờ', value: 360 },
  { title: 'Mỗi ngày', value: 1440 },
]

function openEdit(ch: any) {
  editChannelId.value = ch.id
  editForm.name = ch.name
  editForm.is_active = ch.is_active
  try {
    const meta = JSON.parse(ch.metadata || '{}')
    editForm.sync_files = meta.sync_files || false
    editForm.sync_interval = meta.sync_interval || 15
  } catch {
    editForm.sync_files = false
    editForm.sync_interval = 15
  }
  editDialog.value = true
}

async function saveEdit() {
  savingEdit.value = true
  try {
    await channelStore.updateChannel(tenantId.value, editChannelId.value, {
      name: editForm.name,
      is_active: editForm.is_active,
      metadata: JSON.stringify({ sync_files: editForm.sync_files, sync_interval: editForm.sync_interval }),
    })
    editDialog.value = false
    showSnack(t('success'), 'success')
    channelStore.fetchChannels(tenantId.value)
  } catch {
    showSnack(t('error'), 'error')
  } finally {
    savingEdit.value = false
  }
}
</script>
