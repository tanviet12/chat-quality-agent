<template>
  <div>
    <h1 class="text-h5 font-weight-bold mb-6">{{ $t('activity_logs') }}</h1>

    <v-card style="overflow-x: auto;">
      <v-card-text class="d-flex ga-3 pb-0">
        <v-select
          v-model="filterAction"
          :items="actionOptions"
          :label="$t('filter')"
          density="compact"
          clearable
          style="max-width: 200px"
          @update:model-value="loadLogs"
        />
      </v-card-text>

      <v-table density="compact">
        <thead>
          <tr>
            <th>{{ $t('sent_at') }}</th>
            <th>{{ $t('action') }}</th>
            <th>{{ $t('user') }}</th>
            <th>{{ $t('detail') }}</th>
            <th>{{ $t('error') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="log in logs" :key="log.id">
            <td class="text-caption">{{ formatTime(log.created_at) }}</td>
            <td>
              <v-chip size="x-small" :color="actionColor(log.action)" variant="tonal">{{ log.action }}</v-chip>
            </td>
            <td class="text-body-2">{{ log.user_email || 'system' }}</td>
            <td class="text-body-2" style="max-width: 400px">{{ log.detail?.substring(0, 120) }}</td>
            <td>
              <v-chip v-if="log.error_message" size="x-small" color="error" variant="tonal">{{ log.error_message.substring(0, 80) }}</v-chip>
            </td>
          </tr>
        </tbody>
      </v-table>

      <v-card-actions v-if="totalPages > 1" class="justify-center">
        <v-pagination v-model="page" :length="totalPages" density="compact" />
      </v-card-actions>
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
const page = ref(1)
const total = ref(0)
const perPage = 20
const filterAction = ref('')
const totalPages = computed(() => Math.ceil(total.value / perPage))

const actionOptions = [
  { title: 'Job Run', value: 'job.run' },
  { title: 'Job Create', value: 'job.create' },
  { title: 'Job Delete', value: 'job.delete' },
  { title: 'AI Error', value: 'ai.error' },
  { title: 'Notification', value: 'notification' },
  { title: 'Settings', value: 'settings' },
]

onMounted(() => loadLogs())
watch(page, () => loadLogs())

async function loadLogs() {
  const params: Record<string, any> = { page: page.value, per_page: perPage }
  if (filterAction.value) params.action = filterAction.value
  const { data } = await api.get(`/tenants/${tenantId.value}/activity-logs`, { params })
  logs.value = data.data || []
  total.value = data.total || 0
}

function formatTime(d: string) {
  const dt = new Date(d)
  const dd = String(dt.getDate()).padStart(2, '0')
  const mm = String(dt.getMonth() + 1).padStart(2, '0')
  const hh = String(dt.getHours()).padStart(2, '0')
  const mi = String(dt.getMinutes()).padStart(2, '0')
  return `${dd}/${mm}/${dt.getFullYear()} ${hh}:${mi}`
}

function actionColor(action: string) {
  if (action.includes('error')) return 'error'
  if (action.includes('delete')) return 'warning'
  if (action.includes('create')) return 'success'
  return 'info'
}
</script>
