package mcp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/pkg"
	"golang.org/x/crypto/bcrypt"
)

// Brute force protection for OAuth login
var (
	oauthLoginTracker   = make(map[string]*oauthLoginAttempt)
	oauthLoginTrackerMu sync.Mutex
)

type oauthLoginAttempt struct {
	count    int
	lockedAt time.Time
}

const oauthMaxAttempts = 5
const oauthLockDuration = 15 * time.Minute

func init() {
	// Cleanup expired brute force entries every 5 minutes
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[security] panic in OAuth tracker cleanup: %v", r)
			}
		}()
		for {
			time.Sleep(5 * time.Minute)
			oauthLoginTrackerMu.Lock()
			for k, v := range oauthLoginTracker {
				if !v.lockedAt.IsZero() && time.Since(v.lockedAt) >= oauthLockDuration {
					delete(oauthLoginTracker, k)
				}
			}
			oauthLoginTrackerMu.Unlock()
		}
	}()
}

func checkOAuthLockout(ip string) bool {
	oauthLoginTrackerMu.Lock()
	defer oauthLoginTrackerMu.Unlock()
	a, ok := oauthLoginTracker[ip]
	if !ok {
		return false
	}
	if !a.lockedAt.IsZero() && time.Since(a.lockedAt) < oauthLockDuration {
		return true
	}
	if !a.lockedAt.IsZero() && time.Since(a.lockedAt) >= oauthLockDuration {
		delete(oauthLoginTracker, ip)
	}
	return false
}

func recordOAuthFailure(ip string) {
	oauthLoginTrackerMu.Lock()
	defer oauthLoginTrackerMu.Unlock()
	a, ok := oauthLoginTracker[ip]
	if !ok {
		a = &oauthLoginAttempt{}
		oauthLoginTracker[ip] = a
	}
	a.count++
	if a.count >= oauthMaxAttempts {
		a.lockedAt = time.Now()
		log.Printf("[security] OAuth brute force lockout: ip=%s", ip)
	}
}

func clearOAuthFailure(ip string) {
	oauthLoginTrackerMu.Lock()
	defer oauthLoginTrackerMu.Unlock()
	delete(oauthLoginTracker, ip)
}

// SetupOAuthRoutes adds OAuth 2.0 routes for MCP client authentication.
func SetupOAuthRoutes(r *gin.Engine) {
	oauth := r.Group("/oauth")
	{
		oauth.GET("/authorize", handleAuthorize)
		oauth.POST("/authorize", handleAuthorizeLogin) // consent form submit
		oauth.POST("/token", handleToken)
		oauth.POST("/revoke", handleRevoke)
	}

	// Well-known metadata (RFC 8414 + RFC 9728)
	r.GET("/.well-known/oauth-authorization-server", handleOAuthMetadata)
	r.GET("/.well-known/oauth-protected-resource", handleProtectedResourceMetadata)
}

// OAuth consent page HTML template
var consentPageTmpl = template.Must(template.New("consent").Parse(`<!DOCTYPE html>
<html lang="vi">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Chat Quality Agent — Xác thực MCP</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f5f5; display: flex; justify-content: center; align-items: center; min-height: 100vh; }
  .card { background: white; border-radius: 12px; padding: 40px; max-width: 420px; width: 100%; box-shadow: 0 2px 12px rgba(0,0,0,0.1); }
  h1 { font-size: 20px; margin-bottom: 8px; color: #1a1a1a; }
  .subtitle { font-size: 14px; color: #666; margin-bottom: 24px; }
  .client-info { background: #f0f4ff; border-radius: 8px; padding: 12px; margin-bottom: 20px; font-size: 13px; }
  .client-info strong { color: #3b5998; }
  label { display: block; font-size: 13px; font-weight: 500; margin-bottom: 4px; color: #333; }
  input { width: 100%; padding: 10px 12px; border: 1px solid #ddd; border-radius: 6px; font-size: 14px; margin-bottom: 12px; }
  input:focus { outline: none; border-color: #3b5998; box-shadow: 0 0 0 2px rgba(59,89,152,0.1); }
  .btn { width: 100%; padding: 12px; background: #3b5998; color: white; border: none; border-radius: 6px; font-size: 15px; cursor: pointer; font-weight: 500; }
  .btn:hover { background: #344e86; }
  .btn:disabled { opacity: 0.6; cursor: not-allowed; }
  .error { background: #fef2f2; color: #dc2626; padding: 10px; border-radius: 6px; font-size: 13px; margin-bottom: 12px; }
  .cancel { text-align: center; margin-top: 12px; }
  .cancel a { color: #666; font-size: 13px; text-decoration: none; }
</style>
</head>
<body>
<div class="card">
  <h1>Chat Quality Agent</h1>
  <p class="subtitle">Xác thực để kết nối MCP</p>
  <div class="client-info">
    Ứng dụng <strong>{{.ClientName}}</strong> yêu cầu quyền truy cập dữ liệu của bạn.
  </div>
  {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
  <form method="POST" action="/oauth/authorize">
    <input type="hidden" name="client_id" value="{{.ClientID}}">
    <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
    <input type="hidden" name="state" value="{{.State}}">
    <input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
    <input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">
    <label for="email">Email</label>
    <input type="email" id="email" name="email" required autocomplete="email" value="{{.Email}}">
    <label for="password">Mật khẩu</label>
    <input type="password" id="password" name="password" required autocomplete="current-password">
    <button type="submit" class="btn">Cho phép truy cập</button>
  </form>
  <div class="cancel"><a href="{{.RedirectURI}}?error=access_denied&state={{.State}}">Từ chối</a></div>
</div>
</body>
</html>`))

