package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/vietbui/chat-quality-agent/channels"
	"github.com/vietbui/chat-quality-agent/config"
	"github.com/vietbui/chat-quality-agent/db"
	"github.com/vietbui/chat-quality-agent/db/models"
	"github.com/vietbui/chat-quality-agent/pkg"
	"github.com/vietbui/chat-quality-agent/storage"
)

// SyncEngine handles pulling messages from external channels into the database.
type SyncEngine struct {
	cfg *config.Config
}

func NewSyncEngine(cfg *config.Config) *SyncEngine {
	return &SyncEngine{cfg: cfg}
}

// AttachZaloTokenRefresh gắn callback lưu lại access/refresh token mới sau khi
// adapter Zalo xoay token (refresh token của Zalo dùng một lần, không lưu lại
// thì lần gọi sau sẽ không refresh được nữa).
func AttachZaloTokenRefresh(adapter channels.ChannelAdapter, channelID, encKey string) {
	zaloAdapter, ok := adapter.(*channels.ZaloOAAdapter)
	if !ok {
		return
	}
	zaloAdapter.SetTokenRefreshCallback(func(newAccess, newRefresh string) {
		persistRefreshedTokens(channelID, encKey, newAccess, newRefresh)
	})
}

// persistRefreshedTokens ghi access/refresh token mới vào kênh.
func persistRefreshedTokens(channelID, encKey, newAccess, newRefresh string) {
	var ch models.Channel
	if db.DB.First(&ch, "id = ?", channelID).Error != nil {
		return
	}
	oldCreds, err := pkg.Decrypt(ch.CredentialsEncrypted, encKey)
	if err != nil {
		log.Printf("[sync] decrypt failed for token persist: %v", err)
		return
	}
	var credsMap map[string]interface{}
	if err := json.Unmarshal(oldCreds, &credsMap); err != nil {
		log.Printf("[sync] unmarshal creds failed: %v", err)
		return
	}
	credsMap["access_token"] = newAccess
	credsMap["refresh_token"] = newRefresh
	newCredJSON, _ := json.Marshal(credsMap)
	encrypted, err := pkg.Encrypt(newCredJSON, encKey)
	if err != nil {
		log.Printf("[sync] encrypt failed for token persist: %v", err)
		return
	}
	db.DB.Model(&ch).Update("credentials_encrypted", encrypted)
	log.Printf("[sync] persisted refreshed Zalo tokens for channel %s", channelID)
}

// SyncChannel syncs a single channel: fetches conversations + messages and upserts into DB.
func (s *SyncEngine) SyncChannel(ctx context.Context, channel models.Channel) error {
	log.Printf("[sync] starting sync for channel %s (%s)", channel.Name, channel.ChannelType)

	// Decrypt credentials
	credBytes, err := pkg.Decrypt(channel.CredentialsEncrypted, s.cfg.EncryptionKey)
	if err != nil {
		return s.updateSyncStatus(channel.ID, "error", fmt.Sprintf("decrypt failed: %v", err))
	}

	adapter, err := channels.NewAdapter(channel.ChannelType, credBytes)
	if err != nil {
		return s.updateSyncStatus(channel.ID, "error", fmt.Sprintf("adapter init failed: %v", err))
	}

	// Set token refresh callback for Zalo — persist new tokens to DB after refresh
	AttachZaloTokenRefresh(adapter, channel.ID, s.cfg.EncryptionKey)

	// Determine since — use last_sync_at or default to 7 days ago
	// Subtract 1 hour buffer to avoid missing messages near the boundary
	since := time.Now().AddDate(0, 0, -7)
	if channel.LastSyncAt != nil {
		since = channel.LastSyncAt.Add(-1 * time.Hour)
	}

	// Fetch recent conversations
	conversations, err := adapter.FetchRecentConversations(ctx, since, 100)
	if err != nil {
		return s.updateSyncStatus(channel.ID, "error", fmt.Sprintf("fetch conversations failed: %v", err))
	}

	log.Printf("[sync] channel %s: found %d conversations", channel.Name, len(conversations))

	// Check if file sync is enabled for this channel
	syncFiles := false
	if channel.Metadata != "" {
		var meta map[string]interface{}
		if json.Unmarshal([]byte(channel.Metadata), &meta) == nil {
			if sf, ok := meta["sync_files"]; ok {
				syncFiles, _ = sf.(bool)
			}
		}
	}
	log.Printf("[sync] channel %s: sync_files=%v, metadata=%s", channel.Name, syncFiles, channel.Metadata)

	totalMessages := 0
	for _, conv := range conversations {
		// Upsert conversation
		convID, err := s.upsertConversation(channel.TenantID, channel.ID, conv)
		if err != nil {
			log.Printf("[sync] error upserting conversation %s: %v", conv.ExternalID, err)
			continue
		}

		// Fetch messages for this conversation
		messages, err := adapter.FetchMessages(ctx, conv.ExternalID, since)
		if err != nil {
			log.Printf("[sync] error fetching messages for %s: %v", conv.ExternalID, err)
			continue
		}

		// Upsert messages
		for _, msg := range messages {
			if syncFiles {
				s.downloadAttachments(channel.TenantID, convID, &msg)
			}
			if err := s.upsertMessage(channel.TenantID, convID, msg); err != nil {
				log.Printf("[sync] error upserting message %s: %v", msg.ExternalID, err)
			} else {
				totalMessages++
			}
		}

		// Update conversation message count
		var count int64
		db.DB.Model(&models.Message{}).Where("conversation_id = ?", convID).Count(&count)
		db.DB.Model(&models.Conversation{}).Where("id = ?", convID).Update("message_count", count)
	}

	log.Printf("[sync] channel %s: synced %d conversations, %d messages", channel.Name, len(conversations), totalMessages)

	// Log activity
	db.LogActivity(channel.TenantID, "", "system", "sync.completed", "channel", channel.ID,
		fmt.Sprintf("Sync '%s': %d conversations, %d messages", channel.Name, len(conversations), totalMessages), "", "")

	// Trigger after-sync jobs for this channel
	if sched := GetDefaultScheduler(); sched != nil {
		sched.TriggerAfterSyncJobs(channel.TenantID, channel.ID)
	}

	return s.updateSyncStatus(channel.ID, "success", "")
}

