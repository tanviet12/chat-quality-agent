package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/config"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/engine"
	"github.com/nmtan2001/chat-quality-agent/notifications"
	"github.com/nmtan2001/chat-quality-agent/pkg"
)

type CreateJobRequest struct {
	Name            string          `json:"name" binding:"required,min=2,max=255"`
	Description     string          `json:"description"`
	JobType         string          `json:"job_type" binding:"required,oneof=qc_analysis classification"`
	InputChannelIDs []string        `json:"input_channel_ids" binding:"required,min=1"`
	RulesContent    string          `json:"rules_content"`
	RulesConfig     json.RawMessage `json:"rules_config"`
	SkipConditions  string          `json:"skip_conditions"`
	AIProvider      string          `json:"ai_provider" binding:"required,oneof=claude gemini"`
	AIModel         string          `json:"ai_model"`
	Outputs         json.RawMessage `json:"outputs" binding:"required"`
	OutputSchedule  string          `json:"output_schedule" binding:"required,oneof=instant scheduled cron none"`
	OutputCron      string          `json:"output_cron"`
	OutputAt        *time.Time      `json:"output_at"`
	ScheduleType    string          `json:"schedule_type" binding:"required,oneof=cron after_sync manual"`
	ScheduleCron    string          `json:"schedule_cron"`
}

func ListJobs(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var jobs []models.Job
	db.DB.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&jobs)

	c.JSON(http.StatusOK, jobs)
}

func CreateJob(c *gin.Context) {
	var req CreateJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}

	tenantID := middleware.GetTenantID(c)

	channelIDsJSON, _ := json.Marshal(req.InputChannelIDs)
	rulesConfig := "{}"
	if req.RulesConfig != nil {
		rulesConfig = string(req.RulesConfig)
	}

	now := time.Now()
	job := models.Job{
		ID:              pkg.NewUUID(),
		TenantID:        tenantID,
		Name:            req.Name,
		Description:     req.Description,
		JobType:         req.JobType,
		InputChannelIDs: string(channelIDsJSON),
		RulesContent:    req.RulesContent,
		RulesConfig:     rulesConfig,
		SkipConditions:  req.SkipConditions,
		AIProvider:      req.AIProvider,
		AIModel:         req.AIModel,
		Outputs:         string(req.Outputs),
		OutputSchedule:  req.OutputSchedule,
		OutputCron:      req.OutputCron,
		OutputAt:        req.OutputAt,
		ScheduleType:    req.ScheduleType,
		ScheduleCron:    req.ScheduleCron,
		IsActive:        true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := db.DB.Create(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_job_failed"})
		return
	}

	c.JSON(http.StatusCreated, job)
}

func GetJob(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	jobID := c.Param("jobId")

	var job models.Job
	if err := db.DB.Where("id = ? AND tenant_id = ?", jobID, tenantID).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job_not_found"})
		return
	}

	c.JSON(http.StatusOK, job)
}

// allowedJobUpdateFields is a whitelist of fields that can be updated via the UpdateJob API.
var allowedJobUpdateFields = map[string]bool{
	"name": true, "description": true, "type": true, "status": true,
	"input_channel_ids": true, "outputs": true, "rules_config": true,
	"ai_provider": true, "ai_model": true, "ai_system_prompt": true,
	"schedule_cron": true, "schedule_enabled": true,
	"date_from": true, "date_to": true, "max_conversations": true,
}

