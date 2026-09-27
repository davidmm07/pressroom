package domain

import "regexp"

// ToolSpec describes a capability an agent may call: a lookup in the order
// database, an image operation in the design studio, a message to Slack.
//
// The spec is pure data. Execution lives behind port.ToolRegistry, so the
// domain can reason about tools (allow-lists, approvals) without importing
// any HTTP client or database driver.
type ToolSpec struct {
	Name        string
	Description string
	// InputSchema is a JSON Schema object. Model providers receive it
	// verbatim and the registry validates every call against it.
	InputSchema map[string]any
	// RequiresApproval marks tools with customer-visible or financial side
	// effects. A run that reaches one pauses until a human approves it.
	RequiresApproval bool
}

// Tool names travel to every provider's function-calling API, whose common
// denominator is ^[a-zA-Z0-9_-]{1,64}$, so no dots or spaces.
var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)

// ValidToolName reports whether name is safe to send to any provider.
func ValidToolName(name string) bool { return toolNamePattern.MatchString(name) }
