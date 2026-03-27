package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/nmtan2001/chat-quality-agent/ai"
	"github.com/nmtan2001/chat-quality-agent/config"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/notifications"
	"github.com/nmtan2001/chat-quality-agent/pkg"
)

// Analyzer executes analysis jobs: loads messages, calls AI, saves results.
type Analyzer struct {
	cfg *config.Config
}

func NewAnalyzer(cfg *config.Config) *Analyzer {
	return &Analyzer{cfg: cfg}
}

// RunJobWithLimit runs analysis on a limited number of conversations (for testing).
func (a *Analyzer) RunJobWithLimit(ctx context.Context, job models.Job, limit int) (*models.JobRun, error) {
	return a.runJob(ctx, job, limit, nil)
}

// RunJob executes a single job: analyzes new conversations since last run.
func (a *Analyzer) RunJob(ctx context.Context, job models.Job) (*models.JobRun, error) {
	return a.runJob(ctx, job, 0, nil)
}

// RunJobFull re-analyzes ALL conversations (ignores last_run_at). Use after rule changes.
func (a *Analyzer) RunJobFull(ctx context.Context, job models.Job) (*models.JobRun, error) {
	return a.runJobInternal(ctx, job, 0, nil, true)
}

// RunJobFullWithParams re-analyzes with optional date range and limit.
func (a *Analyzer) RunJobFullWithParams(ctx context.Context, job models.Job, dateFrom, dateTo string, maxConv int) (*models.JobRun, error) {
	return a.runJobInternalExt(ctx, job, maxConv, nil, true, dateFrom, dateTo, nil, false)
}

// RunJobUnanalyzed analyzes all conversations not yet evaluated by this job, regardless of time.
func (a *Analyzer) RunJobUnanalyzed(ctx context.Context, job models.Job, maxConv int) (*models.JobRun, error) {
	return a.runJobInternalExt(ctx, job, maxConv, nil, true, "", "", nil, true)
}

// RunJobSinceLast analyzes conversations newer than the most recently evaluated conversation.
func (a *Analyzer) RunJobSinceLast(ctx context.Context, job models.Job, maxConv int) (*models.JobRun, error) {
	// Find max last_message_at among conversations already evaluated by this job
	var maxMsgAt time.Time
	if err := db.DB.Model(&models.JobResult{}).
		Select("MAX(conversations.last_message_at)").
		Joins("JOIN job_runs ON job_runs.id = job_results.job_run_id").
		Joins("JOIN conversations ON conversations.id = job_results.conversation_id").
		Where("job_runs.job_id = ?", job.ID).
		Scan(&maxMsgAt).Error; err != nil {
		log.Printf("[analyzer] error finding max message_at for job %s: %v", job.ID, err)
	}

	if maxMsgAt.IsZero() {
		// No previous results — fall back to regular incremental (last run / 24h)
		return a.runJob(ctx, job, maxConv, nil)
	}
	return a.runJobInternalExt(ctx, job, maxConv, nil, false, "", "", &maxMsgAt, false)
}

// RunJobWithProvider runs with an injected AI provider (for testing without real API keys).
func (a *Analyzer) RunJobWithProvider(ctx context.Context, job models.Job, limit int, provider ai.AIProvider) (*models.JobRun, error) {
	return a.runJob(ctx, job, limit, provider)
}

func (a *Analyzer) runJob(ctx context.Context, job models.Job, maxConversations int, injectedProvider ai.AIProvider) (*models.JobRun, error) {
	return a.runJobInternal(ctx, job, maxConversations, injectedProvider, false)
}

func (a *Analyzer) runJobInternal(ctx context.Context, job models.Job, maxConversations int, injectedProvider ai.AIProvider, fullRerun bool) (*models.JobRun, error) {
	return a.runJobInternalExt(ctx, job, maxConversations, injectedProvider, fullRerun, "", "", nil, false)
}