func UpdateJob(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	jobID := c.Param("jobId")

	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	// Filter to allowed fields only — prevents mass assignment of tenant_id, id, etc.
	req := make(map[string]interface{})
	for key, val := range raw {
		if allowedJobUpdateFields[key] {
			req[key] = val
		}
	}

	// JSON-encode array/object fields that are stored as strings in DB
	for _, key := range []string{"outputs", "input_channel_ids", "rules_config"} {
		if v, ok := req[key]; ok {
			switch v.(type) {
			case []interface{}, map[string]interface{}:
				encoded, _ := json.Marshal(v)
				req[key] = string(encoded)
			}
		}
	}

	req["updated_at"] = time.Now()

	result := db.DB.Model(&models.Job{}).Where("id = ? AND tenant_id = ?", jobID, tenantID).Updates(req)
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "job_not_found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func DeleteJob(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	jobID := c.Param("jobId")

	// Check job exists
	var job models.Job
	if err := db.DB.Where("id = ? AND tenant_id = ?", jobID, tenantID).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job_not_found"})
		return
	}

	// Cascade delete: results → runs → usage logs → notification logs → job
	var runIDs []string
	db.DB.Model(&models.JobRun{}).Where("job_id = ? AND tenant_id = ?", jobID, tenantID).Pluck("id", &runIDs)
	if len(runIDs) > 0 {
		db.DB.Where("job_run_id IN ? AND tenant_id = ?", runIDs, tenantID).Delete(&models.JobResult{})
	}
	db.DB.Where("job_id = ? AND tenant_id = ?", jobID, tenantID).Delete(&models.JobRun{})
	db.DB.Where("job_id = ? AND tenant_id = ?", jobID, tenantID).Delete(&models.AIUsageLog{})
	db.DB.Where("job_id = ? AND tenant_id = ?", jobID, tenantID).Delete(&models.NotificationLog{})
	db.DB.Where("id = ? AND tenant_id = ?", jobID, tenantID).Delete(&models.Job{})

	db.LogActivity(tenantID, middleware.GetUserID(c), middleware.GetUserEmail(c), "job.delete", "job", jobID, "Deleted job: "+job.Name, "", c.ClientIP())

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func ClearJobResults(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	jobID := c.Param("jobId")

	var job models.Job
	if err := db.DB.Where("id = ? AND tenant_id = ?", jobID, tenantID).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job_not_found"})
		return
	}

	// Delete results + usage logs + notification logs (keep runs)
	var runIDs []string
	db.DB.Model(&models.JobRun{}).Where("job_id = ? AND tenant_id = ?", jobID, tenantID).Pluck("id", &runIDs)
	if len(runIDs) > 0 {
		db.DB.Where("job_run_id IN ? AND tenant_id = ?", runIDs, tenantID).Delete(&models.JobResult{})
	}
	db.DB.Where("job_id = ? AND tenant_id = ?", jobID, tenantID).Delete(&models.AIUsageLog{})
	db.DB.Where("job_id = ? AND tenant_id = ?", jobID, tenantID).Delete(&models.NotificationLog{})

	db.LogActivity(tenantID, middleware.GetUserID(c), middleware.GetUserEmail(c), "job.clear_results", "job", jobID, "Cleared results for job: "+job.Name, "", c.ClientIP())

	c.JSON(http.StatusOK, gin.H{"message": "cleared"})
}

func ClearJobRuns(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	jobID := c.Param("jobId")

	var job models.Job
	if err := db.DB.Where("id = ? AND tenant_id = ?", jobID, tenantID).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job_not_found"})
		return
	}

	// Cascade delete: results → runs → usage logs → notification logs
	var runIDs []string
	db.DB.Model(&models.JobRun{}).Where("job_id = ? AND tenant_id = ?", jobID, tenantID).Pluck("id", &runIDs)
	if len(runIDs) > 0 {
		db.DB.Where("job_run_id IN ? AND tenant_id = ?", runIDs, tenantID).Delete(&models.JobResult{})
	}
	db.DB.Where("job_id = ? AND tenant_id = ?", jobID, tenantID).Delete(&models.JobRun{})
	db.DB.Where("job_id = ? AND tenant_id = ?", jobID, tenantID).Delete(&models.AIUsageLog{})
	db.DB.Where("job_id = ? AND tenant_id = ?", jobID, tenantID).Delete(&models.NotificationLog{})

	// Reset last_run_at
	db.DB.Model(&job).Updates(map[string]interface{}{
		"last_run_at":     nil,
		"last_run_status": "",
		"updated_at":      time.Now(),
	})

	db.LogActivity(tenantID, middleware.GetUserID(c), middleware.GetUserEmail(c), "job.clear_runs", "job", jobID, "Cleared all runs for job: "+job.Name, "", c.ClientIP())

	c.JSON(http.StatusOK, gin.H{"message": "cleared"})
}

func TestRunJob(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	jobID := c.Param("jobId")

	var job models.Job
	if err := db.DB.Where("id = ? AND tenant_id = ?", jobID, tenantID).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job_not_found"})
		return
	}

	// Run in background (async) — AI calls can take 30-120s and SDK may not respect context timeout
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[security] panic in test-run goroutine for job %s: %v", job.Name, r)
			}
		}()
		cfg, _ := config.Load()
		analyzer := engine.NewAnalyzer(cfg)
		ctx := context.Background()
		if _, err := analyzer.RunJobWithLimit(ctx, job, 3); err != nil {
			log.Printf("[test-run] error for job %s: %v", job.Name, err)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "test_run_started"})
}