// luuFileDinhKem lưu một file, ưu tiên kho chính; kho chính hỏng thì ghi xuống
// đĩa để không mất tấm ảnh đó.
//
// Phải tải lại từ đầu cho lượt thử thứ hai vì luồng dữ liệu của lượt trước đã
// bị đọc mất. Đường này hiếm khi chạy nên tải lại một lần là chấp nhận được,
// đổi lại không bao giờ mất ảnh chỉ vì S3 chập chờn.
func luuFileDinhKem(ctx context.Context, chinh, duPhong storage.Store, key string,
	tai func() (io.ReadCloser, int64, string, error)) (noiDaLuu string, err error) {

	body, size, contentType, err := tai()
	if err != nil {
		return "", err
	}
	err = chinh.Put(ctx, key, body, size, contentType)
	body.Close()
	if err == nil {
		return chinh.Kind(), nil
	}
	if chinh == duPhong {
		return "", err
	}

	log.Printf("[sync] kho chính (%s) không nhận file %s: %v — ghi tạm xuống đĩa, chạy migrate-files -up để chuyển lên sau",
		chinh.Kind(), key, err)

	body2, size2, contentType2, err2 := tai()
	if err2 != nil {
		return "", fmt.Errorf("kho chính hỏng (%v) và tải lại cũng hỏng: %w", err, err2)
	}
	err2 = duPhong.Put(ctx, key, body2, size2, contentType2)
	body2.Close()
	if err2 != nil {
		return "", fmt.Errorf("kho chính hỏng (%v) và ghi xuống đĩa cũng hỏng: %w", err, err2)
	}
	return duPhong.Kind() + " (tạm)", nil
}

// SyncAllChannels syncs all active channels for a tenant.
func (s *SyncEngine) SyncAllChannels(ctx context.Context, tenantID string) error {
	var chans []models.Channel
	db.DB.Where("tenant_id = ? AND is_active = true", tenantID).Find(&chans)

	for _, ch := range chans {
		if err := s.SyncChannel(ctx, ch); err != nil {
			log.Printf("[sync] channel %s failed: %v", ch.Name, err)
		}
	}
	return nil
}

func (s *SyncEngine) upsertConversation(tenantID, channelID string, conv channels.SyncedConversation) (string, error) {
	var existing models.Conversation
	result := db.DB.Where("tenant_id = ? AND channel_id = ? AND external_conversation_id = ?",
		tenantID, channelID, conv.ExternalID).First(&existing)

	metadataJSON, _ := json.Marshal(conv.Metadata)

	if result.Error == nil {
		// Update existing
		db.DB.Model(&existing).Updates(map[string]interface{}{
			"customer_name":   conv.CustomerName,
			"last_message_at": conv.LastMessageAt,
			"metadata":        string(metadataJSON),
			"updated_at":      time.Now(),
		})
		return existing.ID, nil
	}

	// Create new
	newConv := models.Conversation{
		ID:                     pkg.NewUUID(),
		TenantID:               tenantID,
		ChannelID:              channelID,
		ExternalConversationID: conv.ExternalID,
		ExternalUserID:         conv.ExternalUserID,
		CustomerName:           conv.CustomerName,
		LastMessageAt:          &conv.LastMessageAt,
		MessageCount:           0,
		Metadata:               string(metadataJSON),
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
	}
	if err := db.DB.Create(&newConv).Error; err != nil {
		return "", err
	}
	return newConv.ID, nil
}