type consentPageData struct {
	ClientID            string
	ClientName          string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Email               string
	Error               string
}

// handleAuthorize shows consent/login page (GET)
func handleAuthorize(c *gin.Context) {
	clientID := c.Query("client_id")
	redirectURI := c.Query("redirect_uri")
	state := c.Query("state")
	codeChallenge := c.Query("code_challenge")
	codeChallengeMethod := c.Query("code_challenge_method")

	if clientID == "" || redirectURI == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_parameters"})
		return
	}

	// Verify client exists
	var client models.OAuthClient
	if err := db.DB.Where("client_id = ?", clientID).First(&client).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_client"})
		return
	}

	// Validate redirect_uri against client's allowed list
	if !isRedirectURIAllowed(redirectURI, client.RedirectURIs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_redirect_uri"})
		return
	}

	// Check if user is already authenticated via JWT cookie (SSO)
	if tokenStr, err := c.Cookie("cqa_access_token"); err == nil && tokenStr != "" {
		claims, err := middleware.ParseToken(tokenStr)
		if err == nil {
			// User already logged in — auto-approve with consent bypass
			issueAuthCode(c, client, claims.UserID, redirectURI, state, codeChallenge, codeChallengeMethod)
			return
		}
	}

	// Render consent/login page
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := consentPageTmpl.Execute(c.Writer, consentPageData{
		ClientID:            clientID,
		ClientName:          client.Name,
		RedirectURI:         redirectURI,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
	}); err != nil {
		log.Printf("[mcp] consent page render error: %v", err)
	}
}

// handleAuthorizeLogin processes the consent form (POST)
func handleAuthorizeLogin(c *gin.Context) {
	clientID := c.PostForm("client_id")
	redirectURI := c.PostForm("redirect_uri")
	state := c.PostForm("state")
	codeChallenge := c.PostForm("code_challenge")
	codeChallengeMethod := c.PostForm("code_challenge_method")
	email := c.PostForm("email")
	password := c.PostForm("password")

	// Brute force check
	ip := c.ClientIP()
	if checkOAuthLockout(ip) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		if err := consentPageTmpl.Execute(c.Writer, consentPageData{
			ClientID: clientID, ClientName: "", RedirectURI: redirectURI, State: state,
			CodeChallenge: codeChallenge, CodeChallengeMethod: codeChallengeMethod,
			Email: email, Error: "Đã vượt quá số lần thử. Vui lòng đợi 15 phút.",
		}); err != nil {
			log.Printf("[mcp] consent page render error: %v", err)
		}
		return
	}

	// Verify client
	var client models.OAuthClient
	if err := db.DB.Where("client_id = ?", clientID).First(&client).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_client"})
		return
	}

	// Validate redirect_uri
	if !isRedirectURIAllowed(redirectURI, client.RedirectURIs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_redirect_uri"})
		return
	}

	// Authenticate user
	var user models.User
	if err := db.DB.Where("email = ?", email).First(&user).Error; err != nil {
		recordOAuthFailure(ip)
		log.Printf("[security] OAuth login failed: email=%s ip=%s reason=user_not_found", email, ip)
		renderConsentError(c, clientID, client.Name, redirectURI, state, codeChallenge, codeChallengeMethod, email, "Email hoặc mật khẩu không đúng")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		recordOAuthFailure(ip)
		log.Printf("[security] OAuth login failed: email=%s ip=%s reason=wrong_password", email, ip)
		renderConsentError(c, clientID, client.Name, redirectURI, state, codeChallenge, codeChallengeMethod, email, "Email hoặc mật khẩu không đúng")
		return
	}

	clearOAuthFailure(ip)
	issueAuthCode(c, client, user.ID, redirectURI, state, codeChallenge, codeChallengeMethod)
}

