package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// OpenAICompatConfig configures an endpoint that speaks the Chat Completions
// protocol. One adapter covers three providers: OpenAI itself, xAI's Grok
// (https://api.x.ai/v1) and self-hosted open-weights models behind vLLM or
// Ollama. Because the adapter targets the protocol rather than a vendor SDK,
// it is written against net/http directly.
type OpenAICompatConfig struct {
	Provider domain.Provider
	BaseURL  string
	APIKey   string
	// MaxTokensField is "max_completion_tokens" for OpenAI reasoning models
	// and "max_tokens" for servers that predate it.
	MaxTokensField string
	HTTPClient     *http.Client
}

// OpenAICompat is the Chat Completions adapter.
type OpenAICompat struct {
	cfg OpenAICompatConfig
}

var _ port.LanguageModel = (*OpenAICompat)(nil)

func NewOpenAICompat(cfg OpenAICompatConfig) *OpenAICompat {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 5 * time.Minute}
	}
	if cfg.MaxTokensField == "" {
		cfg.MaxTokensField = "max_completion_tokens"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &OpenAICompat{cfg: cfg}
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content"` // string, or null on tool-only assistant turns
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   *string        `json:"content"`
			Refusal   *string        `json:"refusal"`
			ToolCalls []chatToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

// Complete sends one turn to the endpoint.
func (o *OpenAICompat) Complete(ctx context.Context, req port.CompletionRequest) (*port.Completion, error) {
	body := map[string]any{
		"model":              req.Model.Name,
		"messages":           toChatMessages(req.System, req.Messages),
		o.cfg.MaxTokensField: req.MaxOutputTokens,
	}
	if len(req.Tools) > 0 {
		tools := make([]chatTool, len(req.Tools))
		for i, s := range req.Tools {
			tools[i] = chatTool{Type: "function", Function: chatFunction{Name: s.Name, Description: s.Description, Parameters: s.InputSchema}}
		}
		body["tools"] = tools
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.cfg.APIKey)
	}

	resp, err := o.cfg.HTTPClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, port.Transient(fmt.Errorf("%s: %w", o.name(), err), 0)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, port.Transient(fmt.Errorf("%s: read response: %w", o.name(), err), 0)
	}
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("%s: %s: %s", o.name(), resp.Status, snippet(raw))
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode >= 500 {
			return nil, port.Transient(err, retryAfter(resp))
		}
		return nil, err
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("%s: decode response: %w", o.name(), err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("%s: response has no choices", o.name())
	}
	return o.toCompletion(parsed, req.Model.Name), nil
}

func (o *OpenAICompat) toCompletion(r chatResponse, requested string) *port.Completion {
	choice := r.Choices[0]
	msg := domain.Message{Role: domain.RoleAssistant}
	if choice.Message.Content != nil {
		msg.Text = *choice.Message.Content
	}
	for _, tc := range choice.Message.ToolCalls {
		args := json.RawMessage(tc.Function.Arguments)
		if !json.Valid(args) {
			// Keep the malformed text so schema validation reports it to the model.
			args, _ = json.Marshal(tc.Function.Arguments)
		}
		msg.ToolCalls = append(msg.ToolCalls, domain.ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: args})
	}

	cached := r.Usage.PromptTokensDetails.CachedTokens
	c := &port.Completion{
		Message: msg,
		Usage: domain.Usage{
			InputTokens: r.Usage.PromptTokens - cached, CachedInputTokens: cached, OutputTokens: r.Usage.CompletionTokens,
		},
		StopReason: port.StopEndTurn,
	}
	if r.Model != "" && !strings.HasPrefix(r.Model, requested) {
		c.ServedBy = r.Model
	}
	switch {
	case choice.Message.Refusal != nil && *choice.Message.Refusal != "":
		c.StopReason, c.Detail = port.StopRefusal, *choice.Message.Refusal
	case choice.FinishReason == "content_filter":
		c.StopReason, c.Detail = port.StopRefusal, "content_filter"
	case choice.FinishReason == "tool_calls" || len(msg.ToolCalls) > 0:
		c.StopReason = port.StopToolUse
	case choice.FinishReason == "length":
		c.StopReason = port.StopMaxTokens
	}
	return c
}

func toChatMessages(system string, msgs []domain.Message) []chatMessage {
	out := make([]chatMessage, 0, len(msgs)+1)
	out = append(out, chatMessage{Role: "system", Content: system})
	for _, m := range msgs {
		switch m.Role {
		case domain.RoleUser:
			// Chat Completions wants one "tool" message per result.
			for _, r := range m.ToolResults {
				content := r.Content
				if r.IsError {
					content = "ERROR: " + content
				}
				out = append(out, chatMessage{Role: "tool", ToolCallID: r.CallID, Content: content})
			}
			if m.Text != "" {
				out = append(out, chatMessage{Role: "user", Content: m.Text})
			}
		case domain.RoleAssistant:
			cm := chatMessage{Role: "assistant"}
			if m.Text != "" {
				cm.Content = m.Text
			}
			for _, c := range m.ToolCalls {
				tc := chatToolCall{ID: c.ID, Type: "function"}
				tc.Function.Name, tc.Function.Arguments = c.Name, string(c.Arguments)
				cm.ToolCalls = append(cm.ToolCalls, tc)
			}
			out = append(out, cm)
		}
	}
	return out
}

func (o *OpenAICompat) name() string { return strings.ToLower(string(o.cfg.Provider)) }

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}
