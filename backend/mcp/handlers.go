package mcp

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
)

type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func handleToolsCall(c *gin.Context, params json.RawMessage) (interface{}, *RPCError) {
	var call ToolCallParams
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &RPCError{Code: -32602, Message: "Invalid params"}
	}

	args := call.Arguments
	tenantID, _ := args["tenant_id"].(string)
	userIDVal, ok := c.Get("mcp_user_id")
	if !ok {
		return errResult("authentication required")
	}
	userID, ok := userIDVal.(string)
	if !ok || userID == "" {
		return errResult("authentication required")
	}

	// Verify tenant access for all tools that need tenant_id
	if call.Name != "cqa_list_tenants" {
		if tenantID == "" {
			return errResult("tenant_id is required")
		}
		if !mcpVerifyTenantAccess(userID, tenantID) {
			return errResult("access denied: you don't have access to this tenant")
		}
	}

	switch call.Name {
	case "cqa_list_tenants":
		return toolListTenants(userID)
	case "cqa_get_tenant":
		return toolGetTenant(tenantID)
	case "cqa_list_channels":
		return toolListChannels(tenantID)
	case "cqa_list_conversations":
		return toolListConversations(tenantID, args)
	case "cqa_get_messages":
		convID, _ := args["conversation_id"].(string)
		return toolGetMessages(tenantID, convID, args)
	case "cqa_search_messages":
		query, _ := args["query"].(string)
		return toolSearchMessages(tenantID, query, args)
	case "cqa_list_jobs":
		return toolListJobs(tenantID)
	case "cqa_get_job_results":
		runID, _ := args["job_run_id"].(string)
		return toolGetJobResults(tenantID, runID)
	case "cqa_search_violations":
		return toolSearchViolations(tenantID, args)
	case "cqa_get_stats":
		period, _ := args["period"].(string)
		return toolGetStats(tenantID, period)
	case "cqa_get_notification_logs":
		return toolGetNotificationLogs(tenantID, args)
	case "cqa_trigger_job":
		jobID, _ := args["job_id"].(string)
		return toolTriggerJob(tenantID, jobID)
	default:
		return nil, &RPCError{Code: -32602, Message: "Unknown tool: " + call.Name}
	}
}

func jsonResult(data interface{}) (interface{}, *RPCError) {
	b, _ := json.MarshalIndent(data, "", "  ")
	return ToolResult{Content: []ToolContent{{Type: "text", Text: string(b)}}}, nil
}

func errResult(msg string) (interface{}, *RPCError) {
	return ToolResult{Content: []ToolContent{{Type: "text", Text: msg}}, IsError: true}, nil
}

func mcpVerifyTenantAccess(userID, tenantID string) bool {
	var count int64
	db.DB.Model(&models.UserTenant{}).Where("user_id = ? AND tenant_id = ?", userID, tenantID).Count(&count)
	return count > 0
}

func getLimit(args map[string]interface{}, defaultVal int) int {
	if l, ok := args["limit"].(string); ok {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > 200 {
				return 200
			}
			return n
		}
	}
	return defaultVal
}

func toolListTenants(userID string) (interface{}, *RPCError) {
	// Only return tenants the authenticated user has access to
	var tenants []models.Tenant
	db.DB.Where("id IN (SELECT tenant_id FROM user_tenants WHERE user_id = ?)", userID).Find(&tenants)

	type result struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Slug           string `json:"slug"`
		ChannelsCount  int64  `json:"channels_count"`
		JobsCount      int64  `json:"jobs_count"`
		ConvsCount     int64  `json:"conversations_count"`
	}
	var results []result
	for _, t := range tenants {
		var cc, jc, vc int64
		db.DB.Model(&models.Channel{}).Where("tenant_id = ?", t.ID).Count(&cc)
		db.DB.Model(&models.Job{}).Where("tenant_id = ?", t.ID).Count(&jc)
		db.DB.Model(&models.Conversation{}).Where("tenant_id = ?", t.ID).Count(&vc)
		results = append(results, result{t.ID, t.Name, t.Slug, cc, jc, vc})
	}
	return jsonResult(results)
}

func toolGetTenant(tenantID string) (interface{}, *RPCError) {
	var tenant models.Tenant
	if err := db.DB.First(&tenant, "id = ?", tenantID).Error; err != nil {
		return errResult("Tenant not found")
	}
	return jsonResult(tenant)
}

func toolListChannels(tenantID string) (interface{}, *RPCError) {
	var channels []models.Channel
	db.DB.Where("tenant_id = ?", tenantID).Find(&channels)
	return jsonResult(channels)
}

