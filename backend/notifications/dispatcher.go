package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nmtan2001/chat-quality-agent/db"
	"github.com/nmtan2001/chat-quality-agent/db/models"
	"github.com/nmtan2001/chat-quality-agent/pkg"
)

// OutputConfig represents a single output destination from job.outputs JSON.
type OutputConfig struct {
	Type           string `json:"type"`            // "telegram" | "email"
	BotToken       string `json:"bot_token"`       // telegram
	ChatID         string `json:"chat_id"`         // telegram
	SMTPHost       string `json:"smtp_host"`       // email
	SMTPPort       int    `json:"smtp_port"`       // email
	SMTPUser       string `json:"smtp_user"`       // email
	SMTPPass       string `json:"smtp_pass"`       // email
	From           string `json:"from"`            // email
	To             string `json:"to"`              // email (comma-separated)
	Template       string `json:"template"`        // "default" | "custom"
	CustomTemplate string `json:"custom_template"` // user-defined template (HTML)
}

// Dispatcher sends notifications for job results and logs every send.
type Dispatcher struct{}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{}
}

// SendJobResults sends notifications for a job run based on the job's output config.
func (d *Dispatcher) SendJobResults(ctx context.Context, job models.Job, run models.JobRun) error {
	var outputs []OutputConfig
	if err := json.Unmarshal([]byte(job.Outputs), &outputs); err != nil {
		// Fallback: outputs might be double-encoded (string wrapping JSON array)
		var raw string
		if err2 := json.Unmarshal([]byte(job.Outputs), &raw); err2 == nil {
			if err3 := json.Unmarshal([]byte(raw), &outputs); err3 != nil {
				return fmt.Errorf("invalid outputs config: %w", err)
			}
		} else {
			return fmt.Errorf("invalid outputs config: %w", err)
		}
	}

	// Get unnotified results for this run
	var results []models.JobResult
	db.DB.Where("job_run_id = ? AND notified_at IS NULL", run.ID).Find(&results)

	if len(results) == 0 {
		return nil
	}

	// Count pass/fail from run summary
	var summary map[string]interface{}
	json.Unmarshal([]byte(run.Summary), &summary)
	total := int(getFloat(summary, "conversations_analyzed"))
	passed := int(getFloat(summary, "conversations_passed"))
	failed := total - passed
	issues := int(getFloat(summary, "issues_found"))

	subject := fmt.Sprintf("[CQA] %s - %d issues found", job.Name, issues)

	for _, output := range outputs {
		notifier, err := d.createNotifier(output)
		if err != nil {
			log.Printf("[dispatcher] create notifier failed for %s: %v", output.Type, err)
			continue
		}

		// Build default body per output type (telegram uses plain link, email uses HTML <a>)
		defaultBody := d.buildNotificationBody(job, results, output.Type)

		// Use custom template if configured
		body := defaultBody
		link := fmt.Sprintf("%s/%s/jobs/%s", d.getBaseURL(), job.TenantID, job.ID)
		if output.Template == "custom" && output.CustomTemplate != "" {
			body = d.renderCustomTemplate(output.CustomTemplate, job.Name, total, passed, failed, issues, defaultBody, link)
		}

		sendErr := notifier.Send(ctx, subject, body)
		status := "sent"
		errMsg := ""
		if sendErr != nil {
			status = "failed"
			errMsg = sendErr.Error()
			log.Printf("[dispatcher] send failed for %s: %v", output.Type, sendErr)
		}

		// Log notification
		recipient := output.ChatID
		if output.Type == "email" {
			recipient = output.To
		}
		logEntry := models.NotificationLog{
			ID:           pkg.NewUUID(),
			TenantID:     job.TenantID,
			JobID:        job.ID,
			JobRunID:     run.ID,
			ChannelType:  output.Type,
			Recipient:    recipient,
			Subject:      subject,
			Body:         body,
			Status:       status,
			ErrorMessage: errMsg,
			SentAt:       time.Now(),
			CreatedAt:    time.Now(),
		}
		db.DB.Create(&logEntry)
	}

	// Mark results as notified
	now := time.Now()
	db.DB.Model(&models.JobResult{}).Where("job_run_id = ? AND notified_at IS NULL", run.ID).
		Update("notified_at", &now)

	return nil
}

func (d *Dispatcher) createNotifier(cfg OutputConfig) (Notifier, error) {
	switch cfg.Type {
	case "telegram":
		return NewTelegramNotifier(cfg.BotToken, cfg.ChatID), nil
	case "email":
		return NewEmailNotifier(
			cfg.SMTPHost, cfg.SMTPPort,
			cfg.SMTPUser, cfg.SMTPPass,
			cfg.From, splitComma(cfg.To),
		), nil
	default:
		return nil, fmt.Errorf("unsupported output type: %s", cfg.Type)
	}
}

func (d *Dispatcher) buildNotificationBody(job models.Job, results []models.JobResult, outputType string) string {
	body := fmt.Sprintf("<b>Kết quả phân tích: %s</b>\n\n", job.Name)

	for i, r := range results {
		if i >= 10 {
			body += fmt.Sprintf("\n... và %d vấn đề khác\n", len(results)-10)
			break
		}
		switch r.ResultType {
		case "qc_violation":
			emoji := "⚠️"
			if r.Severity == "NGHIEM_TRONG" {
				emoji = "🔴"
			}
			body += fmt.Sprintf("%s <b>%s</b> — %s\n📌 %s\n\n", emoji, r.Severity, r.RuleName, r.Evidence)
		case "classification_tag":
			body += fmt.Sprintf("🏷 <b>%s</b> (%.0f%%)\n📌 %s\n\n", r.RuleName, r.Confidence*100, r.Evidence)
		}
	}

	return body
}

func splitComma(s string) []string {
	var result []string
	for _, p := range splitBy(s, ',') {
		p = trimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func splitBy(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func (d *Dispatcher) getBaseURL() string {
	if u := os.Getenv("APP_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

func (d *Dispatcher) renderCustomTemplate(tmpl, jobName string, total, passed, failed, issues int, content, link string) string {
	result := tmpl
	replacements := map[string]string{
		"{{job_name}}": jobName,
		"{{total}}":    fmt.Sprintf("%d", total),
		"{{passed}}":   fmt.Sprintf("%d", passed),
		"{{failed}}":   fmt.Sprintf("%d", failed),
		"{{issues}}":   fmt.Sprintf("%d", issues),
		"{{content}}":  content,
		"{{link}}":     link,
	}
	for k, v := range replacements {
		for {
			idx := -1
			for i := 0; i <= len(result)-len(k); i++ {
				if result[i:i+len(k)] == k {
					idx = i
					break
				}
			}
			if idx < 0 {
				break
			}
			result = result[:idx] + v + result[idx+len(k):]
		}
	}
	return result
}

func getFloat(m map[string]interface{}, key string) float64 {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok {
		return 0
	}
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	return f
}
