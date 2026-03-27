package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
)

func ListActivityLogs(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	action := c.Query("action")

	if page < 1 {
		page = 1
	}
	if perPage > 100 {
		perPage = 100
	}

	query := db.DB.Where("tenant_id = ?", tenantID)
	if action != "" {
		query = query.Where("action LIKE ?", action+"%")
	}

	var total int64
	query.Model(&models.ActivityLog{}).Count(&total)

	var logs []models.ActivityLog
	query.Order("created_at DESC").
		Offset((page - 1) * perPage).
		Limit(perPage).
		Find(&logs)

	c.JSON(http.StatusOK, gin.H{
		"data":     logs,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}
