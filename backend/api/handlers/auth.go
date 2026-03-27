package handlers

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/pkg"
	"golang.org/x/crypto/bcrypt"
)

// Account lockout: 5 failed attempts → 15 min lockout
var (
	failedAttempts   = make(map[string]*loginAttempt)
	failedAttemptsMu sync.Mutex
)

type loginAttempt struct {
	count    int
	lockedAt time.Time
}

const maxFailedAttempts = 5
const lockoutDuration = 15 * time.Minute

func checkLockout(key string) bool {
	failedAttemptsMu.Lock()
	defer failedAttemptsMu.Unlock()
	a, ok := failedAttempts[key]
	if !ok {
		return false
	}
	if !a.lockedAt.IsZero() && time.Since(a.lockedAt) < lockoutDuration {
		return true
	}
	if !a.lockedAt.IsZero() && time.Since(a.lockedAt) >= lockoutDuration {
		delete(failedAttempts, key)
	}
	return false
}

func recordFailedLogin(key string) {
	failedAttemptsMu.Lock()
	defer failedAttemptsMu.Unlock()
	a, ok := failedAttempts[key]
	if !ok {
		a = &loginAttempt{}
		failedAttempts[key] = a
	}
	a.count++
	if a.count >= maxFailedAttempts {
		a.lockedAt = time.Now()
	}
}

func clearFailedLogin(key string) {
	failedAttemptsMu.Lock()
	defer failedAttemptsMu.Unlock()
	delete(failedAttempts, key)
}

// Password complexity: min 8 chars, at least 1 uppercase, 1 digit
var passwordRegex = regexp.MustCompile(`^.{8,}$`)
var hasUpper = regexp.MustCompile(`[A-Z]`)
var hasDigit = regexp.MustCompile(`[0-9]`)

func validatePasswordComplexity(pw string) error {
	if !passwordRegex.MatchString(pw) {
		return fmt.Errorf("Mật khẩu phải có ít nhất 8 ký tự")
	}
	if !hasUpper.MatchString(pw) {
		return fmt.Errorf("Mật khẩu phải có ít nhất 1 chữ hoa")
	}
	if !hasDigit.MatchString(pw) {
		return fmt.Errorf("Mật khẩu phải có ít nhất 1 chữ số")
	}
	return nil
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	Name     string `json:"name" binding:"required"`
}

type SetupRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	Name     string `json:"name"`
}

// SetupStatus returns whether initial setup is needed (no users exist yet)
func SetupStatus(c *gin.Context) {
	var count int64
	if err := db.DB.Model(&models.User{}).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"needs_setup": count == 0})
}

// Setup creates the first admin account. Only works when no users exist.
func Setup(c *gin.Context) {
	var count int64
	if err := db.DB.Model(&models.User{}).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database_error"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "setup_already_completed"})
		return
	}

	var req SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}

	if err := validatePasswordComplexity(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "weak_password", "message": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password_hash_failed"})
		return
	}

	name := req.Name
	if name == "" {
		name = "Admin"
	}

	user := models.User{
		ID:           pkg.NewUUID(),
		Email:        req.Email,
		PasswordHash: string(hash),
		Name:         name,
		IsAdmin:      true,
		Language:     "vi",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := db.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_user_failed"})
		return
	}

	log.Printf("[setup] Admin account created: %s", user.Email)

	accessToken, err := middleware.GenerateAccessToken(user.ID, user.Email, user.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token_generation_failed"})
		return
	}
	refreshToken, err := middleware.GenerateRefreshToken(user.ID, user.TokenVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token_generation_failed"})
		return
	}

	setRefreshCookie(c, refreshToken)

	c.JSON(http.StatusCreated, TokenResponse{
		AccessToken: accessToken,
		ExpiresIn:   900,
	})
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // seconds
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}

	lockoutKey := req.Email + ":" + c.ClientIP()
	if checkLockout(lockoutKey) {
		log.Printf("[security] brute force lockout: email=%s ip=%s", req.Email, c.ClientIP())
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "account_locked", "message": "Tài khoản bị khóa tạm thời do đăng nhập sai nhiều lần. Vui lòng thử lại sau 15 phút."})
		return
	}

	var user models.User
	if err := db.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		recordFailedLogin(lockoutKey)
		log.Printf("[security] failed login: email=%s ip=%s reason=user_not_found", req.Email, c.ClientIP())
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		recordFailedLogin(lockoutKey)
		log.Printf("[security] failed login: email=%s ip=%s reason=wrong_password", req.Email, c.ClientIP())
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
		return
	}

	clearFailedLogin(lockoutKey)

	accessToken, err := middleware.GenerateAccessToken(user.ID, user.Email, user.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token_generation_failed"})
		return
	}

	refreshToken, err := middleware.GenerateRefreshToken(user.ID, user.TokenVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token_generation_failed"})
		return
	}

	// Log login activity
	var firstTenant models.UserTenant
	tenantID := ""
	if db.DB.Where("user_id = ?", user.ID).First(&firstTenant).Error == nil {
		tenantID = firstTenant.TenantID
	}
	ua := c.GetHeader("User-Agent")
	detail := "Login from " + c.ClientIP()
	if ua != "" {
		detail += " | " + ua
	}
	db.LogActivity(tenantID, user.ID, user.Email, "user.login", "user", user.ID, detail, "", c.ClientIP())

	// Set refresh token as HttpOnly cookie (not exposed to JavaScript)
	setRefreshCookie(c, refreshToken)

	c.JSON(http.StatusOK, TokenResponse{
		AccessToken: accessToken,
		ExpiresIn:   900, // 15 minutes
	})
}

