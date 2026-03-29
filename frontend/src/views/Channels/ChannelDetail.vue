<template>
  <div v-if="channel">
    <!-- Header -->
    <div class="d-flex align-center mb-4 flex-wrap ga-2">
      <v-btn icon="mdi-arrow-left" variant="text" size="small" @click="router.back()" class="mr-2" />
      <h1 class="text-h5 font-weight-bold">{{ channel.name }}</h1>
      <v-spacer />
      <template v-if="authStore.canEdit('channels')">
        <v-btn variant="outlined" prepend-icon="mdi-pencil" size="small" @click="editDialog = true">{{ $t('edit') }}</v-btn>
        <v-btn color="primary" prepend-icon="mdi-sync" size="small" :loading="syncing" @click="doSync">
          {{ $t('sync_now') || 'Dong bo ngay' }}
        </v-btn>
        <v-btn variant="outlined" prepend-icon="mdi-connection" size="small" :loading="testing" @click="doTest">
          Kiểm tra kết nối
        </v-btn>
        <v-btn color="warning" variant="outlined" prepend-icon="mdi-delete-sweep" size="small" @click="confirmPurge = true">
          Xóa cuộc chat
        </v-btn>
        <v-btn color="error" variant="outlined" prepend-icon="mdi-delete" size="small" @click="confirmDelete = true">{{ $t('delete') }}</v-btn>
      </template>
    </div>

    <!-- Channel Info -->
    <v-card class="pa-4 mb-4">
      <div class="text-subtitle-1 font-weight-bold mb-3">
        <v-icon start size="small">mdi-information</v-icon>
        Thông tin kênh
      </div>
      <v-row>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">Loại kênh</div>
          <v-chip size="small" :color="channel.channel_type === 'facebook' ? 'blue' : 'green'" variant="tonal">
            {{ channel.channel_type === 'facebook' ? 'Facebook' : 'Zalo OA' }}
          </v-chip>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">Trạng thái</div>
          <v-chip size="small" :color="channel.is_active ? 'success' : 'grey'" variant="tonal">
            {{ channel.is_active ? 'Hoạt động' : 'Tạm dừng' }}
          </v-chip>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">Trạng thái đồng bộ</div>
          <v-chip size="small" :color="channel.last_sync_status === 'success' ? 'success' : channel.last_sync_status === 'error' ? 'error' : 'grey'" variant="tonal">
            {{ channel.last_sync_status === 'success' ? 'Thành công' : channel.last_sync_status === 'error' ? 'Lỗi' : 'Chưa đồng bộ' }}
          </v-chip>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">Đồng bộ lần cuối</div>
          <div>{{ channel.last_sync_at ? formatDateTime(channel.last_sync_at) : 'Chưa đồng bộ' }}</div>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">Tổng cuộc chat</div>
          <a href="#" class="text-primary font-weight-bold" @click.prevent="goToMessages">
            {{ channel.conversation_count || 0 }}
          </a>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">Chu kỳ đồng bộ</div>
          <div>{{ formatSyncInterval(metadata.sync_interval) }}</div>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">Lưu file/ảnh</div>
          <v-chip size="small" :color="metadata.sync_files ? 'success' : 'grey'" variant="tonal">
            {{ metadata.sync_files ? 'Bật' : 'Tắt' }}
          </v-chip>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">Ngày tạo</div>
          <div>{{ formatDateTime(channel.created_at) }}</div>
        </v-col>
      </v-row>
    </v-card>

    <!-- Sync result alert -->
    <v-alert v-if="syncResult" :type="syncResult.type" closable class="mb-4" @click:close="syncResult = null">
      {{ syncResult.message }}
    </v-alert>

    <!-- Sync History -->
    <v-card class="pa-4">
      <div class="text-subtitle-1 font-weight-bold mb-3">
        <v-icon start size="small">mdi-history</v-icon>
        Lịch sử đồng bộ
      </div>
      <v-table density="compact" v-if="channelStore.syncHistory.length > 0">
        <thead>
          <tr>
            <th>Thời gian</th>
            <th>Trạng thái</th>
            <th>Chi tiết</th>
            <th>Lỗi</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="log in channelStore.syncHistory" :key="log.id">
            <td>{{ formatDateTime(log.created_at) }}</td>
            <td>
              <v-chip size="x-small" :color="log.action === 'sync.completed' ? 'success' : 'error'" variant="tonal">
                {{ log.action === 'sync.completed' ? 'Thành công' : 'Lỗi' }}
              </v-chip>
            </td>
            <td class="text-caption">{{ log.detail?.substring(0, 120) }}</td>
            <td class="text-caption text-error">{{ log.error_message }}</td>
          </tr>
        </tbody>
      </v-table>
      <div v-else class="text-center text-grey pa-4">Chưa có lịch sử đồng bộ</div>
      <v-pagination
        v-if="syncTotalPages > 1"
        v-model="syncPage"
        :length="syncTotalPages"
        :total-visible="5"
        density="compact"
        class="mt-3"
      />
    </v-card>

    <!-- Edit Dialog -->
    <v-dialog v-model="editDialog" max-width="500">
      <v-card>
        <v-card-title>Sửa kênh chat</v-card-title>
        <v-card-text>
          <v-text-field v-model="editForm.name" label="Tên kênh" density="compact" class="mb-2" />
          <v-switch v-model="editForm.is_active" label="Hoạt động" density="compact" color="primary" class="mb-2" />
          <v-select v-model="editForm.sync_interval" :items="syncIntervalOptions" label="Chu kỳ đồng bộ" density="compact" class="mb-2" />
          <v-switch v-model="editForm.sync_files" label="Lưu file/ảnh" density="compact" color="primary" />
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="editDialog = false">Hủy</v-btn>
          <v-btn color="primary" :loading="saving" @click="saveEdit">Lưu</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Purge Conversations Confirm -->
    <v-dialog v-model="confirmPurge" max-width="440">
      <v-card>
        <v-card-title>Xóa cuộc chat</v-card-title>
        <v-card-text>
          Xóa tất cả cuộc chat và tin nhắn của kênh <b>{{ channel.name }}</b>.
          Dữ liệu đánh giá QC liên quan cũng sẽ bị xóa. Bạn có thể đồng bộ lại sau khi xóa.
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="confirmPurge = false">Hủy</v-btn>
          <v-btn color="warning" :loading="purging" @click="doPurge">Xóa cuộc chat</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Delete Confirm -->
    <v-dialog v-model="confirmDelete" max-width="400">
      <v-card>
        <v-card-title>Xóa kênh chat</v-card-title>
        <v-card-text>Xóa kênh <b>{{ channel.name }}</b> sẽ xóa tất cả cuộc chat và tin nhắn liên quan. Không thể hoàn tác.</v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="confirmDelete = false">Hủy</v-btn>
          <v-btn color="error" :loading="deleting" @click="doDelete">Xóa</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
  <div v-else class="text-center pa-8">
    <v-progress-circular indeterminate />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useChannelStore } from '../../stores/channels'