func renderConsentError(c *gin.Context, clientID, clientName, redirectURI, state, codeChallenge, codeChallengeMethod, email, errMsg string) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := consentPageTmpl.Execute(c.Writer, consentPageData{
		ClientID: clientID, ClientName: clientName, RedirectURI: redirectURI, State: state,
		CodeChallenge: codeChallenge, CodeChallengeMethod: codeChallengeMethod,
		Email: email, Error: errMsg,
	}); err != nil {
		log.Printf("[mcp] consent page render error: %v", err)
	}
}

func issueAuthCode(c *gin.Context, client models.OAuthClient, userID, redirectURI, state, codeChallenge, codeChallengeMethod string) {
	code := generateRandomString(32)

	// Store auth code in DB
	authCode := models.OAuthAuthorizationCode{
		ID:                  pkg.NewUUID(),
		Code:                code,
		ClientID:            client.ClientID,
		UserID:              userID,
		RedirectURI:         redirectURI,
		Scopes:              client.Scopes,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresAt:           time.Now().Add(10 * time.Minute),
		Used:                false,
		CreatedAt:           time.Now(),
	}
	if err := db.DB.Create(&authCode).Error; err != nil {
		log.Printf("[security] failed to create auth code: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	// Redirect back with auth code
	redirectURL := fmt.Sprintf("%s?code=%s", redirectURI, code)
	if state != "" {
		redirectURL += "&state=" + state
	}
	c.Redirect(http.StatusFound, redirectURL)
}

func isRedirectURIAllowed(uri, allowedJSON string) bool {
	if allowedJSON == "" || allowedJSON == "[]" {
		// No redirect URIs configured — reject (prevent open redirect)
		return false
	}
	var allowed []string
	if err := json.Unmarshal([]byte(allowedJSON), &allowed); err != nil {
		return false
	}
	for _, a := range allowed {
		if a == uri {
			return true
		}
	}
	return false
}

// handleToken exchanges auth code for access token, or refreshes a token.
func handleToken(c *gin.Context) {
	grantType := c.PostForm("grant_type")

	switch grantType {
	case "authorization_code":
		handleAuthCodeExchange(c)
	case "refresh_token":
		handleRefreshTokenExchange(c)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported_grant_type"})
	}
}

func handleAuthCodeExchange(c *gin.Context) {
	code := c.PostForm("code")
	clientID := c.PostForm("client_id")
	clientSecret := c.PostForm("client_secret")
	codeVerifier := c.PostForm("code_verifier")

	// Atomically find and mark auth code as used (prevents race condition)
	var authCode models.OAuthAuthorizationCode
	result := db.DB.Model(&models.OAuthAuthorizationCode{}).
		Where("code = ? AND used = false", code).
		Update("used", true)
	if result.RowsAffected == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant"})
		return
	}
	// Now read the code details
	if err := db.DB.Where("code = ?", code).First(&authCode).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant"})
		return
	}

	// Validate
	if time.Now().After(authCode.ExpiresAt) || authCode.ClientID != clientID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant"})
		return
	}

	// PKCE verification
	if authCode.CodeChallenge != "" {
		if codeVerifier == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant", "error_description": "code_verifier required"})
			return
		}
		if !verifyPKCE(codeVerifier, authCode.CodeChallenge) {
			log.Printf("[security] PKCE verification failed: ip=%s client=%s", c.ClientIP(), clientID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant", "error_description": "PKCE verification failed"})
			return
		}
	}

	// Verify client secret
	var client models.OAuthClient
	if err := db.DB.Where("client_id = ?", clientID).First(&client).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_client"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(client.ClientSecretHash), []byte(clientSecret)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_client"})
		return
	}

	// Generate tokens
	accessToken := generateRandomString(48)
	refreshToken := generateRandomString(48)

	tokenEntry := models.OAuthToken{
		ID:               pkg.NewUUID(),
		ClientID:         clientID,
		UserID:           authCode.UserID,
		AccessTokenHash:  hashToken(accessToken),
		RefreshTokenHash: hashToken(refreshToken),
		Scopes:           authCode.Scopes,
		ExpiresAt:        time.Now().Add(1 * time.Hour),
		CreatedAt:        time.Now(),
	}
	if err := db.DB.Create(&tokenEntry).Error; err != nil {
		log.Printf("[security] failed to create token: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token_creation_failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_type":    "Bearer",
		"expires_in":    3600,
		"scope":         authCode.Scopes,
	})
}

