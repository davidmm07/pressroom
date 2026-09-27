package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/davidmm07/pressroom/internal/domain"
)

// Slack posts to an incoming webhook. With no URL configured it reports that
// nothing was sent instead of failing, so local runs keep working.
type Slack struct {
	WebhookURL string
	Client     *http.Client
}

// Post sends a message.
func (s Slack) Post(ctx context.Context, text string) (bool, error) {
	if s.WebhookURL == "" {
		return false, nil
	}
	payload, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("slack: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("slack: %s", resp.Status)
	}
	return true, nil
}

// NotifyTeam lets an agent flag something for a human channel.
func NotifyTeam(s Slack) Tool {
	return Func(domain.ToolSpec{
		Name:        "notify_team",
		Description: "Post a short message to the owning team's Slack channel, for problems a human should look at today.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {
		    "message":  {"type": "string", "minLength": 10, "maxLength": 1500},
		    "severity": {"type": "string", "enum": ["INFO","WARNING","URGENT"], "default": "INFO"}
		  },
		  "required": ["message"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in struct {
		Message  string `json:"message"`
		Severity string `json:"severity"`
	}) (any, error) {
		prefix := map[string]string{"WARNING": ":warning: ", "URGENT": ":rotating_light: "}[in.Severity]
		sent, err := s.Post(ctx, prefix+in.Message)
		if err != nil {
			return nil, err
		}
		return map[string]any{"delivered": sent}, nil
	})
}
