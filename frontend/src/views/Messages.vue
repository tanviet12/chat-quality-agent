<template>
  <div>
    <h1 class="text-h5 font-weight-bold mb-4">{{ $t('nav_messages') }}
      <span v-if="conversationStore.total" class="text-body-2 text-grey font-weight-regular ml-2">({{ conversationStore.total }})</span>
    </h1>

    <v-row>
      <!-- Conversation List -->
      <v-col v-if="mdAndUp || !selectedConvId" cols="12" :md="selectedConvId ? 5 : 12" :lg="selectedConvId ? 4 : 12">
        <!-- Filters -->
        <v-card class="mb-4" variant="outlined">
          <v-card-text class="pa-3">
            <v-row dense>
              <v-col cols="6" sm="3">
                <v-select
                  v-model="filterChannelType"
                  :items="channelTypes"
                  :label="$t('channel_type')"
                  clearable
                  density="compact"
                  variant="outlined"
                  hide-details
                />
              </v-col>
              <v-col cols="6" sm="3">
                <v-select
                  v-model="filterChannelId"
                  :items="channelOptions"
                  :label="$t('msg_channel')"
                  clearable
                  density="compact"
                  variant="outlined"
                  hide-details
                />
              </v-col>
              <v-col cols="6" sm="3">
                <v-text-field
                  v-model="searchQuery"
                  :label="$t('search')"
                  prepend-inner-icon="mdi-magnify"
                  clearable
                  density="compact"
                  variant="outlined"
                  hide-details
                  @update:model-value="debouncedSearch"
                />
              </v-col>
              <v-col cols="6" sm="3" class="d-flex ga-2 align-center">
                <v-select
                  v-model="filterEvaluation"
                  :items="evaluationFilterOptions"
                  label="Đánh giá"
                  clearable
                  density="compact"
                  variant="outlined"
                  hide-details
                  class="flex-grow-1"
                />
                <v-btn v-if="authStore.canEdit('messages')" size="small" variant="tonal" color="primary" icon="mdi-export" @click="showExportDialog = true" />
              </v-col>
            </v-row>
          </v-card-text>
        </v-card>

        <!-- Export Dialog -->
        <v-dialog v-model="showExportDialog" max-width="450">
          <v-card>
            <v-card-title>Export tin nhắn</v-card-title>
            <v-card-text>
              <div class="text-body-2 text-grey mb-3">Xuất toàn bộ cuộc chat trong khoảng thời gian để AI đọc và phân tích.</div>
              <v-row dense>
                <v-col cols="6">
                  <v-text-field v-model="exportFrom" label="Từ ngày" type="date" density="compact" variant="outlined" hide-details />
                </v-col>
                <v-col cols="6">
                  <v-text-field v-model="exportTo" label="Đến ngày" type="date" density="compact" variant="outlined" hide-details />
                </v-col>
              </v-row>
              <v-select
                v-model="exportFormat"
                :items="[{ title: 'Text (cho AI đọc)', value: 'txt' }, { title: 'CSV (bảng tính)', value: 'csv' }]"
                label="Định dạng"
                density="compact"
                variant="outlined"
                hide-details
                class="mt-3"
              />
              <v-select
                v-model="exportChannelType"
                :items="[{ title: 'Tất cả kênh', value: '' }, { title: 'Zalo OA', value: 'zalo_oa' }, { title: 'Facebook', value: 'facebook' }]"
                label="Kênh"
                density="compact"
                variant="outlined"
                hide-details
                class="mt-3"
              />
            </v-card-text>
            <v-card-actions>
              <v-spacer />
              <v-btn variant="text" @click="showExportDialog = false">Hủy</v-btn>
              <v-btn color="primary" :loading="exporting" :disabled="!exportFrom || !exportTo" @click="doExport">
                <v-icon start>mdi-download</v-icon> Tải về
              </v-btn>
            </v-card-actions>
          </v-card>
        </v-dialog>

        <!-- Conversations -->
        <v-card variant="outlined">
          <v-list v-if="filteredConversations.length > 0" lines="two" density="compact" class="pa-0">
            <template v-for="(conv, index) in filteredConversations" :key="conv.id">
              <v-list-item
                :active="selectedConvId === conv.id"
                color="primary"
                class="px-3 py-1"
                @click="selectConversation(conv.id)"
              >
                <template #prepend>
                  <v-avatar :color="conv.channel_type === 'facebook' ? 'blue' : 'green'" size="32" class="mr-3">
                    <v-icon color="white" size="16">
                      {{ conv.channel_type === 'facebook' ? 'mdi-facebook-messenger' : 'mdi-chat' }}
                    </v-icon>
                  </v-avatar>
                </template>

                <v-list-item-title class="text-body-2 font-weight-medium">
                  {{ conv.customer_name || $t('msg_unknown_customer') }}
                </v-list-item-title>
                <v-list-item-subtitle class="text-caption">
                  <v-chip size="x-small" :color="conv.channel_type === 'facebook' ? 'blue' : 'green'" variant="tonal" class="mr-1">
                    {{ conv.channel_type === 'facebook' ? 'FB' : 'Zalo' }}
                  </v-chip>
                  <v-chip v-if="evaluationMap[conv.id]" size="x-small" :color="evaluationMap[conv.id] === 'PASS' ? 'success' : 'error'" variant="tonal" class="mr-1">
                    {{ evaluationMap[conv.id] === 'PASS' ? 'Đạt' : 'Không đạt' }}
                  </v-chip>
                  {{ conv.message_count }} tin · {{ timeAgo(conv.last_message_at) }}
                </v-list-item-subtitle>
              </v-list-item>
              <v-divider v-if="index < filteredConversations.length - 1" />
            </template>
          </v-list>

          <v-card-text v-else-if="!loading" class="text-center py-8">
            <v-icon size="48" color="grey-lighten-1">mdi-message-text-outline</v-icon>
            <div class="text-grey-darken-1 mt-2">Tin nhắn sẽ hiện ở đây sau khi kết nối và đồng bộ kênh chat.</div>
            <v-btn variant="text" color="primary" size="small" class="mt-2" :to="`/${tenantId}/channels`">Đi tới kênh chat</v-btn>
          </v-card-text>

          <v-card-text v-if="loading" class="text-center py-4">
            <v-progress-circular indeterminate size="24" />
          </v-card-text>

          <!-- Pagination -->
          <v-divider v-if="totalPages > 1" />
          <v-card-actions v-if="totalPages > 1" class="justify-center">
            <v-pagination v-model="currentPage" :length="totalPages" :total-visible="7" density="compact" />
          </v-card-actions>
        </v-card>
      </v-col>

      <!-- Message Detail -->
      <v-col v-if="selectedConvId" cols="12" md="7" lg="8">
        <v-card variant="outlined" class="d-flex flex-column" style="height: calc(100vh - 140px)">
          <!-- Header -->
          <v-card-title class="d-flex align-center pa-4">
            <v-btn icon="mdi-arrow-left" variant="text" size="small" class="d-md-none mr-2" @click="selectedConvId = null" />
            <v-avatar :color="selectedConvChannelType === 'facebook' ? 'blue' : 'green'" size="36" class="mr-3">
              <v-icon color="white" size="18">
                {{ selectedConvChannelType === 'facebook' ? 'mdi-facebook-messenger' : 'mdi-chat' }}
              </v-icon>
            </v-avatar>
            <div class="flex-grow-1">
              <div class="text-subtitle-1 font-weight-medium">
                {{ conversationStore.currentConversation?.customer_name || $t('msg_unknown_customer') }}
              </div>
              <div class="text-caption text-grey">
                {{ conversationStore.currentConversation?.message_count }} {{ $t('msg_messages_count') }}
              </div>
            </div>
            <v-btn icon="mdi-share-variant" variant="text" size="small" color="primary" @click="shareConversation" />
            <v-btn icon="mdi-download" variant="text" size="small" color="primary" @click="downloadConversation" />
          </v-card-title>

          <v-divider />

          <!-- Tabs: Messages | QC | Classification -->
          <v-tabs v-model="detailTab" density="compact" class="px-2" style="flex-shrink: 0;">
            <v-tab value="messages">
              <v-icon start size="small">mdi-chat</v-icon>
              Tin nhắn
            </v-tab>
            <v-tab value="qc">
              <v-icon start size="small">mdi-clipboard-check</v-icon>
              Đánh giá
              <v-chip v-if="qcGroups.length" size="x-small" color="primary" variant="flat" class="ml-1">{{ qcGroups.length }}</v-chip>
            </v-tab>
            <v-tab value="classification">
              <v-icon start size="small">mdi-tag-multiple</v-icon>
              Phân loại
              <v-chip v-if="classGroups.length" size="x-small" color="primary" variant="flat" class="ml-1">{{ classGroups.length }}</v-chip>
            </v-tab>
          </v-tabs>

          <v-divider />

          <!-- Messages tab -->
          <div v-show="detailTab === 'messages'" ref="messagesContainer" class="flex-grow-1 overflow-y-auto pa-4" style="background: rgba(0,0,0,0.02)">
            <div v-if="loadingMessages" class="text-center py-8">
              <v-progress-circular indeterminate />
            </div>
            <template v-else>
              <div
                v-for="msg in conversationStore.messages"
                :key="msg.id"
                class="d-flex mb-2"
                :class="msg.sender_type === 'agent' ? 'justify-end' : 'justify-start'"
              >
                <div
                  class="pa-2 rounded-lg"
                  :class="msg.sender_type === 'agent' ? 'bg-primary text-white' : 'bg-surface'"
                  style="max-width: 75%; word-break: break-word"
                  :style="msg.sender_type !== 'agent' ? 'border: 1px solid rgba(0,0,0,0.12)' : ''"
                >
                  <div class="text-caption font-weight-medium" :class="msg.sender_type === 'agent' ? 'text-white' : 'text-primary'" style="font-size: 11px">
                    {{ msg.sender_name }}
                  </div>
                  <div v-if="msg.content" class="text-body-2" style="white-space: pre-wrap; font-size: 13px; line-height: 1.4">{{ msg.content }}</div>
                  <div v-if="msg.content_type === 'sticker'" class="text-caption font-italic">[Sticker]</div>
                  <div v-if="hasAttachments(msg)" class="mt-1">
                    <template v-for="(att, i) in parseAttachments(msg)" :key="i">
                      <div v-if="isImageAttachment(att)" class="mb-1">
                        <img
                          v-if="authImageCache[getAttachmentUrl(att)] && authImageCache[getAttachmentUrl(att)] !== 'loading'"
                          :src="authImageCache[getAttachmentUrl(att)]"
                          style="max-width: 200px; max-height: 200px; border-radius: 8px; cursor: pointer;"
                          @click="lightboxSrc = authImageCache[getAttachmentUrl(att)]"
                          @error="onImageError($event, att)"
                        />
                        <v-progress-circular v-else-if="authImageCache[getAttachmentUrl(att)] === 'loading'" indeterminate size="24" width="2" class="ma-2" />
                        <v-chip v-else-if="!getAttachmentUrl(att)" size="x-small" variant="tonal" color="grey">
                          <v-icon start size="12">mdi-image</v-icon>
                          {{ att.name || '[Ảnh]' }}
                        </v-chip>
                      </div>
                      <v-chip v-else size="x-small" variant="tonal" class="mr-1" :href="getAttachmentUrl(att)" target="_blank">
                        <v-icon start size="12">mdi-paperclip</v-icon>
                        {{ att.name || att.type || 'File' }}
                      </v-chip>
                    </template>
                  </div>
                  <div v-if="!msg.content && msg.content_type === 'attachment' && !hasAttachments(msg)" class="text-caption font-italic">[File đính kèm]</div>
                  <div class="mt-1" :class="msg.sender_type === 'agent' ? 'text-white-darken-2' : 'text-grey'" style="opacity: 0.6; font-size: 10px">
                    {{ formatMessageTime(msg.sent_at) }}
                  </div>
                </div>
              </div>
            </template>
          </div>

          <!-- QC Evaluation tab -->
          <div v-show="detailTab === 'qc'" class="flex-grow-1 overflow-y-auto pa-4">
            <div v-if="loadingEvaluation" class="text-center py-8">
              <v-progress-circular indeterminate />
            </div>
            <div v-else-if="qcGroups.length === 0" class="text-center py-8">
              <v-icon size="48" color="grey-lighten-1">mdi-clipboard-text-off</v-icon>
              <div class="text-grey mt-3">Cuộc chat này chưa được đánh giá chất lượng.</div>
            </div>
            <div v-else>
              <v-card v-for="g in qcGroups" :key="g.job_run_id" variant="outlined" class="mb-3">
                <v-card-text class="pa-3">
                  <div class="d-flex align-center mb-2">
                    <v-chip size="x-small" :color="getQcVerdict(g) === 'PASS' ? 'success' : getQcVerdict(g) === 'SKIP' ? 'grey' : 'error'" variant="tonal" class="mr-2">
                      {{ getQcVerdict(g) === 'PASS' ? 'Đạt' : getQcVerdict(g) === 'SKIP' ? 'Bỏ qua' : 'Không đạt' }}
                    </v-chip>
                    <v-chip v-if="getQcScore(g) != null" size="x-small" variant="tonal" class="mr-2">{{ getQcScore(g) }}/100</v-chip>
                    <span class="text-body-2 font-weight-medium flex-grow-1">{{ g.job_name }}</span>
                    <span class="text-caption text-grey">{{ formatTime(g.evaluated_at) }}</span>
                  </div>
                  <div v-if="getQcReview(g)" class="text-body-2 text-grey-darken-1 mb-2" style="font-size: 13px;">{{ getQcReview(g) }}</div>
                  <v-btn v-if="getQcViolations(g).length > 0" size="x-small" variant="text" color="primary" @click="toggleQcExpand(g.job_run_id)">
                    {{ expandedQc[g.job_run_id] ? 'Thu gọn' : `Xem chi tiết (${getQcViolations(g).length} vấn đề)` }}
                  </v-btn>
                  <div v-if="expandedQc[g.job_run_id]" class="mt-2">
                    <div v-for="(v, idx) in getQcViolations(g)" :key="idx" class="mb-2">
                      <div class="d-flex align-center mb-1">
                        <v-chip size="x-small" :color="v.severity === 'NGHIEM_TRONG' ? 'error' : 'warning'" variant="tonal" class="mr-2">
                          {{ v.severity === 'NGHIEM_TRONG' ? 'Nghiêm trọng' : 'Cần cải thiện' }}
                        </v-chip>
                        <span class="font-weight-medium text-body-2">{{ v.rule_name }}</span>
                      </div>
                      <div class="text-body-2 bg-orange-lighten-5 pa-2 rounded" style="font-size: 13px; border-left: 3px solid #ff9800;">
                        {{ v.evidence }}
                      </div>
                    </div>
                  </div>
                </v-card-text>
              </v-card>
            </div>
          </div>

          <!-- Classification tab -->
          <div v-show="detailTab === 'classification'" class="flex-grow-1 overflow-y-auto pa-4">
            <div v-if="loadingEvaluation" class="text-center py-8">
              <v-progress-circular indeterminate />
            </div>
            <div v-else-if="classGroups.length === 0" class="text-center py-8">
              <v-icon size="48" color="grey-lighten-1">mdi-tag-off</v-icon>
              <div class="text-grey mt-3">Cuộc chat này chưa được phân loại.</div>
            </div>
            <div v-else>
              <v-card v-for="g in classGroups" :key="g.job_run_id" variant="outlined" class="mb-3">
                <v-card-text class="pa-3">
                  <div class="d-flex align-center mb-2">
                    <span class="text-body-2 font-weight-medium flex-grow-1">{{ g.job_name }}</span>
                    <span class="text-caption text-grey">{{ formatTime(g.evaluated_at) }}</span>
                  </div>
                  <div class="d-flex flex-wrap ga-1 mb-2">
                    <v-chip v-for="tag in getClassTags(g)" :key="tag" size="small" :color="msgTagColor(tag)" variant="tonal">
                      <v-icon start size="small">mdi-tag</v-icon>
                      {{ tag }}
                    </v-chip>
                    <v-chip v-if="getClassTags(g).length === 0" size="small" color="grey" variant="tonal">
                      Không phân loại được
                    </v-chip>
                  </div>
                  <div v-if="getClassSummary(g)" class="text-body-2 text-grey-darken-1" style="font-size: 13px;">{{ getClassSummary(g) }}</div>
                </v-card-text>
              </v-card>
            </div>
          </div>
        </v-card>
      </v-col>
    </v-row>
    <v-snackbar v-model="snackbar" color="success" timeout="2000">{{ snackText }}</v-snackbar>

    <!-- Lightbox overlay -->
    <div v-if="lightboxSrc" class="lightbox-overlay" @click="lightboxSrc = ''">
      <img :src="lightboxSrc" class="lightbox-img" @click.stop />
      <v-btn icon="mdi-close" variant="flat" color="white" size="small" class="lightbox-close" @click="lightboxSrc = ''" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { useDisplay } from 'vuetify'
