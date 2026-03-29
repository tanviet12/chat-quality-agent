package ai

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

type GeminiProvider struct {
	apiKey  string
	model   string
	baseURL string
}

func NewGeminiProvider(apiKey, model string, baseURL string) *GeminiProvider {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &GeminiProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
	}
}

func (g *GeminiProvider) AnalyzeChat(ctx context.Context, systemPrompt string, chatTranscript string) (AIResponse, error) {
	return withRetry(ctx, "gemini", func() (AIResponse, error) {
		clientCfg := &genai.ClientConfig{
			APIKey:     g.apiKey,
			Backend:    genai.BackendGeminiAPI,
			HTTPClient: NewHTTPClientWithTimeout(),
		}
		if g.baseURL != "" {
			clientCfg.HTTPOptions = genai.HTTPOptions{
				BaseURL: g.baseURL,
			}
		}
		client, err := genai.NewClient(ctx, clientCfg)
		if err != nil {
			return AIResponse{}, fmt.Errorf("gemini client error: %w", err)
		}

		result, err := client.Models.GenerateContent(ctx, g.model, genai.Text(chatTranscript), &genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(systemPrompt, "user"),
		})
		if err != nil {
			return AIResponse{}, fmt.Errorf("gemini api error: %w", err)
		}

		text := result.Text()
		if text == "" {
			return AIResponse{}, fmt.Errorf("gemini api returned empty content")
		}

		aiResp := AIResponse{
			Content:  text,
			Model:    g.model,
			Provider: "gemini",
		}
		if result.UsageMetadata != nil {
			aiResp.InputTokens = int(result.UsageMetadata.PromptTokenCount)
			aiResp.OutputTokens = int(result.UsageMetadata.CandidatesTokenCount)
		}

		return aiResp, nil
	})
}

func (g *GeminiProvider) AnalyzeChatBatch(ctx context.Context, systemPrompt string, items []BatchItem) (AIResponse, error) {
	batchPrompt := WrapBatchPrompt(systemPrompt, len(items))
	batchTranscript := FormatBatchTranscript(items)
	return g.AnalyzeChat(ctx, batchPrompt, batchTranscript)
}