func verifyPKCE(codeVerifier, codeChallenge string) bool {
	// S256: SHA256(code_verifier) == code_challenge
	h := sha256.Sum256([]byte(codeVerifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return computed == codeChallenge
}

func handleRefreshTokenExchange(c *gin.Context) {
	refreshToken := c.PostForm("refresh_token")
	hash := hashToken(refreshToken)

	var tokenEntry models.OAuthToken
	if err := db.DB.Where("refresh_token_hash = ?", hash).First(&tokenEntry).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_grant"})
		return
	}

	// Delete old token (rotation)
	if err := db.DB.Delete(&tokenEntry).Error; err != nil {
		log.Printf("[security] failed to delete old token: %v", err)
	}

	// Issue new tokens
	newAccessToken := generateRandomString(48)
	newRefreshToken := generateRandomString(48)

	newEntry := models.OAuthToken{
		ID:               pkg.NewUUID(),
		ClientID:         tokenEntry.ClientID,
		UserID:           tokenEntry.UserID,
		AccessTokenHash:  hashToken(newAccessToken),
		RefreshTokenHash: hashToken(newRefreshToken),
		Scopes:           tokenEntry.Scopes,
		ExpiresAt:        time.Now().Add(1 * time.Hour),
		CreatedAt:        time.Now(),
	}
	if err := db.DB.Create(&newEntry).Error; err != nil {
		log.Printf("[security] failed to create refreshed token: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token_creation_failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  newAccessToken,
		"refresh_token": newRefreshToken,
		"token_type":    "Bearer",
		"expires_in":    3600,
	})
}

func handleRevoke(c *gin.Context) {
	token := c.PostForm("token")
	hash := hashToken(token)
	result := db.DB.Where("access_token_hash = ? OR refresh_token_hash = ?", hash, hash).Delete(&models.OAuthToken{})
	log.Printf("[security] token revoked: ip=%s affected=%d", c.ClientIP(), result.RowsAffected)
	c.JSON(http.StatusOK, gin.H{"message": "revoked"})
}

// OAuth metadata endpoints (RFC 8414 + RFC 9728)
func handleOAuthMetadata(c *gin.Context) {
	baseURL := getBaseURLFromRequest(c)
	c.JSON(http.StatusOK, gin.H{
		"issuer":                             baseURL,
		"authorization_endpoint":             baseURL + "/oauth/authorize",
		"token_endpoint":                     baseURL + "/oauth/token",
		"revocation_endpoint":                baseURL + "/oauth/revoke",
		"response_types_supported":           []string{"code"},
		"grant_types_supported":              []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":   []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post"},
	})
}

func handleProtectedResourceMetadata(c *gin.Context) {
	baseURL := getBaseURLFromRequest(c)
	c.JSON(http.StatusOK, gin.H{
		"resource":               baseURL + "/mcp",
		"authorization_servers":  []string{baseURL},
		"bearer_methods_supported": []string{"header"},
	})
}

func getBaseURLFromRequest(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, c.Request.Host)
}

func generateRandomString(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)[:length]
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// CRUD for MCP OAuth Clients (called from API routes)

func ListMCPClients(c *gin.Context) {
	userID := c.GetString("user_id")
	var clients []models.OAuthClient
	db.DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&clients)

	type clientResponse struct {
		ID        string    `json:"id"`
		ClientID  string    `json:"client_id"`
		Name      string    `json:"name"`
		Scopes    string    `json:"scopes"`
		CreatedAt time.Time `json:"created_at"`
	}
	results := make([]clientResponse, len(clients))
	for i, cl := range clients {
		results[i] = clientResponse{
			ID:        cl.ID,
			ClientID:  cl.ClientID,
			Name:      cl.Name,
			Scopes:    cl.Scopes,
			CreatedAt: cl.CreatedAt,
		}
	}
	c.JSON(http.StatusOK, results)
}

func CreateMCPClient(c *gin.Context) {
	userID := c.GetString("user_id")
	var req struct {
		Name         string   `json:"name" binding:"required"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	clientID := "cqa_" + generateRandomString(24)
	clientSecret := "sk_" + generateRandomString(48)
	secretHash, err := bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	redirectURIsJSON := "[]"
	if len(req.RedirectURIs) > 0 {
		b, _ := json.Marshal(req.RedirectURIs)
		redirectURIsJSON = string(b)
	}

	client := models.OAuthClient{
		ID:               pkg.NewUUID(),
		ClientID:         clientID,
		ClientSecretHash: string(secretHash),
		Name:             req.Name,
		RedirectURIs:     redirectURIsJSON,
		Scopes:           `["read","write"]`,
		UserID:           userID,
		CreatedAt:        time.Now(),
	}
	if err := db.DB.Create(&client).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "client_creation_failed"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":            client.ID,
		"client_id":     clientID,
		"client_secret": clientSecret,
		"name":          client.Name,
		"scopes":        client.Scopes,
	})
}

func DeleteMCPClient(c *gin.Context) {
	userID := c.GetString("user_id")
	clientDBID := c.Param("id")

	result := db.DB.Where("id = ? AND user_id = ?", clientDBID, userID).Delete(&models.OAuthClient{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
