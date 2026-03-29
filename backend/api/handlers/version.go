package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// AppVersion is set from main.go at startup
var AppVersion = "dev"

const (
	githubReleasesURL = "https://api.github.com/repos/tanviet12/chat-quality-agent/releases/latest"
	cacheDuration     = 1 * time.Hour
)

var (
	versionCache     map[string]interface{}
	versionCacheTime time.Time
	versionCacheMu   sync.Mutex
)

func CheckVersion(c *gin.Context) {
	versionCacheMu.Lock()
	if versionCache != nil && time.Since(versionCacheTime) < cacheDuration {
		cached := versionCache
		versionCacheMu.Unlock()
		c.JSON(http.StatusOK, cached)
		return
	}
	versionCacheMu.Unlock()

	// Fetch latest release from GitHub
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(githubReleasesURL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"current":    AppVersion,
			"has_update": false,
			"error":      fmt.Sprintf("check failed: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"current":    AppVersion,
			"has_update": false,
		})
		return
	}

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"current":    AppVersion,
			"has_update": false,
		})
		return
	}

	currentNorm := strings.TrimPrefix(strings.TrimSpace(AppVersion), "v")
	latestNorm := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	hasUpdate := latestNorm != "" && latestNorm != currentNorm
	result := map[string]interface{}{
		"current":       AppVersion,
		"latest":        release.TagName,
		"has_update":    hasUpdate,
		"release_url":   release.HTMLURL,
		"release_notes": release.Body,
	}

	// Cache result
	versionCacheMu.Lock()
	versionCache = result
	versionCacheTime = time.Now()
	versionCacheMu.Unlock()

	c.JSON(http.StatusOK, result)
}