func setRefreshCookie(c *gin.Context, token string) {
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "cqa_refresh_token",
		Value:    token,
		MaxAge:   7 * 24 * 3600,
		Path:     "/api/v1/auth",
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearRefreshCookie(c *gin.Context) {
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "cqa_refresh_token",
		Value:    "",
		MaxAge:   -1,
		Path:     "/api/v1/auth",
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func Logout(c *gin.Context) {
	clearRefreshCookie(c)
	c.JSON(http.StatusOK, gin.H{"message": "logged_out"})
}

func Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}

	if err := validatePasswordComplexity(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "weak_password", "message": err.Error()})
		return
	}

	// Check email doesn't already exist
	var count int64
	db.DB.Model(&models.User{}).Where("email = ?", req.Email).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "email_already_exists"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password_hash_failed"})
		return
	}

	// First user becomes admin
	var totalUsers int64
	db.DB.Model(&models.User{}).Count(&totalUsers)

	user := models.User{
		ID:           pkg.NewUUID(),
		Email:        req.Email,
		PasswordHash: string(hash),
		Name:         req.Name,
		IsAdmin:      totalUsers == 0,
		Language:     "vi",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := db.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_user_failed"})
		return
	}

	accessToken, err := middleware.GenerateAccessToken(user.ID, user.Email, user.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token_generation_failed"})
		return
	}
	refreshToken, err := middleware.GenerateRefreshToken(user.ID, user.TokenVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token_generation_failed"})
		return
	}

	setRefreshCookie(c, refreshToken)

	c.JSON(http.StatusCreated, TokenResponse{
		AccessToken: accessToken,
		ExpiresIn:   900,
	})
}

func RefreshTokenHandler(c *gin.Context) {
	// Read refresh token from HttpOnly cookie only — never accept from JSON body
	refreshTokenStr, err := c.Cookie("cqa_refresh_token")
	if err != nil || refreshTokenStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_refresh_token"})
		return
	}

	userID, tokenVersion, err := middleware.ParseRefreshToken(refreshTokenStr)
	if err != nil {
		log.Printf("[security] refresh token invalid: ip=%s error=%v", c.ClientIP(), err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_refresh_token"})
		return
	}

	var user models.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user_not_found"})
		return
	}

	// Validate token version — rejects old tokens after rotation
	if tokenVersion != user.TokenVersion {
		log.Printf("[security] refresh token revoked: ip=%s user=%s token_ver=%d current_ver=%d", c.ClientIP(), userID, tokenVersion, user.TokenVersion)
		clearRefreshCookie(c)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_refresh_token"})
		return
	}

	// Increment token version to revoke the current refresh token
	newVersion := user.TokenVersion + 1
	db.DB.Model(&user).Update("token_version", newVersion)

	accessToken, err := middleware.GenerateAccessToken(user.ID, user.Email, user.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token_generation_failed"})
		return
	}
	refreshToken, err := middleware.GenerateRefreshToken(user.ID, newVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token_generation_failed"})
		return
	}

	// Rotate refresh token cookie — old token is now invalid (version mismatch)
	setRefreshCookie(c, refreshToken)

	c.JSON(http.StatusOK, TokenResponse{
		AccessToken: accessToken,
		ExpiresIn:   900,
	})
}

func GetProfile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var user models.User
	if err := db.DB.Preload("Tenants").First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

type UpdateProfileRequest struct {
	Name string `json:"name" binding:"required"`
}

func UpdateProfile(c *gin.Context) {
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}

	userID := middleware.GetUserID(c)
	var user models.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
		return
	}

	user.Name = req.Name
	user.UpdatedAt = time.Now()
	if err := db.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update_failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"name": user.Name, "email": user.Email})
}

type ChangeProfilePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

func ChangeProfilePassword(c *gin.Context) {
	var req ChangeProfilePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}

	userID := middleware.GetUserID(c)
	var user models.User
	if err := db.DB.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wrong_current_password"})
		return
	}

	if err := validatePasswordComplexity(req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "weak_password", "message": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password_hash_failed"})
		return
	}

	user.PasswordHash = string(hash)
	user.UpdatedAt = time.Now()
	if err := db.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update_failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password_changed"})
}
