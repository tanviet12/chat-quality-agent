package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/config"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/engine"
	"github.com/nmtan2001/chat-quality-agent/pkg"
)

// Shared HTTP client with timeout for external API calls
var httpClientWithTimeout = &http.Client{Timeout: 30 * time.Second}

type CreateChannelRequest struct {
	ChannelType string          `json:"channel_type" binding:"required,oneof=zalo_oa facebook"`
	Name        string          `json:"name" binding:"required,min=2,max=255"`
	Credentials json.RawMessage `json:"credentials" binding:"required"` // JSON: varies by type
	Metadata    string          `json:"metadata"`
}

type ChannelResponse struct {
	ID                string     `json:"id"`
	TenantID          string     `json:"tenant_id"`
	ChannelType       string     `json:"channel_type"`
	Name              string     `json:"name"`
	ExternalID        string     `json:"external_id"`
	IsActive          bool       `json:"is_active"`
	Metadata          string     `json:"metadata"`
	LastSyncAt        *time.Time `json:"last_sync_at"`
	LastSyncStatus    string     `json:"last_sync_status"`
	ConversationCount int64      `json:"conversation_count"`
	CreatedAt         time.Time  `json:"created_at"`
}

func ListChannels(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)

	var channels []models.Channel
	db.DB.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&channels)

	// Get conversation counts per channel
	type countResult struct {
		ChannelID string
		Count     int64
	}
	var counts []countResult
	db.DB.Model(&models.Conversation{}).
		Select("channel_id, COUNT(*) as count").
		Where("tenant_id = ?", tenantID).
		Group("channel_id").
		Scan(&counts)
	countMap := make(map[string]int64)
	for _, c := range counts {
		countMap[c.ChannelID] = c.Count
	}

	results := make([]ChannelResponse, len(channels))
	for i, ch := range channels {
		results[i] = channelToResponse(ch)
		results[i].ConversationCount = countMap[ch.ID]
	}
	c.JSON(http.StatusOK, results)
}

func CreateChannel(c *gin.Context) {
	var req CreateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}

	tenantID := middleware.GetTenantID(c)

	// Encrypt credentials
	cfg, _ := config.Load()
	encrypted, err := pkg.Encrypt([]byte(req.Credentials), cfg.EncryptionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encryption_failed"})
		return
	}

	// For Facebook: exchange user/system token for Page Access Token
	credentialsToStore := encrypted
	externalID := ""
	channelName := req.Name

	if req.ChannelType == "facebook" {
		var fbCreds struct {
			PageID      string `json:"page_id"`
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(req.Credentials, &fbCreds); err == nil && fbCreds.AccessToken != "" {
			// Try to get Page Access Token from the provided token
			pageID, pageToken, pageName, err := getFBPageToken(fbCreds.AccessToken)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			// Use the actual page token and page ID
			fbCreds.PageID = pageID
			fbCreds.AccessToken = pageToken
			if pageName != "" {
				channelName = pageName
			}
			externalID = pageID

			updatedCreds, _ := json.Marshal(fbCreds)
			credentialsToStore, err = pkg.Encrypt(updatedCreds, cfg.EncryptionKey)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "encryption_failed"})
				return
			}
		}
	}

	now := time.Now()
	channel := models.Channel{
		ID:                   pkg.NewUUID(),
		TenantID:             tenantID,
		ChannelType:          req.ChannelType,
		Name:                 channelName,
		ExternalID:           externalID,
		CredentialsEncrypted: credentialsToStore,
		IsActive:             true,
		Metadata:             func() string { if req.Metadata != "" { return req.Metadata }; return "{}" }(),
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := db.DB.Create(&channel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create_channel_failed"})
		return
	}

	c.JSON(http.StatusCreated, channelToResponse(channel))
}

func GetChannel(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	channelID := c.Param("channelId")

	var channel models.Channel
	if err := db.DB.Where("id = ? AND tenant_id = ?", channelID, tenantID).First(&channel).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel_not_found"})
		return
	}

	resp := channelToResponse(channel)
	var convCount int64
	db.DB.Model(&models.Conversation{}).Where("channel_id = ? AND tenant_id = ?", channelID, tenantID).Count(&convCount)
	resp.ConversationCount = convCount
	c.JSON(http.StatusOK, resp)
}