import { useAuthStore } from '../../stores/auth'

const route = useRoute()
const router = useRouter()
const channelStore = useChannelStore()
const authStore = useAuthStore()

const tenantId = computed(() => route.params.tenantId as string)
const channelId = computed(() => route.params.channelId as string)
const channel = computed(() => channelStore.currentChannel)
const metadata = computed(() => {
  try { return JSON.parse(channel.value?.metadata || '{}') } catch { return {} }
})

const syncing = ref(false)
const testing = ref(false)
const saving = ref(false)
const deleting = ref(false)
const purging = ref(false)
const editDialog = ref(false)
const confirmDelete = ref(false)
const confirmPurge = ref(false)
const syncResult = ref<{ type: 'success' | 'warning' | 'error' | 'info'; message: string } | null>(null)
const syncPage = ref(1)
const syncTotalPages = computed(() => Math.ceil(channelStore.syncHistoryTotal / 10))

const editForm = ref({ name: '', is_active: true, sync_interval: 5, sync_files: false })

const syncIntervalOptions = [
  { title: '1 phút', value: 1 },
  { title: '5 phút', value: 5 },
  { title: '10 phút', value: 10 },
  { title: '15 phút', value: 15 },
  { title: '30 phút', value: 30 },
  { title: '1 giờ', value: 60 },
  { title: '6 giờ', value: 360 },
  { title: '1 ngày', value: 1440 },
]

function formatDateTime(d: string) {
  const dt = new Date(d)
  const dd = String(dt.getDate()).padStart(2, '0')
  const mm = String(dt.getMonth() + 1).padStart(2, '0')
  const hh = String(dt.getHours()).padStart(2, '0')
  const mi = String(dt.getMinutes()).padStart(2, '0')
  return `${dd}/${mm}/${dt.getFullYear()} ${hh}:${mi}`
}

function formatSyncInterval(mins: number) {
  if (!mins) return '5 phút'
  if (mins < 60) return `${mins} phút`
  if (mins < 1440) return `${mins / 60} giờ`
  return `${mins / 1440} ngày`
}