func (a *Analyzer) runJobInternalExt(ctx context.Context, job models.Job, maxConversations int, injectedProvider ai.AIProvider, fullRerun bool, dateFrom, dateTo string, sinceOverride *time.Time, excludeAnalyzed bool) (*models.JobRun, error) {
	now := time.Now()
	run := models.JobRun{
		ID:        pkg.NewUUID(),
		JobID:     job.ID,
		TenantID:  job.TenantID,
		StartedAt: now,
		Status:    "running",
		Summary:   "{}",
		CreatedAt: now,
	}
	if err := db.DB.Create(&run).Error; err != nil {
		return nil, fmt.Errorf("failed to create job run: %w", err)
	}

	// Log run started
	db.LogActivity(job.TenantID, "", "system", "job.run.started", "job", job.ID,
		fmt.Sprintf("Job '%s': started analysis (max=%d, full=%v)", job.Name, maxConversations, fullRerun), "", "")

	// Get AI provider (use injected if provided, otherwise from settings)
	var provider ai.AIProvider
	var err error
	if injectedProvider != nil {
		provider = injectedProvider
	} else {
		provider, err = a.getProvider(job)
		if err != nil {
			return a.failRun(&run, err)
		}
	}

	// Parse input channel IDs
	var channelIDs []string
	if err := json.Unmarshal([]byte(job.InputChannelIDs), &channelIDs); err != nil {
		return a.failRun(&run, fmt.Errorf("invalid input_channel_ids: %w", err))
	}

	// Determine time range
	isTestRun := maxConversations > 0
	var since time.Time
	if sinceOverride != nil {
		since = *sinceOverride
	} else if dateFrom != "" {
		if t, err := time.Parse("2006-01-02", dateFrom); err == nil {
			since = t
		}
	}
	if since.IsZero() && sinceOverride == nil {
		if fullRerun {
			since = time.Time{} // epoch — get ALL conversations
		} else if isTestRun {
			since = time.Now().Add(-7 * 24 * time.Hour)
		} else {
			since = time.Now().Add(-24 * time.Hour)
			if job.LastRunAt != nil {
				since = *job.LastRunAt
			}
		}
	}

	// Fetch conversations with messages in time range
	var conversations []models.Conversation
	q := db.DB.Where("tenant_id = ? AND channel_id IN ?", job.TenantID, channelIDs)
	if !since.IsZero() {
		q = q.Where("last_message_at > ?", since)
	}
	if dateTo != "" {
		if t, err := time.Parse("2006-01-02", dateTo); err == nil {
			q = q.Where("last_message_at < ?", t.Add(24*time.Hour))
		}
	}
	if excludeAnalyzed {
		// Only include conversations not yet evaluated by this job
		// job_results has job_run_id, not job_id — must JOIN through job_runs
		analyzedSubq := db.DB.Model(&models.JobResult{}).
			Select("job_results.conversation_id").
			Joins("JOIN job_runs ON job_runs.id = job_results.job_run_id").
			Where("job_runs.job_id = ?", job.ID)
		q = q.Where("id NOT IN (?)", analyzedSubq)
	}
	if maxConversations > 0 {
		q = q.Limit(maxConversations)
	}
	q.Find(&conversations)

	log.Printf("[analyzer] job %s: channelIDs=%v, sinceZero=%v, excludeAnalyzed=%v, fullRerun=%v, found %d conversations",
		job.Name, channelIDs, since.IsZero(), excludeAnalyzed, fullRerun, len(conversations))

	// Set initial total so frontend can show progress immediately
	initialSummary, _ := json.Marshal(map[string]interface{}{
		"conversations_found": len(conversations),
	})
	db.DB.Model(&run).Update("summary", string(initialSummary))

	// Check batch mode setting (default: enabled with batch size 5)
	batchMode := true
	batchSize := 5
	var batchSetting models.AppSetting
	if err := db.DB.Where("tenant_id = ? AND setting_key = ?", job.TenantID, "ai_batch_mode").First(&batchSetting).Error; err == nil {
		batchMode = batchSetting.ValuePlain != "false"
	}
	var batchSizeSetting models.AppSetting
	if err := db.DB.Where("tenant_id = ? AND setting_key = ?", job.TenantID, "ai_batch_size").First(&batchSizeSetting).Error; err == nil {
		var n int
		if _, err := fmt.Sscanf(batchSizeSetting.ValuePlain, "%d", &n); err == nil && n > 0 && n <= 30 {
			batchSize = n
		}
	}

	issuesFound := 0
	passCount := 0
	analyzedCount := 0
	errorCount := 0

	if batchMode {
		issuesFound, passCount, analyzedCount, errorCount = a.runBatchMode(ctx, provider, job, run, conversations, since, batchSize)
	} else {

	for _, conv := range conversations {
		// Load messages
		var messages []models.Message
		mq := db.DB.Where("conversation_id = ?", conv.ID)
		if !since.IsZero() {
			mq = mq.Where("sent_at > ?", since)
		}
		mq.Order("sent_at ASC").Find(&messages)

		if len(messages) == 0 {
			continue
		}

		// Format transcript
		chatMessages := make([]ai.ChatMessage, len(messages))
		for i, m := range messages {
			chatMessages[i] = ai.ChatMessage{
				SenderType: m.SenderType,
				SenderName: m.SenderName,
				Content:    m.Content,
				SentAt:     m.SentAt.Format("15:04"),
			}
		}
		transcript := ai.FormatChatTranscript(chatMessages)

		// Build prompt based on job type
		var systemPrompt string
		switch job.JobType {
		case "qc_analysis":
			systemPrompt = ai.BuildQCPrompt(job.RulesContent, job.SkipConditions)
		case "classification":
			systemPrompt = ai.BuildClassificationPrompt(job.RulesConfig)
		default:
			continue
		}

		// Call AI (with rate limit delay)
		if analyzedCount > 0 {
			time.Sleep(500 * time.Millisecond) // Avoid rate limiting
		}
		aiResp, err := provider.AnalyzeChat(ctx, systemPrompt, transcript)
		if err != nil {
			log.Printf("[analyzer] AI error for conversation %s: %v", conv.ID, err)
			errorCount++
			// Update progress even on error
			errProgressJSON, _ := json.Marshal(map[string]interface{}{
				"conversations_found":    len(conversations),
				"conversations_analyzed": analyzedCount,
				"conversations_passed":   passCount,
				"conversations_errors":   errorCount,
				"issues_found":           issuesFound,
			})
			db.DB.Model(&run).Update("summary", string(errProgressJSON))
			continue
		}
		analyzedCount++

		// Log AI usage + cost
		cost := ai.CalculateCostUSD(aiResp.Provider, aiResp.Model, aiResp.InputTokens, aiResp.OutputTokens)
		usageLog := models.AIUsageLog{
			ID:           pkg.NewUUID(),
			TenantID:     job.TenantID,
			JobID:        job.ID,
			JobRunID:     run.ID,
			Provider:     aiResp.Provider,
			Model:        aiResp.Model,
			InputTokens:  aiResp.InputTokens,
			OutputTokens: aiResp.OutputTokens,
			CostUSD:      cost,
			CreatedAt:    time.Now(),
		}
		db.DB.Create(&usageLog)

		// Parse and save results
		count, passed, err := a.saveResults(run.ID, job.TenantID, conv.ID, job.JobType, aiResp.Content)
		if err != nil {
			log.Printf("[analyzer] save results error for %s: %v", conv.ID, err)
		}
		issuesFound += count
		if passed {
			passCount++
		}

		// Update progress so frontend can poll real-time status
		progressJSON, _ := json.Marshal(map[string]interface{}{
			"conversations_found":    len(conversations),
			"conversations_analyzed": analyzedCount,
			"conversations_passed":   passCount,
			"conversations_errors":   errorCount,
			"issues_found":           issuesFound,
		})
		db.DB.Model(&run).Update("summary", string(progressJSON))
	}

	} // end else (non-batch mode)

	// Complete run
	finishedAt := time.Now()
	summaryJSON, _ := json.Marshal(map[string]interface{}{
		"conversations_found":    len(conversations),
		"conversations_analyzed": analyzedCount,
		"conversations_passed":   passCount,
		"conversations_errors":   errorCount,
		"issues_found":           issuesFound,
	})
	runStatus := "success"
	if analyzedCount == 0 && errorCount > 0 {
		runStatus = "error"
		run.ErrorMessage = fmt.Sprintf("AI errors: %d/%d conversations failed", errorCount, len(conversations))
	}
	db.DB.Model(&run).Updates(map[string]interface{}{
		"status":        runStatus,
		"finished_at":   &finishedAt,
		"summary":       string(summaryJSON),
		"error_message": run.ErrorMessage,
	})

	// Update job last_run (skip for test runs to avoid affecting future normal runs)
	if !isTestRun {
		db.DB.Model(&job).Updates(map[string]interface{}{
			"last_run_at":     &finishedAt,
			"last_run_status": "success",
			"updated_at":      finishedAt,
		})
	}

	run.Status = "success"
	run.FinishedAt = &finishedAt
	run.Summary = string(summaryJSON)

	log.Printf("[analyzer] job %s completed: %d conversations, %d issues", job.Name, len(conversations), issuesFound)

	// Log activity
	db.LogActivity(job.TenantID, "", "system", "job.run.completed", "job", job.ID,
		fmt.Sprintf("Job '%s': %d analyzed, %d passed, %d issues, %d errors", job.Name, analyzedCount, passCount, issuesFound, errorCount),
		run.ErrorMessage, "")

	// Send notifications via configured outputs (telegram, email, etc.)
	if analyzedCount > 0 && job.OutputSchedule != "none" {
		dispatcher := notifications.NewDispatcher()
		if err := dispatcher.SendJobResults(ctx, job, run); err != nil {
			log.Printf("[analyzer] notification error for job %s: %v", job.Name, err)
			db.LogActivity(job.TenantID, "", "system", "notification.error", "job", job.ID, "", err.Error(), "")
		}
	}

	return &run, nil
}

