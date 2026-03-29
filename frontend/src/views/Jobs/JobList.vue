<template>
  <div>
    <div class="d-flex align-center justify-space-between mb-6">
      <h1 class="text-h5 font-weight-bold">{{ $t('jobs') }}</h1>
      <v-btn v-if="authStore.canEdit('jobs')" color="primary" prepend-icon="mdi-plus" :to="`/${tenantId}/jobs/create`">
        {{ $t('create_job') }}
      </v-btn>
    </div>

    <v-card>
      <v-table v-if="jobStore.jobs.length">
        <thead>
          <tr>
            <th>{{ $t('job_name') }}</th>
            <th>{{ $t('status') }}</th>
            <th>{{ $t('last_run') }}</th>
            <th>{{ $t('actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="job in jobStore.jobs" :key="job.id">
            <td>
              <router-link :to="`/${tenantId}/jobs/${job.id}`" class="text-primary font-weight-medium text-decoration-none">
                {{ job.name }}
              </router-link>
              <div class="text-caption text-grey">{{ job.description }}</div>
            </td>
            <td>
              <v-chip size="small" :color="job.is_active ? 'success' : 'grey'" variant="tonal">
                {{ job.is_active ? $t('active') : $t('inactive') }}
              </v-chip>
            </td>
            <td>
              <span v-if="job.last_run_at" class="text-body-2">
                {{ new Date(job.last_run_at).toLocaleString() }}
                <v-chip size="x-small" :color="job.last_run_status === 'success' ? 'success' : 'error'" variant="tonal" class="ml-1">
                  {{ job.last_run_status }}
                </v-chip>
              </span>
              <span v-else class="text-grey text-body-2">—</span>
            </td>
            <td>
              <v-btn icon="mdi-pencil" size="small" variant="text" :to="`/${tenantId}/jobs/${job.id}/edit`" />
              <v-btn icon="mdi-delete" size="small" variant="text" color="error" @click="remove(job.id)" />
            </td>
          </tr>
        </tbody>
      </v-table>
      <div v-else class="text-center pa-8">
        <v-icon size="48" color="grey-lighten-1" class="mb-3">mdi-briefcase-plus</v-icon>
        <div class="text-grey-darken-1 mb-2">Tạo công việc phân tích để AI đánh giá chất lượng CSKH hoặc phân loại cuộc chat tự động.</div>
        <v-btn color="primary" prepend-icon="mdi-plus" :to="`/${tenantId}/jobs/create`" size="small">Tạo công việc</v-btn>
      </div>
    </v-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useJobStore } from '../../stores/jobs'
import { useAuthStore } from '../../stores/auth'

const route = useRoute()
const jobStore = useJobStore()
const authStore = useAuthStore()
const tenantId = computed(() => route.params.tenantId as string)

onMounted(() => jobStore.fetchJobs(tenantId.value))



async function remove(jobId: string) {
  if (confirm('Delete this job?')) {
    await jobStore.deleteJob(tenantId.value, jobId)
  }
}
</script>