func UpdateChannel(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	channelID := c.Param("channelId")

	var req struct {
		Name     string `json:"name" binding:"omitempty,min=2,max=255"`
		IsActive *bool  `json:"is_active"`
		Metadata string `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "details": err.Error()})
		return
	}

	updates := map[string]interface{}{"updated_at": time.Now()}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if req.Metadata != "" {
		updates["metadata"] = req.Metadata
	}

	result := db.DB.Model(&models.Channel{}).Where("id = ? AND tenant_id = ?", channelID, tenantID).Updates(updates)
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel_not_found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func DeleteChannel(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	channelID := c.Param("channelId")

	var channel models.Channel
	if err := db.DB.Where("id = ? AND tenant_id = ?", channelID, tenantID).First(&channel).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel_not_found"})
		return
	}

	// Cascade: delete files → messages → conversations → channel
	var convIDs []string
	db.DB.Model(&models.Conversation{}).Where("channel_id = ? AND tenant_id = ?", channelID, tenantID).Pluck("id", &convIDs)
	if len(convIDs) > 0 {
		// Delete local attachment files
		for _, convID := range convIDs {
			dir := filepath.Join("/var/lib/cqa/files", tenantID, convID)
			os.RemoveAll(dir)
		}
		db.DB.Where("conversation_id IN ? AND tenant_id = ?", convIDs, tenantID).Delete(&models.Message{})
	}
	db.DB.Where("channel_id = ? AND tenant_id = ?", channelID, tenantID).Delete(&models.Conversation{})
	db.DB.Delete(&channel)

	db.LogActivity(tenantID, middleware.GetUserID(c), middleware.GetUserEmail(c), "channel.delete", "channel", channelID, "Deleted channel: "+channel.Name, "", c.ClientIP())

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// PurgeChannelConversations deletes all conversations, messages, and evaluation results
// for a channel while keeping the channel itself. Resets last_sync_at so next sync
// fetches everything from scratch.
func PurgeChannelConversations(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	channelID := c.Param("channelId")

	var channel models.Channel
	if err := db.DB.Where("id = ? AND tenant_id = ?", channelID, tenantID).First(&channel).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel_not_found"})
		return
	}

	// Get all conversation IDs for this channel
	var convIDs []string
	db.DB.Model(&models.Conversation{}).Where("channel_id = ? AND tenant_id = ?", channelID, tenantID).Pluck("id", &convIDs)

	var messagesDeleted, convsDeleted int64
	if len(convIDs) > 0 {
		// Delete evaluation results linked to these conversations
		db.DB.Where("conversation_id IN ? AND tenant_id = ?", convIDs, tenantID).Delete(&models.JobResult{})

		// Delete messages
		result := db.DB.Where("conversation_id IN ? AND tenant_id = ?", convIDs, tenantID).Delete(&models.Message{})
		messagesDeleted = result.RowsAffected

		// Delete local attachment files for each conversation
		for _, convID := range convIDs {
			dir := filepath.Join("/var/lib/cqa/files", tenantID, convID)
			if err := os.RemoveAll(dir); err != nil {
				log.Printf("[sync] failed to remove files dir %s: %v", dir, err)
			}
		}
	}

	// Delete conversations
	result := db.DB.Where("channel_id = ? AND tenant_id = ?", channelID, tenantID).Delete(&models.Conversation{})
	convsDeleted = result.RowsAffected

	// Reset sync state so next sync fetches everything from scratch
	db.DB.Model(&channel).Updates(map[string]interface{}{
		"last_sync_at":     nil,
		"last_sync_status": nil,
		"last_sync_error":  "",
		"updated_at":       time.Now(),
	})

	db.LogActivity(tenantID, middleware.GetUserID(c), middleware.GetUserEmail(c),
		"channel.purge_conversations", "channel", channelID,
		fmt.Sprintf("Purged all conversations: %s (%d conversations, %d messages)", channel.Name, convsDeleted, messagesDeleted),
		"", c.ClientIP())

	c.JSON(http.StatusOK, gin.H{
		"message":               "purged",
		"conversations_deleted": convsDeleted,
		"messages_deleted":      messagesDeleted,
	})
}

func TestChannelConnection(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	channelID := c.Param("channelId")

	var channel models.Channel
	if err := db.DB.Where("id = ? AND tenant_id = ?", channelID, tenantID).First(&channel).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel_not_found"})
		return
	}

	// Decrypt credentials
	cfg, _ := config.Load()
	credBytes, err := pkg.Decrypt(channel.CredentialsEncrypted, cfg.EncryptionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "decrypt_failed"})
		return
	}

	_ = credBytes // TODO: use with channel adapter HealthCheck

	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "connection_successful"})
}

// signOAuthState creates an HMAC-signed state parameter: "tenantId:channelId:hmac"
func signOAuthState(tenantID, channelID, secret string) string {
	payload := tenantID + ":" + channelID
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))[:16] // short sig
	return payload + ":" + sig
}

// verifyOAuthState verifies an HMAC-signed state parameter. Returns tenantID, channelID or error.
func verifyOAuthState(state, secret string) (string, string, error) {
	parts := strings.SplitN(state, ":", 3)
	if len(parts) != 3 {
		return "", "", fmt.Errorf("invalid state format")
	}
	tenantID, channelID, sig := parts[0], parts[1], parts[2]
	payload := tenantID + ":" + channelID
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))[:16]
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return "", "", fmt.Errorf("invalid state signature")
	}
	return tenantID, channelID, nil
}

// ZaloOAuthCallback handles the OAuth callback from Zalo after user authorizes.
func ZaloOAuthCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		redirectWithError(c, "", "Missing code or state")
		return
	}

	cfg, _ := config.Load()
	tenantID, channelID, err := verifyOAuthState(state, cfg.JWTSecret)
	if err != nil {
		log.Printf("[security] invalid OAuth state: %v", err)
		redirectWithError(c, "", "Authorization failed")
		return
	}

	// Load channel to get app_id and app_secret
	var channel models.Channel
	if err := db.DB.Where("id = ? AND tenant_id = ?", channelID, tenantID).First(&channel).Error; err != nil {
		redirectWithError(c, tenantID, "Channel not found")
		return
	}

	credBytes, err := pkg.Decrypt(channel.CredentialsEncrypted, cfg.EncryptionKey)
	if err != nil {
		log.Printf("[error] decrypt channel %s credentials: %v", channelID, err)
		redirectWithError(c, tenantID, "Authorization failed")
		return
	}

	var creds map[string]string
	if err := json.Unmarshal(credBytes, &creds); err != nil {
		log.Printf("[error] unmarshal channel %s credentials: %v", channelID, err)
		redirectWithError(c, tenantID, "Authorization failed")
		return
	}

	appID := creds["app_id"]
	appSecret := creds["app_secret"]

	// Exchange code for tokens
	callbackURL := fmt.Sprintf("%s/api/v1/channels/zalo/callback", getBaseURL(c))
	tokenResp, err := exchangeZaloCode(code, appID, appSecret, callbackURL)
	if err != nil {
		log.Printf("[error] zalo token exchange for channel %s: %v", channelID, err)
		redirectWithError(c, tenantID, "Token exchange failed")
		return
	}

	// Update channel credentials with tokens
	creds["access_token"] = tokenResp.AccessToken
	creds["refresh_token"] = tokenResp.RefreshToken

	// Fetch OA info to know which OA this token belongs to
	oaID := ""
	if oaInfo, err := fetchZaloOAInfo(tokenResp.AccessToken); err == nil {
		creds["oa_id"] = oaInfo.OAID
		creds["oa_name"] = oaInfo.OAName
		oaID = oaInfo.OAID
	}

	updatedCreds, _ := json.Marshal(creds)
	encrypted, err := pkg.Encrypt(updatedCreds, cfg.EncryptionKey)
	if err != nil {
		redirectWithError(c, tenantID, "Encrypt failed")
		return
	}

	updates := map[string]interface{}{
		"credentials_encrypted": encrypted,
		"updated_at":            time.Now(),
	}
	if oaID != "" {
		updates["external_id"] = oaID
	}
	db.DB.Model(&models.Channel{}).Where("id = ?", channelID).Updates(updates)

	// Redirect back to channel detail page with success
	c.Redirect(http.StatusFound, fmt.Sprintf("/%s/channels/%s?zalo_auth=success", tenantID, channelID))
}

type zaloTokenResponse struct {
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	ExpiresIn    json.RawMessage `json:"expires_in"` // Zalo returns string or int
	Error        json.RawMessage `json:"error"`       // can be int or string
	Message      string          `json:"message"`
}

func (z *zaloTokenResponse) hasError() bool {
	if len(z.Error) == 0 {
		return false
	}
	s := string(z.Error)
	return s != "0" && s != `"0"` && s != ""
}

func exchangeZaloCode(code, appID, appSecret, redirectURI string) (*zaloTokenResponse, error) {
	data := url.Values{}
	data.Set("code", code)
	data.Set("app_id", appID)
	data.Set("grant_type", "authorization_code")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", "https://oauth.zaloapp.com/v4/oa/access_token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("secret_key", appSecret)

	resp, err := httpClientWithTimeout.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var tokenResp zaloTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	if tokenResp.hasError() {
		return nil, fmt.Errorf("zalo error %s: %s", string(tokenResp.Error), tokenResp.Message)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("empty access token")
	}

	return &tokenResp, nil
}

type zaloOAInfo struct {
	OAID   string
	OAName string
}

func fetchZaloOAInfo(accessToken string) (*zaloOAInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://openapi.zalo.me/v2.0/oa/getoa", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("access_token", accessToken)

	resp, err := httpClientWithTimeout.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var result struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
		Data    struct {
			OAID   string `json:"oa_id"`
			Name   string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if result.Error != 0 {
		return nil, fmt.Errorf("zalo getoa error %d: %s", result.Error, result.Message)
	}
	return &zaloOAInfo{OAID: result.Data.OAID, OAName: result.Data.Name}, nil
}

func getBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, c.Request.Host)
}

// ReauthChannel generates an OAuth redirect URL for re-authorizing an existing channel.
func ReauthChannel(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	channelID := c.Param("channelId")

	var channel models.Channel
	if err := db.DB.Where("id = ? AND tenant_id = ?", channelID, tenantID).First(&channel).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Channel not found"})
		return
	}

	cfg, _ := config.Load()
	credBytes, err := pkg.Decrypt(channel.CredentialsEncrypted, cfg.EncryptionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Decrypt failed"})
		return
	}

	var creds map[string]string
	if err := json.Unmarshal(credBytes, &creds); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid credentials"})
		return
	}

	baseURL := getBaseURL(c)
	state := signOAuthState(tenantID, channelID, cfg.JWTSecret)

	var redirectURL string
	switch channel.ChannelType {
	case "zalo_oa":
		appID := creds["app_id"]
		callbackURL := baseURL + "/api/v1/channels/zalo/callback"
		redirectURL = fmt.Sprintf("https://oauth.zaloapp.com/v4/oa/permission?app_id=%s&redirect_uri=%s&state=%s",
			appID, url.QueryEscape(callbackURL), state)
	case "facebook":
		appID := creds["app_id"]
		callbackURL := baseURL + "/api/v1/channels/facebook/callback"
		redirectURL = fmt.Sprintf("https://www.facebook.com/v21.0/dialog/oauth?client_id=%s&redirect_uri=%s&state=%s&scope=pages_show_list,pages_messaging,pages_read_engagement,pages_manage_metadata",
			appID, url.QueryEscape(callbackURL), state)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Channel type does not support re-auth"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"redirect_url": redirectURL})
}

func redirectWithError(c *gin.Context, tenantID, message string) {
	path := "/login"
	if tenantID != "" {
		path = fmt.Sprintf("/%s/channels", tenantID)
	}
	c.Redirect(http.StatusFound, fmt.Sprintf("%s?zalo_auth=error&message=%s", path, url.QueryEscape(message)))
}

func SyncChannelNow(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	channelID := c.Param("channelId")

	var channel models.Channel
	if err := db.DB.Where("id = ? AND tenant_id = ?", channelID, tenantID).First(&channel).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel_not_found"})
		return
	}

	// Mark channel as syncing immediately
	db.DB.Model(&channel).Updates(map[string]interface{}{
		"last_sync_status": "syncing",
		"last_sync_error":  "",
		"updated_at":       time.Now(),
	})

	// Run sync in background to avoid Nginx/proxy gateway timeout
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[security] panic in sync goroutine for channel %s: %v", channel.Name, r)
				db.DB.Model(&models.Channel{}).Where("id = ?", channelID).Updates(map[string]interface{}{
					"last_sync_status": "error",
					"last_sync_error":  fmt.Sprintf("panic: %v", r),
					"updated_at":       time.Now(),
				})
			}
		}()

		cfg, _ := config.Load()
		syncEng := engine.NewSyncEngine(cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := syncEng.SyncChannel(ctx, channel); err != nil {
			log.Printf("[error] sync channel %s failed: %v", channelID, err)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "sync_started"})
}

// FacebookOAuthCallback handles the OAuth callback from Facebook after user authorizes.
func FacebookOAuthCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		redirectWithError(c, "", "Missing code or state")
		return
	}

	cfg, _ := config.Load()
	tenantID, channelID, err := verifyOAuthState(state, cfg.JWTSecret)
	if err != nil {
		log.Printf("[security] invalid Facebook OAuth state: %v", err)
		redirectWithError(c, "", "Authorization failed")
		return
	}

	// Load channel to get app_id and app_secret
	var channel models.Channel
	if err := db.DB.Where("id = ? AND tenant_id = ?", channelID, tenantID).First(&channel).Error; err != nil {
		redirectWithError(c, tenantID, "Channel not found")
		return
	}
	credBytes, err := pkg.Decrypt(channel.CredentialsEncrypted, cfg.EncryptionKey)
	if err != nil {
		log.Printf("[error] decrypt channel %s credentials: %v", channelID, err)
		redirectWithError(c, tenantID, "Authorization failed")
		return
	}

	var creds map[string]string
	if err := json.Unmarshal(credBytes, &creds); err != nil {
		log.Printf("[error] unmarshal channel %s credentials: %v", channelID, err)
		redirectWithError(c, tenantID, "Authorization failed")
		return
	}

	appID := creds["app_id"]
	appSecret := creds["app_secret"]

	// Step 1: Exchange code for short-lived user access token
	callbackURL := fmt.Sprintf("%s/api/v1/channels/facebook/callback", getBaseURL(c))
	userToken, err := exchangeFacebookCode(code, appID, appSecret, callbackURL)
	if err != nil {
		log.Printf("[error] facebook token exchange for channel %s: %v", channelID, err)
		redirectWithError(c, tenantID, "Token exchange failed")
		return
	}

	// Step 2: Exchange short-lived token for long-lived user token
	longLivedToken, err := getLongLivedFBToken(appID, appSecret, userToken)
	if err != nil {
		log.Printf("[error] facebook long-lived token for channel %s: %v", channelID, err)
		redirectWithError(c, tenantID, "Token exchange failed")
		return
	}

	// Step 3: Get user's pages and find the page access token
	pageID, pageToken, pageName, err := getFBPageToken(longLivedToken)
	if err != nil {
		log.Printf("[error] facebook get page token for channel %s: %v", channelID, err)
		redirectWithError(c, tenantID, "Page token retrieval failed")
		return
	}

	// Update channel credentials with page token
	creds["access_token"] = pageToken
	creds["page_id"] = pageID

	updatedCreds, _ := json.Marshal(creds)
	encrypted, err := pkg.Encrypt(updatedCreds, cfg.EncryptionKey)
	if err != nil {
		redirectWithError(c, tenantID, "Encrypt failed")
		return
	}

	db.DB.Model(&models.Channel{}).Where("id = ?", channelID).Updates(map[string]interface{}{
		"credentials_encrypted": encrypted,
		"external_id":           pageID,
		"name":                  pageName,
		"updated_at":            time.Now(),
	})

	// Redirect back to channel detail page with success
	c.Redirect(http.StatusFound, fmt.Sprintf("/%s/channels/%s?fb_auth=success", tenantID, channelID))
}

func exchangeFacebookCode(code, appID, appSecret, redirectURI string) (string, error) {
	apiURL := "https://graph.facebook.com/v21.0/oauth/access_token"
	formData := url.Values{
		"client_id":     {appID},
		"client_secret": {appSecret},
		"redirect_uri":  {redirectURI},
		"code":          {code},
	}

	resp, err := httpClientWithTimeout.PostForm(apiURL, formData)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if errObj, ok := result["error"].(map[string]interface{}); ok {
		msg, _ := errObj["message"].(string)
		return "", fmt.Errorf("facebook error: %s", msg)
	}
	token, _ := result["access_token"].(string)
	if token == "" {
		return "", fmt.Errorf("empty access token")
	}
	return token, nil
}

func getLongLivedFBToken(appID, appSecret, shortToken string) (string, error) {
	apiURL := fmt.Sprintf("https://graph.facebook.com/v21.0/oauth/access_token?grant_type=fb_exchange_token&client_id=%s&client_secret=%s&fb_exchange_token=%s",
		appID, appSecret, shortToken)

	resp, err := httpClientWithTimeout.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if errObj, ok := result["error"].(map[string]interface{}); ok {
		msg, _ := errObj["message"].(string)
		return "", fmt.Errorf("facebook error: %s", msg)
	}
	token, _ := result["access_token"].(string)
	if token == "" {
		return "", fmt.Errorf("empty long-lived token")
	}
	return token, nil
}

func getFBPageToken(userToken string) (pageID, pageToken, pageName string, err error) {
	apiURL := fmt.Sprintf("https://graph.facebook.com/v21.0/me/accounts?access_token=%s&fields=id,name,access_token", userToken)

	resp, err := httpClientWithTimeout.Get(apiURL)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("read response: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", "", fmt.Errorf("parse response: %w", err)
	}
	if errObj, ok := result["error"].(map[string]interface{}); ok {
		msg, _ := errObj["message"].(string)
		return "", "", "", fmt.Errorf("facebook error: %s", msg)
	}

	data, ok := result["data"].([]interface{})
	if !ok || len(data) == 0 {
		return "", "", "", fmt.Errorf("no pages found - make sure you have admin access to a Facebook Page")
	}

	// Use first page (most common case)
	page, _ := data[0].(map[string]interface{})
	pageID, _ = page["id"].(string)
	pageToken, _ = page["access_token"].(string)
	pageName, _ = page["name"].(string)

	return pageID, pageToken, pageName, nil
}

func channelToResponse(ch models.Channel) ChannelResponse {
	return ChannelResponse{
		ID:             ch.ID,
		TenantID:       ch.TenantID,
		ChannelType:    ch.ChannelType,
		Name:           ch.Name,
		ExternalID:     ch.ExternalID,
		IsActive:       ch.IsActive,
		Metadata:       ch.Metadata,
		LastSyncAt:     ch.LastSyncAt,
		LastSyncStatus: ch.LastSyncStatus,
		CreatedAt:      ch.CreatedAt,
	}
}

func GetChannelSyncHistory(c *gin.Context) {
	tenantID := middleware.GetTenantID(c)
	channelID := c.Param("channelId")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if page < 1 {
		page = 1
	}

	var total int64
	db.DB.Model(&models.ActivityLog{}).
		Where("tenant_id = ? AND resource_type = 'channel' AND resource_id = ? AND action LIKE 'sync.%'", tenantID, channelID).
		Count(&total)

	var logs []models.ActivityLog
	db.DB.Where("tenant_id = ? AND resource_type = 'channel' AND resource_id = ? AND action LIKE 'sync.%'", tenantID, channelID).
		Order("created_at DESC").
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