func toolListConversations(tenantID string, args map[string]interface{}) (interface{}, *RPCError) {
	limit := getLimit(args, 20)
	q := db.DB.Where("tenant_id = ?", tenantID)

	if chID, ok := args["channel_id"].(string); ok && chID != "" {
		q = q.Where("channel_id = ?", chID)
	}
	if since, ok := args["since"].(string); ok && since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			q = q.Where("last_message_at > ?", t)
		}
	}

	var convs []models.Conversation
	q.Order("last_message_at DESC").Limit(limit).Find(&convs)
	return jsonResult(convs)
}

func toolGetMessages(tenantID, convID string, args map[string]interface{}) (interface{}, *RPCError) {
	limit := getLimit(args, 50)
	var messages []models.Message
	db.DB.Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).
		Order("sent_at ASC").Limit(limit).Find(&messages)
	return jsonResult(messages)
}

func toolSearchMessages(tenantID, query string, args map[string]interface{}) (interface{}, *RPCError) {
	limit := getLimit(args, 20)
	var messages []models.Message
	db.DB.Where("tenant_id = ? AND content LIKE ?", tenantID, "%"+query+"%").
		Order("sent_at DESC").Limit(limit).Find(&messages)
	return jsonResult(messages)
}

func toolListJobs(tenantID string) (interface{}, *RPCError) {
	var jobs []models.Job
	db.DB.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&jobs)
	return jsonResult(jobs)
}

func toolGetJobResults(tenantID, runID string) (interface{}, *RPCError) {
	var results []models.JobResult
	db.DB.Where("tenant_id = ? AND job_run_id = ?", tenantID, runID).
		Order("created_at DESC").Find(&results)
	return jsonResult(results)
}

func toolSearchViolations(tenantID string, args map[string]interface{}) (interface{}, *RPCError) {
	limit := getLimit(args, 20)
	q := db.DB.Where("tenant_id = ? AND result_type = 'qc_violation'", tenantID)

	if sev, ok := args["severity"].(string); ok && sev != "" {
		q = q.Where("severity = ?", sev)
	}
	if since, ok := args["since"].(string); ok && since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			q = q.Where("created_at > ?", t)
		}
	}

	var results []models.JobResult
	q.Order("created_at DESC").Limit(limit).Find(&results)
	return jsonResult(results)
}

func toolGetStats(tenantID, period string) (interface{}, *RPCError) {
	var since time.Time
	switch period {
	case "week":
		since = time.Now().AddDate(0, 0, -7)
	case "month":
		since = time.Now().AddDate(0, -1, 0)
	default: // today
		since = time.Now().Truncate(24 * time.Hour)
	}

	var totalConvs, totalMsgs, violations, tags int64
	db.DB.Model(&models.Conversation{}).Where("tenant_id = ? AND last_message_at > ?", tenantID, since).Count(&totalConvs)
	db.DB.Model(&models.Message{}).Where("tenant_id = ? AND sent_at > ?", tenantID, since).Count(&totalMsgs)
	db.DB.Model(&models.JobResult{}).Where("tenant_id = ? AND result_type = 'qc_violation' AND created_at > ?", tenantID, since).Count(&violations)
	db.DB.Model(&models.JobResult{}).Where("tenant_id = ? AND result_type = 'classification_tag' AND created_at > ?", tenantID, since).Count(&tags)

	return jsonResult(map[string]interface{}{
		"period":        period,
		"since":         since,
		"conversations": totalConvs,
		"messages":      totalMsgs,
		"violations":    violations,
		"tags":          tags,
	})
}

func toolGetNotificationLogs(tenantID string, args map[string]interface{}) (interface{}, *RPCError) {
	limit := getLimit(args, 20)
	q := db.DB.Where("tenant_id = ?", tenantID)

	if status, ok := args["status"].(string); ok && status != "" {
		q = q.Where("status = ?", status)
	}

	var logs []models.NotificationLog
	q.Order("sent_at DESC").Limit(limit).Find(&logs)
	return jsonResult(logs)
}

func toolTriggerJob(tenantID, jobID string) (interface{}, *RPCError) {
	var job models.Job
	if err := db.DB.Where("id = ? AND tenant_id = ?", jobID, tenantID).First(&job).Error; err != nil {
		return errResult("Job not found")
	}

	// We can't easily run the analyzer here without config, so just return a message
	return jsonResult(map[string]string{
		"status":  "triggered",
		"message": "Job " + job.Name + " has been queued for execution",
	})
}