import { useConversationStore, type Message } from '../stores/conversations'
import { useChannelStore } from '../stores/channels'
import { useAuthStore } from '../stores/auth'
import api from '../api'

const route = useRoute()
const { mdAndUp } = useDisplay()
const conversationStore = useConversationStore()
const channelStore = useChannelStore()
const authStore = useAuthStore()

const tenantId = computed(() => route.params.tenantId as string)

const loading = ref(false)
const loadingMessages = ref(false)
const selectedConvId = ref<string | null>(null)
const selectedConvChannelType = ref('')
const currentPage = ref(1)
const detailTab = ref('messages')

// Evaluation state
const loadingEvaluation = ref(false)
const evaluation = ref<any>(null)

// Evaluation map: conversation_id -> verdict (PASS/FAIL)
const evaluationMap = ref<Record<string, string>>({})

async function loadEvaluationMap() {
  try {
    const { data } = await api.get(`/tenants/${tenantId.value}/conversations/evaluated`)
    evaluationMap.value = data || {}
  } catch { /* ignore */ }
}

const filteredConversations = computed(() => conversationStore.conversations)

function shareConversation() {
  const url = `${window.location.origin}/${tenantId.value}/messages?conv=${selectedConvId.value}`
  navigator.clipboard.writeText(url)
  snackText.value = 'Đã sao chép link'
  snackbar.value = true
}

