<template>
  <div>
    <h1 class="text-h5 font-weight-bold mb-6">{{ $t('cost_logs') }}</h1>

    <v-card>
      <v-card-text class="d-flex ga-3 pb-0 flex-wrap">
        <v-select
          v-model="filterProvider"
          :items="['claude', 'gemini']"
          :label="$t('ai_provider')"
          density="compact"
          clearable
          style="max-width: 200px"
          @update:model-value="loadLogs"
        />
        <v-text-field v-model="dateFrom" type="date" label="Từ ngày" density="compact" style="max-width: 160px" @change="loadLogs" />
        <v-text-field v-model="dateTo" type="date" label="Đến ngày" density="compact" style="max-width: 160px" @change="loadLogs" />
      </v-card-text>

      <div style="overflow-x: auto;">
      <v-table density="compact">
        <thead>
          <tr>
            <th>{{ $t('sent_at') }}</th>
            <th>Provider</th>
            <th>Model</th>
            <th class="text-right">Input Tokens</th>
            <th class="text-right">Output Tokens</th>
            <th class="text-right">{{ $t('cost') }} (USD)</th>
            <th class="text-right">{{ $t('cost') }} (VND)</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="log in logs" :key="log.id">
            <td class="text-caption">{{ formatTime(log.created_at) }}</td>
            <td>
              <v-chip size="x-small" :color="log.provider === 'claude' ? 'deep-purple' : 'blue'" variant="tonal">{{ log.provider }}</v-chip>
            </td>
            <td class="text-body-2">{{ log.model }}</td>
            <td class="text-right text-body-2">{{ log.input_tokens?.toLocaleString() }}</td>
            <td class="text-right text-body-2">{{ log.output_tokens?.toLocaleString() }}</td>
            <td class="text-right text-body-2">${{ log.cost_usd?.toFixed(4) }}</td>
            <td class="text-right text-body-2">{{ (log.cost_usd * exchangeRate).toLocaleString('vi-VN', { maximumFractionDigits: 0 }) }}d</td>
          </tr>
        </tbody>
        <tfoot v-if="logs.length">
          <tr class="font-weight-bold">
            <td colspan="5" class="text-right">{{ $t('total') }}:</td>
            <td class="text-right">${{ totalCostUSD.toFixed(4) }}</td>
            <td class="text-right">{{ (totalCostUSD * exchangeRate).toLocaleString('vi-VN', { maximumFractionDigits: 0 }) }}d</td>
          </tr>
        </tfoot>
      </v-table>
      </div>

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
const exchangeRate = ref(26000)
const filterProvider = ref('')
const dateFrom = ref('')
const dateTo = ref('')
const totalPages = computed(() => Math.ceil(total.value / perPage))
const totalCostUSD = computed(() => logs.value.reduce((sum: number, l: any) => sum + (l.cost_usd || 0), 0))

onMounted(() => loadLogs())
watch(page, () => loadLogs())

async function loadLogs() {
  const params: Record<string, any> = { page: page.value, per_page: perPage }
  if (filterProvider.value) params.provider = filterProvider.value
  if (dateFrom.value) params.from = dateFrom.value
  if (dateTo.value) params.to = dateTo.value
  const { data } = await api.get(`/tenants/${tenantId.value}/cost-logs`, { params })
  logs.value = data.data || []
  total.value = data.total || 0
  exchangeRate.value = data.exchange_rate || 26000
}

function formatTime(d: string) {
  const dt = new Date(d)
  const dd = String(dt.getDate()).padStart(2, '0')
  const mm = String(dt.getMonth() + 1).padStart(2, '0')
  const hh = String(dt.getHours()).padStart(2, '0')
  const mi = String(dt.getMinutes()).padStart(2, '0')
  return `${dd}/${mm}/${dt.getFullYear()} ${hh}:${mi}`
}
</script>
