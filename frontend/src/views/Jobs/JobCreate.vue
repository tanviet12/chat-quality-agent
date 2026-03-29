<template>
  <div>
    <div class="d-flex align-center mb-6">
      <v-btn icon="mdi-arrow-left" variant="text" :to="`/${tenantId}/jobs`" />
      <h1 class="text-h5 font-weight-bold ml-2">{{ $t('create_job') }}</h1>
    </div>

    <v-card class="pa-6">
      <v-stepper v-model="step" :items="stepItems" :alt-labels="mdAndUp" hide-actions>
        <template #[`item.1`]>
          <StepType v-model:form="form" />
        </template>
        <template #[`item.2`]>
          <StepInput v-model:form="form" />
        </template>
        <template #[`item.3`]>
          <StepRules v-model:form="form" />
        </template>
        <template #[`item.4`]>
          <StepOutput v-model:form="form" />
        </template>
        <template #[`item.5`]>
          <StepOutputSchedule v-model:form="form" />
        </template>
        <template #[`item.6`]>
          <StepConfirm v-model:form="form" />
        </template>
      </v-stepper>

      <div class="d-flex justify-space-between mt-6">
        <v-btn v-if="step > 1" variant="text" @click="step--">
          <v-icon start>mdi-chevron-left</v-icon>
          {{ $t('back') }}
        </v-btn>
        <v-spacer />
        <template v-if="step < 6">
          <v-btn color="primary" :disabled="!canProceed" @click="step++">
            {{ $t('next') }}
            <v-icon end>mdi-chevron-right</v-icon>
          </v-btn>
          <div v-if="!canProceed" class="text-caption text-error ml-2 align-self-center">
            {{ validationMessage }}
          </div>
        </template>
        <v-btn v-if="step === 6" color="success" :loading="creating" @click="submitJob">
          <v-icon start>mdi-check-circle</v-icon>
          {{ $t('confirm') }}
        </v-btn>
      </div>
    </v-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDisplay } from 'vuetify'
import { useI18n } from 'vue-i18n'
import { useJobStore } from '../../stores/jobs'
import StepType from '../../components/JobWizard/StepType.vue'
import StepInput from '../../components/JobWizard/StepInput.vue'
import StepRules from '../../components/JobWizard/StepRules.vue'
import StepOutput from '../../components/JobWizard/StepOutput.vue'
import StepOutputSchedule from '../../components/JobWizard/StepOutputSchedule.vue'
import StepConfirm from '../../components/JobWizard/StepConfirm.vue'

const route = useRoute()
const router = useRouter()
const { mdAndUp } = useDisplay()
const { t } = useI18n()
const jobStore = useJobStore()

const tenantId = computed(() => route.params.tenantId as string)
const step = ref(1)
const creating = ref(false)

const canProceed = computed(() => {
  switch (step.value) {
    case 1: return form.value.name.trim().length >= 2
    case 2: return form.value.input_channel_ids.length > 0
    case 3: {
      if (form.value.job_type === 'classification') {
        try { return JSON.parse(form.value.rules_config || '[]').length > 0 } catch { return false }
      }
      return form.value.rules_content.trim().length > 0
    }
    case 4: {
      try {
        const outputs = JSON.parse(form.value.outputs || '[]')
        if (outputs.length === 0) return true // optional — no output configured
        // All outputs must have valid fields AND pass test
        return form.value.outputs_validated === true
      } catch { return false }
    }
    case 5: {
      if (form.value.schedule_type === 'cron' && !form.value.schedule_cron.trim()) return false
      if (form.value.output_schedule === 'cron' && !form.value.output_cron.trim()) return false
      return true
    }
    default: return true
  }
})

const validationMessage = computed(() => {
  switch (step.value) {
    case 1: return t('validation_min_chars', { min: 2 })
    case 2: return t('validation_select_channel')
    case 3: return t('validation_enter_rules')
    case 4: return 'Vui lòng điền đầy đủ thông tin đầu ra và test gửi thành công'
    default: return ''
  }
})

const form = ref({
  name: '',
  description: '',
  job_type: 'qc_analysis',
  input_channel_ids: [] as string[],
  rules_content: '',
  rules_config: '[]',
  skip_conditions: '',
  ai_provider: '',
  ai_model: '',
  outputs: '[]',
  outputs_validated: true,
  output_schedule: 'instant',
  output_cron: '',
  output_at: '',
  schedule_type: 'cron',
  schedule_cron: '0 7 * * *',
})

const stepItems = computed(() => [
  { title: t('job_wizard_step_type'), value: 1 },
  { title: t('job_wizard_step_input'), value: 2 },
  { title: t('job_wizard_step_rules'), value: 3 },
  { title: t('job_wizard_step_output'), value: 4 },
  { title: t('job_wizard_step_schedule'), value: 5 },
  { title: t('job_wizard_step_confirm'), value: 6 },
])

async function submitJob() {
  creating.value = true
  try {
    const payload = {
      ...form.value,
      outputs: JSON.parse(form.value.outputs || '[]'),
      rules_config: form.value.job_type === 'classification' ? JSON.parse(form.value.rules_config) : undefined,
      output_at: form.value.output_at || undefined,
    }
    await jobStore.createJob(tenantId.value, payload)
    router.push(`/${tenantId.value}/jobs`)
  } catch (err) {
    console.error('Create job failed:', err)
  } finally {
    creating.value = false
  }
}
</script>
