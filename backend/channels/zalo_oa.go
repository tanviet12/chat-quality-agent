package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	zaloAPIBaseV2 = "https://openapi.zalo.me/v2.0/oa"
	zaloAPIBaseV3 = "https://openapi.zalo.me/v3.0/oa"
	zaloOAuthURL  = "https://oauth.zaloapp.com/v4/oa/access_token"
)

// ZaloOACredentials holds the credentials needed for Zalo OA API.
type ZaloOACredentials struct {
	AppID        string `json:"app_id"`
	AppSecret    string `json:"app_secret"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	OAId         string `json:"oa_id"`
}

// OnTokenRefresh is called when tokens are refreshed — caller should persist new creds.
type OnTokenRefresh func(newAccessToken, newRefreshToken string)

type ZaloOAAdapter struct {
	creds          ZaloOACredentials
	client         *http.Client
	mu             sync.Mutex
	onTokenRefresh OnTokenRefresh
}

func NewZaloOAAdapter(creds ZaloOACredentials) *ZaloOAAdapter {
	return &ZaloOAAdapter{
		creds:  creds,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (z *ZaloOAAdapter) SetTokenRefreshCallback(cb OnTokenRefresh) {
	z.onTokenRefresh = cb
}

// refreshToken performs Zalo token refresh (single-use rotation).
func (z *ZaloOAAdapter) refreshToken(ctx context.Context) error {
	z.mu.Lock()
	defer z.mu.Unlock()

	form := url.Values{
		"refresh_token": {z.creds.RefreshToken},
		"app_id":        {z.creds.AppID},
		"grant_type":    {"refresh_token"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", zaloOAuthURL, nil)
	if err != nil {
		return fmt.Errorf("create zalo token refresh request: %w", err)
	}
	req.URL.RawQuery = form.Encode()
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("secret_key", z.creds.AppSecret)

	resp, err := z.client.Do(req)
	if err != nil {
		return fmt.Errorf("zalo token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Error        int    `json:"error"`
		Message      string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("zalo token refresh decode failed: %w", err)
	}
	if result.Error != 0 {
		return fmt.Errorf("zalo token refresh error %d: %s", result.Error, result.Message)
	}

	z.creds.AccessToken = result.AccessToken
	z.creds.RefreshToken = result.RefreshToken

	if z.onTokenRefresh != nil {
		z.onTokenRefresh(result.AccessToken, result.RefreshToken)
	}

	return nil
}

// doRequest makes an authenticated Zalo API request with auto-retry on token expiry.
func (z *ZaloOAAdapter) doRequest(ctx context.Context, method, apiURL string, params map[string]interface{}) (map[string]interface{}, error) {
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create zalo api request: %w", err)
		}

		// Zalo API: params go as JSON-encoded `data` query param
		if params != nil {
			q := req.URL.Query()
			paramsJSON, _ := json.Marshal(params)
			q.Set("data", string(paramsJSON))
			req.URL.RawQuery = q.Encode()
		}

		z.mu.Lock()
		token := z.creds.AccessToken
		z.mu.Unlock()
		req.Header.Set("access_token", token)

		resp, err := z.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("zalo api request failed: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("zalo api read body failed: %w", err)
		}

		log.Printf("[zalo] API %s: status=%d len=%d", apiURL, resp.StatusCode, len(body))

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("zalo api decode failed: %w", err)
		}

		// Check for token expired error (error=-216)
		if errCode, ok := result["error"].(float64); ok && errCode == -216 && attempt == 0 {
			if refreshErr := z.refreshToken(ctx); refreshErr != nil {
				return nil, fmt.Errorf("token refresh failed: %w", refreshErr)
			}
			continue
		}

		if errCode, ok := result["error"].(float64); ok && errCode != 0 {
			msg, _ := result["message"].(string)
			return nil, fmt.Errorf("zalo api error %v: %s", errCode, msg)
		}

		return result, nil
	}
	return nil, fmt.Errorf("zalo api failed after retry")
}

func (z *ZaloOAAdapter) FetchRecentConversations(ctx context.Context, since time.Time, limit int) ([]SyncedConversation, error) {
	var conversations []SyncedConversation
	offset := 0
	pageSize := 10 // Zalo max is 10

	for {
		if limit > 0 && len(conversations) >= limit {
			break
		}

		result, err := z.doRequest(ctx, "GET", zaloAPIBaseV2+"/listrecentchat", map[string]interface{}{
			"offset": offset,
			"count":  pageSize,
		})
		if err != nil {
			return conversations, err
		}

		// Zalo response can be {"data": [...]} or {"data": {"data": [...]}}
		data := extractZaloDataArray(result)
		log.Printf("[zalo] extractZaloDataArray returned %d items, result keys: %v, data type: %T", len(data), mapKeys(result), result["data"])
		if len(data) == 0 {
			break
		}

		for _, item := range data {
			conv, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			// Zalo listrecentchat: src=0 means OA sent (from=OA, to=customer), src=1 means customer sent
			var userID, displayName string
			src, _ := conv["src"].(float64)
			if src == 0 {
				// OA sent last message → customer is "to"
				userID, _ = conv["to_id"].(string)
				displayName, _ = conv["to_display_name"].(string)
			} else {
				// Customer sent last message → customer is "from"
				userID, _ = conv["from_id"].(string)
				displayName, _ = conv["from_display_name"].(string)
			}

			// Parse timestamp (Zalo uses milliseconds)
			var lastMsgAt time.Time
			if ts, ok := conv["time"].(float64); ok {
				lastMsgAt = time.UnixMilli(int64(ts))
			}

			// Don't filter by since here — let DB dedup handle it.
			// Zalo listrecentchat is already sorted newest first and max 10 per page.

			conversations = append(conversations, SyncedConversation{
				ExternalID:     userID, // Zalo uses user_id as conversation key
				ExternalUserID: userID,
				CustomerName:   displayName,
				LastMessageAt:  lastMsgAt,
				Metadata:       conv,
			})
		}

		if len(data) < pageSize {
			break
		}
		offset += pageSize
	}

	return conversations, nil
}

func (z *ZaloOAAdapter) FetchMessages(ctx context.Context, conversationID string, since time.Time) ([]SyncedMessage, error) {
	var messages []SyncedMessage
	offset := 0
	pageSize := 10

	for {
		result, err := z.doRequest(ctx, "GET", zaloAPIBaseV2+"/conversation", map[string]interface{}{
			"user_id": conversationID,
			"offset":  offset,
			"count":   pageSize,
		})
		if err != nil {
			return messages, err
		}

		data := extractZaloDataArray(result)
		if len(data) == 0 {
			break
		}

		for _, item := range data {
			msg, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			var sentAt time.Time
			if ts, ok := msg["time"].(float64); ok {
				sentAt = time.UnixMilli(int64(ts))
			}

			// Don't filter by since — let DB dedup handle duplicates

			msgID := fmt.Sprintf("%v", msg["message_id"])
			content, _ := msg["message"].(string)
			senderType := "customer"
			senderName := ""

			if src, ok := msg["src"].(float64); ok && src == 0 {
				senderType = "agent"
				senderName = "OA"
			}
			if from, ok := msg["from_display_name"].(string); ok && senderType == "customer" {
				senderName = from
			}

			syncedMsg := SyncedMessage{
				ExternalID:  msgID,
				SenderType:  senderType,
				SenderName:  senderName,
				Content:     content,
				ContentType: "text",
				SentAt:      sentAt,
				RawData:     msg,
			}

			// Check for attachments (image, file, sticker, gif, etc.)
			if msgType, ok := msg["type"].(string); ok && msgType != "text" {
				syncedMsg.ContentType = msgType

				// Extract attachment URL from Zalo message
				aURL := ""
				aName := ""
				if u, ok := msg["url"].(string); ok && u != "" {
					aURL = u
				} else if u, ok := msg["thumb"].(string); ok && u != "" {
					aURL = u
				}
				// For file type: check links array
				if links, ok := msg["links"].([]interface{}); ok && len(links) > 0 {
					if link, ok := links[0].(map[string]interface{}); ok {
						if u, ok := link["url"].(string); ok {
							aURL = u
						}
						if n, ok := link["name"].(string); ok {
							aName = n
						}
					}
				}
				if aURL != "" {
					if aName == "" {
						aName = fmt.Sprintf("%s-%s", msgType, msgID)
					}
					syncedMsg.Attachments = append(syncedMsg.Attachments, Attachment{
						Type: msgType,
						URL:  aURL,
						Name: aName,
					})
				}
			}

			messages = append(messages, syncedMsg)
		}

		if len(data) < pageSize {
			break
		}
		offset += pageSize
	}

	return messages, nil
}

// extractZaloDataArray handles both {"data": [...]} and {"data": {"data": [...]}}
func extractZaloDataArray(result map[string]interface{}) []interface{} {
	if arr, ok := result["data"].([]interface{}); ok {
		return arr
	}
	if nested, ok := result["data"].(map[string]interface{}); ok {
		if arr, ok := nested["data"].([]interface{}); ok {
			return arr
		}
	}
	return nil
}

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (z *ZaloOAAdapter) HealthCheck(ctx context.Context) error {
	_, err := z.doRequest(ctx, "GET", zaloAPIBaseV2+"/getoa", nil)
	return err
}
