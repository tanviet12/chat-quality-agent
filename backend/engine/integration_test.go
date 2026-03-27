// +build integration

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/nmtan2001/chat-quality-agent/ai"
	"github.com/nmtan2001/chat-quality-agent/config"
	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/pkg"
)

// SmartMockProvider analyzes the transcript and returns appropriate PASS/FAIL.
type SmartMockProvider struct{}

func (m *SmartMockProvider) AnalyzeChat(ctx context.Context, systemPrompt string, chatTranscript string) (ai.AIResponse, error) {
	// Simple heuristic: if transcript contains rude words → FAIL
	isRude := false
	rudeWords := []string{"Gi?", "Tu xem", "Khong biet", "De do"}
	for _, w := range rudeWords {
		if containsStr(chatTranscript, w) {
			isRude = true
			break
		}
	}

	var resp map[string]interface{}
	if isRude {
		resp = map[string]interface{}{
			"verdict": "FAIL",
			"score":   25,
			"review":  "Nhan vien tra loi coc loc, khong lich su.",
			"violations": []map[string]interface{}{
				{
					"severity":    "NGHIEM_TRONG",
					"rule":        "Chao hoi lich su",
					"evidence":    "NV: Gi?",
					"explanation": "Nhan vien khong chao hoi, tra loi thieu ton trong.",
					"suggestion":  "Nen bat dau bang loi chao than thien.",
				},
			},
			"summary": "Cuoc chat can cai thien nghiem tuc.",
		}
	} else {
		resp = map[string]interface{}{
			"verdict":    "PASS",
			"score":      90,
			"review":     "Nhan vien lich su, ho tro tot.",
			"violations": []interface{}{},
			"summary":    "Cuoc chat dat chuan.",
		}
	}

	respJSON, _ := json.Marshal(resp)
	return ai.AIResponse{
		Content:      string(respJSON),
		InputTokens:  200,
		OutputTokens: 100,
		Model:        "mock-model",
		Provider:     "mock",
	}, nil
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && findSubstr(s, substr)
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestIntegrationFullJobFlow(t *testing.T) {
	// Connect to test DB (uses env vars or defaults to Docker container)
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "cqa:cqa_password@tcp(127.0.0.1:3306)/cqa?charset=utf8mb4&parseTime=True&loc=UTC"
	}

	if err := db.Connect(dsn, false); err != nil {
		t.Skipf("Skipping integration test - DB not available: %v", err)
		return
	}
	defer db.Close()

	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	tenantID := "inttest-" + pkg.NewUUID()[:8]
	channelID := "ch-inttest-" + pkg.NewUUID()[:8]

	// Setup: Use raw SQL to avoid JSON validation issues with GORM
	now := time.Now()
	convBadID := "conv-int-bad-" + tenantID[:8]
	convGoodID := "conv-int-good-" + tenantID[:8]
	jobID := "job-inttest-" + tenantID[:8]
	channelIDsJSON, _ := json.Marshal([]string{channelID})

	// Insert via raw SQL to handle JSON columns properly
	db.DB.Exec(`INSERT INTO tenants (id, name, slug, settings, created_at, updated_at) VALUES (?, ?, ?, '{}', NOW(), NOW())`,
		tenantID, "Integration Test", "inttest-"+tenantID[:8])
	defer db.DB.Exec("DELETE FROM tenants WHERE id = ?", tenantID)

	db.DB.Exec(`INSERT INTO channels (id, tenant_id, channel_type, name, external_id, credentials_encrypted, is_active, metadata, created_at, updated_at) VALUES (?, ?, 'facebook', 'Test Channel', 'fake', X'00', true, '{}', NOW(), NOW())`,
		channelID, tenantID)
	defer db.DB.Exec("DELETE FROM channels WHERE id = ?", channelID)

	db.DB.Exec(`INSERT INTO conversations (id, tenant_id, channel_id, external_conversation_id, customer_name, last_message_at, message_count, metadata, created_at, updated_at) VALUES (?, ?, ?, 'ext-bad', 'Khach Xau', ?, 2, '{}', NOW(), NOW())`,
		convBadID, tenantID, channelID, now)
	db.DB.Exec(`INSERT INTO conversations (id, tenant_id, channel_id, external_conversation_id, customer_name, last_message_at, message_count, metadata, created_at, updated_at) VALUES (?, ?, ?, 'ext-good', 'Khach Tot', ?, 2, '{}', NOW(), NOW())`,
		convGoodID, tenantID, channelID, now)
	defer db.DB.Exec("DELETE FROM conversations WHERE tenant_id = ?", tenantID)

	// Messages
	db.DB.Exec(`INSERT INTO messages (id, tenant_id, conversation_id, external_message_id, sender_type, sender_name, content, sent_at, created_at) VALUES (?, ?, ?, 'm1', 'customer', 'Khach', 'Xin chao', ?, NOW())`,
		pkg.NewUUID(), tenantID, convBadID, now.Add(-5*time.Minute))
	db.DB.Exec(`INSERT INTO messages (id, tenant_id, conversation_id, external_message_id, sender_type, sender_name, content, sent_at, created_at) VALUES (?, ?, ?, 'm2', 'agent', 'NV', 'Gi? Tu xem tren web di', ?, NOW())`,
		pkg.NewUUID(), tenantID, convBadID, now.Add(-3*time.Minute))
	db.DB.Exec(`INSERT INTO messages (id, tenant_id, conversation_id, external_message_id, sender_type, sender_name, content, sent_at, created_at) VALUES (?, ?, ?, 'm3', 'customer', 'Khach', 'Chao ban', ?, NOW())`,
		pkg.NewUUID(), tenantID, convGoodID, now.Add(-5*time.Minute))
	db.DB.Exec(`INSERT INTO messages (id, tenant_id, conversation_id, external_message_id, sender_type, sender_name, content, sent_at, created_at) VALUES (?, ?, ?, 'm4', 'agent', 'NV', 'Xin chao! Em rat vui duoc ho tro. Anh can gi a?', ?, NOW())`,
		pkg.NewUUID(), tenantID, convGoodID, now.Add(-3*time.Minute))
	defer db.DB.Exec("DELETE FROM messages WHERE tenant_id = ?", tenantID)

	// Create job
	db.DB.Exec(`INSERT INTO jobs (id, tenant_id, name, job_type, input_channel_ids, rules_content, rules_config, schedule_type, is_active, outputs, created_at, updated_at) VALUES (?, ?, 'QC Test Job', 'qc_analysis', ?, 'Nhan vien phai chao hoi lich su, tra loi day du.', '[]', 'manual', true, '[]', NOW(), NOW())`,
		jobID, tenantID, string(channelIDsJSON))

	// Load job from DB to get proper model
	var job models.Job
	db.DB.First(&job, "id = ?", jobID)
	defer db.DB.Exec("DELETE FROM jobs WHERE id = ?", jobID)

	// Run with mock provider
	cfg := &config.Config{}
	analyzer := NewAnalyzer(cfg)
	mockProvider := &SmartMockProvider{}

	run, err := analyzer.RunJobWithProvider(context.Background(), job, 3, mockProvider)
	if err != nil {
		t.Fatalf("RunJobWithProvider failed: %v", err)
	}

	// Verify run completed
	if run.Status != "success" {
		t.Errorf("expected status 'success', got '%s' (error: %s)", run.Status, run.ErrorMessage)
	}

	// Parse summary
	var summary map[string]interface{}
	json.Unmarshal([]byte(run.Summary), &summary)
	log.Printf("Run summary: %s", run.Summary)

	analyzed := int(summary["conversations_analyzed"].(float64))
	passed := int(summary["conversations_passed"].(float64))
	issues := int(summary["issues_found"].(float64))

	if analyzed != 2 {
		t.Errorf("expected 2 conversations analyzed, got %d", analyzed)
	}
	if passed != 1 {
		t.Errorf("expected 1 passed (good conversation), got %d", passed)
	}
	if issues < 1 {
		t.Errorf("expected at least 1 issue (bad conversation), got %d", issues)
	}

	// Verify job_results in DB
	var results []models.JobResult
	db.DB.Where("job_run_id = ?", run.ID).Find(&results)

	evalCount := 0
	violationCount := 0
	for _, r := range results {
		switch r.ResultType {
		case "conversation_evaluation":
			evalCount++
		case "qc_violation":
			violationCount++
		}
	}

	if evalCount != 2 {
		t.Errorf("expected 2 conversation_evaluation records, got %d", evalCount)
	}
	if violationCount < 1 {
		t.Errorf("expected at least 1 qc_violation record, got %d", violationCount)
	}

	// Verify AI usage log
	var usageLogs []models.AIUsageLog
	db.DB.Where("job_run_id = ?", run.ID).Find(&usageLogs)
	if len(usageLogs) != 2 {
		t.Errorf("expected 2 usage logs (1 per conversation), got %d", len(usageLogs))
	}
	for _, u := range usageLogs {
		if u.InputTokens != 200 || u.OutputTokens != 100 {
			t.Errorf("unexpected token counts: input=%d output=%d", u.InputTokens, u.OutputTokens)
		}
	}

	// Cleanup
	db.DB.Where("job_run_id = ?", run.ID).Delete(&models.JobResult{})
	db.DB.Where("job_run_id = ?", run.ID).Delete(&models.AIUsageLog{})
	db.DB.Where("id = ?", run.ID).Delete(&models.JobRun{})

	fmt.Printf("\n✅ Integration test PASSED:\n")
	fmt.Printf("   Conversations analyzed: %d\n", analyzed)
	fmt.Printf("   Passed: %d, Failed: %d\n", passed, analyzed-passed)
	fmt.Printf("   Violations found: %d\n", violationCount)
	fmt.Printf("   Usage logs: %d\n", len(usageLogs))
}
