package db

import (
	"time"

	"github.com/google/uuid"
	"github.com/nmtan2001/chat-quality-agent/db/models"
)

// LogActivity records a system activity for audit trail.
func LogActivity(tenantID, userID, userEmail, action, resourceType, resourceID, detail, errMsg, ip string) {
	log := models.ActivityLog{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		UserID:       userID,
		UserEmail:    userEmail,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Detail:       detail,
		ErrorMessage: errMsg,
		IPAddress:    ip,
		CreatedAt:    time.Now(),
	}
	DB.Create(&log)
}
