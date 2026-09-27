package domain

import (
	"regexp"
	"strings"
)

// Provider is the vendor (or runtime) that serves a model.
type Provider string

const (
	ProviderAnthropic  Provider = "ANTHROPIC"
	ProviderOpenAI     Provider = "OPENAI"
	ProviderXAI        Provider = "XAI"
	ProviderOpenSource Provider = "OPEN_SOURCE" // self-hosted, OpenAI-compatible (vLLM, Ollama)
	ProviderSandbox    Provider = "SANDBOX"     // deterministic planner for demos and tests
)

// Providers lists every supported provider in display order.
var Providers = []Provider{ProviderAnthropic, ProviderOpenAI, ProviderXAI, ProviderOpenSource, ProviderSandbox}

func (p Provider) Valid() bool {
	for _, known := range Providers {
		if p == known {
			return true
		}
	}
	return false
}

// ModelRef names a model independently of any SDK, e.g. anthropic/claude-opus-5.
// Agents store a ModelRef, never a client, so swapping vendors is a data
// change instead of a code change (Open/Closed Principle).
type ModelRef struct {
	Provider Provider
	Name     string
}

var modelNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,79}$`)

// ParseModelRef parses "provider/name". The name may itself contain slashes
// (open-source models are often namespaced, e.g. open_source/qwen/qwen3-32b).
func ParseModelRef(s string) (ModelRef, error) {
	provider, name, ok := strings.Cut(s, "/")
	if !ok {
		return ModelRef{}, NewValidationError("model", CodeInvalidFmt, `must look like "provider/model-name"`)
	}
	ref := ModelRef{Provider: Provider(strings.ToUpper(provider)), Name: name}
	return ref, ref.Validate()
}

// Validate checks both parts of the reference.
func (m ModelRef) Validate() error {
	var v Validator
	v.Enum("provider", m.Provider.Valid(), string(m.Provider))
	v.Pattern("name", m.Name, modelNamePattern, "must be a model identifier such as claude-opus-5")
	return v.Err()
}

func (m ModelRef) String() string { return strings.ToLower(string(m.Provider)) + "/" + m.Name }

// IsZero reports whether the reference is unset.
func (m ModelRef) IsZero() bool { return m.Provider == "" && m.Name == "" }
