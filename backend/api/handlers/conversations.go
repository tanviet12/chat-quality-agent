package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
)

func ListConversations(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}

	channelID := c.Query("channel_id")
	channelType := c.Query("channel_type")
	search := c.Query("search")
	evalFilter := c.Query("evaluation") // evaluated | not_evaluated | PASS | FAIL

	query := db.DB.Where("conversations.tenant_id = ?", tenantID)

	if channelID != "" {
		query = query.Where("conversations.channel_id = ?", channelID)
	}
	if channelType != "" {
		query = query.Joins("JOIN channels ON channels.id = conversations.channel_id").
			Where("channels.channel_type = ?", channelType)
	}
	if search != "" {
		query = query.Where("conversations.customer_name LIKE ?", "%"+search+"%")
	}

	// Evaluation filter
	if evalFilter != "" {
		evalSubquery := "conversations.id IN (SELECT DISTINCT conversation_id FROM job_results WHERE tenant_id = ? AND result_type = 'conversation_evaluation')"
		switch evalFilter {
		case "evaluated":
			query = query.Where(evalSubquery, tenantID)
		case "not_evaluated":
			query = query.Where("conversations.id NOT IN (SELECT DISTINCT conversation_id FROM job_results WHERE tenant_id = ? AND result_type = 'conversation_evaluation')", tenantID)
		case "PASS":
			query = query.Where("conversations.id IN (SELECT jr.conversation_id FROM job_results jr INNER JOIN (SELECT conversation_id, MAX(created_at) as mc FROM job_results WHERE tenant_id = ? AND result_type = 'conversation_evaluation' GROUP BY conversation_id) latest ON jr.conversation_id = latest.conversation_id AND jr.created_at = latest.mc WHERE jr.severity = 'PASS')", tenantID)
		case "FAIL":
			query = query.Where("conversations.id IN (SELECT jr.conversation_id FROM job_results jr INNER JOIN (SELECT conversation_id, MAX(created_at) as mc FROM job_results WHERE tenant_id = ? AND result_type = 'conversation_evaluation' GROUP BY conversation_id) latest ON jr.conversation_id = latest.conversation_id AND jr.created_at = latest.mc WHERE jr.severity = 'FAIL')", tenantID)
		}
	}

	var total int64
	query.Model(&models.Conversation{}).Count(&total)

	var conversations []models.Conversation
	query.Order("conversations.last_message_at DESC").
		Offset((page - 1) * perPage).
		Limit(perPage).
		Find(&conversations)

	// Get channel info for each conversation
	channelMap := make(map[string]models.Channel)
	var channelIDs []string
	for _, conv := range conversations {
		if _, ok := channelMap[conv.ChannelID]; !ok {
			channelIDs = append(channelIDs, conv.ChannelID)
		}
	}
	if len(channelIDs) > 0 {
		var channels []models.Channel
		db.DB.Where("id IN ?", channelIDs).Find(&channels)
		for _, ch := range channels {
			channelMap[ch.ID] = ch
		}
	}

	type ConvResponse struct {
		ID             string  `json:"id"`
		ChannelID      string  `json:"channel_id"`
		ChannelName    string  `json:"channel_name"`
		ChannelType    string  `json:"channel_type"`
		CustomerName   string  `json:"customer_name"`
		LastMessageAt  *string `json:"last_message_at"`
		MessageCount   int     `json:"message_count"`
		CreatedAt      string  `json:"created_at"`
	}

	results := make([]ConvResponse, len(conversations))
	for i, conv := range conversations {
		var lastMsg *string
		if conv.LastMessageAt != nil {
			s := conv.LastMessageAt.Format("2006-01-02T15:04:05Z")
			lastMsg = &s
		}
		ch := channelMap[conv.ChannelID]
		results[i] = ConvResponse{
			ID:            conv.ID,
			ChannelID:     conv.ChannelID,
			ChannelName:   ch.Name,
			ChannelType:   ch.ChannelType,
			CustomerName:  conv.CustomerName,
			LastMessageAt: lastMsg,
			MessageCount:  conv.MessageCount,
			CreatedAt:     conv.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":     results,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func GetConversationMessages(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	conversationID := c.Param("conversationId")

	// Verify conversation belongs to tenant
	var conv models.Conversation
	if err := db.DB.Where("id = ? AND tenant_id = ?", conversationID, tenantID).First(&conv).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation_not_found"})
		return
	}

	var messages []models.Message
	db.DB.Where("conversation_id = ? AND tenant_id = ?", conversationID, tenantID).
		Order("sent_at ASC").
		Find(&messages)

	type MsgResponse struct {
		ID          string `json:"id"`
		SenderType  string `json:"sender_type"`
		SenderName  string `json:"sender_name"`
		Content     string `json:"content"`
		ContentType string `json:"content_type"`
		Attachments string `json:"attachments"`
		SentAt      string `json:"sent_at"`
	}

	results := make([]MsgResponse, len(messages))
	for i, msg := range messages {
		results[i] = MsgResponse{
			ID:          msg.ID,
			SenderType:  msg.SenderType,
			SenderName:  msg.SenderName,
			Content:     msg.Content,
			ContentType: msg.ContentType,
			Attachments: msg.Attachments,
			SentAt:      msg.SentAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"conversation": gin.H{
			"id":            conv.ID,
			"customer_name": conv.CustomerName,
			"message_count": conv.MessageCount,
		},
		"messages": results,
	})
}