async function doExport() {
  exporting.value = true
  try {
    let url = `/tenants/${tenantId.value}/conversations/export?from=${exportFrom.value}&to=${exportTo.value}&format=${exportFormat.value}`
    if (exportChannelType.value) url += `&channel_type=${exportChannelType.value}`
    const { data } = await api.get(url, { responseType: exportFormat.value === 'csv' ? 'text' : 'text' })
    if (data?.error) {
      snackText.value = data.error
      snackbar.value = true
      return
    }
    const blob = new Blob([data], { type: exportFormat.value === 'csv' ? 'text/csv;charset=utf-8' : 'text/plain;charset=utf-8' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `messages_${exportFrom.value}_${exportTo.value}.${exportFormat.value}`
    a.click()
    URL.revokeObjectURL(a.href)
    showExportDialog.value = false
  } catch {
    snackText.value = 'Lỗi export'
    snackbar.value = true
  } finally {
    exporting.value = false
  }
}

function downloadConversation() {
  const conv = conversationStore.currentConversation
  const msgs = conversationStore.messages
  if (!conv || !msgs.length) return

  let text = `Cuộc chat: ${conv.customer_name || 'Không rõ'}\n`
  text += `Số tin nhắn: ${msgs.length}\n`
  text += '─'.repeat(50) + '\n\n'
  for (const m of msgs) {
    const time = new Date(m.sent_at).toLocaleString('vi-VN')
    text += `[${time}] ${m.sender_name || m.sender_type}: ${m.content || ''}\n\n`
  }

  // Add evaluation if available
  if (qcGroups.value.length > 0) {
    text += '─'.repeat(50) + '\n'
    for (const g of qcGroups.value) {
      const verdict = getQcVerdict(g)
      text += `Đánh giá (${g.job_name}): ${verdict === 'PASS' ? 'Đạt' : verdict === 'SKIP' ? 'Bỏ qua' : 'Không đạt'}\n`
      const review = getQcReview(g)
      if (review) text += `Nhận xét: ${review}\n`
    }
  }

  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `chat-${conv.customer_name || conv.id}.txt`
  a.click()
  URL.revokeObjectURL(url)
}

// Evaluation groups by type
const qcGroups = computed(() => {
  if (!evaluation.value?.groups) return []
  return evaluation.value.groups.filter((g: any) => g.job_type === 'qc_analysis')
})
const classGroups = computed(() => {
  if (!evaluation.value?.groups) return []
  return evaluation.value.groups.filter((g: any) => g.job_type === 'classification')
})

const MSG_TAG_COLORS = ['#7E57C2', '#1E88E5', '#00897B', '#FB8C00', '#D81B60', '#00ACC1', '#3949AB', '#E64A19', '#7CB342', '#6D4C41']
const allClassTags = computed(() => {
  const tagSet = new Set<string>()
  for (const g of classGroups.value) {
    for (const t of getClassTags(g)) tagSet.add(t)
  }
  return Array.from(tagSet).sort()
})
function msgTagColor(tag: string): string {
  const idx = allClassTags.value.indexOf(tag)
  return idx >= 0 ? MSG_TAG_COLORS[idx % MSG_TAG_COLORS.length] : MSG_TAG_COLORS[0]
}

// QC group helpers
function getQcVerdict(g: any): string {
  const ev = g.results?.find((r: any) => r.result_type === 'conversation_evaluation')
  return ev?.severity || ''
}
function getQcScore(g: any): number | null {
  const ev = g.results?.find((r: any) => r.result_type === 'conversation_evaluation')
  if (!ev?.detail) return null
  try { return JSON.parse(ev.detail)?.score ?? null } catch { return null }
}
function getQcReview(g: any): string {
  const ev = g.results?.find((r: any) => r.result_type === 'conversation_evaluation')
  return ev?.evidence || ''
}
function getQcViolations(g: any): any[] {
  return (g.results || []).filter((r: any) => r.result_type === 'qc_violation')
}
const expandedQc = ref<Record<string, boolean>>({})
function toggleQcExpand(runId: string) {
  expandedQc.value[runId] = !expandedQc.value[runId]
}

// Classification group helpers
function getClassTags(g: any): string[] {
  return (g.results || [])
    .filter((r: any) => r.result_type === 'classification_tag')
    .map((r: any) => r.rule_name || r.evidence || '')
    .filter(Boolean)
}
function getClassSummary(g: any): string {
  const ev = g.results?.find((r: any) => r.result_type === 'conversation_evaluation')
  if (!ev?.detail) return ''
  try { return JSON.parse(ev.detail)?.summary ?? '' } catch { return ev?.evidence || '' }
}

const filterChannelType = ref<string | null>(null)
const filterChannelId = ref<string | null>(null)
const filterEvaluation = ref<string | null>(null)

// Export
const showExportDialog = ref(false)
const exporting = ref(false)
const exportFormat = ref('txt')
const exportChannelType = ref('')
const exportFrom = ref(new Date(Date.now() - 7 * 86400000).toISOString().slice(0, 10))
const exportTo = ref(new Date().toISOString().slice(0, 10))
const evaluationFilterOptions = [
  { title: 'Đã đánh giá', value: 'evaluated' },
  { title: 'Chưa đánh giá', value: 'not_evaluated' },
  { title: 'Đạt', value: 'PASS' },
  { title: 'Không đạt', value: 'FAIL' },
]
const snackbar = ref(false)
const lightboxSrc = ref('')
const snackText = ref('')
const searchQuery = ref('')
const messagesContainer = ref<HTMLElement | null>(null)

const perPage = 9

const channelTypes = [
  { title: 'Facebook Fanpage', value: 'facebook' },
  { title: 'Zalo OA', value: 'zalo_oa' },
]

const channelOptions = computed(() => {
  let filtered = channelStore.channels
  if (filterChannelType.value) {
    filtered = filtered.filter(c => c.channel_type === filterChannelType.value)
  }
  return filtered.map(c => ({ title: c.name, value: c.id }))
})

const totalPages = computed(() => Math.ceil(conversationStore.total / perPage))

let searchTimeout: ReturnType<typeof setTimeout> | null = null
function debouncedSearch() {
  if (searchTimeout) clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    currentPage.value = 1
    loadConversations()
  }, 300)
}

