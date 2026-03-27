package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nmtan2001/chat-quality-agent/ai"
)

// MockAIProvider returns a predefined response without calling any API.
type MockAIProvider struct {
	Response ai.AIResponse
	Err      error
}

func (m *MockAIProvider) AnalyzeChat(ctx context.Context, systemPrompt string, chatTranscript string) (ai.AIResponse, error) {
	if m.Err != nil {
		return ai.AIResponse{}, m.Err
	}
	return m.Response, nil
}

func TestMockAIProviderQCPass(t *testing.T) {
	passResponse := map[string]interface{}{
		"verdict": "PASS",
		"score":   90,
		"review":  "Cuộc chat tốt, nhân viên lịch sự và giải đáp đầy đủ.",
		"violations": []interface{}{},
		"summary": "Khách hàng hỏi về sản phẩm, nhân viên trả lời chi tiết.",
	}
	respJSON, _ := json.Marshal(passResponse)

	mock := &MockAIProvider{
		Response: ai.AIResponse{
			Content:      string(respJSON),
			InputTokens:  150,
			OutputTokens: 80,
			Model:        "claude-sonnet-4-6",
			Provider:     "claude",
		},
	}

	resp, err := mock.AnalyzeChat(context.Background(), "test prompt", "test transcript")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.InputTokens != 150 {
		t.Errorf("expected 150 input tokens, got %d", resp.InputTokens)
	}
	if resp.OutputTokens != 80 {
		t.Errorf("expected 80 output tokens, got %d", resp.OutputTokens)
	}
	if resp.Provider != "claude" {
		t.Errorf("expected provider claude, got %s", resp.Provider)
	}

	// Verify we can parse the response
	var qcResult struct {
		Verdict    string        `json:"verdict"`
		Score      int           `json:"score"`
		Violations []interface{} `json:"violations"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &qcResult); err != nil {
		t.Fatalf("failed to parse QC response: %v", err)
	}
	if qcResult.Verdict != "PASS" {
		t.Errorf("expected PASS, got %s", qcResult.Verdict)
	}
	if qcResult.Score != 90 {
		t.Errorf("expected score 90, got %d", qcResult.Score)
	}
}

func TestMockAIProviderQCFail(t *testing.T) {
	failResponse := map[string]interface{}{
		"verdict": "FAIL",
		"score":   30,
		"review":  "Nhân viên không chào hỏi, trả lời cộc lốc.",
		"violations": []map[string]interface{}{
			{
				"severity":    "NGHIEM_TRONG",
				"rule":        "Chào hỏi lịch sự",
				"evidence":    "Khách: Xin chào. NV: Gì?",
				"explanation": "Nhân viên không chào hỏi lại, trả lời thô lỗ.",
				"suggestion":  "Nên bắt đầu bằng lời chào thân thiện.",
			},
		},
		"summary": "Cuộc chat cần cải thiện.",
	}
	respJSON, _ := json.Marshal(failResponse)

	mock := &MockAIProvider{
		Response: ai.AIResponse{
			Content:      string(respJSON),
			InputTokens:  200,
			OutputTokens: 120,
			Model:        "gemini-2.0-flash",
			Provider:     "gemini",
		},
	}

	resp, err := mock.AnalyzeChat(context.Background(), "test prompt", "test transcript")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var qcResult struct {
		Verdict    string `json:"verdict"`
		Violations []struct {
			Severity string `json:"severity"`
			Rule     string `json:"rule"`
		} `json:"violations"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &qcResult); err != nil {
		t.Fatalf("failed to parse QC response: %v", err)
	}
	if qcResult.Verdict != "FAIL" {
		t.Errorf("expected FAIL, got %s", qcResult.Verdict)
	}
	if len(qcResult.Violations) != 1 {
		t.Errorf("expected 1 violation, got %d", len(qcResult.Violations))
	}
	if qcResult.Violations[0].Severity != "NGHIEM_TRONG" {
		t.Errorf("expected NGHIEM_TRONG, got %s", qcResult.Violations[0].Severity)
	}
}

func TestCalculateCostUSD(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		input    int
		output   int
		minCost  float64
		maxCost  float64
	}{
		{"claude sonnet small", "claude", "claude-sonnet-4-6", 1000, 500, 0.01, 0.02},
		{"claude haiku cheap", "claude", "claude-haiku-3-5", 1000, 500, 0.001, 0.005},
		{"gemini flash very cheap", "gemini", "gemini-2.0-flash", 1000, 500, 0.0001, 0.001},
		{"zero tokens", "claude", "claude-sonnet-4-6", 0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := ai.CalculateCostUSD(tt.provider, tt.model, tt.input, tt.output)
			if cost < tt.minCost || cost > tt.maxCost {
				t.Errorf("cost %f out of range [%f, %f]", cost, tt.minCost, tt.maxCost)
			}
		})
	}
}