// ListEvaluatedConversations returns a map of conversation_id -> verdict for all evaluated conversations.
func ListEvaluatedConversations(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	type evalResult struct {
		ConversationID string
		Severity       string
	}
	var results []evalResult
	// Get latest evaluation per conversation (conversation_evaluation type, ordered by created_at desc)
	db.DB.Raw(`
		SELECT jr.conversation_id, jr.severity
		FROM job_results jr
		INNER JOIN (
			SELECT conversation_id, MAX(created_at) as max_created
			FROM job_results
			WHERE tenant_id = ? AND result_type = 'conversation_evaluation'
			GROUP BY conversation_id
		) latest ON jr.conversation_id = latest.conversation_id AND jr.created_at = latest.max_created
		WHERE jr.tenant_id = ? AND jr.result_type = 'conversation_evaluation'
	`, tenantID, tenantID).Scan(&results)

	evalMap := make(map[string]string)
	for _, r := range results {
		evalMap[r.ConversationID] = r.Severity
	}
	c.JSON(http.StatusOK, evalMap)
}

// GetConversationEvaluations returns ALL evaluation results for a conversation, grouped by job run.
func GetConversationEvaluations(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	convID := c.Param("conversationId")

	// Verify conversation belongs to tenant
	var conv models.Conversation
	if err := db.DB.Where("id = ? AND tenant_id = ?", convID, tenantID).First(&conv).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation_not_found"})
		return
	}

	// Get all results for this conversation
	var results []models.JobResult
	db.DB.Where("conversation_id = ? AND tenant_id = ?", convID, tenantID).
		Order("created_at DESC").Find(&results)

	if len(results) == 0 {
		c.JSON(http.StatusOK, gin.H{"has_evaluation": false, "groups": []interface{}{}})
		return
	}

	// Collect unique job_run_ids
	runIDSet := map[string]bool{}
	for _, r := range results {
		runIDSet[r.JobRunID] = true
	}
	var runIDs []string
	for id := range runIDSet {
		runIDs = append(runIDs, id)
	}

	// Fetch job_runs with job info
	type runInfo struct {
		RunID       string    `json:"run_id"`
		JobName     string    `json:"job_name"`
		JobType     string    `json:"job_type"`
		EvaluatedAt time.Time `json:"evaluated_at"`
	}
	var runs []runInfo
	db.DB.Model(&models.JobRun{}).
		Select("job_runs.id as run_id, jobs.name as job_name, jobs.job_type as job_type, job_runs.started_at as evaluated_at").
		Joins("LEFT JOIN jobs ON jobs.id = job_runs.job_id").
		Where("job_runs.id IN ?", runIDs).
		Order("job_runs.started_at DESC").
		Find(&runs)

	runMap := map[string]runInfo{}
	for _, r := range runs {
		runMap[r.RunID] = r
	}

	// Group results by job_run_id
	type evalGroup struct {
		JobRunID    string             `json:"job_run_id"`
		JobName     string             `json:"job_name"`
		JobType     string             `json:"job_type"`
		EvaluatedAt time.Time          `json:"evaluated_at"`
		Results     []models.JobResult `json:"results"`
	}
	groupMap := map[string]*evalGroup{}
	var groupOrder []string
	for _, r := range results {
		if _, ok := groupMap[r.JobRunID]; !ok {
			info := runMap[r.JobRunID]
			groupMap[r.JobRunID] = &evalGroup{
				JobRunID:    r.JobRunID,
				JobName:     info.JobName,
				JobType:     info.JobType,
				EvaluatedAt: info.EvaluatedAt,
			}
			groupOrder = append(groupOrder, r.JobRunID)
		}
		groupMap[r.JobRunID].Results = append(groupMap[r.JobRunID].Results, r)
	}

	var groups []evalGroup
	for _, id := range groupOrder {
		groups = append(groups, *groupMap[id])
	}

	c.JSON(http.StatusOK, gin.H{
		"has_evaluation": true,
		"groups":         groups,
	})
}