func (s *SyncEngine) upsertMessage(tenantID, conversationID string, msg channels.SyncedMessage) error {
	// Check if message already exists (dedup by external_message_id)
	var existing models.Message
	result := db.DB.Where("tenant_id = ? AND conversation_id = ? AND external_message_id = ?",
		tenantID, conversationID, msg.ExternalID).First(&existing)
	if result.Error == nil {
		// Message exists — update attachments if we have new local paths
		hasLocalPath := false
		for _, att := range msg.Attachments {
			if att.LocalPath != "" {
				hasLocalPath = true
				break
			}
		}
		if hasLocalPath {
			attachmentsJSON, _ := json.Marshal(msg.Attachments)
			db.DB.Model(&existing).Update("attachments", string(attachmentsJSON))
		}
		return nil
	}

	attachmentsJSON, _ := json.Marshal(msg.Attachments)
	rawDataJSON, _ := json.Marshal(msg.RawData)

	message := models.Message{
		ID:                pkg.NewUUID(),
		TenantID:          tenantID,
		ConversationID:    conversationID,
		ExternalMessageID: msg.ExternalID,
		SenderType:        msg.SenderType,
		SenderName:        msg.SenderName,
		Content:           msg.Content,
		ContentType:       msg.ContentType,
		Attachments:       string(attachmentsJSON),
		SentAt:            msg.SentAt,
		RawData:           string(rawDataJSON),
		CreatedAt:         time.Now(),
	}
	return db.DB.Create(&message).Error
}

func (s *SyncEngine) updateSyncStatus(channelID, status, errMsg string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"last_sync_at":     &now,
		"last_sync_status": status,
		"last_sync_error":  errMsg,
		"updated_at":       now,
	}
	db.DB.Model(&models.Channel{}).Where("id = ?", channelID).Updates(updates)
	if errMsg != "" {
		// Log error to activity logs
		var ch models.Channel
		if db.DB.Where("id = ?", channelID).First(&ch).Error == nil {
			db.LogActivity(ch.TenantID, "", "system", "sync.error", "channel", channelID, "Sync failed: "+ch.Name, errMsg, "")
		}
		return fmt.Errorf("sync failed: %s", errMsg)
	}
	return nil
}

// downloadAttachments tải file đính kèm về nơi cất file đang cấu hình — đĩa
// máy chủ hoặc S3. Khoá của file vẫn là "<tenant>/<cuộc chat>/<tên file>" như
// trước, nên đổi nơi cất không phải đụng vào dữ liệu đã lưu.
func (s *SyncEngine) downloadAttachments(tenantID, convID string, msg *channels.SyncedMessage) {
	// Kho trên đĩa luôn dựng sẵn làm lưới an toàn. Mất một tấm ảnh là mất hẳn —
	// link ảnh bên Zalo và Facebook hết hạn sau ít lâu, không tải lại được nữa —
	// nên S3 trục trặc thì thà ghi tạm xuống đĩa rồi chuyển lên sau, hơn là bỏ.
	duPhong, err := storage.NewLocal(s.cfg.StorageLocalDir)
	if err != nil {
		log.Printf("[sync] không dựng được kho trên đĩa: %v", err)
		return
	}

	// Mỗi công ty có kho riêng: công ty này để trên S3, công ty kia vẫn trên đĩa.
	store, err := storage.ForTenant(tenantID)
	if err != nil {
		log.Printf("[sync] không lấy được nơi cất file của công ty %s (%v) — tạm ghi xuống đĩa", tenantID, err)
		store = duPhong
	}

	for i, att := range msg.Attachments {
		if att.URL == "" {
			continue
		}

		// Tên file đến từ API bên ngoài nên không được tin: chỉ lấy phần tên,
		// bỏ mọi thành phần đường dẫn.
		name := filepath.Base(att.Name)
		if name == "" || name == "." || name == "/" || strings.Contains(name, "..") {
			name = fmt.Sprintf("%s-%d", att.Type, time.Now().UnixMilli())
		}
		key := path.Join(tenantID, convID, name)

		tai := func() (io.ReadCloser, int64, string, error) {
			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Get(att.URL)
			if err != nil {
				return nil, 0, "", err
			}
			if resp.StatusCode != 200 {
				resp.Body.Close()
				return nil, 0, "", fmt.Errorf("status %d", resp.StatusCode)
			}
			// ContentLength là -1 khi máy chủ không báo độ dài; nơi cất file
			// hiểu -1 là "chưa biết trước".
			return resp.Body, resp.ContentLength, resp.Header.Get("Content-Type"), nil
		}

		noiDaLuu, err := luuFileDinhKem(context.Background(), store, duPhong, key, tai)
		if err != nil {
			log.Printf("[sync] lưu file %s hỏng: %v", key, err)
			continue
		}

		// Trường LocalPath giữ nguyên tên cũ để không phải đổi dữ liệu đã lưu,
		// nay mang nghĩa khoá của file trong nơi cất.
		msg.Attachments[i].LocalPath = key
		log.Printf("[sync] downloaded %s → %s (%s)", att.URL, key, noiDaLuu)
	}
}