func TriggerJob(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	jobID := c.Param("jobId")

	var job models.Job
	if err := db.DB.Where("id = ? AND tenant_id = ?", jobID, tenantID).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job_not_found"})
		return
	}

	// mode: "unanalyzed" | "since_last" | "conditional"
	// backward compat: if full=true treat as conditional
	mode := c.Query("mode")
	if mode == "" && c.Query("full") == "true" {
		mode = "conditional"
	}
	if mode == "" {
		mode = "since_last"
	}
	dateFrom := c.Query("from")
	dateTo := c.Query("to")
	limitStr := c.Query("limit")
	var maxConv int
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			maxConv = n
		}
	}

	// Run in background
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[security] panic in trigger goroutine for job %s: %v", job.Name, r)
			}
		}()
		cfg, _ := config.Load()
		analyzer := engine.NewAnalyzer(cfg)
		ctx := context.Background()
		var err error
		switch mode {
		case "unanalyzed":
			_, err = analyzer.RunJobUnanalyzed(ctx, job, maxConv)
		case "conditional":
			_, err = analyzer.RunJobFullWithParams(ctx, job, dateFrom, dateTo, maxConv)
		default: // "since_last"
			_, err = analyzer.RunJobSinceLast(ctx, job, maxConv)
		}
		if err != nil {
			log.Printf("[trigger] error for job %s: %v", job.Name, err)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "job_triggered"})
}

func ListJobRuns(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	jobID := c.Param("jobId")

	var runs []models.JobRun
	db.DB.Where("job_id = ? AND tenant_id = ?", jobID, tenantID).
		Order("started_at DESC").Limit(50).Find(&runs)

	c.JSON(http.StatusOK, runs)
}

func TestOutput(c *gin.Context) {
	var req struct {
		Type     string `json:"type" binding:"required,oneof=telegram email"`
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
		// Email fields can be added later
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}

	ctx := c.Request.Context()

	switch req.Type {
	case "telegram":
		if req.BotToken == "" || req.ChatID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bot_token and chat_id are required"})
			return
		}
		notifier := notifications.NewTelegramNotifier(req.BotToken, req.ChatID)
		err := notifier.Send(ctx, "CQA - Test", "Đây là tin nhắn thử nghiệm từ Chat Quality Agent.\nKết nối Telegram thành công!")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "Telegram message sent"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported output type"})
	}
}

func ListJobResults(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	runID := c.Param("runId")

	var results []models.JobResult
	db.DB.Where("job_run_id = ? AND tenant_id = ?", runID, tenantID).
		Order("created_at DESC").Find(&results)

	c.JSON(http.StatusOK, results)
}

// JobResultWithConvDate extends JobResult with the conversation's start date and customer name.
type JobResultWithConvDate struct {
	models.JobResult
	ConversationDate *time.Time `json:"conversation_date"`
	CustomerName     string     `json:"customer_name"`
}

