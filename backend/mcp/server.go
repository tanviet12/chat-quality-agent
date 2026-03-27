package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
)

// JSONRPCRequest represents an MCP JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ServerInfo for MCP initialize response.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Capabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// mcpBearerAuth validates OAuth Bearer tokens for MCP endpoints.
func mcpBearerAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			c.Header("WWW-Authenticate", `Bearer realm="MCP", resource="/.well-known/oauth-protected-resource"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "bearer_token_required"})
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		tokenHash := sha256Hash(token)

		var oauthToken models.OAuthToken
		if err := db.DB.Where("access_token_hash = ?", tokenHash).First(&oauthToken).Error; err != nil {
			log.Printf("[security] MCP auth failed: ip=%s error=invalid_token", c.ClientIP())
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}
		if time.Now().After(oauthToken.ExpiresAt) {
			log.Printf("[security] MCP auth failed: ip=%s error=token_expired", c.ClientIP())
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token_expired"})
			return
		}

		c.Set("mcp_user_id", oauthToken.UserID)
		c.Set("mcp_client_id", oauthToken.ClientID)
		c.Next()
	}
}

func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// SetupMCPRoutes adds MCP Streamable HTTP transport routes to a Gin engine.
func SetupMCPRoutes(r *gin.Engine) {
	mcpGroup := r.Group("/mcp", mcpBearerAuth())
	{
		mcpGroup.POST("", handleMCPRequest)
	}
}

func handleMCPRequest(c *gin.Context) {
	var req JSONRPCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error:   &RPCError{Code: -32700, Message: "Parse error"},
		})
		return
	}

	var result interface{}
	var rpcErr *RPCError

	switch req.Method {
	case "initialize":
		result = handleInitialize()
	case "tools/list":
		result = handleToolsList()
	case "tools/call":
		result, rpcErr = handleToolsCall(c, req.Params)
	default:
		rpcErr = &RPCError{Code: -32601, Message: "Method not found: " + req.Method}
	}

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
		Error:   rpcErr,
	}
	c.JSON(http.StatusOK, resp)
}

func handleInitialize() interface{} {
	return map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities": Capabilities{
			Tools: &ToolsCapability{ListChanged: false},
		},
		"serverInfo": ServerInfo{
			Name:    "Chat Quality Agent",
			Version: "1.0.0",
		},
	}
}
