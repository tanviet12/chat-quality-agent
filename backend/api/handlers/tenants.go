package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/pkg"
)

type CreateTenantRequest struct {
	Name string `json:"name" binding:"required,min=2,max=255"`
	Slug string `json:"slug" binding:"required,min=2,max=100"`
}

type TenantResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	ChannelsCount int64  `json:"channels_count"`
	JobsCount     int64  `json:"jobs_count"`
}

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

func ListTenants(c *gin.Context) {
	userID := middleware.GetUserID(c)

	var userTenants []models.UserTenant
	if err := db.DB.Where("user_id = ?", userID).Find(&userTenants).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fetch_tenants_failed"})
		return
	}

	tenantIDs := make([]string, len(userTenants))
	for i, ut := range userTenants {
		tenantIDs[i] = ut.TenantID
	}

	if len(tenantIDs) == 0 {
		c.JSON(http.StatusOK, []TenantResponse{})
		return
	}

	var tenants []models.Tenant
	db.DB.Where("id IN ?", tenantIDs).Find(&tenants)

	results := make([]TenantResponse, len(tenants))
	for i, t := range tenants {
		var channelsCount, jobsCount int64
		db.DB.Model(&models.Channel{}).Where("tenant_id = ?", t.ID).Count(&channelsCount)
		db.DB.Model(&models.Job{}).Where("tenant_id = ?", t.ID).Count(&jobsCount)
		results[i] = TenantResponse{
			ID:            t.ID,
			Name:          t.Name,
			Slug:          t.Slug,
			ChannelsCount: channelsCount,
			JobsCount:     jobsCount,
		}
	}

	c.JSON(http.StatusOK, results)
}

func CreateTenant(c *gin.Context) {
	var req CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}

	if !slugRegex.MatchString(req.Slug) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_slug", "details": "slug must be lowercase alphanumeric with hyphens"})
		return
	}

	// Check slug uniqueness
	var count int64
	db.DB.Model(&models.Tenant{}).Where("slug = ?", req.Slug).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "slug_already_exists"})
		return
	}

	now := time.Now()
	tenant := models.Tenant{
		ID:        pkg.NewUUID(),
		Name:      req.Name,
		Slug:      req.Slug,
		Settings:  "{}",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := db.DB.Create(&tenant).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_tenant_failed"})
		return
	}

	// Add creator as owner
	userID := middleware.GetUserID(c)
	ut := models.UserTenant{
		UserID:   userID,
		TenantID: tenant.ID,
		Role:     "owner",
	}
	db.DB.Create(&ut)

	c.JSON(http.StatusCreated, TenantResponse{
		ID:   tenant.ID,
		Name: tenant.Name,
		Slug: tenant.Slug,
	})
}

func GetTenant(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var tenant models.Tenant
	if err := db.DB.First(&tenant, "id = ?", tenantID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant_not_found"})
		return
	}

	var channelsCount, jobsCount int64
	db.DB.Model(&models.Channel{}).Where("tenant_id = ?", tenantID).Count(&channelsCount)
	db.DB.Model(&models.Job{}).Where("tenant_id = ?", tenantID).Count(&jobsCount)

	c.JSON(http.StatusOK, TenantResponse{
		ID:            tenant.ID,
		Name:          tenant.Name,
		Slug:          tenant.Slug,
		ChannelsCount: channelsCount,
		JobsCount:     jobsCount,
	})
}

func UpdateTenant(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		Name string `json:"name" binding:"required,min=2,max=255"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}

	result := db.DB.Model(&models.Tenant{}).Where("id = ?", tenantID).Updates(map[string]interface{}{
		"name":       req.Name,
		"updated_at": time.Now(),
	})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update_tenant_failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func DeleteTenant(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	// Full cascade delete: child → parent order

	// 0. Delete all local attachment files for this tenant
	os.RemoveAll(filepath.Join("/var/lib/cqa/files", tenantID))

	// 1. Messages (via conversations)
	var convIDs []string
	db.DB.Model(&models.Conversation{}).Where("tenant_id = ?", tenantID).Pluck("id", &convIDs)
	if len(convIDs) > 0 {
		db.DB.Where("conversation_id IN ?", convIDs).Delete(&models.Message{})
	}
	// 2. Conversations
	db.DB.Where("tenant_id = ?", tenantID).Delete(&models.Conversation{})

	// 3. JobResults (via job_runs)
	var runIDs []string
	db.DB.Model(&models.JobRun{}).Where("tenant_id = ?", tenantID).Pluck("id", &runIDs)
	if len(runIDs) > 0 {
		db.DB.Where("job_run_id IN ?", runIDs).Delete(&models.JobResult{})
	}
	// 4. JobRuns
	db.DB.Where("tenant_id = ?", tenantID).Delete(&models.JobRun{})
	// 5. AIUsageLogs
	db.DB.Where("tenant_id = ?", tenantID).Delete(&models.AIUsageLog{})
	// 6. NotificationLogs
	db.DB.Where("tenant_id = ?", tenantID).Delete(&models.NotificationLog{})
	// 7. ActivityLogs
	db.DB.Where("tenant_id = ?", tenantID).Delete(&models.ActivityLog{})
	// 8. Jobs
	db.DB.Where("tenant_id = ?", tenantID).Delete(&models.Job{})
	// 9. AppSettings
	db.DB.Where("tenant_id = ?", tenantID).Delete(&models.AppSetting{})
	// 10. Channels
	db.DB.Where("tenant_id = ?", tenantID).Delete(&models.Channel{})
	// 11. UserTenants
	db.DB.Where("tenant_id = ?", tenantID).Delete(&models.UserTenant{})
	// 12. Tenant
	db.DB.Where("id = ?", tenantID).Delete(&models.Tenant{})

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
