package llm

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// Mode decides what happens when a provider has no credentials.
type Mode string

const (
	ModeLive    Mode = "live"    // unconfigured providers are an error
	ModeSandbox Mode = "sandbox" // every request goes to the sandbox
	ModeAuto    Mode = "auto"    // real provider when configured, sandbox otherwise
)

// ParseMode validates a mode string.
func ParseMode(s string) (Mode, error) {
	switch m := Mode(strings.ToLower(s)); m {
	case ModeLive, ModeSandbox, ModeAuto:
		return m, nil
	}
	return "", fmt.Errorf("unknown model mode %q (want live, sandbox or auto)", s)
}

// Router dispatches each request to the adapter for its provider (Strategy
// selected at runtime from data). New providers register here; nothing that
// calls the Router changes (Open/Closed Principle).
type Router struct {
	mode      Mode
	providers map[domain.Provider]port.LanguageModel
	sandbox   port.LanguageModel
	log       *slog.Logger
}

var _ port.LanguageModel = (*Router)(nil)

// NewRouter creates a router; register providers with Register.
func NewRouter(mode Mode, log *slog.Logger) *Router {
	return &Router{mode: mode, providers: map[domain.Provider]port.LanguageModel{}, sandbox: Sandbox{}, log: log}
}

// Register adds or replaces the adapter for a provider.
func (r *Router) Register(p domain.Provider, m port.LanguageModel) { r.providers[p] = m }

// Available reports whether requests for p will be served.
func (r *Router) Available(p domain.Provider) bool {
	if p == domain.ProviderSandbox || r.mode != ModeLive {
		return true
	}
	_, ok := r.providers[p]
	return ok
}

// Complete routes the request.
func (r *Router) Complete(ctx context.Context, req port.CompletionRequest) (*port.Completion, error) {
	if req.Model.Provider == domain.ProviderSandbox || r.mode == ModeSandbox {
		return r.sandbox.Complete(ctx, req)
	}
	if m, ok := r.providers[req.Model.Provider]; ok {
		return m.Complete(ctx, req)
	}
	if r.mode == ModeAuto {
		r.log.DebugContext(ctx, "provider not configured, using sandbox", "model", req.Model.String())
		return r.sandbox.Complete(ctx, req)
	}
	return nil, fmt.Errorf("provider %s is not configured: set its API key or run with PRESSROOM_MODEL_MODE=auto", req.Model.Provider)
}