// ListAllJobResults returns all results across all runs for a job.
func ListAllJobResults(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	jobID := c.Param("jobId")

	// Get all run IDs for this job
	var runIDs []string
	db.DB.Model(&models.JobRun{}).Where("job_id = ? AND tenant_id = ?", jobID, tenantID).
		Pluck("id", &runIDs)

	if len(runIDs) == 0 {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	var results []JobResultWithConvDate
	db.DB.Model(&models.JobResult{}).
		Select("job_results.*, (SELECT MIN(m.sent_at) FROM messages m WHERE m.conversation_id = job_results.conversation_id) as conversation_date, conversations.customer_name as customer_name").
		Joins("LEFT JOIN conversations ON conversations.id = job_results.conversation_id").
		Where("job_results.job_run_id IN ? AND job_results.tenant_id = ?", runIDs, tenantID).
		Order("job_results.created_at DESC").
		Find(&results)

	c.JSON(http.StatusOK, results)
}

// exportConvRow holds one grouped conversation row for export.
type exportConvRow struct {
	CustomerName     string
	ConversationDate string
	EvalDate         string
	Review           string
	Verdict          string
	Score            string
	Issues           string
}

// buildExportRows groups raw JobResultWithConvDate records by conversation and returns sorted rows.
func buildExportRows(results []JobResultWithConvDate) []exportConvRow {
	type convGroup struct {
		customerName     string
		conversationDate string
		evalDate         string
		review           string
		verdict          string
		score            string
		violations       []string
	}
	groups := map[string]*convGroup{}
	order := []string{}

	for _, r := range results {
		cid := r.ConversationID
		if _, ok := groups[cid]; !ok {
			convDate := ""
			if r.ConversationDate != nil {
				convDate = r.ConversationDate.Format("2006-01-02 15:04")
			}
			groups[cid] = &convGroup{
				customerName:     r.CustomerName,
				conversationDate: convDate,
			}
			order = append(order, cid)
		}
		g := groups[cid]
		if r.ResultType == "conversation_evaluation" {
			verdict := r.Severity
			if verdict == "PASS" {
				g.verdict = "Đạt"
			} else if verdict == "SKIP" {
				g.verdict = "Bỏ qua"
			} else {
				g.verdict = "Không đạt"
			}
			g.review = r.Evidence
			g.evalDate = r.CreatedAt.Format("2006-01-02 15:04")
			// Parse score from detail JSON
			var detail map[string]interface{}
			if err := json.Unmarshal([]byte(r.Detail), &detail); err == nil {
				if s, ok := detail["score"]; ok {
					g.score = fmt.Sprintf("%v", s)
				}
			}
		} else if r.ResultType != "conversation_evaluation" {
			issue := r.RuleName
			if r.Evidence != "" {
				issue += ": " + r.Evidence
			}
			g.violations = append(g.violations, issue)
		}
	}

	rows := make([]exportConvRow, 0, len(order))
	for _, cid := range order {
		g := groups[cid]
		rows = append(rows, exportConvRow{
			CustomerName:     g.customerName,
			ConversationDate: g.conversationDate,
			EvalDate:         g.evalDate,
			Review:           g.review,
			Verdict:          g.verdict,
			Score:            g.score,
			Issues:           strings.Join(g.violations, "; "),
		})
	}
	return rows
}

// ExportJobResults returns all results as CSV or XLSX for download.
func ExportJobResults(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	jobID := c.Param("jobId")
	format := c.DefaultQuery("format", "csv")

	var runIDs []string
	db.DB.Model(&models.JobRun{}).Where("job_id = ? AND tenant_id = ?", jobID, tenantID).
		Pluck("id", &runIDs)

	var results []JobResultWithConvDate
	if len(runIDs) > 0 {
		db.DB.Model(&models.JobResult{}).
			Select("job_results.*, (SELECT MIN(m.sent_at) FROM messages m WHERE m.conversation_id = job_results.conversation_id) as conversation_date, conversations.customer_name as customer_name").
			Joins("LEFT JOIN conversations ON conversations.id = job_results.conversation_id").
			Where("job_results.job_run_id IN ? AND job_results.tenant_id = ?", runIDs, tenantID).
			Order("job_results.created_at DESC").
			Find(&results)
	}

	// Detect job type
	var jobType string
	db.DB.Model(&models.Job{}).Where("id = ? AND tenant_id = ?", jobID, tenantID).Pluck("job_type", &jobType)

	if jobType == "classification" {
		exportClassification(c, results, format)
		return
	}

	rows := buildExportRows(results)
	headers := []string{"Tên", "Ngày phát sinh chat", "Ngày đánh giá", "Kết quả đánh giá chi tiết", "Đánh giá", "Điểm", "Vấn đề"}

	if format == "xlsx" {
		f := excelize.NewFile()
		sheet := "Results"
		f.SetSheetName("Sheet1", sheet)
		for i, h := range headers {
			cell, _ := excelize.CoordinatesToCellName(i+1, 1)
			f.SetCellValue(sheet, cell, h)
		}
		for i, r := range rows {
			row := i + 2
			f.SetCellValue(sheet, cellName(1, row), r.CustomerName)
			f.SetCellValue(sheet, cellName(2, row), r.ConversationDate)
			f.SetCellValue(sheet, cellName(3, row), r.EvalDate)
			f.SetCellValue(sheet, cellName(4, row), r.Review)
			f.SetCellValue(sheet, cellName(5, row), r.Verdict)
			f.SetCellValue(sheet, cellName(6, row), r.Score)
			f.SetCellValue(sheet, cellName(7, row), r.Issues)
		}
		c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		c.Header("Content-Disposition", "attachment; filename=results.xlsx")
		f.Write(c.Writer)
		return
	}

	// CSV format
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=results.csv")
	csvStr := "\xEF\xBB\xBF"
	csvStr += strings.Join(headers, ",") + "\n"
	for _, r := range rows {
		escape := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
		csvStr += fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s\n",
			escape(r.CustomerName), escape(r.ConversationDate), escape(r.EvalDate),
			escape(r.Review), escape(r.Verdict), escape(r.Score), escape(r.Issues))
	}
	c.String(http.StatusOK, csvStr)
}

