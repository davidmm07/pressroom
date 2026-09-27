// Package llm adapts model vendors to port.LanguageModel.
//
// Each vendor gets an Adapter that translates the provider-neutral request
// into its wire format and back (Adapter pattern). The Router picks the
// adapter for a request's provider (Strategy), and cross-cutting behaviour
// such as retries and logging wraps any of them without touching their code
// (Decorator pattern, Open/Closed Principle).
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// AnthropicConfig configures the Claude adapter.
type AnthropicConfig struct {
	APIKey  string
	BaseURL string // optional, for proxies and tests
}

// Anthropic calls Claude through the official Go SDK's Messages API.
type Anthropic struct {
	client anthropic.Client
}

var _ port.LanguageModel = (*Anthropic)(nil)

// NewAnthropic builds the adapter. SDK-level retries are disabled because
// the Retrying decorator applies one retry policy to every provider.
func NewAnthropic(cfg AnthropicConfig) *Anthropic {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey), option.WithMaxRetries(0)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &Anthropic{client: anthropic.NewClient(opts...)}
}

// Complete sends one turn to Claude.
func (a *Anthropic) Complete(ctx context.Context, req port.CompletionRequest) (*port.Completion, error) {
	messages, err := toAnthropicMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	params := anthropic.BetaMessageNewParams{
		Model:     req.Model.Name,
		MaxTokens: int64(req.MaxOutputTokens),
		System:    []anthropic.BetaTextBlockParam{{Text: req.System}},
		Messages:  messages,
		Tools:     toAnthropicTools(req.Tools),
		// The agent loop resends a growing transcript every turn; automatic
		// caching bills the repeated prefix at the cache-read rate.
		CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
	}
	if supportsAdaptiveThinking(req.Model.Name) {
		params.Thinking = anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}}
	}
	if supportsServerFallbacks(req.Model.Name) {
		// A safety-classifier refusal is re-served by Anthropic's recommended
		// fallback model inside the same call instead of failing the run.
		params.Fallbacks = anthropic.BetaFallbacksParamOfDefault()
		params.Betas = append(params.Betas, anthropic.AnthropicBetaServerSideFallback2026_07_01)
	}

	resp, err := a.client.Beta.Messages.New(ctx, params)
	if err != nil {
		return nil, classifyAnthropicError(err)
	}
	return fromAnthropicMessage(resp, req.Model.Name)
}

func toAnthropicMessages(msgs []domain.Message) ([]anthropic.BetaMessageParam, error) {
	out := make([]anthropic.BetaMessageParam, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case domain.RoleUser:
			blocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(m.ToolResults)+1)
			for _, r := range m.ToolResults {
				blocks = append(blocks, anthropic.NewBetaToolResultBlock(r.CallID, r.Content, r.IsError))
			}
			if m.Text != "" {
				blocks = append(blocks, anthropic.NewBetaTextBlock(m.Text))
			}
			out = append(out, anthropic.NewBetaUserMessage(blocks...))

		case domain.RoleAssistant:
			// Replay Claude's own turn verbatim (Memento): thinking blocks must
			// come back unchanged while a tool loop is in progress.
			if m.StateProvider == domain.ProviderAnthropic && len(m.ProviderState) > 0 {
				var p anthropic.BetaMessageParam
				if err := json.Unmarshal(m.ProviderState, &p); err != nil {
					return nil, fmt.Errorf("anthropic: restore assistant turn: %w", err)
				}
				out = append(out, p)
				continue
			}
			// A turn produced by another provider: rebuild it from neutral fields.
			blocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(m.ToolCalls)+1)
			if m.Text != "" {
				blocks = append(blocks, anthropic.NewBetaTextBlock(m.Text))
			}
			for _, c := range m.ToolCalls {
				blocks = append(blocks, anthropic.NewBetaToolUseBlock(c.ID, json.RawMessage(c.Arguments), c.Name))
			}
			out = append(out, anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleAssistant, Content: blocks})
		}
	}
	return out, nil
}

