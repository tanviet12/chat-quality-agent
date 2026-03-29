package ai

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	maxRetries     = 3
	initialBackoff = 5 * time.Second
)

// retryableError checks if an error should be retried (rate limit, server error, network).
func retryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// Rate limit
	if strings.Contains(msg, "429") || strings.Contains(msg, "rate") || strings.Contains(msg, "Rate") ||
		strings.Contains(msg, "RESOURCE_EXHAUSTED") || strings.Contains(msg, "quota") {
		return true
	}
	// Server errors
	if strings.Contains(msg, "500") || strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") || strings.Contains(msg, "529") {
		return true
	}
	// Network errors
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "connection") ||
		strings.Contains(msg, "EOF") || strings.Contains(msg, "reset") {
		return true
	}
	return false
}

// withRetry wraps an AI call with exponential backoff retry for transient errors.
func withRetry(ctx context.Context, provider string, fn func() (AIResponse, error)) (AIResponse, error) {
	var lastErr error
	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[%s] retry attempt %d/%d after error: %v (backoff: %v)", provider, attempt, maxRetries, lastErr, backoff)
			select {
			case <-ctx.Done():
				return AIResponse{}, fmt.Errorf("%s retry cancelled: %w", provider, ctx.Err())
			case <-time.After(backoff):
			}
			backoff *= 3 // exponential: 5s → 15s → 45s
		}

		resp, err := fn()
		if err == nil {
			return resp, nil
		}
		lastErr = err

		if !retryableError(err) {
			return AIResponse{}, err // non-retryable, fail immediately
		}
	}

	return AIResponse{}, fmt.Errorf("%s failed after %d retries: %w", provider, maxRetries, lastErr)
}

// NewHTTPClientWithTimeout creates an HTTP client with explicit timeout per go-safety rules.
func NewHTTPClientWithTimeout() *http.Client {
	return &http.Client{Timeout: 2 * time.Minute}
}