async function loadConversations() {
  loading.value = true
  try {
    const params: Record<string, string | number> = {
      page: currentPage.value,
      per_page: perPage,
    }
    if (filterChannelType.value) params.channel_type = filterChannelType.value
    if (filterChannelId.value) params.channel_id = filterChannelId.value
    if (searchQuery.value) params.search = searchQuery.value
    if (filterEvaluation.value) params.evaluation = filterEvaluation.value

    await conversationStore.fetchConversations(tenantId.value, params)
  } finally {
    loading.value = false
  }
}

async function selectConversation(convId: string, tab?: string) {
  selectedConvId.value = convId
  detailTab.value = tab === 'evaluation' ? 'qc' : tab === 'classification' ? 'classification' : 'messages'
  const conv = conversationStore.conversations.find(c => c.id === convId)
  if (conv) selectedConvChannelType.value = conv.channel_type

  loadingMessages.value = true
  try {
    await conversationStore.fetchMessages(tenantId.value, convId)
    await nextTick()
    scrollToBottom()
  } finally {
    loadingMessages.value = false
  }

  // Load evaluation in background
  loadingEvaluation.value = true
  evaluation.value = null
  try {
    const { data } = await api.get(`/tenants/${tenantId.value}/conversations/${convId}/evaluations`)
    evaluation.value = data
  } catch {
    evaluation.value = null
  } finally {
    loadingEvaluation.value = false
  }
}

