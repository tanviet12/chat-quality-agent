<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-6">
      <h1 class="text-h5 font-weight-bold">{{ $t('tenants') }}</h1>
      <v-btn v-if="isAdmin()" color="primary" prepend-icon="mdi-plus" @click="showDialog = true">{{ $t('create_tenant') }}</v-btn>
    </div>
    <v-row>
      <v-col v-for="tenant in tenantStore.tenants" :key="tenant.id" cols="12" sm="6" md="4">
        <v-card class="pa-4 cursor-pointer" hover @click="selectTenant(tenant)">
          <div class="d-flex align-center">
            <v-card-title class="flex-grow-1 pa-0 text-truncate" style="min-width: 0">{{ tenant.name }}</v-card-title>
            <v-btn v-if="isAdmin()" icon="mdi-delete" size="x-small" variant="text" color="error" class="flex-shrink-0" @click.stop="confirmDelete(tenant)" />
          </div>
          <v-card-subtitle class="pl-0 text-truncate">{{ tenant.slug }}</v-card-subtitle>
          <v-card-text class="d-flex ga-4">
            <v-chip size="small" color="primary" variant="tonal">
              {{ $t('channels_count', { count: tenant.channels_count || 0 }) }}
            </v-chip>
            <v-chip size="small" color="secondary" variant="tonal">
              {{ $t('jobs_count', { count: tenant.jobs_count || 0 }) }}
            </v-chip>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
    <div v-if="!tenantStore.tenants.length" class="text-center text-grey mt-12">
      {{ $t('no_data') }}
    </div>

    <!-- Create Tenant Dialog -->
    <v-dialog v-model="showDialog" max-width="500">
      <v-card>
        <v-card-title>{{ $t('create_tenant') }}</v-card-title>
        <v-card-text>
          <v-form ref="createFormRef">
          <v-text-field
            v-model="form.name"
            :label="$t('tenant_name')"
            :rules="[v => !!v || $t('required'), v => v.length >= 2 || 'Tối thiểu 2 ký tự', v => v.length <= 100 || 'Tối đa 100 ký tự']"
            counter="100"
            class="mb-4"
          />
          <v-text-field
            v-model="form.slug"
            :label="'Slug'"
            :hint="$t('slug_hint')"
            persistent-hint
            :rules="[v => !!v || $t('required'), v => v.length >= 2 || 'Tối thiểu 2 ký tự', v => /^[a-z0-9-]+$/.test(v) || 'Chỉ chứa a-z, 0-9, dấu -']"
          />
          </v-form>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="showDialog = false">{{ $t('cancel') }}</v-btn>
          <v-btn color="primary" :loading="creating" :disabled="!form.name || !form.slug" @click="createTenant">{{ $t('save') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Delete Tenant Dialog -->
    <v-dialog v-model="deleteDialog" max-width="480">
      <v-card>
        <v-card-title class="text-error">Xóa công ty</v-card-title>
        <v-card-text>
          <v-alert type="error" variant="tonal" class="mb-3">
            Tất cả dữ liệu của <strong>{{ deletingTenant?.name }}</strong> sẽ bị xóa vĩnh viễn: kênh chat, tin nhắn, công việc, kết quả đánh giá, nhật ký chi phí. Hành động này không thể hoàn tác.
          </v-alert>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="deleteDialog = false">Hủy</v-btn>
          <v-btn color="error" variant="flat" :loading="deleting" @click="doDelete">Xóa vĩnh viễn</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-snackbar v-model="snackbar" :color="snackbarColor" :timeout="3000">{{ snackbarText }}</v-snackbar>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useTenantStore } from '../stores/tenants'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const tenantStore = useTenantStore()
const authStore = useAuthStore()
const isAdmin = () => authStore.user?.is_admin === true

const showDialog = ref(false)
const creating = ref(false)
const form = ref({ name: '', slug: '' })
const createFormRef = ref<any>(null)
const snackbar = ref(false)
const snackbarText = ref('')
const snackbarColor = ref('success')

// Auto-generate slug from name
watch(() => form.value.name, (name) => {
  form.value.slug = name
    .toLowerCase()
    .normalize('NFD').replace(/[\u0300-\u036f]/g, '') // remove diacritics
    .replace(/đ/g, 'd').replace(/Đ/g, 'd')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
})

onMounted(() => {
  tenantStore.fetchTenants()
})

function selectTenant(tenant: { id: string }) {
  router.push(`/${tenant.id}`)
}

const deleteDialog = ref(false)
const deleting = ref(false)
const deletingTenant = ref<{ id: string; name: string } | null>(null)

function confirmDelete(tenant: { id: string; name: string }) {
  deletingTenant.value = tenant
  deleteDialog.value = true
}

async function doDelete() {
  if (!deletingTenant.value) return
  deleting.value = true
  try {
    await tenantStore.deleteTenant(deletingTenant.value.id)
    deleteDialog.value = false
    snackbarColor.value = 'success'
    snackbarText.value = 'Đã xóa công ty'
    snackbar.value = true
    if (!tenantStore.tenants.length) {
      router.push('/tenants')
    } else if (tenantStore.currentTenant) {
      router.push(`/${tenantStore.currentTenant.id}`)
    }
  } catch (e: any) {
    snackbarColor.value = 'error'
    snackbarText.value = e?.response?.data?.error || 'Lỗi xóa công ty'
    snackbar.value = true
  } finally {
    deleting.value = false
  }
}

async function createTenant() {
  const { valid } = await createFormRef.value?.validate() || {}
  if (!valid) return
  creating.value = true
  try {
    await tenantStore.createTenant(form.value.name, form.value.slug)
    showDialog.value = false
    form.value = { name: '', slug: '' }
    snackbarColor.value = 'success'
    snackbarText.value = 'Tạo công ty thành công!'
    snackbar.value = true
    await tenantStore.fetchTenants()
  } catch (e: any) {
    snackbarColor.value = 'error'
    const err = e?.response?.data?.error
    snackbarText.value = err === 'invalid_request' ? 'Vui lòng kiểm tra lại thông tin' : err === 'slug_already_exists' ? 'Slug đã tồn tại' : err || 'Lỗi tạo công ty'
    snackbar.value = true
  } finally {
    creating.value = false
  }
}
</script>
