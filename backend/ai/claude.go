package ai

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

var claudeHTTPClient = NewHTTPClientWithTimeout()

type ClaudeProvider struct {
	apiKey    string
	model     string
	maxTokens int
	baseURL   string
}

func NewClaudeProvider(apiKey, model string, maxTokens int, baseURL string) *ClaudeProvider {
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	if maxTokens <= 0 {
		maxTokens = 16384
	}
	return &ClaudeProvider{
		apiKey:    apiKey,
		model:     model,
		maxTokens: maxTokens,
		baseURL:   baseURL,
	}
}

func (c *ClaudeProvider) AnalyzeChat(ctx context.Context, systemPrompt string, chatTranscript string) (AIResponse, error) {
	return withRetry(ctx, "claude", func() (AIResponse, error) {
		opts := []option.RequestOption{
			option.WithAPIKey(c.apiKey),
			option.WithHTTPClient(claudeHTTPClient),
		}
		if c.baseURL != "" {
			opts = append(opts, option.WithBaseURL(c.baseURL))
		}
		client := anthropic.NewClient(opts...)

		message, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(c.model),
			MaxTokens: int64(c.maxTokens),
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(chatTranscript)),
			},
		})
		if err != nil {
			return AIResponse{}, fmt.Errorf("claude api error: %w", err)
		}

		// Extract text from response content blocks
		var text string
		for _, block := range message.Content {
			if block.Type == "text" {
				text = block.Text
				break
			}
		}
		if text == "" {
			return AIResponse{}, fmt.Errorf("claude api returned empty content")
		}

		return AIResponse{
			Content:      text,
			InputTokens:  int(message.Usage.InputTokens),
			OutputTokens: int(message.Usage.OutputTokens),
			Model:        string(message.Model),
			Provider:     "claude",
		}, nil
	})
}

func (c *ClaudeProvider) AnalyzeChatBatch(ctx context.Context, systemPrompt string, items []BatchItem) (AIResponse, error) {
	batchPrompt := WrapBatchPrompt(systemPrompt, len(items))
	batchTranscript := FormatBatchTranscript(items)
	return c.AnalyzeChat(ctx, batchPrompt, batchTranscript)
}
