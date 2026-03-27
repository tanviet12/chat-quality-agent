package middleware

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
)

// TenantContext extracts tenant_id from URL param and verifies user has access.
func TenantContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("tenantId")
		if tenantID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "tenant_id_required"})
			return
		}

		userID := GetUserID(c)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization_required"})
			return
		}

		// Check user has access to this tenant
		var ut models.UserTenant
		result := db.DB.Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&ut)
		if result.Error != nil {
			log.Printf("[security] tenant access denied: user=%s tenant=%s ip=%s", userID, tenantID, c.ClientIP())
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "tenant_access_denied"})
			return
		}

		c.Set("tenant_id", tenantID)
		c.Set("tenant_role", ut.Role)
		c.Set("tenant_permissions", ut.Permissions)
		c.Next()
	}
}

// GetTenantID extracts tenant ID from gin context.
func GetTenantID(c *gin.Context) string {
	if v, exists := c.Get("tenant_id"); exists {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetTenantRole extracts tenant role from gin context.
func GetTenantRole(c *gin.Context) string {
	if v, exists := c.Get("tenant_role"); exists {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// RequireRole checks that the user has at least the specified role.
func RequireRole(roles ...string) gin.HandlerFunc {
	roleMap := make(map[string]bool)
	for _, r := range roles {
		roleMap[r] = true
	}
	return func(c *gin.Context) {
		role := GetTenantRole(c)
		if !roleMap[role] {
			log.Printf("[security] RBAC denied: user=%s role=%s required=%v path=%s", GetUserID(c), role, roles, c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient_role"})
			return
		}
		c.Next()
	}
}

// RequirePermission checks role (owner/admin always pass) or member permission for a resource+action.
// resource: "channels", "jobs", "messages", "settings"
// action: "r" (read), "w" (write), "d" (delete)
func RequirePermission(resource, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := GetTenantRole(c)
		// Owner and admin always have full access
		if role == "owner" || role == "admin" {
			c.Next()
			return
		}
		// Member: check permissions JSON
		perms := c.GetString("tenant_permissions")
		if perms == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "no_permissions"})
			return
		}
		var permMap map[string]string
		if err := json.Unmarshal([]byte(perms), &permMap); err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid_permissions"})
			return
		}
		resourcePerms, ok := permMap[resource]
		if !ok || !containsChar(resourcePerms, action) {
			log.Printf("[security] permission denied: user=%s resource=%s action=%s path=%s", GetUserID(c), resource, action, c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "permission_denied"})
			return
		}
		c.Next()
	}
}

func containsChar(s, char string) bool {
	for _, c := range s {
		if string(c) == char {
			return true
		}
	}
	return false
}