function scrollToBottom() {
  if (messagesContainer.value) {
    messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
  }
}

const dayNamesShort = ['CN', 'T2', 'T3', 'T4', 'T5', 'T6', 'T7']

function formatTime(dateStr: string | null) {
  if (!dateStr) return '—'
  const d = new Date(dateStr)
  const day = dayNamesShort[d.getDay()]
  const dd = String(d.getDate()).padStart(2, '0')
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const hh = String(d.getHours()).padStart(2, '0')
  const mi = String(d.getMinutes()).padStart(2, '0')
  return `${day} ${dd}/${mm}/${d.getFullYear()} ${hh}:${mi}`
}

function timeAgo(dateStr: string | null) {
  if (!dateStr) return '—'
  const diff = Date.now() - new Date(dateStr).getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return 'Vừa xong'
  if (mins < 60) return `${mins} phút trước`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours} giờ trước`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days} ngày trước`
  return formatTime(dateStr)
}

function formatMessageTime(dateStr: string) {
  const d = new Date(dateStr)
  const day = dayNamesShort[d.getDay()]
  const dd = String(d.getDate()).padStart(2, '0')
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const hh = String(d.getHours()).padStart(2, '0')
  const mi = String(d.getMinutes()).padStart(2, '0')
  return `${day} ${dd}/${mm} ${hh}:${mi}`
}

