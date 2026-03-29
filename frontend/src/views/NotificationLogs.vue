<template>
  <div>
    <h1 class="text-h5 font-weight-bold mb-6">{{ $t('nav_notification_logs') }}</h1>

    <v-card style="overflow-x: auto;">
      <v-table v-if="logs.length" density="compact">
        <thead>
          <tr>
            <th>{{ $t('sent_at') }}</th>
            <th>{{ $t('notification_channel') }}</th>
            <th>{{ $t('recipient') }}</th>
            <th>{{ $t('status') }}</th>
            <th>{{ $t('actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <template v-for="log in logs" :key="log.id">
            <tr>
              <td class="text-body-2">{{ new Date(log.sent_at).toLocaleString() }}</td>
              <td>
                <v-chip size="x-small" :color="log.channel_type === 'telegram' ? 'blue' : 'orange'" variant="tonal">
                  {{ log.channel_type }}
                </v-chip>
              </td>
              <td class="text-body-2">{{ log.recipient }}</td>
              <td>
                <v-chip size="x-small" :color="log.status === 'sent' ? 'success' : 'error'" variant="tonal">
                  {{ log.status === 'sent' ? 'Đã gửi' : 'Lỗi' }}
                </v-chip>
              </td>
              <td>
                <v-btn size="small" variant="text" color="primary" @click="expandedId = expandedId === log.id ? '' : log.id">
                  <v-icon start size="small">{{ expandedId === log.id ? 'mdi-chevron-up' : 'mdi-eye' }}</v-icon>
                  {{ expandedId === log.id ? 'Ẩn' : 'Xem' }}
                </v-btn>
              </td>
            </tr>
            <tr v-if="expandedId === log.id">
              <td colspan="5" class="bg-grey-lighten-5 pa-4">
                <div class="text-caption text-grey mb-2">Nội dung đã gửi:</div>
                <div class="text-body-2 pa-3 rounded" style="background: white; border: 1px solid #e0e0e0; white-space: pre-wrap;">{{ log.body }}</div>
                <div v-if="log.subject" class="text-caption text-grey mt-2">Tiêu đề: {{ log.subject }}</div>
                <div v-if="log.error_message" class="text-error text-body-2 mt-2">
                  Lỗi: {{ log.error_message }}
                </div>
              </td>
            </tr>
          </template>
        </tbody>
      </v-table>
      <v-card-actions v-if="totalPages > 1" class="justify-center">
        <v-pagination v-model="page" :length="totalPages" :total-visible="7" density="compact" />
      </v-card-actions>
      <div v-else-if="!logs.length" class="text-center pa-8">
        <v-icon size="48" color="grey-lighten-1" class="mb-3">mdi-bell-outline</v-icon>
        <div class="text-grey">Chưa có thông báo nào. Thông báo sẽ được ghi nhận khi công việc chạy và gửi kết quả qua Telegram/Email.</div>
      </div>
    </v-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import api from '../api'

const route = useRoute()
const tenantId = computed(() => route.params.tenantId as string)
const logs = ref<any[]>([])
const expandedId = ref('')
const page = ref(1)
const total = ref(0)
const perPage = 20
const totalPages = computed(() => Math.ceil(total.value / perPage))

async function loadLogs() {
  try {
    const { data } = await api.get(`/tenants/${tenantId.value}/notification-logs`, { params: { page: page.value, per_page: perPage } })
    logs.value = data.data || data || []
    total.value = data.total || 0
  } catch {
    // Not available yet
  }
}

onMounted(loadLogs)
watch(page, loadLogs)
</script>
