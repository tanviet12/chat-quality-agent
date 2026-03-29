<template>
  <div>
    <!-- Header -->
    <div class="d-flex align-center mb-4">
      <v-btn icon="mdi-arrow-left" variant="text" size="small" :to="`/${tenantId}/jobs`" />
      <h1 class="text-subtitle-1 text-md-h5 font-weight-bold ml-1 flex-grow-1" style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">{{ job?.name || '...' }}</h1>
      <template v-if="authStore.canEdit('jobs')">
        <v-tooltip v-if="!mdAndUp" text="Sửa" location="bottom">
          <template #activator="{ props }">
            <v-btn v-bind="props" variant="outlined" icon="mdi-pencil" size="small" :to="`/${tenantId}/jobs/${jobId}/edit`" class="ml-1" />
          </template>
        </v-tooltip>
        <v-btn v-else variant="outlined" prepend-icon="mdi-pencil" size="small" :to="`/${tenantId}/jobs/${jobId}/edit`" class="ml-2">{{ $t('edit') }}</v-btn>

        <v-tooltip v-if="!mdAndUp" text="Chạy thử" location="bottom">
          <template #activator="{ props }">
            <v-btn v-bind="props" variant="outlined" color="primary" icon="mdi-test-tube" size="small" :loading="isJobRunning" :disabled="isJobRunning" class="ml-1" @click="testRun" />
          </template>
        </v-tooltip>
        <v-btn v-else variant="outlined" color="primary" prepend-icon="mdi-test-tube" size="small" :loading="isJobRunning" :disabled="isJobRunning" class="ml-2" @click="testRun">Chạy thử (3 hội thoại)</v-btn>

        <v-tooltip v-if="!mdAndUp" text="Chạy ngay" location="bottom">
          <template #activator="{ props }">
            <v-btn v-bind="props" color="primary" icon="mdi-play" size="small" :disabled="isJobRunning" class="ml-1" @click="openRunDialog" />
          </template>
        </v-tooltip>
        <v-btn v-else color="primary" prepend-icon="mdi-play" size="small" :disabled="isJobRunning" class="ml-2" @click="openRunDialog">{{ $t('run_now') }}</v-btn>

        <v-btn v-if="isJobRunning" color="error" variant="outlined" prepend-icon="mdi-stop" size="small" class="ml-2" :loading="cancelling" @click="cancelJob">{{ mdAndUp ? 'Dừng' : '' }}</v-btn>
      </template>
    </div>

    <!-- Run options dialog -->
    <v-dialog v-model="runDialog" max-width="560">
      <v-card>
        <v-card-title>{{ $t('run_now') }}</v-card-title>
        <v-card-text>
          <v-radio-group v-model="runMode" class="mb-1">
            <v-radio value="unanalyzed">
              <template #label>
                <div>
                  <div class="font-weight-medium">Chạy cho những cuộc chat chưa được đánh giá</div>
                  <div class="text-caption text-grey">Đánh giá tất cả cuộc chat chưa được công việc này phân tích lần nào, bất kể thời gian.</div>
                </div>
              </template>
            </v-radio>
            <v-radio value="since_last">
              <template #label>
                <div>
                  <div class="font-weight-medium">Chạy từ lần gần nhất</div>
                  <div class="text-caption text-grey">Lấy cuộc chat gần nhất đã đánh giá làm mốc. Cuộc chat cũ hơn mốc sẽ không được đánh giá dù chưa phân tích.</div>
                </div>
              </template>
            </v-radio>
            <v-radio value="conditional">
              <template #label>
                <div>
                  <div class="font-weight-medium">Chạy theo điều kiện</div>
                  <div class="text-caption text-grey">Chọn điều kiện thời gian và/hoặc giới hạn số cuộc chat. Phải có ít nhất một điều kiện.</div>
                </div>
              </template>
            </v-radio>
          </v-radio-group>

          <!-- Conditional fields -->
          <template v-if="runMode === 'conditional'">
            <div class="d-flex ga-3 mt-2">
              <v-text-field
                v-model="runDateFrom"
                type="date"
                label="Từ ngày"
                density="compact"
                :error-messages="runDateFromError"
                hide-details="auto"
              />
              <v-text-field
                v-model="runDateTo"
                type="date"
                label="Đến ngày"
                density="compact"
                :error-messages="runDateToError"
                hide-details="auto"
              />
            </div>
            <v-text-field
              v-model.number="runLimit"
              type="number"
              label="Giới hạn số cuộc chat"
              density="compact"
              hide-details="auto"
              class="mt-3"
              placeholder="Để trống nếu không muốn áp dụng"
              :min="1"
              clearable
            />
            <v-alert v-if="runConditionalError" type="error" variant="tonal" density="compact" class="mt-3 text-caption">
              {{ runConditionalError }}
            </v-alert>
          </template>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="runDialog = false">{{ $t('cancel') }}</v-btn>
          <v-btn color="primary" :disabled="!!runConditionalError && runMode === 'conditional'" @click="confirmRun">{{ $t('confirm') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Job Info Card -->
    <v-card class="pa-4 mb-4" v-if="job">
      <div class="text-subtitle-1 font-weight-bold mb-3">
        <v-icon start size="small">mdi-information</v-icon>
        {{ $t('job_info') }}
      </div>
      <v-row dense>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">{{ $t('job_type') }}</div>
          <v-chip size="small" :color="job.job_type === 'qc_analysis' ? 'primary' : 'secondary'" variant="tonal">
            {{ job.job_type === 'qc_analysis' ? $t('job_qc') : $t('job_classification') }}
          </v-chip>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">{{ $t('ai_model') }}</div>
          <div class="text-body-2">{{ tenantAIProvider }} / {{ tenantAIModel }}</div>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">{{ $t('job_wizard_step_analysis_schedule') }}</div>
          <div class="text-body-2">{{ formatSchedule(job.schedule_type, job.schedule_cron) }}</div>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">{{ $t('status') }}</div>
          <v-chip size="small" :color="job.is_active ? 'success' : 'grey'" variant="tonal">
            {{ job.is_active ? $t('active') : $t('inactive') }}
          </v-chip>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">{{ $t('job_input_channels') }}</div>
          <div class="text-body-2">{{ parsedChannelCount }} kênh</div>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">{{ $t('job_output') }}</div>
          <div class="d-flex flex-wrap ga-1">
            <v-chip v-for="(o, i) in parsedOutputs" :key="i" size="x-small" variant="tonal" :prepend-icon="o.type === 'telegram' ? 'mdi-send' : 'mdi-email'">
              {{ o.type === 'telegram' ? 'Telegram' : 'Email' }}
            </v-chip>
            <span v-if="!parsedOutputs.length" class="text-body-2 text-grey">—</span>
          </div>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">{{ $t('job_last_run') }}</div>
          <div class="text-body-2" v-if="job.last_run_at">
            {{ formatDateTime(job.last_run_at) }}
            <v-chip size="x-small" :color="statusColor(job.last_run_status)" variant="tonal" class="ml-1">{{ job.last_run_status }}</v-chip>
          </div>
          <div v-else class="text-body-2 text-grey">—</div>
        </v-col>
        <v-col cols="6" sm="3">
          <div class="text-caption text-grey">{{ $t('job_created_at') }}</div>
          <div class="text-body-2">{{ formatDateTime(job.created_at) }}</div>
        </v-col>
      </v-row>
    </v-card>

    <!-- Progress bar for running job -->
    <v-alert v-if="currentRunProgress" type="info" variant="tonal" class="mb-4">
      <v-progress-linear :model-value="progressPercent" color="primary" height="8" rounded class="mb-2" />
      <div class="text-body-2">
        Đang phân tích {{ currentRunProgress.analyzed }}/{{ currentRunProgress.total }} cuộc hội thoại
        <span v-if="currentRunProgress.passed"> — {{ currentRunProgress.passed }} đạt</span>
        <span v-if="currentRunProgress.errors"> — {{ currentRunProgress.errors }} lỗi</span>
      </div>
    </v-alert>

    <!-- KPI Stat Cards: QC -->
    <v-row class="mb-4" v-if="groupedResults.length && !isClassification">
      <v-col cols="6" sm="3">
        <v-card class="pa-4">
          <div class="d-flex justify-space-between align-center">
            <div>
              <div class="text-body-2 text-grey">{{ $t('conversations_analyzed') }}</div>
              <div class="text-h5 font-weight-bold mt-1">{{ aggregateStats.analyzed }}</div>
            </div>
            <v-icon color="primary" size="32" class="opacity-50">mdi-message-text</v-icon>
          </div>
        </v-card>
      </v-col>
      <v-col cols="6" sm="3">
        <v-card class="pa-4">
          <div class="d-flex justify-space-between align-center">
            <div>
              <div class="text-body-2 text-grey">{{ $t('job_pass_rate') }}</div>
              <div class="text-h5 font-weight-bold mt-1">{{ aggregateStats.passRate }}%</div>
            </div>
            <v-icon color="success" size="32" class="opacity-50">mdi-check-circle</v-icon>
          </div>
        </v-card>
      </v-col>
      <v-col cols="6" sm="3">
        <v-card class="pa-4">
          <div class="d-flex justify-space-between align-center">
            <div>
              <div class="text-body-2 text-grey">{{ $t('issues_found') }}</div>
              <div class="text-h5 font-weight-bold mt-1">{{ aggregateStats.issues }}</div>
            </div>
            <v-icon color="error" size="32" class="opacity-50">mdi-alert-circle</v-icon>
          </div>
        </v-card>
      </v-col>
      <v-col cols="6" sm="3">
        <v-card class="pa-4">
          <div class="d-flex justify-space-between align-center">
            <div>
              <div class="text-body-2 text-grey">{{ $t('job_avg_score') }}</div>
              <div class="text-h5 font-weight-bold mt-1">{{ aggregateStats.avgScore }}/100</div>
            </div>
            <v-icon color="warning" size="32" class="opacity-50">mdi-star</v-icon>
          </div>
        </v-card>
      </v-col>
    </v-row>

    <!-- KPI Stat Cards: Classification -->
    <v-row class="mb-4" v-if="groupedResults.length && isClassification">
      <v-col cols="6" sm="4">
        <v-card class="pa-4">
          <div class="d-flex justify-space-between align-center">
            <div>
              <div class="text-body-2 text-grey">Tổng cuộc chat</div>
              <div class="text-h5 font-weight-bold mt-1">{{ groupedResults.length }}</div>
            </div>
            <v-icon color="primary" size="32" class="opacity-50">mdi-message-text</v-icon>
          </div>
        </v-card>
      </v-col>
      <v-col cols="6" sm="4">
        <v-card class="pa-4">
          <div class="d-flex justify-space-between align-center">
            <div>
              <div class="text-body-2 text-grey">Đã phân loại</div>
              <div class="text-h5 font-weight-bold mt-1">{{ groupedResults.filter(g => g.verdict !== 'SKIP').length }}</div>
            </div>
            <v-icon color="secondary" size="32" class="opacity-50">mdi-tag-check</v-icon>
          </div>
        </v-card>
      </v-col>
      <v-col cols="6" sm="4">
        <v-card class="pa-4">
          <div class="d-flex justify-space-between align-center">
            <div>
              <div class="text-body-2 text-grey">Bỏ qua</div>
              <div class="text-h5 font-weight-bold mt-1">{{ groupedResults.filter(g => g.verdict === 'SKIP').length }}</div>
            </div>
            <v-icon color="grey" size="32" class="opacity-50">mdi-tag-off</v-icon>
          </div>
        </v-card>
      </v-col>
    </v-row>

    <!-- Trend Chart (QC only) -->
    <v-card class="pa-4 mb-4" v-if="jobStore.jobResults.length > 0 && !isClassification">
      <div class="text-subtitle-1 font-weight-bold mb-3">
        <v-icon start size="small">mdi-chart-line</v-icon>
        {{ $t('job_trend') }}
      </div>
      <div style="max-height: 200px;">
        <Line :data="trendChartData" :options="chartOptions" />
      </div>
    </v-card>

    <!-- Tabbed Content: History + Results -->
    <v-card class="pa-4">
      <v-tabs v-model="activeTab" density="compact" class="mb-3">
        <v-tab value="results">
          <v-icon start size="small">mdi-magnify</v-icon>
          {{ $t('tab_results') }}
        </v-tab>
        <v-tab value="history">
          <v-icon start size="small">mdi-history</v-icon>
          {{ $t('run_history') }}
        </v-tab>
      </v-tabs>

      <!-- Tab: Results -->
      <div v-if="activeTab === 'results'">
        <!-- Filter + toolbar in one row -->
        <div class="d-flex align-center flex-wrap ga-2 mb-3">
          <!-- Filter chips: Classification -->
          <template v-if="isClassification">
            <v-chip size="small" :variant="resultFilter === 'classified' ? 'flat' : 'outlined'" :color="resultFilter === 'classified' ? 'secondary' : ''" @click="resultFilter = 'classified'; resultPage = 1">
              Đã phân loại: {{ groupedResults.filter(g => g.verdict !== 'SKIP').length }}
            </v-chip>
            <v-chip size="small" :variant="resultFilter === 'all' ? 'flat' : 'outlined'" :color="resultFilter === 'all' ? 'primary' : ''" @click="resultFilter = 'all'; resultPage = 1">
              {{ $t('filter_all') }}: {{ groupedResults.length }}
            </v-chip>
            <v-chip size="small" :variant="resultFilter === 'skip' ? 'flat' : 'outlined'" :color="resultFilter === 'skip' ? 'grey' : ''" @click="resultFilter = 'skip'; resultPage = 1">
              Bỏ qua: {{ groupedResults.filter(g => g.verdict === 'SKIP').length }}
            </v-chip>
            <v-select
              v-model="tagFilter"
              :items="availableTags"
              label="Lọc loại"
              clearable
              density="compact"
              variant="outlined"
              hide-details
              style="max-width: 200px;"
            />
          </template>
          <!-- Filter chips: QC -->
          <template v-else>
            <v-chip size="small" :variant="resultFilter === 'all' ? 'flat' : 'outlined'" :color="resultFilter === 'all' ? 'primary' : ''" @click="resultFilter = 'all'; resultPage = 1">
              {{ $t('filter_all') }}: {{ groupedResults.length }}
            </v-chip>
            <v-chip size="small" :variant="resultFilter === 'fail' ? 'flat' : 'outlined'" :color="resultFilter === 'fail' ? 'error' : ''" @click="resultFilter = 'fail'; resultPage = 1">
              {{ $t('filter_failed') }}: {{ groupedResults.filter(g => g.verdict === 'FAIL').length }}
            </v-chip>
            <v-chip size="small" :variant="resultFilter === 'pass' ? 'flat' : 'outlined'" :color="resultFilter === 'pass' ? 'success' : ''" @click="resultFilter = 'pass'; resultPage = 1">
              {{ $t('filter_passed') }}: {{ groupedResults.filter(g => g.verdict === 'PASS').length }}
            </v-chip>
            <v-chip size="small" :variant="resultFilter === 'skip' ? 'flat' : 'outlined'" :color="resultFilter === 'skip' ? 'grey' : ''" @click="resultFilter = 'skip'; resultPage = 1">
              Bỏ qua: {{ groupedResults.filter(g => g.verdict === 'SKIP').length }}
            </v-chip>
          </template>
          <v-btn v-if="selectedRunId" variant="text" size="small" prepend-icon="mdi-refresh" color="primary" @click="loadAllResults">
            {{ $t('all_results') }}
          </v-btn>
          <v-spacer />
          <v-btn-toggle v-model="viewMode" mandatory density="compact" variant="outlined" class="mr-2">
            <v-btn value="card" size="small"><v-icon size="small">mdi-view-list</v-icon></v-btn>
            <v-btn value="table" size="small"><v-icon size="small">mdi-table</v-icon></v-btn>
          </v-btn-toggle>
          <v-btn variant="outlined" size="x-small" prepend-icon="mdi-file-delimited" @click="exportResults('csv')">CSV</v-btn>
          <v-btn variant="outlined" size="x-small" prepend-icon="mdi-file-excel" @click="exportResults('xlsx')" class="mr-2">Excel</v-btn>
          <v-btn variant="outlined" size="x-small" prepend-icon="mdi-delete-sweep" color="error" @click="clearResultsDialog = true">
            Xóa kết quả
          </v-btn>
        </div>

        <div v-if="!filteredGroupedResults.length" class="text-center text-grey pa-4">
          {{ $t('no_issues') }}
        </div>
        <!-- Table view: Classification -->
        <div v-else-if="viewMode === 'table' && isClassification">
          <v-table density="compact" hover>
            <thead>
              <tr>
                <th>Tên</th>
                <th>Ngày chat</th>
                <th style="min-width: 200px">Loại</th>
                <th style="min-width: 300px">Đánh giá chi tiết</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="group in paginatedResults" :key="group.conversationId" style="cursor:pointer" @click="openDetail(group)">
                <td class="text-body-2">{{ group.customerName || group.conversationId.substring(0, 8) + '...' }}</td>
                <td class="text-body-2 text-no-wrap">{{ formatTime(group.conversationDate) }}</td>
                <td class="text-body-2" style="white-space: pre-line;">{{ group.tags.length ? group.tags.map(t => '- ' + t).join('\n') : group.verdict === 'SKIP' ? 'Bỏ qua' : '—' }}</td>
                <td class="text-body-2" style="white-space: normal; max-width: 400px;">{{ classificationSummary(group) }}</td>
              </tr>
            </tbody>
          </v-table>
          <v-pagination v-if="totalResultPages > 1" v-model="resultPage" :length="totalResultPages" :total-visible="5" density="compact" class="mt-3" />
        </div>
        <!-- Table view: QC -->
        <div v-else-if="viewMode === 'table'">
          <v-table density="compact" hover>
            <thead>
              <tr>
                <th>Tên</th>
                <th>Ngày chat</th>
                <th>Kết quả</th>
                <th style="max-width: 300px">Đánh giá</th>
                <th>Điểm</th>
                <th>Vấn đề</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="group in paginatedResults" :key="group.conversationId" style="cursor:pointer" @click="openDetail(group)">
                <td class="text-body-2">{{ group.customerName || group.conversationId.substring(0, 8) + '...' }}</td>
                <td class="text-body-2 text-no-wrap">{{ formatTime(group.conversationDate) }}</td>
                <td>
                  <v-chip size="x-small" :color="group.verdict === 'PASS' ? 'success' : group.verdict === 'SKIP' ? 'grey' : 'error'" variant="tonal">
                    {{ group.verdict === 'PASS' ? 'Đạt' : group.verdict === 'SKIP' ? 'Bỏ qua' : 'Không đạt' }}
                  </v-chip>
                </td>
                <td class="text-body-2" style="max-width: 300px; white-space: normal;">
                  <span style="display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;">{{ group.review }}</span>
                </td>
                <td>
                  <v-chip v-if="group.score != null" size="x-small" :color="group.score >= 80 ? 'success' : group.score >= 50 ? 'warning' : 'error'" variant="tonal">
                    {{ group.score }}/100
                  </v-chip>
                  <span v-else class="text-grey">—</span>
                </td>
                <td class="text-body-2">{{ group.violations.length > 0 ? group.violations.length + ' vấn đề' : '—' }}</td>
              </tr>
            </tbody>
          </v-table>
          <v-pagination v-if="totalResultPages > 1" v-model="resultPage" :length="totalResultPages" :total-visible="5" density="compact" class="mt-3" />
        </div>
        <div v-else>
          <v-card v-for="group in paginatedResults" :key="group.conversationId" variant="outlined" class="mb-3">
            <!-- Conversation header -->
            <div class="d-flex align-center pa-3" style="cursor: pointer" @click="toggleExpand(group.conversationId)">
              <!-- Classification card header -->
              <template v-if="isClassification">
                <v-chip v-if="group.verdict === 'SKIP'" size="small" color="grey" variant="tonal" class="mr-3">Bỏ qua</v-chip>
                <v-chip v-else size="small" color="success" variant="tonal" class="mr-3">Đã phân loại</v-chip>
                <div class="flex-grow-1">
                  <div class="d-flex align-center ga-2">
                    <span class="font-weight-medium text-body-2">{{ group.customerName || group.conversationId.substring(0, 8) + '...' }}</span>
                    <span class="text-caption text-grey">{{ formatTime(group.conversationDate) }}</span>
                  </div>
                  <div v-if="group.tags.length" class="d-flex flex-wrap ga-1 mt-1">
                    <v-chip v-for="tag in group.tags" :key="tag" size="x-small" :color="tagColor(tag)" variant="tonal">{{ tag }}</v-chip>
                  </div>
                  <div v-if="classificationSummary(group) !== '—'" class="text-caption text-grey-darken-1 mt-1" style="max-width: 600px; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;">{{ classificationSummary(group) }}</div>
                </div>
              </template>
              <!-- QC card header -->
              <template v-else>
                <v-chip size="small" :color="group.verdict === 'PASS' ? 'success' : group.verdict === 'SKIP' ? 'grey' : 'error'" variant="tonal" class="mr-3">
                  {{ group.verdict === 'PASS' ? $t('verdict_pass') : group.verdict === 'SKIP' ? 'Bỏ qua' : $t('verdict_fail') }}
                </v-chip>
                <div class="flex-grow-1">
                  <div class="d-flex align-center ga-2">
                    <span class="font-weight-medium text-body-2">{{ group.customerName || group.conversationId.substring(0, 8) + '...' }}</span>
                    <span class="text-caption text-grey">{{ formatTime(group.conversationDate) }}</span>
                  </div>
                  <div v-if="group.review" class="text-caption text-grey-darken-1 mt-1" style="max-width: 600px; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;">{{ group.review }}</div>
                </div>
                <v-chip v-if="group.score != null" size="x-small" :color="group.score >= 80 ? 'success' : group.score >= 50 ? 'warning' : 'error'" variant="tonal" class="mr-2">
                  {{ group.score }}/100
                </v-chip>
              </template>
              <span class="text-caption text-grey mr-2">{{ group.violations.length }} {{ $t('issues_label') }}</span>
              <v-icon>{{ expandedMap[group.conversationId] ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon>
            </div>

            <!-- Expanded: Transcript + Violations side-by-side -->
            <div v-if="expandedMap[group.conversationId]" class="px-3 pb-3">
              <v-divider class="mb-3" />
              <v-row>
                <!-- Left: Chat transcript -->
                <v-col cols="12" md="7">
                  <div class="d-flex align-center mb-2">
                    <div class="text-caption text-grey font-weight-bold">
                      <v-icon size="x-small" class="mr-1">mdi-chat</v-icon>
                      Diễn biến cuộc chat
                    </div>
                    <v-btn :to="`/${tenantId}/messages?conv=${group.conversationId}`" variant="text" size="x-small" color="primary" class="ml-2 pa-0" style="min-width: 0; height: auto;">
                      <v-icon size="x-small" class="mr-1">mdi-open-in-new</v-icon>Xem tại Tin nhắn
                    </v-btn>
                  </div>
                  <div v-if="!chatMessages[group.conversationId]" class="text-center pa-4">
                    <v-progress-circular indeterminate size="24" />
                    <div class="text-caption text-grey mt-2">Đang tải...</div>
                  </div>
                  <div v-else class="chat-transcript pa-2 rounded" style="background: #f5f5f5; max-height: 500px; overflow-y: auto;">
                    <div v-for="msg in chatMessages[group.conversationId]" :key="msg.id" class="mb-2">
                      <div
                        class="pa-2 rounded"
                        :class="msg.sender_type === 'agent' ? 'bg-blue-lighten-5 ml-8' : 'bg-white mr-8'"
                        :style="isHighlighted(group, msg) ? 'border: 2px solid #ff9800;' : 'border: 1px solid #e0e0e0;'"
                      >
                        <div class="d-flex align-center mb-1">
                          <span class="text-caption font-weight-bold" :class="msg.sender_type === 'agent' ? 'text-blue' : 'text-grey-darken-2'">
                            {{ msg.sender_name }}
                          </span>
                          <v-spacer />
                          <span class="text-caption text-grey">{{ formatTime(msg.sent_at) }}</span>
                        </div>
                        <div v-if="msg.content" class="text-body-2" style="font-size: 13px;">{{ msg.content }}</div>
                        <div v-if="msg.content_type === 'sticker'" class="text-caption font-italic">[Sticker]</div>
                        <div v-if="hasAttachments(msg)" class="mt-1">
                          <template v-for="(att, ai) in parseAttachments(msg)" :key="ai">
                            <div v-if="isImageAttachment(att)" class="mb-1">
                              <img v-if="authImageCache[getAttachmentUrl(att)] && authImageCache[getAttachmentUrl(att)] !== 'loading'" :src="authImageCache[getAttachmentUrl(att)]" style="max-width: 180px; max-height: 180px; border-radius: 8px; cursor: pointer;" @click="lightboxSrc = authImageCache[getAttachmentUrl(att)]" />
                              <v-progress-circular v-else-if="authImageCache[getAttachmentUrl(att)] === 'loading'" indeterminate size="20" width="2" class="ma-2" />
                            </div>
                            <v-chip v-else size="x-small" variant="tonal" class="mr-1" :href="getAttachmentUrl(att)" target="_blank"><v-icon start size="12">mdi-paperclip</v-icon>{{ att.name || 'File' }}</v-chip>
                          </template>
                        </div>
                        <div v-if="!msg.content && !hasAttachments(msg) && msg.content_type !== 'text'" class="text-caption font-italic">[{{ msg.content_type || 'File' }}]</div>
                      </div>
                    </div>
                  </div>
                </v-col>

                <!-- Right: Violations -->
                <v-col cols="12" md="5">
                  <div class="text-caption text-grey font-weight-bold mb-2">
                    <v-icon size="x-small" class="mr-1">mdi-alert-circle</v-icon>
                    Đánh giá chi tiết
                  </div>
                  <v-alert v-if="group.review" :type="group.verdict === 'PASS' ? 'success' : 'warning'" variant="tonal" density="compact" class="mb-3 text-body-2">
                    {{ group.review }}
                  </v-alert>
                  <div v-for="(v, idx) in group.violations" :key="idx" class="mb-3">
                    <div class="d-flex align-center mb-1">
                      <v-chip size="x-small" :color="v.severity === 'NGHIEM_TRONG' ? 'error' : 'warning'" variant="tonal" class="mr-2">
                        {{ v.severity === 'NGHIEM_TRONG' ? $t('severity_critical') : $t('severity_warning') }}
                      </v-chip>
                      <span class="font-weight-medium text-body-2">{{ v.rule_name }}</span>
                    </div>
                    <div class="text-body-2 bg-orange-lighten-5 pa-2 rounded mb-1" style="font-size: 13px; border-left: 3px solid #ff9800;">
                      {{ v.evidence }}
                    </div>
                    <div v-if="parseDetail(v.detail)?.explanation" class="text-caption text-grey-darken-1">
                      {{ parseDetail(v.detail).explanation }}
                    </div>
                    <div v-if="parseDetail(v.detail)?.suggestion" class="text-caption text-success mt-1">
                      <v-icon size="x-small" class="mr-1">mdi-lightbulb</v-icon>
                      {{ parseDetail(v.detail).suggestion }}
                    </div>
                  </div>
                  <div v-if="!group.violations.length && group.verdict === 'PASS'" class="text-center text-grey pa-4">
                    <v-icon size="32" color="success">mdi-check-circle</v-icon>
                    <div class="text-body-2 mt-2">Cuộc chat đạt chất lượng</div>
                  </div>
                </v-col>
              </v-row>
            </div>
          </v-card>

          <v-pagination v-if="totalResultPages > 1" v-model="resultPage" :length="totalResultPages" :total-visible="5" density="compact" class="mt-3" />
        </div>
      </div>

      <!-- Tab: Run History -->
      <div v-if="activeTab === 'history'">
        <div class="d-flex align-center flex-wrap ga-2 mb-3">
          <v-spacer />
          <v-btn variant="outlined" size="small" prepend-icon="mdi-delete-sweep" color="error" @click="clearRunsDialog = true">
            Xóa lịch sử chạy
          </v-btn>
        </div>

        <v-table v-if="jobStore.jobRuns.length" density="compact">
          <thead>
            <tr>
              <th>{{ $t('sent_at') }}</th>
              <th>{{ $t('status') }}</th>
              <th>{{ $t('conversations_analyzed') }}</th>
              <th>{{ $t('conversations_passed') }}</th>
              <th>{{ $t('issues_found') }}</th>
              <th>{{ $t('actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="run in paginatedRuns" :key="run.id">
              <td class="text-body-2">{{ formatDateTime(run.started_at) }}</td>
              <td>
                <v-chip size="x-small" :color="statusColor(run.status)" variant="tonal">{{ run.status }}</v-chip>
              </td>
              <td>{{ parseSummary(run.summary).conversations_analyzed || 0 }}</td>
              <td>
                <span class="text-success font-weight-medium">{{ parseSummary(run.summary).conversations_passed || 0 }}</span>
                <span class="text-grey"> / {{ parseSummary(run.summary).conversations_analyzed || 0 }}</span>
              </td>
              <td>{{ parseSummary(run.summary).issues_found || 0 }}</td>
              <td>
                <span v-if="run.error_message" class="text-caption text-error">{{ run.error_message }}</span>
                <v-btn v-else size="small" variant="text" color="primary" @click="loadResults(run.id)">
                  {{ $t('view_results') }}
                </v-btn>
              </td>
            </tr>
          </tbody>
        </v-table>
        <v-pagination v-if="totalRunPages > 1" v-model="runPage" :length="totalRunPages" :total-visible="7" density="compact" class="mt-2" />
        <div v-if="!jobStore.jobRuns.length" class="text-center text-grey pa-4">{{ $t('no_runs') }}</div>
      </div>
    </v-card>

    <!-- Clear results dialog -->
    <v-dialog v-model="clearResultsDialog" max-width="450">
      <v-card class="pa-6">
        <v-card-title class="text-error">Xóa tất cả kết quả</v-card-title>
        <v-card-text>
          Xóa toàn bộ kết quả đánh giá và chi phí AI. Lịch sử chạy vẫn được giữ lại. Hành động không thể hoàn tác.
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="clearResultsDialog = false">{{ $t('cancel') }}</v-btn>
          <v-btn color="error" :loading="clearingResults" @click="clearResults">{{ $t('delete') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Detail dialog (table view row click) -->
    <v-dialog v-model="detailDialog" max-width="1000" scrollable>
      <v-card v-if="dialogGroup">
        <v-card-title class="d-flex align-center pa-4">
          <v-chip size="small" :color="dialogGroup.verdict === 'PASS' ? 'success' : dialogGroup.verdict === 'SKIP' ? 'grey' : 'error'" variant="tonal" class="mr-3">
            {{ dialogGroup.verdict === 'PASS' ? 'Đạt' : dialogGroup.verdict === 'SKIP' ? 'Bỏ qua' : 'Không đạt' }}
          </v-chip>
          <span class="text-body-1 font-weight-bold">{{ dialogGroup.customerName || dialogGroup.conversationId.substring(0, 8) + '...' }}</span>
          <span class="text-caption text-grey ml-2">{{ formatTime(dialogGroup.conversationDate) }}</span>
          <v-chip v-if="dialogGroup.score != null" size="x-small" :color="dialogGroup.score >= 80 ? 'success' : dialogGroup.score >= 50 ? 'warning' : 'error'" variant="tonal" class="ml-auto">
            {{ dialogGroup.score }}/100
          </v-chip>
        </v-card-title>
        <v-divider />
        <v-card-text class="pa-4">
          <v-row>
            <v-col cols="12" md="7">
              <div class="d-flex align-center mb-2">
                <div class="text-caption text-grey font-weight-bold">
                  <v-icon size="x-small" class="mr-1">mdi-chat</v-icon>
                  Diễn biến cuộc chat
                </div>
                <v-btn :to="`/${tenantId}/messages?conv=${dialogGroup.conversationId}`" variant="text" size="x-small" color="primary" class="ml-2 pa-0" style="min-width: 0; height: auto;" @click="detailDialog = false">
                  <v-icon size="x-small" class="mr-1">mdi-open-in-new</v-icon>Xem tại Tin nhắn
                </v-btn>
              </div>
              <div v-if="!chatMessages[dialogGroup.conversationId]" class="text-center pa-4">
                <v-progress-circular indeterminate size="24" />
                <div class="text-caption text-grey mt-2">Đang tải...</div>
              </div>
              <div v-else class="chat-transcript pa-2 rounded" style="background: #f5f5f5; max-height: 450px; overflow-y: auto;">
                <div v-for="msg in chatMessages[dialogGroup.conversationId]" :key="msg.id" class="mb-2">
                  <div
                    class="pa-2 rounded"
                    :class="msg.sender_type === 'agent' ? 'bg-blue-lighten-5 ml-8' : 'bg-white mr-8'"
                    :style="isHighlighted(dialogGroup, msg) ? 'border: 2px solid #ff9800;' : 'border: 1px solid #e0e0e0;'"
                  >
                    <div class="d-flex align-center mb-1">
                      <span class="text-caption font-weight-bold" :class="msg.sender_type === 'agent' ? 'text-blue' : 'text-grey-darken-2'">{{ msg.sender_name }}</span>
                      <v-spacer />
                      <span class="text-caption text-grey">{{ formatTime(msg.sent_at) }}</span>
                    </div>
                    <div v-if="msg.content" class="text-body-2" style="font-size: 13px;">{{ msg.content }}</div>
                    <div v-if="msg.content_type === 'sticker'" class="text-caption font-italic">[Sticker]</div>
                    <div v-if="hasAttachments(msg)" class="mt-1">
                      <template v-for="(att, ai) in parseAttachments(msg)" :key="ai">
                        <div v-if="isImageAttachment(att)" class="mb-1">
                          <img v-if="authImageCache[getAttachmentUrl(att)] && authImageCache[getAttachmentUrl(att)] !== 'loading'" :src="authImageCache[getAttachmentUrl(att)]" style="max-width: 180px; max-height: 180px; border-radius: 8px; cursor: pointer;" @click="lightboxSrc = authImageCache[getAttachmentUrl(att)]" />
                          <v-progress-circular v-else-if="authImageCache[getAttachmentUrl(att)] === 'loading'" indeterminate size="20" width="2" class="ma-2" />
                        </div>
                        <v-chip v-else size="x-small" variant="tonal" class="mr-1" :href="getAttachmentUrl(att)" target="_blank"><v-icon start size="12">mdi-paperclip</v-icon>{{ att.name || 'File' }}</v-chip>
                      </template>
                    </div>
                    <div v-if="!msg.content && !hasAttachments(msg) && msg.content_type !== 'text'" class="text-caption font-italic">[{{ msg.content_type || 'File' }}]</div>
                  </div>
                </div>
              </div>
            </v-col>
            <v-col cols="12" md="5">
              <div class="text-caption text-grey font-weight-bold mb-2">
                <v-icon size="x-small" class="mr-1">mdi-alert-circle</v-icon>
                Đánh giá chi tiết
              </div>
              <v-alert v-if="dialogGroup.review" :type="dialogGroup.verdict === 'PASS' ? 'success' : 'warning'" variant="tonal" density="compact" class="mb-3 text-body-2">
                {{ dialogGroup.review }}
              </v-alert>
              <div v-for="(v, idx) in dialogGroup.violations" :key="idx" class="mb-3">
                <div class="d-flex align-center mb-1">
                  <v-chip size="x-small" :color="v.severity === 'NGHIEM_TRONG' ? 'error' : 'warning'" variant="tonal" class="mr-2">
                    {{ v.severity === 'NGHIEM_TRONG' ? $t('severity_critical') : $t('severity_warning') }}
                  </v-chip>
                  <span class="font-weight-medium text-body-2">{{ v.rule_name }}</span>
                </div>
                <div class="text-body-2 bg-orange-lighten-5 pa-2 rounded mb-1" style="font-size: 13px; border-left: 3px solid #ff9800;">{{ v.evidence }}</div>
                <div v-if="parseDetail(v.detail)?.explanation" class="text-caption text-grey-darken-1">{{ parseDetail(v.detail).explanation }}</div>
                <div v-if="parseDetail(v.detail)?.suggestion" class="text-caption text-success mt-1">
                  <v-icon size="x-small" class="mr-1">mdi-lightbulb</v-icon>{{ parseDetail(v.detail).suggestion }}
                </div>
              </div>
              <div v-if="!dialogGroup.violations.length && dialogGroup.verdict === 'PASS'" class="text-center text-grey pa-4">
                <v-icon size="32" color="success">mdi-check-circle</v-icon>
                <div class="text-body-2 mt-2">Cuộc chat đạt chất lượng</div>
              </div>
            </v-col>
          </v-row>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="detailDialog = false">Đóng</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- AI not configured dialog -->
    <v-dialog v-model="aiNotConfiguredDialog" max-width="450">
      <v-card class="pa-6">
        <v-card-title>
          <v-icon start color="warning">mdi-alert</v-icon>
          Chưa cấu hình AI Provider
        </v-card-title>
        <v-card-text>
          Bạn cần cấu hình API key của AI Provider (Claude hoặc Gemini) trước khi chạy tác vụ phân tích.
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="aiNotConfiguredDialog = false">Đóng</v-btn>
          <v-btn color="primary" variant="flat" :to="`/${tenantId}/settings`" @click="aiNotConfiguredDialog = false">
            <v-icon start>mdi-cog</v-icon>
            Đi tới cài đặt
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Clear runs dialog -->
    <v-dialog v-model="clearRunsDialog" max-width="450">
      <v-card class="pa-6">
        <v-card-title class="text-error">Xóa lịch sử chạy</v-card-title>
        <v-card-text>
          Xóa toàn bộ lịch sử chạy, kết quả đánh giá và chi phí AI của công việc này. Hành động không thể hoàn tác.
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn @click="clearRunsDialog = false">{{ $t('cancel') }}</v-btn>
          <v-btn color="error" :loading="clearingRuns" @click="clearRuns">{{ $t('delete') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
    <!-- Lightbox overlay for image zoom -->
    <div v-if="lightboxSrc" class="lightbox-overlay" @click="lightboxSrc = ''">
      <img :src="lightboxSrc" class="lightbox-img" @click.stop />
      <v-btn icon="mdi-close" variant="flat" color="white" size="small" class="lightbox-close" @click="lightboxSrc = ''" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useDisplay } from 'vuetify'
import { useJobStore, type JobResult } from '../../stores/jobs'
import { useAuthStore } from '../../stores/auth'
import api from '../../api'
import { Line } from 'vue-chartjs'
import { Chart as ChartJS, CategoryScale, LinearScale, BarElement, PointElement, LineElement, Title, Tooltip, Filler, Legend } from 'chart.js'

ChartJS.register(CategoryScale, LinearScale, BarElement, PointElement, LineElement, Title, Tooltip, Filler, Legend)

const route = useRoute()
const { mdAndUp } = useDisplay()
const jobStore = useJobStore()
const authStore = useAuthStore()
const tenantId = computed(() => route.params.tenantId as string)
const jobId = computed(() => route.params.jobId as string)
const job = ref<Record<string, any> | null>(null)
const tenantAIProvider = ref('')
const tenantAIModel = ref('')
const isClassification = computed(() => job.value?.job_type === 'classification')

const TAG_COLORS = ['#7E57C2', '#1E88E5', '#00897B', '#FB8C00', '#D81B60', '#00ACC1', '#3949AB', '#E64A19', '#7CB342', '#6D4C41']
function tagColor(tag: string): string {
  const tags = availableTags.value
  const idx = tags.indexOf(tag)
  return idx >= 0 ? TAG_COLORS[idx % TAG_COLORS.length] : TAG_COLORS[0]
}
const selectedRunId = ref<string | null>(null)
const cancelling = ref(false)
const isJobRunning = computed(() => jobStore.jobRuns?.[0]?.status === 'running')
let pollTimer: ReturnType<typeof setTimeout> | null = null
const expandedMap = ref<Record<string, boolean>>({})
const runDialog = ref(false)
const clearResultsDialog = ref(false)
const clearRunsDialog = ref(false)
const clearingResults = ref(false)
const clearingRuns = ref(false)
const runMode = ref<'unanalyzed' | 'since_last' | 'conditional'>('since_last')
const runDateFrom = ref('')
const runDateTo = ref('')
const runLimit = ref<number | null>(null)

const runDateFromError = computed(() => {
  if (runMode.value !== 'conditional') return ''
  if (runDateFrom.value && !runDateTo.value) return 'Cần chọn đến ngày'
  if (runDateFrom.value && runDateTo.value && runDateFrom.value > runDateTo.value) return 'Từ ngày phải nhỏ hơn đến ngày'
  return ''
})
const runDateToError = computed(() => {
  if (runMode.value !== 'conditional') return ''
  if (runDateTo.value && !runDateFrom.value) return 'Cần chọn từ ngày'
  return ''
})
const runConditionalError = computed(() => {
  if (runMode.value !== 'conditional') return ''
  if (!runDateFrom.value && !runDateTo.value && !runLimit.value) return 'Vui lòng chọn ít nhất một điều kiện (thời gian hoặc số lượng)'
  if (runDateFromError.value) return runDateFromError.value
  if (runDateToError.value) return runDateToError.value
  return ''
})
const activeTab = ref('results')
const resultFilter = ref<'all' | 'fail' | 'pass' | 'skip' | 'classified'>('all')
const tagFilter = ref<string | null>(null)
const viewMode = ref<'card' | 'table'>('card')
const detailDialog = ref(false)
const dialogGroup = ref<ConversationGroup | null>(null)

// Run history pagination
const runPage = ref(1)
const runsPerPage = 5
const totalRunPages = computed(() => Math.ceil(jobStore.jobRuns.length / runsPerPage))
const paginatedRuns = computed(() => {
  const start = (runPage.value - 1) * runsPerPage
  return jobStore.jobRuns.slice(start, start + runsPerPage)
})

// Results pagination
const resultPage = ref(1)
const resultsPerPage = 10
const totalResultPages = computed(() => Math.ceil(filteredGroupedResults.value.length / resultsPerPage))
const paginatedResults = computed(() => {
  const start = (resultPage.value - 1) * resultsPerPage
  return filteredGroupedResults.value.slice(start, start + resultsPerPage)
})

// Chat messages cache
const chatMessages = ref<Record<string, any[]>>({})
const lightboxSrc = ref('')
const authImageCache = ref<Record<string, string>>({})

function hasAttachments(msg: any) {
  if (!msg.attachments || msg.attachments === '[]' || msg.attachments === 'null') return false
  try { const arr = JSON.parse(msg.attachments); return Array.isArray(arr) && arr.length > 0 } catch { return false }
}
function parseAttachments(msg: any) {
  try { return JSON.parse(msg.attachments) || [] } catch { return [] }
}
function isImageAttachment(att: any): boolean {
  if (!att.type) return false
  const t = att.type.toLowerCase()
  return t.startsWith('image') || t === 'photo' || t === 'gif' || t === 'sticker'
}
function getAttachmentUrl(att: any): string {
  if (att.local_path) return `/api/v1/files/${att.local_path}`
  return att.url || ''
}
async function loadAuthImage(url: string) {
  if (!url || authImageCache.value[url]) return
  if (!url.startsWith('/api/')) { authImageCache.value[url] = url; return }
  authImageCache.value[url] = 'loading'
  try {
    const token = localStorage.getItem('cqa_access_token')
    const resp = await fetch(url, { headers: token ? { 'Authorization': `Bearer ${token}` } : {} })
    if (resp.ok) { const blob = await resp.blob(); authImageCache.value[url] = URL.createObjectURL(blob) }
    else { delete authImageCache.value[url] }
  } catch { delete authImageCache.value[url] }
}
function loadImagesForMessages(msgs: any[]) {
  for (const msg of msgs) {
    if (msg.attachments) {
      try {
        const atts = typeof msg.attachments === 'string' ? JSON.parse(msg.attachments) : msg.attachments
        if (!Array.isArray(atts)) continue
        for (const att of atts) { if (isImageAttachment(att)) { const url = getAttachmentUrl(att); if (url) loadAuthImage(url) } }
      } catch { continue }
    }
  }
}
watch(chatMessages, (val) => { for (const msgs of Object.values(val)) { if (msgs?.length) loadImagesForMessages(msgs) } }, { deep: true })
onUnmounted(() => {
  stopPolling()
  for (const url of Object.values(authImageCache.value)) { if (url?.startsWith('blob:')) URL.revokeObjectURL(url) }
})

// Parsed job fields
const parsedOutputs = computed(() => {
  try { return JSON.parse(job.value?.outputs || '[]') } catch { return [] }
})
const parsedChannelCount = computed(() => {
  try { return JSON.parse(job.value?.input_channel_ids || '[]').length } catch { return 0 }
})

// Aggregate stats from results
const aggregateStats = computed(() => {
  const groups = groupedResults.value
  if (!groups.length) return { analyzed: 0, passRate: 0, issues: 0, avgScore: 0 }
  // Exclude SKIP from all stats
  const evaluated = groups.filter(g => g.verdict !== 'SKIP')
  const passed = evaluated.filter(g => g.verdict === 'PASS').length
  const totalViolations = evaluated.reduce((sum, g) => sum + g.violations.length, 0)
  const scores = evaluated.filter(g => g.score != null).map(g => g.score!)
  const avgScore = scores.length ? Math.round(scores.reduce((a, b) => a + b, 0) / scores.length) : 0
  return {
    analyzed: evaluated.length,
    passRate: evaluated.length ? Math.round(passed / evaluated.length * 100) : 0,
    issues: totalViolations,
    avgScore,
  }
})

// Trend chart — group theo ngày cuộc chat (conversation date), đếm theo conversation
const trendChartData = computed(() => {
  const results = jobStore.jobResults
  // Bước 1: Group results theo conversation_id → xác định pass/fail/skip per conversation
  const convMap = new Map<string, { date: string; hasViolation: boolean; isSkip: boolean }>()
  for (const r of results) {
    const existing = convMap.get(r.conversation_id) || {
      date: r.conversation_date || r.created_at,
      hasViolation: false,
      isSkip: false,
    }
    if (r.result_type === 'qc_violation') existing.hasViolation = true
    if (r.result_type === 'conversation_evaluation' && r.severity === 'SKIP') existing.isSkip = true
    convMap.set(r.conversation_id, existing)
  }
  // Bước 2: Group conversations theo ngày → đếm passed/failed
  const byDate = new Map<string, { passed: number; failed: number; label: string }>()
  for (const [, conv] of convMap) {
    const d = new Date(conv.date)
    const label = `${String(d.getDate()).padStart(2, '0')}/${String(d.getMonth() + 1).padStart(2, '0')}`
    const sortKey = d.toISOString().slice(0, 10)
    const existing = byDate.get(sortKey) || { passed: 0, failed: 0, label }
    if (conv.isSkip) {
      // exclude SKIP from trend chart
    } else if (conv.hasViolation) {
      existing.failed += 1
    } else {
      existing.passed += 1
    }
    byDate.set(sortKey, existing)
  }
  const sorted = [...byDate.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  return {
    labels: sorted.map(([, v]) => v.label),
    datasets: [
      { label: 'Đạt', data: sorted.map(([, v]) => v.passed), borderColor: '#66BB6A', backgroundColor: '#66BB6A', fill: false, tension: 0.3, pointRadius: 4 },
      { label: 'Không đạt', data: sorted.map(([, v]) => v.failed), borderColor: '#EF5350', backgroundColor: '#EF5350', fill: false, tension: 0.3, pointRadius: 4 },
    ],
  }
})

const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: { legend: { display: true, position: 'bottom' as const } },
  scales: {
    x: { grid: { display: false } },
    y: { beginAtZero: true, ticks: { stepSize: 1 } },
  },
}

// Filtered results
// Available tags for classification filter dropdown
const availableTags = computed(() => {
  const tagSet = new Set<string>()
  for (const g of groupedResults.value) {
    for (const t of g.tags) tagSet.add(t)
  }
  return Array.from(tagSet).sort()
})

const filteredGroupedResults = computed(() => {
  let results = groupedResults.value
  if (resultFilter.value === 'pass') results = results.filter(g => g.verdict === 'PASS')
  else if (resultFilter.value === 'fail') results = results.filter(g => g.verdict === 'FAIL')
  else if (resultFilter.value === 'skip') results = results.filter(g => g.verdict === 'SKIP')
  else if (resultFilter.value === 'classified') results = results.filter(g => g.verdict !== 'SKIP')
  // Apply tag filter for classification
  if (tagFilter.value) {
    results = results.filter(g => g.tags.includes(tagFilter.value!))
  }
  return results
})

interface ConversationGroup {
  conversationId: string
  customerName: string
  conversationDate: string
  verdict: string
  score: number | null
  review: string
  violations: JobResult[]
  tags: string[]
}

async function toggleExpand(id: string) {
  expandedMap.value[id] = !expandedMap.value[id]
  if (expandedMap.value[id] && !chatMessages.value[id]) {
    try {
      const { data } = await api.get(`/tenants/${tenantId.value}/conversations/${id}/messages`)
      const messages = data.messages || []
      chatMessages.value[id] = messages
      // Extract customer name
      if (messages.length) {
        const customer = messages.find((m: any) => m.sender_type !== 'agent')
        if (customer) {
          const g = groupedResults.value.find(x => x.conversationId === id)
          if (g) g.customerName = customer.sender_name
        }
      }
    } catch {
      chatMessages.value[id] = []
    }
  }
}

async function openDetail(group: ConversationGroup) {
  dialogGroup.value = group
  detailDialog.value = true
  if (!chatMessages.value[group.conversationId]) {
    try {
      const { data } = await api.get(`/tenants/${tenantId.value}/conversations/${group.conversationId}/messages`)
      chatMessages.value[group.conversationId] = data.messages || []
    } catch { chatMessages.value[group.conversationId] = [] }
  }
}

function isHighlighted(group: ConversationGroup, msg: any): boolean {
  return group.violations.some(v => {
    const evidence = (v.evidence || '').toLowerCase()
    const content = (msg.content || '').toLowerCase()
    return content.length > 10 && evidence.includes(content.substring(0, Math.min(30, content.length)))
  })
}

const dayNamesShort = ['CN', 'T2', 'T3', 'T4', 'T5', 'T6', 'T7']

function formatTime(dateStr: string): string {
  try {
    const d = new Date(dateStr)
    const day = dayNamesShort[d.getDay()]
    const dd = String(d.getDate()).padStart(2, '0')
    const mm = String(d.getMonth() + 1).padStart(2, '0')
    const hh = String(d.getHours()).padStart(2, '0')
    const mi = String(d.getMinutes()).padStart(2, '0')
    return `${day} ${dd}/${mm} ${hh}:${mi}`
  } catch { return '' }
}

const groupedResults = computed<ConversationGroup[]>(() => {
  const results = jobStore.jobResults
  if (!results.length) return []

  const latestRunPerConv = new Map<string, string>()
  for (const r of results) {
    const cid = r.conversation_id
    const existing = latestRunPerConv.get(cid)
    if (!existing || r.created_at > (results.find(x => x.job_run_id === existing && x.conversation_id === cid)?.created_at || '')) {
      latestRunPerConv.set(cid, r.job_run_id)
    }
  }

  const groups = new Map<string, ConversationGroup>()
  for (const r of results) {
    const cid = r.conversation_id
    if (r.job_run_id !== latestRunPerConv.get(cid)) continue

    if (!groups.has(cid)) {
      groups.set(cid, {
        conversationId: cid,
        customerName: r.customer_name || '',
        conversationDate: r.conversation_date || r.created_at,
        verdict: 'PASS',
        score: null,
        review: '',
        violations: [],
        tags: [],
      })
    }
    const g = groups.get(cid)!

    if (r.result_type === 'conversation_evaluation') {
      g.verdict = r.severity
      g.review = r.evidence
      const detail = parseDetail(r.detail)
      g.score = detail?.score ?? null
    } else if (r.result_type === 'classification_tag') {
      g.tags.push(r.rule_name)
      g.violations.push(r)
      // Fallback: if no review yet, try to get summary from classification_tag detail
      if (!g.review) {
        const tagDetail = parseDetail(r.detail)
        if (tagDetail?.summary) g.review = tagDetail.summary
      }
    } else {
      g.violations.push(r)
    }
  }

  return Array.from(groups.values()).sort((a, b) => {
    return b.conversationDate.localeCompare(a.conversationDate)
  })
})

onMounted(async () => {
  job.value = await jobStore.fetchJob(tenantId.value, jobId.value)
  if (job.value?.job_type === 'classification') resultFilter.value = 'classified'
  await jobStore.fetchJobRuns(tenantId.value, jobId.value)
  await jobStore.fetchAllJobResults(tenantId.value, jobId.value)
  // Load tenant AI settings (jobs use global settings)
  try {
    const { data } = await api.get(`/tenants/${tenantId.value}/settings`)
    tenantAIProvider.value = data?.settings?.ai_provider || 'claude'
    tenantAIModel.value = data?.settings?.ai_model || ''
  } catch { /* fallback empty */ }
  // Auto-start polling if job is currently running (e.g. after F5)
  if (isJobRunning.value) {
    startPolling()
  }
})

function startPolling() {
  stopPolling()
  async function tick() {
    try {
      await jobStore.fetchJobRuns(tenantId.value, jobId.value)
      if (!isJobRunning.value) {
        // Job finished — fetch final results
        await jobStore.fetchAllJobResults(tenantId.value, jobId.value)
        job.value = await jobStore.fetchJob(tenantId.value, jobId.value)
        stopPolling()
        return
      }
    } catch { /* ignore network errors, retry next tick */ }
    pollTimer = setTimeout(tick, 3000)
  }
  // Small delay before first poll to let backend create the run record
  pollTimer = setTimeout(tick, 2000)
}

function stopPolling() {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
}

const currentRunProgress = computed(() => {
  const run = jobStore.jobRuns[0]
  if (!run || run.status !== 'running') return null
  try {
    const s = JSON.parse(run.summary || '{}')
    if (!s.conversations_found) return null
    return {
      total: s.conversations_found,
      analyzed: (s.conversations_analyzed || 0) + (s.conversations_errors || 0),
      passed: s.conversations_passed || 0,
      errors: s.conversations_errors || 0,
      issues: s.issues_found || 0,
    }
  } catch { return null }
})

const progressPercent = computed(() => {
  if (!currentRunProgress.value) return 0
  return Math.round(currentRunProgress.value.analyzed / currentRunProgress.value.total * 100)
})

// AI provider check
const aiNotConfiguredDialog = ref(false)
async function checkAIConfigured(): Promise<boolean> {
  try {
    const { data } = await api.get(`/tenants/${tenantId.value}/settings`)
    if (data.settings?.ai_api_key) return true
  } catch { /* ignore */ }
  aiNotConfiguredDialog.value = true
  return false
}

async function openRunDialog() {
  if (!(await checkAIConfigured())) return
  runDialog.value = true
}

async function testRun() {
  if (!(await checkAIConfigured())) return
  try {
    await jobStore.testRunJob(tenantId.value, jobId.value)
    startPolling()
  } catch {
    await jobStore.fetchJobRuns(tenantId.value, jobId.value)
  }
}

async function confirmRun() {
  if (runConditionalError.value) return
  runDialog.value = false
  try {
    const params: Record<string, string> = {}
    if (runMode.value === 'conditional') {
      if (runDateFrom.value) params.from = runDateFrom.value
      if (runDateTo.value) params.to = runDateTo.value
    }
    if (runLimit.value && runLimit.value > 0) params.limit = String(runLimit.value)
    await jobStore.triggerJob(tenantId.value, jobId.value, runMode.value, params)
    startPolling()
  } catch {
    await jobStore.fetchJobRuns(tenantId.value, jobId.value)
  }
}

async function cancelJob() {
  cancelling.value = true
  try {
    await api.post(`/tenants/${tenantId.value}/jobs/${jobId.value}/cancel`)
    stopPolling()
    await jobStore.fetchJobRuns(tenantId.value, jobId.value)
    await jobStore.fetchAllJobResults(tenantId.value, jobId.value)
  } catch { /* ignore */ }
  finally { cancelling.value = false }
}

async function loadResults(runId: string) {
  selectedRunId.value = runId
  resultFilter.value = 'all'
  resultPage.value = 1
  activeTab.value = 'results'
  await jobStore.fetchJobResults(tenantId.value, jobId.value, runId)
}

async function loadAllResults() {
  selectedRunId.value = null
  resultFilter.value = 'all'
  resultPage.value = 1
  await jobStore.fetchAllJobResults(tenantId.value, jobId.value)
}

async function clearResults() {
  clearingResults.value = true
  try {
    await api.delete(`/tenants/${tenantId.value}/jobs/${jobId.value}/results`)
    clearResultsDialog.value = false
    jobStore.jobResults = []
    await jobStore.fetchJobRuns(tenantId.value, jobId.value)
  } catch (err) {
    console.error('Clear results failed:', err)
  } finally {
    clearingResults.value = false
  }
}

async function clearRuns() {
  clearingRuns.value = true
  try {
    await api.delete(`/tenants/${tenantId.value}/jobs/${jobId.value}/runs`)
    clearRunsDialog.value = false
    jobStore.jobResults = []
    await jobStore.fetchJobRuns(tenantId.value, jobId.value)
    job.value = await jobStore.fetchJob(tenantId.value, jobId.value)
  } catch (err) {
    console.error('Clear runs failed:', err)
  } finally {
    clearingRuns.value = false
  }
}

const dayNames = ['Chủ nhật', 'Thứ 2', 'Thứ 3', 'Thứ 4', 'Thứ 5', 'Thứ 6', 'Thứ 7']

function formatSchedule(type: string, cron: string) {
  if (type === 'manual' || !cron) return 'Thủ công'
  const parts = cron.trim().split(/\s+/)
  if (parts.length < 5) return cron
  const [min, hour, dom, , dow] = parts
  const time = `${hour.padStart(2, '0')}:${min.padStart(2, '0')}`
  if (dow === '*' && dom === '*') return `Hàng ngày lúc ${time}`
  if (dow === '1-5' && dom === '*') return `Thứ 2-6 lúc ${time}`
  if (dow === '0-6' && dom === '*') return `Hàng ngày lúc ${time}`
  if (dow !== '*' && dom === '*') {
    const days = dow.split(',').map(d => dayNames[parseInt(d)] || d).join(', ')
    return `${days} lúc ${time}`
  }
  if (dom !== '*' && dow === '*') return `Ngày ${dom} hàng tháng lúc ${time}`
  return cron
}

function formatDateTime(d: string) {
  const dt = new Date(d)
  const dd = String(dt.getDate()).padStart(2, '0')
  const mm = String(dt.getMonth() + 1).padStart(2, '0')
  const hh = String(dt.getHours()).padStart(2, '0')
  const mi = String(dt.getMinutes()).padStart(2, '0')
  return `${dd}/${mm}/${dt.getFullYear()} ${hh}:${mi}`
}

async function exportResults(format: string = 'csv') {
  try {
    const { data } = await api.get(`/tenants/${tenantId.value}/jobs/${jobId.value}/results/export?format=${format}`, {
      responseType: format === 'xlsx' ? 'blob' : 'text',
    })
    const blob = format === 'xlsx'
      ? new Blob([data], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
      : new Blob([data], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `results.${format === 'xlsx' ? 'xlsx' : 'csv'}`
    a.click()
    URL.revokeObjectURL(url)
  } catch { /* ignore */ }
}

function statusColor(status: string) {
  if (status === 'success') return 'success'
  if (status === 'error') return 'error'
  return 'info'
}

function parseSummary(s: string) {
  try { return JSON.parse(s) } catch { return {} }
}

function parseDetail(s: string) {
  try { return JSON.parse(s) } catch { return {} }
}

function classificationSummary(group: any): string {
  // Try to get detailed evidence from classification_tag violations
  const evidences = group.violations
    ?.map((v: any) => v.evidence)
    .filter((e: string) => e && !e.startsWith('Cuộc chat được phân loại'))
  if (evidences?.length) return evidences.join('; ')
  // Fallback: try explanation from detail
  const explanations = group.violations
    ?.map((v: any) => parseDetail(v.detail)?.explanation)
    .filter(Boolean)
  if (explanations?.length) return explanations.join('; ')
  // Last fallback
  return group.review || '—'
}
</script>

<style scoped>
.lightbox-overlay { position: fixed; top: 0; left: 0; width: 100vw; height: 100vh; background: rgba(0,0,0,0.85); display: flex; align-items: center; justify-content: center; z-index: 9999; cursor: pointer; }
.lightbox-img { max-width: 90vw; max-height: 90vh; object-fit: contain; border-radius: 8px; cursor: default; }
.lightbox-close { position: fixed; top: 16px; right: 16px; }
</style>