func toAnthropicTools(specs []domain.ToolSpec) []anthropic.BetaToolUnionParam {
	tools := make([]anthropic.BetaToolUnionParam, 0, len(specs))
	for _, s := range specs {
		schema := anthropic.BetaToolInputSchemaParam{Properties: s.InputSchema["properties"], ExtraFields: map[string]any{}}
		for k, v := range s.InputSchema {
			switch k {
			case "properties", "type":
			case "required":
				schema.Required = toStrings(v)
			default:
				schema.ExtraFields[k] = v
			}
		}
		tools = append(tools, anthropic.BetaToolUnionParam{OfTool: &anthropic.BetaToolParam{
			Name: s.Name, Description: anthropic.String(s.Description), InputSchema: schema,
		}})
	}
	return tools
}

func fromAnthropicMessage(resp *anthropic.BetaMessage, requested string) (*port.Completion, error) {
	msg := domain.Message{Role: domain.RoleAssistant, StateProvider: domain.ProviderAnthropic}
	var text []string
	for _, block := range resp.Content {
		switch b := block.AsAny().(type) {
		case anthropic.BetaTextBlock:
			text = append(text, b.Text)
		case anthropic.BetaToolUseBlock:
			msg.ToolCalls = append(msg.ToolCalls, domain.ToolCall{ID: b.ID, Name: b.Name, Arguments: json.RawMessage(b.JSON.Input.Raw())})
		}
	}
	msg.Text = strings.Join(text, "\n")

	state, err := json.Marshal(resp.ToParam())
	if err != nil {
		return nil, fmt.Errorf("anthropic: snapshot assistant turn: %w", err)
	}
	msg.ProviderState = state

	c := &port.Completion{
		Message: msg,
		Usage: domain.Usage{
			// Cache writes are billed near the input rate; count them as input.
			InputTokens:       int(resp.Usage.InputTokens + resp.Usage.CacheCreationInputTokens),
			CachedInputTokens: int(resp.Usage.CacheReadInputTokens),
			OutputTokens:      int(resp.Usage.OutputTokens),
		},
		StopReason: port.StopEndTurn,
	}
	if string(resp.Model) != requested {
		c.ServedBy = string(resp.Model)
	}
	switch resp.StopReason {
	case anthropic.BetaStopReasonToolUse:
		c.StopReason = port.StopToolUse
	case anthropic.BetaStopReasonMaxTokens:
		c.StopReason = port.StopMaxTokens
	case anthropic.BetaStopReasonRefusal:
		c.StopReason = port.StopRefusal
		c.Detail = string(resp.StopDetails.Category)
	}
	return c, nil
}

// classifyAnthropicError separates retryable failures (rate limits,
// overload, 5xx, network) from permanent ones (bad request, auth, unknown
// model), using the SDK's typed error rather than string matching.
func classifyAnthropicError(err error) error {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		wrapped := fmt.Errorf("anthropic: %w", err)
		switch code := apiErr.StatusCode; {
		case code == http.StatusTooManyRequests, code == http.StatusRequestTimeout,
			code == http.StatusConflict, code >= 500:
			return port.Transient(wrapped, retryAfter(apiErr.Response))
		default:
			return wrapped
		}
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	// Transport failures (DNS, reset connections, timeouts) are worth a retry.
	return port.Transient(fmt.Errorf("anthropic: %w", err), 0)
}

func retryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}

// Claude Haiku 4.5 predates adaptive thinking; every newer model supports it.
func supportsAdaptiveThinking(model string) bool {
	return !strings.HasPrefix(model, "claude-haiku-4-5")
}

// Server-side refusal fallbacks are offered for the Opus 5 and Fable 5 tiers.
func supportsServerFallbacks(model string) bool {
	return strings.HasPrefix(model, "claude-opus-5") || strings.HasPrefix(model, "claude-fable-5")
}

func toStrings(v any) []string {
	switch vals := v.(type) {
	case []string:
		return vals
	case []any:
		out := make([]string, 0, len(vals))
		for _, x := range vals {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
