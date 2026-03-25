<template>
  <div>
    <h3 class="text-h6 mb-2">{{ $t('job_wizard_step_analysis_schedule') }}</h3>
    <p class="text-body-2 text-grey mb-4">{{ $t('analysis_schedule_desc') }}</p>

    <v-radio-group v-model="form.schedule_type">
      <v-radio value="cron" :label="$t('schedule_cron')" />
      <v-radio value="after_sync" :label="$t('schedule_after_sync')" />
      <v-radio value="manual" :label="$t('schedule_manual')" />
    </v-radio-group>

    <v-text-field
      v-if="form.schedule_type === 'cron'"
      v-model="form.schedule_cron"
      :label="$t('cron_expression')"
      placeholder="0 7 * * *"
      hint="Ví dụ: 0 7 * * * (mỗi sáng lúc 7h)"
      persistent-hint
      class="mt-3"
    />

    <v-alert type="info" variant="tonal" density="compact" class="mt-4">
      {{ $t('ai_provider') }}: {{ $t('job_uses_global_ai_settings') }}
      <router-link :to="`/${form.tenant_id || $route.params.tenantId}/settings`" class="ml-1">{{ $t('settings') }}</router-link>
    </v-alert>
  </div>
</template>

<script setup lang="ts">
const form = defineModel<Record<string, any>>('form', { required: true })
</script>
