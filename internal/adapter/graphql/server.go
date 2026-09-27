package graphql

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/davidmm07/pressroom/internal/adapter/graphql/generated"
)

// ServerConfig tunes the GraphQL endpoint.
type ServerConfig struct {
	// Introspection lets tools such as the dashboard's codegen read the
	// schema. Disable it in production if the schema should stay private.
	Introspection bool
	// ComplexityLimit rejects queries that would fan out too far, e.g.
	// agents { runs(first: 100) { edges { node { agent { runs ... } } } } }.
	ComplexityLimit int
}

// NewHandler builds the /graphql endpoint.
func NewHandler(r *Resolver, cfg ServerConfig, log *slog.Logger) http.Handler {
	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: r}))
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	if cfg.Introspection {
		srv.Use(extension.Introspection{})
	}
	srv.Use(extension.AutomaticPersistedQuery{Cache: lru.New[string](100)})
	if cfg.ComplexityLimit > 0 {
		srv.Use(extension.FixedComplexityLimit(cfg.ComplexityLimit))
	}
	srv.SetErrorPresenter(ErrorPresenter(log))
	srv.SetRecoverFunc(RecoverFunc(log))
	return r.LoaderMiddleware(http.TimeoutHandler(srv, 30*time.Second, `{"errors":[{"message":"the request took too long","extensions":{"code":"TIMEOUT"}}]}`))
}

// Playground serves GraphiQL for local exploration.
func Playground(endpoint string) http.Handler {
	return playground.Handler("Pressroom GraphQL", endpoint)
}
