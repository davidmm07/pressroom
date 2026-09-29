package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// Sandbox is a deterministic stand-in for a real model. It lets the whole
// platform run end to end (demo, CI, local development) without API keys:
//
//  1. It collects facts from the run input and from successful tool results.
//  2. It calls the next allowed tool, in allow-list order, whose required
//     arguments it can fill from those facts.
//  3. When no tool is left, it answers with a summary of what it did.
//
// It is not smart, and it does not need to be: its job is to exercise the
// executor, approvals, budgets and storage exactly as a real model would.
type Sandbox struct{}

var _ port.LanguageModel = Sandbox{}

// Complete plans the next step.
func (Sandbox) Complete(_ context.Context, req port.CompletionRequest) (*port.Completion, error) {
	facts := map[string]any{}
	skip := map[string]bool{} // tools already called or reported as not needed
	var log []string

	for i, m := range req.Messages {
		if i == 0 {
			mergeObject(facts, extractJSON(m.Text))
		}
		for _, c := range m.ToolCalls {
			skip[c.Name] = true
		}
		for _, r := range m.ToolResults {
			if r.IsError {
				log = append(log, fmt.Sprintf("%s failed (%s)", r.Name, firstLine(r.Content)))
				continue
			}
			out := extractJSON(r.Content)
			for _, name := range toStrings(out["notNeeded"]) {
				skip[name] = true
			}
			mergeObject(facts, out)
			log = append(log, r.Name+" succeeded")
		}
	}

	promptChars := len(req.System)
	for _, m := range req.Messages {
		promptChars += len(m.Text)
		for _, r := range m.ToolResults {
			promptChars += len(r.Content)
		}
	}
	usage := domain.Usage{InputTokens: 400 + promptChars/4, OutputTokens: 60}

	for _, tool := range req.Tools {
		if skip[tool.Name] {
			continue
		}
		args, ok := fillArguments(tool.InputSchema, facts)
		if !ok {
			continue
		}
		raw, _ := json.Marshal(args)
		usage.OutputTokens += len(raw) / 4
		return &port.Completion{
			Message: domain.Message{
				Text:      "Next I will call " + tool.Name + ".",
				ToolCalls: []domain.ToolCall{{ID: fmt.Sprintf("sbx_%d_%s", len(req.Messages), tool.Name), Name: tool.Name, Arguments: raw}},
			},
			Usage:      usage,
			StopReason: port.StopToolUse,
		}, nil
	}

	summary := "Sandbox run finished with no tool calls."
	if len(log) > 0 {
		summary = "Sandbox run finished: " + strings.Join(log, "; ") + "."
	}
	usage.OutputTokens += len(summary) / 4
	return &port.Completion{Message: domain.Message{Text: summary}, Usage: usage, StopReason: port.StopEndTurn}, nil
}

// fillArguments builds tool arguments from known facts. It gives up when a
// required argument has no matching fact and cannot be written as prose.
func fillArguments(schema map[string]any, facts map[string]any) (map[string]any, bool) {
	props, _ := schema["properties"].(map[string]any)
	required := map[string]bool{}
	for _, r := range toStrings(schema["required"]) {
		required[r] = true
	}
	args := map[string]any{}
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if v, ok := lookupFact(facts, name); ok {
			args[name] = v
			continue
		}
		prop, _ := props[name].(map[string]any)
		if prop["type"] == "string" && proseFields[name] {
			args[name] = compose(name, facts)
			continue
		}
		if required[name] {
			return nil, false
		}
	}
	return args, true
}

// lookupFact matches a parameter to a fact by exact name, then by suffix,
// so a tool asking for "factor" finds a previous tool's "upscaleFactor".
func lookupFact(facts map[string]any, name string) (any, bool) {
	if v, ok := facts[name]; ok {
		return v, true
	}
	if len(name) < 5 { // "id" or "to" would match far too much
		return nil, false
	}
	suffix := strings.ToLower(name)
	keys := make([]string, 0, len(facts))
	for k := range facts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.HasSuffix(strings.ToLower(k), suffix) {
			return facts[k], true
		}
	}
	return nil, false
}

var proseFields = map[string]bool{"body": true, "message": true, "note": true, "summary": true, "reason": true, "text": true, "subject": true, "headline": true}

func compose(field string, facts map[string]any) string {
	var parts []string
	for _, k := range []string{
		"customerName", "orderId", "status", "product", "carrier", "trackingNumber", "estimatedDelivery", "issue", "proofUrl",
		"remedy", "verdict", "recommendation", "category", "reviewCount", "launchScore", "disqualifiedEntries",
	} {
		if v, ok := facts[k]; ok {
			parts = append(parts, k+" "+strings.TrimSuffix(fmt.Sprint(v), "."))
		}
	}
	if len(parts) == 0 {
		return "Automated " + field + " from the Pressroom sandbox."
	}
	return "Automated " + field + ": " + strings.Join(parts, ", ") + "."
}

func extractJSON(s string) map[string]any {
	start := strings.Index(s, "{")
	if start < 0 {
		return nil
	}
	var obj map[string]any
	if err := json.NewDecoder(strings.NewReader(s[start:])).Decode(&obj); err != nil {
		return nil
	}
	return obj
}

// mergeObject copies scalar fields and flattens one level of nesting, so
// {"order":{"id":"ORD-1"}} also yields "id".
func mergeObject(dst, src map[string]any) {
	for k, v := range src {
		switch val := v.(type) {
		case map[string]any:
			for nk, nv := range val {
				if _, isMap := nv.(map[string]any); !isMap {
					dst[nk] = nv
				}
			}
		default:
			dst[k] = val
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:120] + "..."
	}
	return s
}