// GetConversationPage returns which page a conversation is on in the default sorted list.
func GetConversationPage(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	convID := c.Param("conversationId")
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "9"))
	if perPage < 1 || perPage > 100 {
		perPage = 9
	}

	// Find the target conversation
	var conv models.Conversation
	if err := db.DB.Where("id = ? AND tenant_id = ?", convID, tenantID).First(&conv).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation_not_found"})
		return
	}

	// Count conversations that come before this one (ordered by last_message_at DESC)
	var position int64
	db.DB.Model(&models.Conversation{}).
		Where("tenant_id = ? AND last_message_at > ?", tenantID, conv.LastMessageAt).
		Count(&position)

	page := int(position)/perPage + 1

	c.JSON(http.StatusOK, gin.H{"page": page})
}

// ExportMessages exports conversations + messages as plain text or CSV within a date range.
// Query params: from, to (YYYY-MM-DD), format (txt|csv), channel_id, channel_type
func ExportMessages(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	format := c.DefaultQuery("format", "txt")

	// Parse date range
	fromStr := c.Query("from")
	toStr := c.Query("to")
	if fromStr == "" || toStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cần chọn ngày bắt đầu (from) và ngày kết thúc (to)"})
		return
	}

	fromDate, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Ngày bắt đầu không hợp lệ"})
		return
	}
	toDate, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Ngày kết thúc không hợp lệ"})
		return
	}
	toDate = toDate.Add(24*time.Hour - time.Second) // include the whole end day

	// Query conversations in date range
	query := db.DB.Where("conversations.tenant_id = ? AND conversations.last_message_at >= ? AND conversations.last_message_at <= ?",
		tenantID, fromDate, toDate)

	if chID := c.Query("channel_id"); chID != "" {
		query = query.Where("conversations.channel_id = ?", chID)
	}
	if chType := c.Query("channel_type"); chType != "" {
		query = query.Joins("JOIN channels ON channels.id = conversations.channel_id").
			Where("channels.channel_type = ?", chType)
	}

	var conversations []models.Conversation
	query.Order("conversations.last_message_at ASC").Find(&conversations)

	if len(conversations) == 0 {
		c.JSON(http.StatusOK, gin.H{"error": "Không có cuộc chat nào trong khoảng thời gian này"})
		return
	}

	// Fetch all messages for these conversations
	convIDs := make([]string, len(conversations))
	for i, conv := range conversations {
		convIDs[i] = conv.ID
	}

	var allMessages []models.Message
	db.DB.Where("tenant_id = ? AND conversation_id IN ?", tenantID, convIDs).
		Order("conversation_id, sent_at ASC").
		Find(&allMessages)

	// Group messages by conversation
	msgMap := make(map[string][]models.Message)
	for _, msg := range allMessages {
		msgMap[msg.ConversationID] = append(msgMap[msg.ConversationID], msg)
	}

	if format == "csv" {
		exportMessagesCSV(c, conversations, msgMap, fromStr, toStr)
		return
	}

	// Default: plain text format optimized for AI reading
	exportMessagesTXT(c, conversations, msgMap, fromStr, toStr)
}