func (a *Analyzer) getProvider(job models.Job) (ai.AIProvider, error) {
	// Get AI provider from tenant settings (fallback to job's ai_provider)
	provider := job.AIProvider
	var providerSetting models.AppSetting
	if err := db.DB.Where("tenant_id = ? AND setting_key = ?", job.TenantID, "ai_provider").First(&providerSetting).Error; err == nil {
		provider = providerSetting.ValuePlain
	}

	// Get API key from tenant settings
	var setting models.AppSetting
	result := db.DB.Where("tenant_id = ? AND setting_key = ?", job.TenantID, "ai_api_key").First(&setting)
	if result.Error != nil {
		return nil, fmt.Errorf("API key not configured - go to Settings > AI Config")
	}

	apiKey := setting.ValuePlain
	if setting.ValueEncrypted != nil && len(setting.ValueEncrypted) > 0 {
		decrypted, err := pkg.Decrypt(setting.ValueEncrypted, a.cfg.EncryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt API key: %w", err)
		}
		apiKey = string(decrypted)
	}

	// Get model from tenant settings (fallback to job's ai_model)
	model := job.AIModel
	var modelSetting models.AppSetting
	if err := db.DB.Where("tenant_id = ? AND setting_key = ?", job.TenantID, "ai_model").First(&modelSetting).Error; err == nil && modelSetting.ValuePlain != "" {
		model = modelSetting.ValuePlain
	}

	switch provider {
	case "claude":
		return ai.NewClaudeProvider(apiKey, model, a.cfg.AIMaxTokens), nil
	case "gemini":
		return ai.NewGeminiProvider(apiKey, model), nil
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", provider)
	}
}

