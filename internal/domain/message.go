package domain

import "encoding/json"

// Role identifies who authored a message in a run's transcript.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ToolCall is a model's request to invoke a tool.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ToolResult is the observation sent back to the model for one ToolCall.
type ToolResult struct {
	CallID  string
	Name    string
	Content string
	IsError bool
}

// Message is one provider-neutral turn of a conversation. Every model adapter
// translates to and from this shape, which is what lets a run on Claude and
// a run on Grok share the same executor, storage and dashboard.
type Message struct {
	Role        Role
	Text        string
	ToolCalls   []ToolCall   // assistant turns only
	ToolResults []ToolResult // user turns only

	// ProviderState is an opaque snapshot that an adapter may attach to the
	// assistant turns it produced (Memento pattern). Claude, for example,
	// requires its thinking blocks to be replayed unchanged while a tool loop
	// is in progress. The domain stores the bytes but never interprets them.
	ProviderState json.RawMessage
	// StateProvider records which provider wrote ProviderState, so a
	// different adapter knows to ignore it.
	StateProvider Provider
}
