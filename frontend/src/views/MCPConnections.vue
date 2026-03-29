<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-6">
      <h1 class="text-h5 font-weight-bold">{{ $t('mcp_connections') }}</h1>
      <v-btn color="primary" prepend-icon="mdi-plus" @click="createDialog = true">
        {{ $t('create_connection') }}
      </v-btn>
    </div>

    <v-card v-if="clients.length">
      <v-table density="compact">
        <thead>
          <tr>
            <th>{{ $t('name') }}</th>
            <th>{{ $t('client_id') }}</th>
            <th>Redirect URIs</th>
            <th>{{ $t('scopes') }}</th>
            <th>Tạo lúc</th>
            <th>{{ $t('actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="client in clients" :key="client.id">
            <td class="font-weight-medium">{{ client.name }}</td>
            <td class="text-body-2 font-mono">{{ client.client_id }}</td>
            <td>
              <template v-if="parseJSON(client.redirect_uris).length">
                <v-chip v-for="uri in parseJSON(client.redirect_uris)" :key="uri" size="x-small" variant="tonal" class="mr-1 mb-1">{{ uri }}</v-chip>
              </template>
              <span v-else class="text-grey text-caption">Chưa cấu hình</span>
            </td>
            <td>
              <v-chip v-for="scope in parseJSON(client.scopes)" :key="scope" size="x-small" variant="tonal" color="primary" class="mr-1">{{ scope }}</v-chip>
            </td>
            <td class="text-caption">{{ new Date(client.created_at).toLocaleString('vi-VN') }}</td>
            <td>
              <v-btn size="small" variant="text" color="error" @click="revokeClient(client.id)">
                {{ $t('revoke') }}
              </v-btn>
            </td>
          </tr>
        </tbody>
      </v-table>
    </v-card>
    <div v-else class="text-center pa-8">
      <v-icon size="48" color="grey-lighten-1" class="mb-3">mdi-connection</v-icon>
      <div class="text-grey-darken-1 mb-2">Kết nối MCP cho phép Claude Web/Desktop truy vấn dữ liệu qua giao thức MCP.</div>
      <v-btn color="primary" prepend-icon="mdi-plus" @click="createDialog = true">Tạo kết nối</v-btn>
    </div>

    <!-- Create Dialog -->
    <v-dialog v-model="createDialog" max-width="560">
      <v-card class="pa-6">
        <v-card-title>{{ $t('create_connection') }}</v-card-title>

        <v-text-field v-model="newName" :label="$t('name')" class="mt-4" hint="Tên hiển thị cho kết nối này" persistent-hint />

        <v-combobox
          v-model="newRedirectURIs"
          label="Redirect URIs"
          multiple
          chips
          closable-chips
          class="mt-4"
          hint="Nhập URL callback rồi nhấn Enter (vd: https://claude.ai/oauth/callback)"
          persistent-hint
        />

        <v-select
          v-model="newScopes"
          :items="scopeOptions"
          label="Phân quyền (Scopes)"
          multiple
          chips
          class="mt-4"
          hint="Chọn quyền truy cập cho kết nối"
          persistent-hint
        />

        <div v-if="generatedSecret" class="bg-grey-lighten-4 pa-4 rounded mt-4">
          <div class="text-caption text-grey mb-1">{{ $t('client_id') }}</div>
          <div class="font-mono text-body-2 mb-3">{{ generatedClientId }}</div>
          <div class="text-caption text-grey mb-1">{{ $t('client_secret') }}</div>
          <div class="font-mono text-body-2 text-error mb-2">{{ generatedSecret }}</div>
          <v-alert type="warning" variant="tonal" density="compact">
            Secret chỉ hiện 1 lần. Hãy copy ngay!
          </v-alert>
          <v-btn variant="outlined" size="small" class="mt-2" @click="copySecret">
            <v-icon start size="small">mdi-content-copy</v-icon>
            Copy Secret
          </v-btn>
        </div>

        <v-card-actions class="px-0 mt-4">
          <v-spacer />
          <v-btn variant="text" @click="closeDialog">{{ generatedSecret ? 'Đóng' : $t('cancel') }}</v-btn>
          <v-btn v-if="!generatedSecret" color="primary" :loading="creating" :disabled="!newName" @click="generateClient">{{ $t('create') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-snackbar v-model="snackbar" color="success" timeout="2000">{{ snackText }}</v-snackbar>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'

const clients = ref<any[]>([])
const createDialog = ref(false)
const newName = ref('')
const newRedirectURIs = ref<string[]>([])
const newScopes = ref<string[]>(['read', 'write'])
const scopeOptions = ['read', 'write']
const generatedClientId = ref('')
const generatedSecret = ref('')
const creating = ref(false)
const snackbar = ref(false)
const snackText = ref('')

onMounted(loadClients)

function parseJSON(val: string): string[] {
  try {
    return JSON.parse(val) || []
  } catch {
    return []
  }
}

async function loadClients() {
  try {
    const { data } = await api.get('/mcp/clients')
    clients.value = data || []
  } catch { /* ignore */ }
}

async function generateClient() {
  creating.value = true
  try {
    const { data } = await api.post('/mcp/clients', {
      name: newName.value,
      redirect_uris: newRedirectURIs.value,
      scopes: newScopes.value,
    })
    generatedClientId.value = data.client_id
    generatedSecret.value = data.client_secret
    await loadClients()
    newName.value = ''
    newRedirectURIs.value = []
    newScopes.value = ['read', 'write']
  } catch (err: any) {
    snackText.value = err.response?.data?.error || 'Lỗi tạo kết nối'
    snackbar.value = true
  } finally {
    creating.value = false
  }
}

async function revokeClient(id: string) {
  if (!confirm('Thu hồi kết nối này?')) return
  try {
    await api.delete(`/mcp/clients/${id}`)
    await loadClients()
  } catch { /* ignore */ }
}

function closeDialog() {
  createDialog.value = false
  generatedSecret.value = ''
  generatedClientId.value = ''
}

function copySecret() {
  navigator.clipboard.writeText(generatedSecret.value)
  snackText.value = 'Đã copy secret'
  snackbar.value = true
}
</script>