func (a *Analyzer) saveResults(runID, tenantID, conversationID, jobType, aiResponse string) (int, bool, error) {
	now := time.Now()
	count := 0
	passed := false

	// Strip markdown code fences (```json ... ```) that AI sometimes wraps around JSON
	aiResponse = strings.TrimSpace(aiResponse)
	if strings.HasPrefix(aiResponse, "```") {
		// Remove opening fence (```json or ```)
		if idx := strings.Index(aiResponse, "\n"); idx != -1 {
			aiResponse = aiResponse[idx+1:]
		}
		// Remove closing fence
		if idx := strings.LastIndex(aiResponse, "```"); idx != -1 {
			aiResponse = aiResponse[:idx]
		}
		aiResponse = strings.TrimSpace(aiResponse)
	}

	switch jobType {
	case "qc_analysis":
		var qcResult struct {
			Verdict    string `json:"verdict"`
			Violations []struct {
				Severity    string `json:"severity"`
				Rule        string `json:"rule"`
				Evidence    string `json:"evidence"`
				Explanation string `json:"explanation"`
				Suggestion  string `json:"suggestion"`
			} `json:"violations"`
			Score   int    `json:"score"`
			Review  string `json:"review"`
			Summary string `json:"summary"`
		}
		if err := json.Unmarshal([]byte(aiResponse), &qcResult); err != nil {
			return 0, false, fmt.Errorf("failed to parse QC response: %w", err)
		}

		// Determine pass/fail (SKIP counts as not passed)
		passed = qcResult.Verdict == "PASS"

		// Save conversation evaluation record (for every conversation)
		evalDetailJSON, _ := json.Marshal(map[string]interface{}{
			"review":  qcResult.Review,
			"score":   qcResult.Score,
			"summary": qcResult.Summary,
		})
		evalResult := models.JobResult{
			ID:             pkg.NewUUID(),
			JobRunID:       runID,
			TenantID:       tenantID,
			ConversationID: conversationID,
			ResultType:     "conversation_evaluation",
			Severity:       qcResult.Verdict,
			Evidence:       qcResult.Review,
			Detail:         string(evalDetailJSON),
			AIRawResponse:  aiResponse,
			Confidence:     1.0,
			CreatedAt:      now,
		}
		db.DB.Create(&evalResult)

		// SKIP conversations have no violations — stop here
		if qcResult.Verdict == "SKIP" {
			return 0, false, nil
		}

		// Save individual violations
		for _, v := range qcResult.Violations {
			detailJSON, _ := json.Marshal(map[string]interface{}{
				"explanation": v.Explanation,
				"suggestion":  v.Suggestion,
				"score":       qcResult.Score,
				"summary":     qcResult.Summary,
			})
			result := models.JobResult{
				ID:             pkg.NewUUID(),
				JobRunID:       runID,
				TenantID:       tenantID,
				ConversationID: conversationID,
				ResultType:     "qc_violation",
				Severity:       v.Severity,
				RuleName:       v.Rule,
				Evidence:       v.Evidence,
				Detail:         string(detailJSON),
				AIRawResponse:  aiResponse,
				Confidence:     1.0,
				CreatedAt:      now,
			}
			db.DB.Create(&result)
			count++
		}

	case "classification":
		var classResult struct {
			Tags []struct {
				RuleName    string  `json:"rule_name"`
				Confidence  float64 `json:"confidence"`
				Evidence    string  `json:"evidence"`
				Explanation string  `json:"explanation"`
			} `json:"tags"`
			Summary string `json:"summary"`
		}
		if err := json.Unmarshal([]byte(aiResponse), &classResult); err != nil {
			return 0, false, fmt.Errorf("failed to parse classification response: %w", err)
		}

		for _, t := range classResult.Tags {
			detailJSON, _ := json.Marshal(map[string]interface{}{
				"explanation": t.Explanation,
				"summary":     classResult.Summary,
			})
			result := models.JobResult{
				ID:             pkg.NewUUID(),
				JobRunID:       runID,
				TenantID:       tenantID,
				ConversationID: conversationID,
				ResultType:     "classification_tag",
				RuleName:       t.RuleName,
				Evidence:       t.Evidence,
				Detail:         string(detailJSON),
				AIRawResponse:  aiResponse,
				Confidence:     t.Confidence,
				CreatedAt:      now,
			}
			db.DB.Create(&result)
			count++
		}

		// Create conversation_evaluation record for classified conversations
		if len(classResult.Tags) > 0 {
			evalDetail, _ := json.Marshal(map[string]interface{}{
				"summary": classResult.Summary,
			})
			db.DB.Create(&models.JobResult{
				ID:             pkg.NewUUID(),
				JobRunID:       runID,
				TenantID:       tenantID,
				ConversationID: conversationID,
				ResultType:     "conversation_evaluation",
				Severity:       "PASS",
				Evidence:       classResult.Summary,
				Detail:         string(evalDetail),
				AIRawResponse:  aiResponse,
				Confidence:     1.0,
				CreatedAt:      now,
			})
		}

		// No tags matched — mark conversation as SKIP
		if len(classResult.Tags) == 0 {
			skipDetail, _ := json.Marshal(map[string]interface{}{
				"summary": classResult.Summary,
			})
			db.DB.Create(&models.JobResult{
				ID:             pkg.NewUUID(),
				JobRunID:       runID,
				TenantID:       tenantID,
				ConversationID: conversationID,
				ResultType:     "conversation_evaluation",
				Severity:       "SKIP",
				Evidence:       "Cuộc chat không khớp với bất kỳ nhãn phân loại nào.",
				Detail:         string(skipDetail),
				AIRawResponse:  aiResponse,
				Confidence:     1.0,
				CreatedAt:      now,
			})
		}
	}

	return count, passed, nil
}

