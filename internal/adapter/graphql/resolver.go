// Package graphql is the delivery adapter for the dashboard and API clients.
// Resolvers are thin: they translate transport types to use-case calls and
// back, and never hold business rules (Single Responsibility Principle).
package graphql

import (
	"github.com/davidmm07/pressroom/internal/app"
	"github.com/davidmm07/pressroom/internal/port"
)

// Resolver is the root of the resolver tree. Dependencies are injected by
// the composition root in cmd/api (Dependency Injection).
type Resolver struct {
	Agents        *app.AgentService
	Runs          *app.RunService
	Evaluator     *app.EvaluationService
	Experiments   *app.ExperimentService
	Opportunities *app.OpportunityService
	Tools         port.ToolRegistry
	Models        port.ModelCatalog
}