func exportMessagesTXT(c *gin.Context, conversations []models.Conversation, msgMap map[string][]models.Message, fromStr, toStr string) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== EXPORT TIN NHẮN: %s đến %s ===\n", fromStr, toStr))
	sb.WriteString(fmt.Sprintf("Tổng số cuộc chat: %d\n\n", len(conversations)))

	for i, conv := range conversations {
		msgs := msgMap[conv.ID]
		var firstMsg time.Time
		if len(msgs) > 0 {
			firstMsg = msgs[0].SentAt
		}

		sb.WriteString(fmt.Sprintf("--- Cuộc chat #%d: %s ---\n", i+1, conv.CustomerName))
		sb.WriteString(fmt.Sprintf("Ngày: %s | Số tin nhắn: %d\n", firstMsg.Format("02/01/2006 15:04"), len(msgs)))
		sb.WriteString("\n")

		for _, msg := range msgs {
			ts := msg.SentAt.Format("15:04")
			name := msg.SenderName
			if name == "" {
				if msg.SenderType == "agent" {
					name = "OA"
				} else {
					name = conv.CustomerName
				}
			}
			content := msg.Content
			if content == "" && msg.ContentType != "text" {
				content = fmt.Sprintf("[%s]", msg.ContentType)
			}
			sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", ts, name, content))
		}
		sb.WriteString("\n")
	}

	filename := fmt.Sprintf("messages_%s_%s.txt", fromStr, toStr)
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.String(http.StatusOK, sb.String())
}

func exportMessagesCSV(c *gin.Context, conversations []models.Conversation, msgMap map[string][]models.Message, fromStr, toStr string) {
	var sb strings.Builder
	sb.WriteString("\xEF\xBB\xBF") // UTF-8 BOM
	sb.WriteString("Khách hàng,Ngày chat,Người gửi,Loại,Thời gian,Nội dung\n")

	escape := func(s string) string {
		s = strings.ReplaceAll(s, `"`, `""`)
		s = strings.ReplaceAll(s, "\n", " ")
		s = strings.ReplaceAll(s, "\r", "")
		return `"` + s + `"`
	}

	for _, conv := range conversations {
		msgs := msgMap[conv.ID]
		var convDate string
		if len(msgs) > 0 {
			convDate = msgs[0].SentAt.Format("02/01/2006")
		}

		for _, msg := range msgs {
			name := msg.SenderName
			if name == "" {
				if msg.SenderType == "agent" {
					name = "OA"
				} else {
					name = conv.CustomerName
				}
			}
			content := msg.Content
			if content == "" && msg.ContentType != "text" {
				content = fmt.Sprintf("[%s]", msg.ContentType)
			}
			sb.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%s\n",
				escape(conv.CustomerName),
				escape(convDate),
				escape(name),
				escape(msg.SenderType),
				escape(msg.SentAt.Format("15:04")),
				escape(content),
			))
		}
	}

	filename := fmt.Sprintf("messages_%s_%s.csv", fromStr, toStr)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.String(http.StatusOK, sb.String())
}