// runBatchMode processes conversations in batches of batchSize, sending multiple conversations per AI call.
func (a *Analyzer) runBatchMode(ctx context.Context, provider ai.AIProvider, job models.Job, run models.JobRun, conversations []models.Conversation, since time.Time, batchSize int) (issuesFound, passCount, analyzedCount, errorCount int) {
	// Build system prompt once
	var systemPrompt string
	switch job.JobType {
	case "qc_analysis":
		systemPrompt = ai.BuildQCPrompt(job.RulesContent, job.SkipConditions)
	case "classification":
		systemPrompt = ai.BuildClassificationPrompt(job.RulesConfig)
	default:
		return
	}

	// Prepare all conversations with transcripts
	type convWithTranscript struct {
		Conv       models.Conversation
		Transcript string
	}
	var prepared []convWithTranscript
	for _, conv := range conversations {
		var messages []models.Message
		bmq := db.DB.Where("conversation_id = ?", conv.ID)
		if !since.IsZero() {
			bmq = bmq.Where("sent_at > ?", since)
		}
		bmq.Order("sent_at ASC").Find(&messages)
		if len(messages) == 0 {
			continue
		}
		chatMessages := make([]ai.ChatMessage, len(messages))
		for i, m := range messages {
			chatMessages[i] = ai.ChatMessage{
				SenderType: m.SenderType,
				SenderName: m.SenderName,
				Content:    m.Content,
				SentAt:     m.SentAt.Format("15:04"),
			}
		}
		prepared = append(prepared, convWithTranscript{
			Conv:       conv,
			Transcript: ai.FormatChatTranscript(chatMessages),
		})
	}

	// Process in batches
	for i := 0; i < len(prepared); i += batchSize {
		end := i + batchSize
		if end > len(prepared) {
			end = len(prepared)
		}
		batch := prepared[i:end]

		// Build batch items
		items := make([]ai.BatchItem, len(batch))
		for j, b := range batch {
			items[j] = ai.BatchItem{
				ConversationID: b.Conv.ID,
				Transcript:     b.Transcript,
			}
		}

		// Call AI batch
		aiResp, err := provider.AnalyzeChatBatch(ctx, systemPrompt, items)
		if err != nil {
			log.Printf("[analyzer-batch] AI error for batch starting at %d: %v", i, err)
			errorCount += len(batch)
			continue
		}

		// Log AI usage
		cost := ai.CalculateCostUSD(aiResp.Provider, aiResp.Model, aiResp.InputTokens, aiResp.OutputTokens)
		usageLog := models.AIUsageLog{
			ID:           pkg.NewUUID(),
			TenantID:     job.TenantID,
			JobID:        job.ID,
			JobRunID:     run.ID,
			Provider:     aiResp.Provider,
			Model:        aiResp.Model,
			InputTokens:  aiResp.InputTokens,
			OutputTokens: aiResp.OutputTokens,
			CostUSD:      cost,
			CreatedAt:    time.Now(),
		}
		db.DB.Create(&usageLog)

		// Parse batch response — expect JSON array
		content := strings.TrimSpace(aiResp.Content)
		if strings.HasPrefix(content, "```") {
			if idx := strings.Index(content, "\n"); idx != -1 {
				content = content[idx+1:]
			}
			if idx := strings.LastIndex(content, "```"); idx != -1 {
				content = content[:idx]
			}
			content = strings.TrimSpace(content)
		}

		var batchResults []json.RawMessage
		if err := json.Unmarshal([]byte(content), &batchResults); err != nil {
			// Fallback: try to parse as single result (batch of 1)
			log.Printf("[analyzer-batch] failed to parse batch response as array, trying individual: %v", err)
			for _, b := range batch {
				count, passed, saveErr := a.saveResults(run.ID, job.TenantID, b.Conv.ID, job.JobType, content)
				if saveErr != nil {
					errorCount++
				} else {
					analyzedCount++
					issuesFound += count
					if passed {
						passCount++
					}
				}
			}
		} else {
			// Process each result
			for j, rawResult := range batchResults {
				if j >= len(batch) {
					break
				}
				convID := batch[j].Conv.ID

				// Extract conversation_id from result if present, match by order otherwise
				var resultMap map[string]interface{}
				if json.Unmarshal(rawResult, &resultMap) == nil {
					if cid, ok := resultMap["conversation_id"].(string); ok && cid != "" {
						convID = cid
					}
				}

				count, passed, saveErr := a.saveResults(run.ID, job.TenantID, convID, job.JobType, string(rawResult))
				if saveErr != nil {
					log.Printf("[analyzer-batch] save error for %s: %v", convID, saveErr)
					errorCount++
				} else {
					analyzedCount++
					issuesFound += count
					if passed {
						passCount++
					}
				}
			}
		}

		// Update progress
		progressJSON, _ := json.Marshal(map[string]interface{}{
			"conversations_found":    len(conversations),
			"conversations_analyzed": analyzedCount,
			"conversations_passed":   passCount,
			"conversations_errors":   errorCount,
			"issues_found":           issuesFound,
		})
		db.DB.Model(&run).Update("summary", string(progressJSON))

		// Rate limit between batches
		if end < len(prepared) {
			time.Sleep(1 * time.Second)
		}
	}

	log.Printf("[analyzer-batch] job %s: %d conversations in %d batches of %d", job.Name, len(prepared), (len(prepared)+batchSize-1)/batchSize, batchSize)
	return
}

func (a *Analyzer) failRun(run *models.JobRun, err error) (*models.JobRun, error) {
	finishedAt := time.Now()
	db.DB.Model(run).Updates(map[string]interface{}{
		"status":        "error",
		"finished_at":   &finishedAt,
		"error_message": err.Error(),
	})
	run.Status = "error"
	run.FinishedAt = &finishedAt
	run.ErrorMessage = err.Error()
	return run, err
}