// exportClassification exports classification results with Tags + Issues + Chat content columns.
func exportClassification(c *gin.Context, results []JobResultWithConvDate, format string) {
	type classRow struct {
		CustomerName     string
		ConversationDate string
		EvalDate         string
		Tags             string
		Issues           string
		ChatContent      string
	}

	type convGroup struct {
		customerName     string
		conversationDate string
		evalDate         string
		convID           string
		tags             []string
		issues           []string
	}
	groups := map[string]*convGroup{}
	order := []string{}

	for _, r := range results {
		cid := r.ConversationID
		if _, ok := groups[cid]; !ok {
			convDate := ""
			if r.ConversationDate != nil {
				convDate = r.ConversationDate.Format("2006-01-02 15:04")
			}
			groups[cid] = &convGroup{
				customerName:     r.CustomerName,
				conversationDate: convDate,
				convID:           cid,
			}
			order = append(order, cid)
		}
		g := groups[cid]
		if r.ResultType == "conversation_evaluation" {
			g.evalDate = r.CreatedAt.Format("2006-01-02 15:04")
		} else if r.ResultType == "classification_tag" {
			g.tags = append(g.tags, r.RuleName)
			if r.Evidence != "" {
				g.issues = append(g.issues, r.Evidence)
			}
		}
	}

	// Fetch chat messages for each conversation
	tenantID := middleware.GetTenantID(c)
	chatMap := map[string]string{}
	for _, cid := range order {
		var messages []models.Message
		db.DB.Where("conversation_id = ? AND tenant_id = ?", cid, tenantID).Order("sent_at ASC").Find(&messages)
		var lines []string
		for _, m := range messages {
			name := m.SenderName
			if name == "" {
				name = m.SenderType
			}
			if m.Content != "" {
				lines = append(lines, fmt.Sprintf("[%s] %s", name, m.Content))
			}
		}
		chatMap[cid] = strings.Join(lines, "\n")
	}

	rows := make([]classRow, 0, len(order))
	for _, cid := range order {
		g := groups[cid]
		rows = append(rows, classRow{
			CustomerName:     g.customerName,
			ConversationDate: g.conversationDate,
			EvalDate:         g.evalDate,
			Tags:             strings.Join(g.tags, "\n"),
			Issues:           strings.Join(g.issues, "\n"),
			ChatContent:      chatMap[cid],
		})
	}

	headers := []string{"Tên", "Ngày phát sinh chat", "Ngày đánh giá", "Loại", "Vấn đề", "Nội dung chat"}

	if format == "xlsx" {
		f := excelize.NewFile()
		sheet := "Results"
		f.SetSheetName("Sheet1", sheet)
		for i, h := range headers {
			cell, _ := excelize.CoordinatesToCellName(i+1, 1)
			f.SetCellValue(sheet, cell, h)
		}
		for i, r := range rows {
			row := i + 2
			f.SetCellValue(sheet, cellName(1, row), r.CustomerName)
			f.SetCellValue(sheet, cellName(2, row), r.ConversationDate)
			f.SetCellValue(sheet, cellName(3, row), r.EvalDate)
			f.SetCellValue(sheet, cellName(4, row), r.Tags)
			f.SetCellValue(sheet, cellName(5, row), r.Issues)
			f.SetCellValue(sheet, cellName(6, row), r.ChatContent)
		}
		c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		c.Header("Content-Disposition", "attachment; filename=classification.xlsx")
		f.Write(c.Writer)
		return
	}

	// CSV
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=classification.csv")
	csvStr := "\xEF\xBB\xBF"
	csvStr += strings.Join(headers, ",") + "\n"
	escape := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
	for _, r := range rows {
		csvStr += fmt.Sprintf("%s,%s,%s,%s,%s,%s\n",
			escape(r.CustomerName), escape(r.ConversationDate), escape(r.EvalDate),
			escape(r.Tags), escape(r.Issues), escape(r.ChatContent))
	}
	c.String(http.StatusOK, csvStr)
}

func cellName(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}
