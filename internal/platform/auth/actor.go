// Package auth identifies the operator behind a request. Use cases receive
// the operator's email explicitly (who approved a refund, who reviewed a run);
// this package only carries it from the edge to the resolvers.
package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/mail"
	"strings"
)

type actorKey struct{}

// WithActor stores the operator's email in the context.
func WithActor(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, actorKey{}, email)
}

// ActorFrom returns the operator's email, or "" when unknown.
func ActorFrom(ctx context.Context) string {
	email, _ := ctx.Value(actorKey{}).(string)
	return email
}

// Config selects how requests are authenticated.
type Config struct {
	// TrustIAP reads the identity Google's Identity-Aware Proxy puts in
	// X-Goog-Authenticated-User-Email. Enable it only behind IAP.
	TrustIAP bool
	// APIToken, when set, is required as a bearer token. The dashboard then
	// names the operator in X-Pressroom-Actor.
	APIToken string
	// DefaultActor is used for local development when nothing else applies.
	DefaultActor string
}

// Middleware authenticates the request and records the actor.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := identify(cfg, r)
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer realm="pressroom"`)
				http.Error(w, `{"errors":[{"message":"unauthenticated","extensions":{"code":"UNAUTHENTICATED"}}]}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithActor(r.Context(), actor)))
		})
	}
}

func identify(cfg Config, r *http.Request) (string, bool) {
	if cfg.TrustIAP {
		// Format: "accounts.google.com:jane@example.com"
		v := r.Header.Get("X-Goog-Authenticated-User-Email")
		_, email, found := strings.Cut(v, ":")
		return email, found && validEmail(email)
	}
	if cfg.APIToken != "" {
		token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !found || subtle.ConstantTimeCompare([]byte(token), []byte(cfg.APIToken)) != 1 {
			return "", false
		}
	}
	if actor := r.Header.Get("X-Pressroom-Actor"); validEmail(actor) {
		return actor, true
	}
	return cfg.DefaultActor, true
}

func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}