function hasAttachments(msg: Message) {
  if (!msg.attachments || msg.attachments === '[]' || msg.attachments === 'null') return false
  try {
    const arr = JSON.parse(msg.attachments)
    return Array.isArray(arr) && arr.length > 0
  } catch {
    return false
  }
}


function getAttachmentUrl(att: any): string {
  if (att.local_path) return `/api/v1/files/${att.local_path}`
  return att.url || ''
}

// Auth image loading: fetch with JWT header, create blob URL
const authImageCache = ref<Record<string, string>>({})

async function loadAuthImage(url: string) {
  if (!url || authImageCache.value[url]) return
  // For external URLs (not /api/), load directly
  if (!url.startsWith('/api/')) {
    authImageCache.value[url] = url
    return
  }
  authImageCache.value[url] = 'loading'
  try {
    const token = localStorage.getItem('cqa_access_token')
    const resp = await fetch(url, {
      headers: token ? { 'Authorization': `Bearer ${token}` } : {},
    })
    if (resp.ok) {
      const blob = await resp.blob()
      authImageCache.value[url] = URL.createObjectURL(blob)
    } else {
      delete authImageCache.value[url] // allow retry
    }
  } catch {
    delete authImageCache.value[url] // allow retry
  }
}

// Load auth images when messages change
watch(() => conversationStore.messages, () => {
  const msgs = conversationStore.messages
  if (!msgs || !Array.isArray(msgs)) return
  for (const msg of msgs) {
    if (msg.attachments) {
      try {
        const atts = typeof msg.attachments === 'string' ? JSON.parse(msg.attachments) : msg.attachments
        if (!Array.isArray(atts)) continue
        for (const att of atts) {
          if (isImageAttachment(att)) {
            const url = getAttachmentUrl(att)
            if (url) loadAuthImage(url)
          }
        }
      } catch { continue }
    }
  }
}, { immediate: true })

