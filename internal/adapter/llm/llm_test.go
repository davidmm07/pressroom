package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/davidmm07/pressroom/internal/adapter/llm"
	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

var lookupOrder = domain.ToolSpec{
	Name:        "lookup_order",
	Description: "Look up an order",
	InputSchema: map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"orderId": map[string]any{"type": "string"}},
		"required":             []any{"orderId"},
		"additionalProperties": false,
	},
}

// capture records the last request an httptest server received.
type capture struct {
	body   map[string]any
	header http.Header
}

func serve(t *testing.T, status int, response string, c *capture) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if c != nil {
			c.header = r.Header.Clone()
			_ = json.Unmarshal(raw, &c.body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(srv.Close)
	return srv
}

const claudeToolUse = `{
  "id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5",
  "content": [
    {"type": "thinking", "thinking": "", "signature": "sig-abc"},
    {"type": "text", "text": "Let me check the order."},
    {"type": "tool_use", "id": "toolu_1", "name": "lookup_order", "input": {"orderId": "ORD-1042"}}
  ],
  "stop_reason": "tool_use", "stop_sequence": null, "stop_details": null,
  "usage": {"input_tokens": 900, "output_tokens": 120, "cache_creation_input_tokens": 100, "cache_read_input_tokens": 2000}
}`

func TestAnthropicAdapterRoundTrip(t *testing.T) {
	var got capture
	srv := serve(t, http.StatusOK, claudeToolUse, &got)
	model := llm.NewAnthropic(llm.AnthropicConfig{APIKey: "test", BaseURL: srv.URL})

	req := port.CompletionRequest{
		Model:           domain.ModelRef{Provider: domain.ProviderAnthropic, Name: "claude-opus-5"},
		System:          "You check orders.",
		Messages:        []domain.Message{{Role: domain.RoleUser, Text: "Where is ORD-1042?"}},
		Tools:           []domain.ToolSpec{lookupOrder},
		MaxOutputTokens: 16000,
	}
	c, err := model.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if c.StopReason != port.StopToolUse || len(c.Message.ToolCalls) != 1 || c.Message.Text != "Let me check the order." {
		t.Fatalf("completion = %+v", c)
	}
	if call := c.Message.ToolCalls[0]; call.ID != "toolu_1" || string(call.Arguments) != `{"orderId": "ORD-1042"}` {
		t.Fatalf("tool call = %+v (%s)", call, call.Arguments)
	}
	if c.Usage != (domain.Usage{InputTokens: 1000, CachedInputTokens: 2000, OutputTokens: 120}) {
		t.Fatalf("usage = %+v", c.Usage)
	}

	// Request shape: adaptive thinking, default refusal fallbacks, tools with schema.
	if got.body["thinking"].(map[string]any)["type"] != "adaptive" {
		t.Errorf("thinking = %v", got.body["thinking"])
	}
	if got.body["fallbacks"] != "default" || !strings.Contains(got.header.Get("Anthropic-Beta"), "server-side-fallback-2026-07-01") {
		t.Errorf("fallbacks = %v, beta = %q", got.body["fallbacks"], got.header.Get("Anthropic-Beta"))
	}
	tool := got.body["tools"].([]any)[0].(map[string]any)
	schema := tool["input_schema"].(map[string]any)
	if tool["name"] != "lookup_order" || schema["additionalProperties"] != false || schema["required"].([]any)[0] != "orderId" {
		t.Errorf("tool = %v", tool)
	}

	// Next turn: the assistant turn must be replayed with its thinking block.
	req.Messages = append(req.Messages, c.Message, domain.Message{Role: domain.RoleUser, ToolResults: []domain.ToolResult{
		{CallID: "toolu_1", Content: `{"status":"SHIPPED"}`},
	}})
	if _, err := model.Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	msgs := got.body["messages"].([]any)
	assistant := msgs[1].(map[string]any)["content"].([]any)
	if first := assistant[0].(map[string]any); first["type"] != "thinking" || first["signature"] != "sig-abc" {
		t.Fatalf("thinking block not replayed: %v", assistant)
	}
	result := msgs[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	if result["type"] != "tool_result" || result["tool_use_id"] != "toolu_1" {
		t.Fatalf("tool result = %v", result)
	}
}

func TestAnthropicAdapterMapsRefusalsAndErrors(t *testing.T) {
	refusal := `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5","content":[],
	  "stop_reason":"refusal","stop_sequence":null,"stop_details":{"type":"refusal","category":"cyber","explanation":null},
	  "usage":{"input_tokens":0,"output_tokens":0}}`
	srv := serve(t, http.StatusOK, refusal, nil)
	req := port.CompletionRequest{Model: domain.ModelRef{Provider: domain.ProviderAnthropic, Name: "claude-opus-5"}, MaxOutputTokens: 100,
		Messages: []domain.Message{{Role: domain.RoleUser, Text: "hi"}}}
	c, err := llm.NewAnthropic(llm.AnthropicConfig{APIKey: "k", BaseURL: srv.URL}).Complete(context.Background(), req)
	if err != nil || c.StopReason != port.StopRefusal || c.Detail != "cyber" {
		t.Fatalf("got %+v, %v", c, err)
	}

	for status, transient := range map[int]bool{429: true, 529: true, 500: true, 400: false, 401: false} {
		srv := serve(t, status, `{"type":"error","error":{"type":"x","message":"boom"}}`, nil)
		_, err := llm.NewAnthropic(llm.AnthropicConfig{APIKey: "k", BaseURL: srv.URL}).Complete(context.Background(), req)
		te, ok := port.IsTransient(err)
		if ok != transient {
			t.Errorf("status %d: transient=%v, want %v (%v)", status, ok, transient, err)
		}
		if ok && status == 429 && te.RetryAfter != 7*time.Second {
			t.Errorf("status 429: retry after %s, want 7s", te.RetryAfter)
		}
	}
}

func TestOpenAICompatAdapter(t *testing.T) {
	response := `{"model":"grok-4-0709","choices":[{"finish_reason":"tool_calls","message":{"content":null,
	  "tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup_order","arguments":"{\"orderId\":\"ORD-7\"}"}}]}}],
	  "usage":{"prompt_tokens":500,"completion_tokens":40,"prompt_tokens_details":{"cached_tokens":200}}}`
	var got capture
	srv := serve(t, http.StatusOK, response, &got)
	model := llm.NewOpenAICompat(llm.OpenAICompatConfig{Provider: domain.ProviderXAI, BaseURL: srv.URL, APIKey: "xai-key"})

	c, err := model.Complete(context.Background(), port.CompletionRequest{
		Model: domain.ModelRef{Provider: domain.ProviderXAI, Name: "grok-4"}, System: "sys", MaxOutputTokens: 1000,
		Tools: []domain.ToolSpec{lookupOrder},
		Messages: []domain.Message{
			{Role: domain.RoleUser, Text: "task"},
			{Role: domain.RoleAssistant, ToolCalls: []domain.ToolCall{{ID: "call_0", Name: "lookup_order", Arguments: json.RawMessage(`{"orderId":"ORD-6"}`)}}},
			{Role: domain.RoleUser, ToolResults: []domain.ToolResult{{CallID: "call_0", Content: "not found", IsError: true}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.StopReason != port.StopToolUse || c.Message.ToolCalls[0].Name != "lookup_order" || c.Usage.CachedInputTokens != 200 || c.Usage.InputTokens != 300 {
		t.Fatalf("completion = %+v", c)
	}
	if got.header.Get("Authorization") != "Bearer xai-key" || got.body["max_completion_tokens"] != float64(1000) {
		t.Fatalf("headers/body wrong: %v %v", got.header, got.body)
	}
	msgs := got.body["messages"].([]any)
	if len(msgs) != 4 || msgs[0].(map[string]any)["role"] != "system" || msgs[3].(map[string]any)["role"] != "tool" ||
		msgs[3].(map[string]any)["content"] != "ERROR: not found" {
		t.Fatalf("messages = %v", msgs)
	}
}

func TestSandboxPlansFromFacts(t *testing.T) {
	trackShipment := domain.ToolSpec{Name: "track_shipment", InputSchema: map[string]any{
		"properties": map[string]any{"trackingNumber": map[string]any{"type": "string"}}, "required": []any{"trackingNumber"},
	}}
	draftReply := domain.ToolSpec{Name: "draft_reply", InputSchema: map[string]any{
		"properties": map[string]any{"ticketId": map[string]any{"type": "string"}, "body": map[string]any{"type": "string"}},
		"required":   []any{"ticketId", "body"},
	}}
	req := port.CompletionRequest{
		Tools:    []domain.ToolSpec{lookupOrder, trackShipment, draftReply},
		Messages: []domain.Message{{Role: domain.RoleUser, Text: "Task input (JSON):\n{\"orderId\":\"ORD-1\",\"ticketId\":\"T-1\"}\n\nDo it."}},
	}
	sb := llm.Sandbox{}
	step := func() *port.Completion {
		c, err := sb.Complete(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		req.Messages = append(req.Messages, c.Message)
		return c
	}

	if c := step(); c.Message.ToolCalls[0].Name != "lookup_order" {
		t.Fatalf("first call = %+v", c.Message.ToolCalls)
	}
	req.Messages = append(req.Messages, domain.Message{Role: domain.RoleUser, ToolResults: []domain.ToolResult{
		{CallID: "x", Name: "lookup_order", Content: `{"orderId":"ORD-1","trackingNumber":"1ZABC","status":"SHIPPED"}`},
	}})
	if c := step(); c.Message.ToolCalls[0].Name != "track_shipment" || !strings.Contains(string(c.Message.ToolCalls[0].Arguments), "1ZABC") {
		t.Fatalf("second call = %+v", c.Message.ToolCalls)
	}
	req.Messages = append(req.Messages, domain.Message{Role: domain.RoleUser, ToolResults: []domain.ToolResult{{CallID: "y", Name: "track_shipment", Content: `{}`}}})
	if c := step(); c.Message.ToolCalls[0].Name != "draft_reply" {
		t.Fatalf("third call = %+v", c.Message.ToolCalls)
	}
	req.Messages = append(req.Messages, domain.Message{Role: domain.RoleUser, ToolResults: []domain.ToolResult{{CallID: "z", Name: "draft_reply", Content: `{}`}}})
	if c := step(); c.StopReason != port.StopEndTurn || !strings.Contains(c.Message.Text, "draft_reply succeeded") {
		t.Fatalf("final = %+v", c)
	}
}

type flaky struct {
	failures int
	calls    int
	err      error
}

func (f *flaky) Complete(context.Context, port.CompletionRequest) (*port.Completion, error) {
	f.calls++
	if f.calls <= f.failures {
		return nil, f.err
	}
	return &port.Completion{StopReason: port.StopEndTurn}, nil
}

func TestRetryDecorator(t *testing.T) {
	policy := llm.RetryPolicy{Attempts: 3, BaseWait: time.Millisecond, MaxWait: 50 * time.Millisecond}

	t.Run("retries transient errors", func(t *testing.T) {
		f := &flaky{failures: 2, err: port.Transient(errors.New("overloaded"), 0)}
		if _, err := llm.WithRetry(f, policy).Complete(context.Background(), port.CompletionRequest{}); err != nil || f.calls != 3 {
			t.Fatalf("calls=%d err=%v", f.calls, err)
		}
	})
	t.Run("does not retry permanent errors", func(t *testing.T) {
		f := &flaky{failures: 5, err: errors.New("bad request")}
		if _, err := llm.WithRetry(f, policy).Complete(context.Background(), port.CompletionRequest{}); err == nil || f.calls != 1 {
			t.Fatalf("calls=%d err=%v", f.calls, err)
		}
	})
	t.Run("hands long waits back to the queue", func(t *testing.T) {
		f := &flaky{failures: 5, err: port.Transient(errors.New("rate limited"), time.Minute)}
		_, err := llm.WithRetry(f, policy).Complete(context.Background(), port.CompletionRequest{})
		if _, ok := port.IsTransient(err); !ok || f.calls != 1 {
			t.Fatalf("calls=%d err=%v", f.calls, err)
		}
	})
}

func TestRouterModes(t *testing.T) {
	req := port.CompletionRequest{Model: domain.ModelRef{Provider: domain.ProviderOpenAI, Name: "gpt-5"}}
	if _, err := llm.NewRouter(llm.ModeLive, discard()).Complete(context.Background(), req); err == nil {
		t.Fatal("live mode must reject an unconfigured provider")
	}
	if c, err := llm.NewRouter(llm.ModeAuto, discard()).Complete(context.Background(), req); err != nil || c == nil {
		t.Fatalf("auto mode should fall back to the sandbox: %v", err)
	}
}

func TestCatalogPricesAndOverrides(t *testing.T) {
	cat, err := llm.NewCatalog(`{"openai/gpt-5":{"input":2,"output":12}}`, func(domain.Provider) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if p := cat.Price(domain.ModelRef{Provider: domain.ProviderOpenAI, Name: "gpt-5"}); p.InputPerMTok != 2 {
		t.Fatalf("override not applied: %+v", p)
	}
	if p := cat.Price(domain.ModelRef{Provider: domain.ProviderAnthropic, Name: "claude-opus-5"}); p.OutputPerMTok != 25 {
		t.Fatalf("opus price = %+v", p)
	}
	if p := cat.Price(domain.ModelRef{Provider: domain.ProviderXAI, Name: "grok-99"}); p.OutputPerMTok != 50 {
		t.Fatalf("unknown models must be priced pessimistically: %+v", p)
	}
	if _, err := llm.NewCatalog(`{"nonsense":{}}`, nil); err == nil {
		t.Fatal("invalid override keys must be rejected")
	}
}
