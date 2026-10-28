package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vietbui/chat-quality-agent/channels"
	"github.com/vietbui/chat-quality-agent/config"
	"github.com/vietbui/chat-quality-agent/db/models"
)

// stubAdapter là adapter giả, không gọi mạng thật.
type stubAdapter struct {
	healthErr error
}

func (s *stubAdapter) FetchRecentConversations(ctx context.Context, since time.Time, limit int) ([]channels.SyncedConversation, error) {
	return nil, nil
}

func (s *stubAdapter) FetchMessages(ctx context.Context, conversationID string, since time.Time) ([]channels.SyncedMessage, error) {
	return nil, nil
}

func (s *stubAdapter) HealthCheck(ctx context.Context) error {
	return s.healthErr
}

func withStubAdapter(t *testing.T, healthErr error) {
	t.Helper()
	old := newChannelAdapter
	newChannelAdapter = func(channelType string, credentialsJSON []byte) (channels.ChannelAdapter, error) {
		return &stubAdapter{healthErr: healthErr}, nil
	}
	t.Cleanup(func() { newChannelAdapter = old })
}

// auditChannel là kênh giả, chỉ dùng id và loại kênh.
func auditChannel(channelType string) models.Channel {
	return models.Channel{ID: "chan-000000000000", TenantID: "tenant-000000000000", ChannelType: channelType}
}

func auditConfig() *config.Config {
	return &config.Config{EncryptionKey: "0123456789abcdef0123456789abcdef"}
}

func TestCheckChannelHealth(t *testing.T) {
	withStubAdapter(t, nil)
	err := checkChannelHealth(context.Background(), auditChannel("facebook"),
		[]byte(`{"page_id":"page-000000000000","access_token":"fake-token-audit"}`), auditConfig())
	if err != nil {
		t.Fatalf("kênh khoẻ mà báo lỗi: %v", err)
	}
}

func TestCheckChannelHealthBaoLoiKhiTokenHetHan(t *testing.T) {
	withStubAdapter(t, errors.New("facebook api error: (#190) Invalid OAuth token"))
	err := checkChannelHealth(context.Background(), auditChannel("facebook"),
		[]byte(`{"page_id":"page-000000000000","access_token":"fake-token-audit"}`), auditConfig())
	if err == nil {
		t.Fatal("token hết hạn mà không báo lỗi")
	}
}

func TestCheckChannelHealthBaoLoiKhiKhongTaoDuocAdapter(t *testing.T) {
	old := newChannelAdapter
	newChannelAdapter = func(channelType string, credentialsJSON []byte) (channels.ChannelAdapter, error) {
		return nil, errors.New("unsupported channel type: " + channelType)
	}
	t.Cleanup(func() { newChannelAdapter = old })

	err := checkChannelHealth(context.Background(), auditChannel("khong_co"), []byte(`{}`), auditConfig())
	if err == nil {
		t.Fatal("không tạo được adapter mà không báo lỗi")
	}
}
