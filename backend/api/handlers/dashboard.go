package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
)

func GetDashboard(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	// Date filter (optional)
	now := time.Now()
	today := now.Truncate(24 * time.Hour)
	from := today
	to := now

	if f := c.Query("from"); f != "" {
		if t, err := time.Parse("2006-01-02", f); err == nil {
			from = t
		}
	}
	if t := c.Query("to"); t != "" {
		if parsed, err := time.Parse("2006-01-02", t); err == nil {
			to = parsed.Add(24*time.Hour - time.Second) // end of day
		}
	}

	// Static stats (not time-dependent)
	var activeChannels, activeJobs int64
	db.DB.Model(&models.Channel{}).Where("tenant_id = ? AND is_active = true", tenantID).Count(&activeChannels)
	db.DB.Model(&models.Job{}).Where("tenant_id = ? AND is_active = true", tenantID).Count(&activeJobs)

	// Time-dependent stats
	var totalConversations, issuesToday int64
	db.DB.Model(&models.Conversation{}).Where("tenant_id = ? AND last_message_at BETWEEN ? AND ?", tenantID, from, to).Count(&totalConversations)
	db.DB.Model(&models.JobResult{}).Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, from, to).Count(&issuesToday)

	// Conversations by channel type
	type ChannelCount struct {
		ChannelType string `json:"channel_type"`
		Count       int64  `json:"count"`
	}
	var channelCounts []ChannelCount
	db.DB.Model(&models.Conversation{}).
		Joins("JOIN channels ON channels.id = conversations.channel_id").
		Where("conversations.tenant_id = ? AND conversations.last_message_at BETWEEN ? AND ?", tenantID, from, to).
		Select("channels.channel_type, COUNT(*) as count").
		Group("channels.channel_type").
		Scan(&channelCounts)

	// QC Alerts: only qc_violation (real quality issues)
	var qcAlerts []models.JobResult
	db.DB.Where("tenant_id = ? AND result_type = 'qc_violation' AND created_at BETWEEN ? AND ?", tenantID, from, to).
		Order("created_at DESC").Limit(5).Find(&qcAlerts)

	// Classification recent: only classification_tag
	type ClassificationItem struct {
		models.JobResult
		CustomerName string `json:"customer_name"`
	}
	var classRecent []ClassificationItem
	db.DB.Model(&models.JobResult{}).
		Select("job_results.*, conversations.customer_name").
		Joins("LEFT JOIN conversations ON conversations.id = job_results.conversation_id").
		Where("job_results.tenant_id = ? AND job_results.result_type = 'classification_tag' AND job_results.created_at BETWEEN ? AND ?", tenantID, from, to).
		Order("job_results.created_at DESC").Limit(10).Find(&classRecent)

	// AI cost
	var costPeriod float64
	db.DB.Model(&models.AIUsageLog{}).Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, from, to).
		Select("COALESCE(SUM(cost_usd), 0)").Scan(&costPeriod)

	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	var costMonth float64
	db.DB.Model(&models.AIUsageLog{}).Where("tenant_id = ? AND created_at >= ?", tenantID, monthStart).
		Select("COALESCE(SUM(cost_usd), 0)").Scan(&costMonth)

	// Cost by day
	type DayCost struct {
		Date         string  `json:"date"`
		TotalCost    float64 `json:"total_cost"`
		InputTokens  int64   `json:"input_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		CallCount    int64   `json:"call_count"`
	}
	var costByDay []DayCost
	thirtyDaysAgo := today.Add(-30 * 24 * time.Hour)
	db.DB.Model(&models.AIUsageLog{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, thirtyDaysAgo).
		Select("DATE(created_at) as date, SUM(cost_usd) as total_cost, SUM(input_tokens) as input_tokens, SUM(output_tokens) as output_tokens, COUNT(*) as call_count").
		Group("DATE(created_at)").
		Order("date DESC").
		Scan(&costByDay)

	// Messages by day (with chat count + reply count)
	type DayMessages struct {
		Date       string `json:"date"`
		Count      int64  `json:"count"`
		ChatCount  int64  `json:"chat_count"`  // distinct conversations with customer messages
		ReplyCount int64  `json:"reply_count"` // agent replies
	}
	var messagesByDay []DayMessages
	db.DB.Model(&models.Message{}).
		Where("tenant_id = ? AND sent_at >= ?", tenantID, thirtyDaysAgo).
		Select(`DATE(sent_at) as date,
			COUNT(*) as count,
			COUNT(DISTINCT CASE WHEN sender_type = 'customer' THEN conversation_id END) as chat_count,
			SUM(CASE WHEN sender_type = 'agent' THEN 1 ELSE 0 END) as reply_count`).
		Group("DATE(sent_at)").
		Order("date ASC").
		Scan(&messagesByDay)

	c.JSON(http.StatusOK, gin.H{
		"total_conversations":      totalConversations,
		"active_channels":          activeChannels,
		"active_jobs":              activeJobs,
		"issues_today":             issuesToday,
		"conversations_by_channel": channelCounts,
		"qc_alerts":                qcAlerts,
		"classification_recent":    classRecent,
		"cost_today":               costPeriod,
		"cost_this_month":          costMonth,
		"cost_by_day":              costByDay,
		"messages_by_day":          messagesByDay,
	})
}

func GetOnboardingStatus(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	// Step 1: Has channels?
	var channelCount int64
	db.DB.Model(&models.Channel{}).Where("tenant_id = ?", tenantID).Count(&channelCount)

	// Step 2: Has conversations (synced)?
	var convCount int64
	db.DB.Model(&models.Conversation{}).Where("tenant_id = ?", tenantID).Count(&convCount)

	// Step 3: AI configured?
	var aiSetting models.AppSetting
	aiConfigured := db.DB.Where("tenant_id = ? AND setting_key = ? AND value_plain != ''", tenantID, "ai_provider").First(&aiSetting).Error == nil

	// Step 4: Has jobs?
	var jobCount int64
	db.DB.Model(&models.Job{}).Where("tenant_id = ?", tenantID).Count(&jobCount)

	// Step 5: Has job runs?
	var runCount int64
	db.DB.Model(&models.JobRun{}).Where("tenant_id = ?", tenantID).Count(&runCount)

	// Check if dismissed
	var dismissSetting models.AppSetting
	dismissed := false
	if db.DB.Where("tenant_id = ? AND setting_key = ?", tenantID, "onboarding_dismissed").First(&dismissSetting).Error == nil {
		dismissed = dismissSetting.ValuePlain == "true"
	}

	c.JSON(http.StatusOK, gin.H{
		"dismissed": dismissed,
		"steps": []gin.H{
			{"key": "channel", "title": "Kết nối kênh chat", "done": channelCount > 0, "link": "channels"},
			{"key": "sync", "title": "Đồng bộ tin nhắn", "done": convCount > 0, "link": "messages"},
			{"key": "ai", "title": "Cấu hình AI Provider", "done": aiConfigured, "link": "settings"},
			{"key": "job", "title": "Tạo công việc phân tích", "done": jobCount > 0, "link": "jobs/create"},
			{"key": "run", "title": "Chạy thử phân tích", "done": runCount > 0, "link": "jobs"},
		},
	})
}

func ListNotificationLogs(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage > 100 {
		perPage = 100
	}

	var total int64
	db.DB.Model(&models.NotificationLog{}).Where("tenant_id = ?", tenantID).Count(&total)

	var logs []models.NotificationLog
	db.DB.Where("tenant_id = ?", tenantID).Order("sent_at DESC").
		Offset((page - 1) * perPage).Limit(perPage).Find(&logs)

	c.JSON(http.StatusOK, gin.H{
		"data":     logs,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}