function goToMessages() {
  router.push(`/${tenantId.value}/messages?channel_id=${channelId.value}`)
}

async function doSync() {
  syncing.value = true
  syncResult.value = null
  try {
    await channelStore.syncChannel(tenantId.value, channelId.value)
    // Poll channel status until sync completes (max 3 minutes)
    let pollAttempts = 0
    const maxPollAttempts = 60
    while (pollAttempts < maxPollAttempts) {
      await new Promise(r => setTimeout(r, 3000))
      const ch = await channelStore.fetchChannel(tenantId.value, channelId.value)
      if (ch.last_sync_status !== 'syncing') break
      pollAttempts++
    }
    if (pollAttempts >= maxPollAttempts) {
      syncResult.value = { type: 'error', message: 'Đồng bộ quá lâu, vui lòng kiểm tra lại sau' }
      syncing.value = false
      return
    }
    await channelStore.fetchSyncHistory(tenantId.value, channelId.value, syncPage.value)
    const ch = channelStore.currentChannel
    if (ch?.last_sync_status === 'success') {
      syncResult.value = { type: 'success', message: 'Đồng bộ thành công' }
    } else {
      syncResult.value = { type: 'error', message: ch?.last_sync_error || 'Đồng bộ thất bại' }
    }
  } catch (err: any) {
    syncResult.value = { type: 'error', message: err.response?.data?.error || 'Đồng bộ thất bại' }
  } finally {
    syncing.value = false
  }
}

async function doTest() {
  testing.value = true
  try {
    const result = await channelStore.testConnection(tenantId.value, channelId.value)
    syncResult.value = { type: 'success', message: result.message || 'Kết nối thành công' }
  } catch (err: any) {
    syncResult.value = { type: 'error', message: err.response?.data?.error || 'Kết nối thất bại' }
  } finally {
    testing.value = false
  }
}

async function saveEdit() {
  saving.value = true
  try {
    await channelStore.updateChannel(tenantId.value, channelId.value, {
      name: editForm.value.name,
      is_active: editForm.value.is_active,
      metadata: JSON.stringify({ sync_interval: editForm.value.sync_interval, sync_files: editForm.value.sync_files }),
    })
    editDialog.value = false
    await channelStore.fetchChannel(tenantId.value, channelId.value)
  } finally {
    saving.value = false
  }
}

async function doPurge() {
  purging.value = true
  try {
    const result = await channelStore.purgeConversations(tenantId.value, channelId.value)
    confirmPurge.value = false
    syncResult.value = {
      type: 'success',
      message: `Đã xóa ${result.conversations_deleted} cuộc chat, ${result.messages_deleted} tin nhắn. Bạn có thể đồng bộ lại.`
    }
    // Refresh channel data to reflect reset sync state
    await channelStore.fetchChannel(tenantId.value, channelId.value)
  } catch {
    syncResult.value = { type: 'error', message: 'Xóa cuộc chat thất bại' }
  } finally {
    purging.value = false
  }
}

async function doDelete() {
  deleting.value = true
  try {
    await channelStore.deleteChannel(tenantId.value, channelId.value)
    router.push(`/${tenantId.value}/channels`)
  } finally {
    deleting.value = false
  }
}

watch(editDialog, (v) => {
  if (v && channel.value) {
    editForm.value = {
      name: channel.value.name,
      is_active: channel.value.is_active,
      sync_interval: metadata.value.sync_interval || 5,
      sync_files: metadata.value.sync_files || false,
    }
  }
})

watch(syncPage, (p) => {
  channelStore.fetchSyncHistory(tenantId.value, channelId.value, p)
})

onMounted(async () => {
  // Handle OAuth callback redirect
  const params = new URLSearchParams(window.location.search)
  if (params.get('zalo_auth') === 'success' || params.get('fb_auth') === 'success') {
    syncResult.value = { type: 'success', message: 'Xác thực thành công! Bấm "Đồng bộ ngay" để lấy tin nhắn.' }
    window.history.replaceState({}, '', window.location.pathname)
  } else if (params.get('zalo_auth') === 'error' || params.get('fb_auth') === 'error') {
    syncResult.value = { type: 'error', message: params.get('message') || 'Xác thực thất bại. Vui lòng thử lại.' }
    window.history.replaceState({}, '', window.location.pathname)
  }

  await channelStore.fetchChannel(tenantId.value, channelId.value)
  await channelStore.fetchSyncHistory(tenantId.value, channelId.value, 1)
})
</script>
