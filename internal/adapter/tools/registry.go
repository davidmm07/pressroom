// Package tools is the toolbox the crew works with: the order database, the
// image studio, the carrier, the helpdesk, payments and Slack.
//
// Every tool declares a JSON Schema that is sent to the model verbatim and
// enforced here before the tool runs, so a hallucinated or malformed call
// never reaches a real system.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// Tool is one capability. Implementations do one thing each (Single
// Responsibility) and are added to the Registry without changing it
// (Open/Closed).
type Tool interface {
	Spec() domain.ToolSpec
	Call(ctx context.Context, args json.RawMessage) (any, error)
}

// Func adapts a typed function to Tool. Arguments are validated against the
// schema by the Registry first, then decoded into In.
func Func[In any](spec domain.ToolSpec, fn func(ctx context.Context, in In) (any, error)) Tool {
	return funcTool[In]{spec: spec, fn: fn}
}

type funcTool[In any] struct {
	spec domain.ToolSpec
	fn   func(context.Context, In) (any, error)
}

func (t funcTool[In]) Spec() domain.ToolSpec { return t.spec }

func (t funcTool[In]) Call(ctx context.Context, args json.RawMessage) (any, error) {
	var in In
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, &port.ToolInputError{Tool: t.spec.Name, Detail: err.Error()}
	}
	return t.fn(ctx, in)
}

// Schema parses a JSON Schema literal. It panics on malformed input because
// schemas are compile-time constants; a typo should fail at startup.
func Schema(src string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(src), &m); err != nil {
		panic(fmt.Sprintf("tools: invalid schema literal: %v", err))
	}
	return m
}

type entry struct {
	tool   Tool
	schema *jsonschema.Schema
}

// Registry implements port.ToolRegistry (Registry pattern): tools are looked
// up by name, validated and invoked through one entry point.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]entry
}

var _ port.ToolRegistry = (*Registry)(nil)

func NewRegistry() *Registry { return &Registry{tools: map[string]entry{}} }

// Register compiles the tool's schema and adds it.
func (r *Registry) Register(tools ...Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range tools {
		spec := t.Spec()
		if !domain.ValidToolName(spec.Name) {
			return fmt.Errorf("tools: invalid name %q", spec.Name)
		}
		if _, dup := r.tools[spec.Name]; dup {
			return fmt.Errorf("tools: %q registered twice", spec.Name)
		}
		c := jsonschema.NewCompiler()
		c.AssertFormat() // "email" and "uri" are checks, not just annotations
		url := "tool://" + spec.Name + ".json"
		if err := c.AddResource(url, spec.InputSchema); err != nil {
			return fmt.Errorf("tools: %s schema: %w", spec.Name, err)
		}
		sch, err := c.Compile(url)
		if err != nil {
			return fmt.Errorf("tools: %s schema: %w", spec.Name, err)
		}
		r.tools[spec.Name] = entry{tool: t, schema: sch}
	}
	return nil
}

// Catalog lists every tool, sorted by name.
func (r *Registry) Catalog() []domain.ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.ToolSpec, 0, len(r.tools))
	for _, e := range r.tools {
		out = append(out, e.tool.Spec())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Lookup returns a tool's spec.
func (r *Registry) Lookup(name string) (domain.ToolSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.tools[name]
	if !ok {
		return domain.ToolSpec{}, false
	}
	return e.tool.Spec(), true
}

// Invoke validates arguments, runs the tool and returns its JSON output.
func (r *Registry) Invoke(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	r.mu.RLock()
	e, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("tools: unknown tool %q", name)
	}
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage(`{}`)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(args))
	if err != nil {
		return nil, &port.ToolInputError{Tool: name, Detail: "arguments are not valid JSON: " + err.Error()}
	}
	if err := e.schema.Validate(instance); err != nil {
		return nil, &port.ToolInputError{Tool: name, Detail: describe(err)}
	}
	out, err := e.tool.Call(ctx, args)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// describe flattens a schema violation into lines a model can act on, e.g.
// "/amountCents: must be <= 50000".
func describe(err error) string {
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return err.Error()
	}
	var lines []string
	for _, unit := range ve.BasicOutput().Errors {
		if unit.Error == nil {
			continue
		}
		loc := unit.InstanceLocation
		if loc == "" {
			loc = "(arguments)"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", loc, unit.Error))
	}
	if len(lines) == 0 {
		return err.Error()
	}
	return strings.Join(lines, "; ")
}
