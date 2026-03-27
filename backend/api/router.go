package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/api/handlers"
	"github.com/nmtan2001/chat-quality-agent/api/middleware"
	"github.com/nmtan2001/chat-quality-agent/config"
	"github.com/nmtan2001/chat-quality-agent/mcp"
)

func SetupRouter(cfg *config.Config) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// Serve static frontend files in production
	if cfg.IsProduction() {
		r.Static("/assets", "./static/assets")
		r.Static("/guides", "./static/guides")
		r.StaticFile("/favicon.png", "./static/favicon.png")
		r.StaticFile("/", "./static/index.html")
		r.NoRoute(func(c *gin.Context) {
			// SPA fallback: serve index.html for non-API routes
			if len(c.Request.URL.Path) > 4 && c.Request.URL.Path[:4] == "/api" {
				c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
				return
			}
			c.File("./static/index.html")
		})
	}

	// Serve uploaded files (requires JWT auth)
	r.GET("/api/v1/files/*filepath", middleware.JWTAuth(), func(c *gin.Context) {
		fp := c.Param("filepath")
		// Security: clean path and verify it stays within base directory
		cleanPath := filepath.Clean(fp)
		if strings.Contains(cleanPath, "..") || strings.HasPrefix(cleanPath, "/") && strings.Contains(cleanPath[1:], "..") {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		fullPath := filepath.Join("/var/lib/cqa/files", cleanPath)
		// Verify resolved path is within base directory
		if !strings.HasPrefix(fullPath, "/var/lib/cqa/files") {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.File(fullPath)
	})

	// CORS
	r.Use(corsMiddleware(cfg))

	// Rate limiting (500 req/min per IP by default)
	r.Use(middleware.RateLimit(cfg.RateLimitPerIP))

	// Security headers
	r.Use(securityHeaders())

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Public API
	api := r.Group("/api/v1")
	{
		// Public webhooks (no auth - external services call these)
		api.POST("/webhooks/guesty", handlers.GuestyWebhook)
		api.GET("/webhooks/guesty", handlers.GuestyWebhookChallenge)

		// Initial setup (only works when no users exist)
		api.GET("/setup/status", handlers.SetupStatus)
		api.POST("/setup", handlers.Setup)

		auth := api.Group("/auth")
		{
			auth.POST("/login", handlers.Login)
			auth.POST("/refresh", handlers.RefreshTokenHandler)
			auth.POST("/logout", handlers.Logout)
		}
	}

	// OAuth callbacks (public — platforms redirect here)
	api.GET("/channels/zalo/callback", handlers.ZaloOAuthCallback)
	api.GET("/channels/facebook/callback", handlers.FacebookOAuthCallback)

	// Authenticated API
	authed := api.Group("")
	authed.Use(middleware.JWTAuth())
	{
		authed.GET("/profile", handlers.GetProfile)
		authed.PUT("/profile", handlers.UpdateProfile)
		authed.PUT("/profile/password", handlers.ChangeProfilePassword)

		// Tenants (list + create don't need tenant context)
		authed.GET("/tenants", handlers.ListTenants)
		authed.POST("/tenants", handlers.CreateTenant)

		// Tenant-scoped routes
		tenant := authed.Group("/tenants/:tenantId")
		tenant.Use(middleware.TenantContext())
		{
			tenant.GET("", handlers.GetTenant)
			tenant.PUT("", middleware.RequireRole("owner", "admin"), handlers.UpdateTenant)
			tenant.DELETE("", middleware.RequireRole("owner"), handlers.DeleteTenant)

			// Channels
			tenant.GET("/channels", handlers.ListChannels)
			tenant.POST("/channels", middleware.RequireRole("owner", "admin"), handlers.CreateChannel)
			tenant.GET("/channels/:channelId", handlers.GetChannel)
			tenant.PUT("/channels/:channelId", middleware.RequireRole("owner", "admin"), handlers.UpdateChannel)
			tenant.DELETE("/channels/:channelId", middleware.RequirePermission("channels", "d"), handlers.DeleteChannel)
			tenant.POST("/channels/:channelId/test", handlers.TestChannelConnection)
			tenant.POST("/channels/:channelId/sync", middleware.RequireRole("owner", "admin"), handlers.SyncChannelNow)
			tenant.POST("/channels/:channelId/reauth", middleware.RequireRole("owner", "admin"), handlers.ReauthChannel)
			tenant.GET("/channels/:channelId/sync-history", handlers.GetChannelSyncHistory)
			tenant.DELETE("/channels/:channelId/conversations", middleware.RequireRole("owner", "admin"), handlers.PurgeChannelConversations)

			// Conversations & Messages
			tenant.GET("/onboarding-status", handlers.GetOnboardingStatus)
			tenant.GET("/conversations", handlers.ListConversations)
			tenant.GET("/conversations/export", handlers.ExportMessages)
			tenant.GET("/conversations/evaluated", handlers.ListEvaluatedConversations)
			tenant.GET("/conversations/:conversationId/messages", handlers.GetConversationMessages)
			tenant.GET("/conversations/:conversationId/evaluations", handlers.GetConversationEvaluations)
			tenant.GET("/conversations/:conversationId/page", handlers.GetConversationPage)

			// Dashboard
			tenant.GET("/dashboard", handlers.GetDashboard)

			// Jobs
			tenant.GET("/jobs", handlers.ListJobs)
			tenant.POST("/jobs", middleware.RequireRole("owner", "admin"), handlers.CreateJob)
			tenant.GET("/jobs/:jobId", handlers.GetJob)
			tenant.PUT("/jobs/:jobId", middleware.RequireRole("owner", "admin"), handlers.UpdateJob)
			tenant.DELETE("/jobs/:jobId", middleware.RequirePermission("jobs", "d"), handlers.DeleteJob)
			tenant.POST("/jobs/:jobId/trigger", middleware.RequireRole("owner", "admin"), handlers.TriggerJob)
			tenant.POST("/jobs/:jobId/test-run", middleware.RequireRole("owner", "admin"), handlers.TestRunJob)
			tenant.GET("/jobs/:jobId/runs", handlers.ListJobRuns)
			tenant.GET("/jobs/:jobId/runs/:runId/results", handlers.ListJobResults)
			tenant.POST("/test-output", handlers.TestOutput)

			// Activity Logs
			tenant.GET("/activity-logs", handlers.ListActivityLogs)

			// Cost Logs
			tenant.GET("/cost-logs", handlers.ListCostLogs)

			// Users (tenant members management)
			tenant.GET("/users", handlers.ListTenantUsers)
			tenant.POST("/users/invite", middleware.RequireRole("owner", "admin"), handlers.InviteUser)
			tenant.PUT("/users/:userId/role", middleware.RequireRole("owner"), handlers.UpdateUserRole)
			tenant.PUT("/users/:userId/reset-password", middleware.RequireRole("owner", "admin"), handlers.ResetUserPassword)
			tenant.DELETE("/users/:userId", middleware.RequireRole("owner"), handlers.RemoveUserFromTenant)

			// Job all results + export
			tenant.GET("/jobs/:jobId/results", handlers.ListAllJobResults)
			tenant.GET("/jobs/:jobId/results/export", handlers.ExportJobResults)
			tenant.DELETE("/jobs/:jobId/results", middleware.RequirePermission("jobs", "d"), handlers.ClearJobResults)
			tenant.DELETE("/jobs/:jobId/runs", middleware.RequirePermission("jobs", "d"), handlers.ClearJobRuns)

			// Settings
			tenant.GET("/settings", handlers.GetSettings)
			tenant.PUT("/settings", handlers.SaveSetting)
			tenant.PUT("/settings/ai", middleware.RequireRole("owner", "admin"), handlers.SaveAISettings)
			tenant.PUT("/settings/analysis", middleware.RequireRole("owner", "admin"), handlers.SaveAnalysisSettings)
			tenant.POST("/settings/ai/test", middleware.RequireRole("owner", "admin"), handlers.TestAIKey)
			tenant.PUT("/settings/general", middleware.RequireRole("owner", "admin"), handlers.SaveGeneralSettings)
			tenant.PUT("/settings/password", handlers.ChangePassword)

			// Notification logs
			tenant.GET("/notification-logs", handlers.ListNotificationLogs)

			// Demo data
			tenant.GET("/demo/status", handlers.GetDemoStatus)
			tenant.POST("/demo/import", middleware.RequireRole("owner", "admin"), handlers.ImportDemoData)
			tenant.DELETE("/demo/reset", middleware.RequireRole("owner", "admin"), handlers.ResetDemoData)
		}
	}

	// Agent API (for Company OS integration, requires auth)
	agentAPI := api.Group("/agents", middleware.JWTAuth())
	{
		agentAPI.GET("", handlers.ListAgents)
		agentAPI.GET("/capabilities", handlers.ListAgents)
		agentAPI.POST("/:agentName/run", handlers.AgentRun)
		agentAPI.GET("/:agentName/query", handlers.AgentQuery)
		agentAPI.GET("/:agentName/health", handlers.AgentHealth)
	}

	// MCP Server (Streamable HTTP transport)
	mcp.SetupMCPRoutes(r)

	// OAuth 2.0 (for Claude Web MCP authentication)
	mcp.SetupOAuthRoutes(r)

	// MCP Client management (requires JWT auth)
	mcpAPI := api.Group("/mcp/clients", middleware.JWTAuth())
	{
		mcpAPI.GET("", mcp.ListMCPClients)
		mcpAPI.POST("", mcp.CreateMCPClient)
		mcpAPI.DELETE("/:id", mcp.DeleteMCPClient)
	}

	return r
}

func corsMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestOrigin := c.GetHeader("Origin")
		allowedOrigin := requestOrigin

		if cfg.IsProduction() && requestOrigin != "" {
			// In production, only allow same-origin or configured origins
			host := c.Request.Host
			if !strings.Contains(requestOrigin, host) {
				allowedOrigin = ""
			}
		}

		if allowedOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; connect-src 'self' https:; font-src 'self' data:; frame-ancestors 'none'")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Next()
	}
}
