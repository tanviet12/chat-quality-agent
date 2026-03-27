package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/config"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/engine"
)

// verifyTenantAccess checks user has access to the specified tenant.
func verifyTenantAccess(c *gin.Context, tenantID string) bool {
	userID := middleware.GetUserID(c)
	var ut models.UserTenant
	if db.DB.Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&ut).Error != nil {
		log.Printf("[security] agent API tenant access denied: user=%s tenant=%s ip=%s", userID, tenantID, c.ClientIP())
		c.JSON(http.StatusForbidden, gin.H{"error": "tenant_access_denied"})
		return false
	}
	return true
}

// Agent capability descriptor for Company OS discovery.
type AgentInfo struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
}

func ListAgents(c *gin.Context) {
	agents := []AgentInfo{
		{
			Name:         "cqa.sync",
			Description:  "Sync chat messages from external channels (Zalo OA, Facebook) into CQA database",
			Version:      "1.0.0",
			Capabilities: []string{"sync_all", "sync_channel", "query:conversations", "query:messages"},
		},
		{
			Name:         "cqa.qc",
			Description:  "Analyze customer service chat quality against defined rules using AI",
			Version:      "1.0.0",
			Capabilities: []string{"analyze_quality", "query:violations", "query:scores"},
		},
		{
			Name:         "cqa.classify",
			Description:  "Classify and tag conversations using AI-powered rule matching",
			Version:      "1.0.0",
			Capabilities: []string{"classify_conversations", "query:tags", "query:rules"},
		},
	}
	c.JSON(http.StatusOK, agents)
}

type AgentRunRequest struct {
	TenantID string                 `json:"tenant_id" binding:"required"`
	Action   string                 `json:"action" binding:"required"`
	Params   map[string]interface{} `json:"params"`
}

type AgentRunResponse struct {
	Status  string                 `json:"status"`
	Summary map[string]interface{} `json:"summary,omitempty"`
	Errors  []string               `json:"errors,omitempty"`
}

func AgentRun(c *gin.Context) {
	agentName := c.Param("agentName")

	var req AgentRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}

	// Verify user has access to the requested tenant
	if !verifyTenantAccess(c, req.TenantID) {
		return
	}

	cfg, _ := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	switch agentName {
	case "cqa.sync":
		result := handleSyncAgent(ctx, cfg, req)
		c.JSON(http.StatusOK, result)
	case "cqa.qc", "cqa.classify":
		result := handleAnalysisAgent(ctx, cfg, req, agentName)
		c.JSON(http.StatusOK, result)
	default:
		c.JSON(http.StatusNotFound, gin.H{"error": "agent_not_found"})
	}
}

func AgentQuery(c *gin.Context) {
	agentName := c.Param("agentName")
	tenantID := c.Query("tenant_id")
	resource := c.Query("resource")

	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id_required"})
		return
	}

	// Verify user has access to the requested tenant
	if !verifyTenantAccess(c, tenantID) {
		return
	}

	switch agentName {
	case "cqa.sync":
		handleSyncQuery(c, tenantID, resource)
	case "cqa.qc":
		handleQCQuery(c, tenantID, resource)
	case "cqa.classify":
		handleClassifyQuery(c, tenantID, resource)
	default:
		c.JSON(http.StatusNotFound, gin.H{"error": "agent_not_found"})
	}
}

func AgentHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy", "timestamp": time.Now()})
}

func handleSyncAgent(ctx context.Context, cfg *config.Config, req AgentRunRequest) AgentRunResponse {
	syncEngine := engine.NewSyncEngine(cfg)

	switch req.Action {
	case "sync_all":
		err := syncEngine.SyncAllChannels(ctx, req.TenantID)
		if err != nil {
			return AgentRunResponse{Status: "error", Errors: []string{err.Error()}}
		}
		return AgentRunResponse{Status: "success"}
	case "sync_channel":
		channelID, _ := req.Params["channel_id"].(string)
		var channel models.Channel
		if err := db.DB.Where("id = ? AND tenant_id = ?", channelID, req.TenantID).First(&channel).Error; err != nil {
			return AgentRunResponse{Status: "error", Errors: []string{"channel not found"}}
		}
		err := syncEngine.SyncChannel(ctx, channel)
		if err != nil {
			return AgentRunResponse{Status: "error", Errors: []string{err.Error()}}
		}
		return AgentRunResponse{Status: "success"}
	default:
		return AgentRunResponse{Status: "error", Errors: []string{"unknown action: " + req.Action}}
	}
}

func handleAnalysisAgent(ctx context.Context, cfg *config.Config, req AgentRunRequest, agentName string) AgentRunResponse {
	analyzer := engine.NewAnalyzer(cfg)

	jobType := "qc_analysis"
	if agentName == "cqa.classify" {
		jobType = "classification"
	}

	// Find matching active jobs
	var jobs []models.Job
	db.DB.Where("tenant_id = ? AND job_type = ? AND is_active = true", req.TenantID, jobType).Find(&jobs)

	if len(jobs) == 0 {
		return AgentRunResponse{Status: "error", Errors: []string{"no active jobs found for type: " + jobType}}
	}

	var errs []string
	for _, job := range jobs {
		if _, err := analyzer.RunJob(ctx, job); err != nil {
			errs = append(errs, err.Error())
		}
	}

	status := "success"
	if len(errs) > 0 {
		status = "partial"
	}
	return AgentRunResponse{Status: status, Errors: errs}
}

func handleSyncQuery(c *gin.Context, tenantID, resource string) {
	switch resource {
	case "conversations":
		var convs []models.Conversation
		db.DB.Where("tenant_id = ?", tenantID).Order("last_message_at DESC").Limit(50).Find(&convs)
		c.JSON(http.StatusOK, convs)
	case "messages":
		convID := c.Query("conversation_id")
		var msgs []models.Message
		db.DB.Where("tenant_id = ? AND conversation_id = ?", tenantID, convID).Order("sent_at ASC").Limit(100).Find(&msgs)
		c.JSON(http.StatusOK, msgs)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown resource: " + resource})
	}
}

func handleQCQuery(c *gin.Context, tenantID, resource string) {
	switch resource {
	case "violations":
		var results []models.JobResult
		db.DB.Where("tenant_id = ? AND result_type = 'qc_violation'", tenantID).
			Order("created_at DESC").Limit(50).Find(&results)
		c.JSON(http.StatusOK, results)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown resource: " + resource})
	}
}

func handleClassifyQuery(c *gin.Context, tenantID, resource string) {
	switch resource {
	case "tags":
		var results []models.JobResult
		db.DB.Where("tenant_id = ? AND result_type = 'classification_tag'", tenantID).
			Order("created_at DESC").Limit(50).Find(&results)
		c.JSON(http.StatusOK, results)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown resource: " + resource})
	}
}
