package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"golang.org/x/crypto/bcrypt"
)

type TenantUserResponse struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	Permissions string `json:"permissions"`
}

func ListTenantUsers(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var userTenants []models.UserTenant
	db.DB.Where("tenant_id = ?", tenantID).Find(&userTenants)

	var results []TenantUserResponse
	for _, ut := range userTenants {
		var user models.User
		if err := db.DB.Where("id = ?", ut.UserID).First(&user).Error; err == nil {
			results = append(results, TenantUserResponse{
				UserID:      user.ID,
				Email:       user.Email,
				Name:        user.Name,
				Role:        ut.Role,
				Permissions: ut.Permissions,
			})
		}
	}

	c.JSON(http.StatusOK, results)
}

func InviteUser(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		Name        string `json:"name" binding:"required"`
		Email       string `json:"email" binding:"required,email"`
		Password    string `json:"password" binding:"required"`
		Role        string `json:"role" binding:"required,oneof=admin member"`
		Permissions string `json:"permissions"` // JSON permissions for member role
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}
	if err := validatePasswordComplexity(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "weak_password", "message": err.Error()})
		return
	}

	// Check if user already exists
	var user models.User
	err := db.DB.Where("email = ?", req.Email).First(&user).Error
	if err != nil {
		// User doesn't exist — create new user
		hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		user = models.User{
			ID:           uuid.New().String(),
			Email:        req.Email,
			Name:         req.Name,
			PasswordHash: string(hash),
			Language:     "vi",
		}
		if err := db.DB.Create(&user).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed_to_create_user"})
			return
		}
	}

	// Check if already in tenant
	var existing models.UserTenant
	if db.DB.Where("user_id = ? AND tenant_id = ?", user.ID, tenantID).First(&existing).Error == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "user_already_in_tenant"})
		return
	}

	// Add to tenant
	ut := models.UserTenant{
		UserID:      user.ID,
		TenantID:    tenantID,
		Role:        req.Role,
		Permissions: req.Permissions,
	}
	db.DB.Create(&ut)

	c.JSON(http.StatusOK, TenantUserResponse{
		UserID: user.ID,
		Email:  user.Email,
		Name:   user.Name,
		Role:   req.Role,
	})
}

func UpdateUserRole(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	userID := c.Param("userId")
	currentUserID := middleware.GetUserID(c)
	currentRole := middleware.GetTenantRole(c)

	// Can't change own role
	if userID == currentUserID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot_change_own_role"})
		return
	}

	var req struct {
		Role        string `json:"role" binding:"required,oneof=owner admin member"`
		Permissions string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	// Role hierarchy: owner can manage all, admin can only manage members
	var targetUT models.UserTenant
	if err := db.DB.Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&targetUT).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user_not_in_tenant"})
		return
	}
	if currentRole == "admin" && (targetUT.Role == "owner" || targetUT.Role == "admin") {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin_cannot_manage_owner_or_admin"})
		return
	}

	updates := map[string]interface{}{"role": req.Role}
	if req.Permissions != "" {
		updates["permissions"] = req.Permissions
	}
	db.DB.Model(&models.UserTenant{}).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Updates(updates)

	c.JSON(http.StatusOK, gin.H{"message": "role_updated"})
}

func ResetUserPassword(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	userID := c.Param("userId")

	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	// Verify user belongs to this tenant
	var ut models.UserTenant
	if err := db.DB.Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&ut).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user_not_in_tenant"})
		return
	}

	// Validate password complexity (same rules as registration)
	if err := validatePasswordComplexity(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "weak_password", "message": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password_hash_failed"})
		return
	}

	result := db.DB.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"password_hash": string(hash),
		"token_version": db.DB.Raw("token_version + 1"), // invalidate existing sessions
	})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password_reset_failed"})
		return
	}

	log.Printf("[security] password reset: user=%s reset_by=%s tenant=%s", userID, middleware.GetUserID(c), tenantID)
	c.JSON(http.StatusOK, gin.H{"message": "password_reset"})
}

func RemoveUserFromTenant(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	userID := c.Param("userId")
	currentUserID := middleware.GetUserID(c)

	// Can't remove self
	if userID == currentUserID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot_remove_self"})
		return
	}

	result := db.DB.Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Delete(&models.UserTenant{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "user_not_in_tenant"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user_removed"})
}