onUnmounted(() => {
  // Cleanup blob URLs to prevent memory leaks
  for (const blobUrl of Object.values(authImageCache.value)) {
    if (blobUrl && blobUrl.startsWith('blob:')) URL.revokeObjectURL(blobUrl)
  }
})

function isImageAttachment(att: any): boolean {
  if (!att.type) return false
  const t = att.type.toLowerCase()
  return t.startsWith('image') || t === 'photo' || t === 'gif' || t === 'sticker'
}

function onImageError(event: Event, att: any) {
  const img = event.target as HTMLImageElement
  // Replace broken image with fallback chip
  const fallback = document.createElement('span')
  fallback.className = 'v-chip v-chip--size-x-small v-theme--light v-chip--density-default v-chip--variant-tonal'
  fallback.style.cssText = 'font-size: 10px; padding: 0 8px; height: 24px; display: inline-flex; align-items: center; border-radius: 12px; background: rgba(0,0,0,0.06);'
  fallback.textContent = att.name || '[Ảnh]'
  img.replaceWith(fallback)
}

function parseAttachments(msg: Message) {
  try {
    return JSON.parse(msg.attachments) || []
  } catch {
    return []
  }
}

watch(currentPage, () => loadConversations())
watch(filterChannelType, () => {
  currentPage.value = 1
  filterChannelId.value = null
  loadConversations()
})
watch(filterChannelId, () => {
  currentPage.value = 1
  loadConversations()
})

watch(filterEvaluation, () => {
  currentPage.value = 1
  loadConversations()
})

onMounted(async () => {
  // Reset pagination state
  currentPage.value = 1
  conversationStore.total = 0

  // Pre-fill filter from query params (e.g. from channels page)
  if (route.query.channel_id) {
    filterChannelId.value = route.query.channel_id as string
  }

  await channelStore.fetchChannels(tenantId.value)
  await loadConversations()
  await loadEvaluationMap()

  // Auto-select conversation from query param (deep link)
  if (route.query.conv) {
    const convId = route.query.conv as string
    try {
      // Find which page this conversation is on
      const { data } = await api.get(`/tenants/${tenantId.value}/conversations/${convId}/page`, { params: { per_page: perPage } })
      if (data?.page && data.page !== currentPage.value) {
        currentPage.value = data.page
        await loadConversations()
      }
    } catch { /* fallback: stay on page 1 */ }
    selectConversation(convId, route.query.tab as string)
  }
})
</script>

<style scoped>
.lightbox-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100vw;
  height: 100vh;
  background: rgba(0, 0, 0, 0.85);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  cursor: pointer;
}
.lightbox-img {
  max-width: 90vw;
  max-height: 90vh;
  object-fit: contain;
  border-radius: 8px;
  cursor: default;
}
.lightbox-close {
  position: fixed;
  top: 16px;
  right: 16px;
}
</style>
