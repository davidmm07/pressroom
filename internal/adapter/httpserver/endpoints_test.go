package httpserver_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidmm07/pressroom/internal/adapter/httpserver"
	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/platform/auth"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func push(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/events/pubsub", strings.NewReader(body)))
	return rec
}

func envelope(eventType, data string) string {
	b, _ := json.Marshal(map[string]any{
		"message": map[string]any{
			"messageId": "m-1", "data": base64.StdEncoding.EncodeToString([]byte(data)),
			"attributes": map[string]string{"eventType": eventType},
		},
		"subscription": "projects/p/subscriptions/pressroom-events",
	})
	return string(b)
}

func TestPubSubPushAcksAndRetriesCorrectly(t *testing.T) {
	var gotID, gotType string
	ok := httpserver.PubSubPush(func(_ context.Context, id, typ string, _ json.RawMessage) ([]*domain.Run, error) {
		gotID, gotType = id, typ
		return []*domain.Run{{ID: "r1"}}, nil
	}, quiet)
	if rec := push(t, ok, envelope("ticket.created", `{"ticketId":"T-1"}`)); rec.Code != http.StatusOK || gotID != "m-1" || gotType != "ticket.created" {
		t.Fatalf("valid event: %d %s %s", rec.Code, gotID, gotType)
	}

	// Redelivering a malformed message cannot fix it, so it is acked.
	if rec := push(t, ok, `not json`); rec.Code != http.StatusNoContent {
		t.Fatalf("malformed body: %d", rec.Code)
	}
	if rec := push(t, ok, envelope("", `{}`)); rec.Code != http.StatusNoContent {
		t.Fatalf("missing type: %d", rec.Code)
	}
	invalid := httpserver.PubSubPush(func(context.Context, string, string, json.RawMessage) ([]*domain.Run, error) {
		return nil, domain.NewValidationError("input", domain.CodeInvalidFmt, "must be a JSON object")
	}, quiet)
	if rec := push(t, invalid, envelope("ticket.created", `[]`)); rec.Code != http.StatusNoContent {
		t.Fatalf("invalid event: %d", rec.Code)
	}

	// A database outage is worth retrying: Pub/Sub redelivers on 5xx.
	broken := httpserver.PubSubPush(func(context.Context, string, string, json.RawMessage) ([]*domain.Run, error) {
		return nil, errors.New("connection refused")
	}, quiet)
	if rec := push(t, broken, envelope("ticket.created", `{}`)); rec.Code != http.StatusInternalServerError {
		t.Fatalf("transient failure: %d", rec.Code)
	}
}

func TestAuthMiddleware(t *testing.T) {
	var actor string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { actor = auth.ActorFrom(r.Context()) })
	call := func(cfg auth.Config, headers map[string]string) int {
		actor = ""
		req := httptest.NewRequest(http.MethodPost, "/graphql", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		auth.Middleware(cfg)(next).ServeHTTP(rec, req)
		return rec.Code
	}

	token := auth.Config{APIToken: "s3cret", DefaultActor: "ops@pressroom.local"}
	if code := call(token, nil); code != http.StatusUnauthorized {
		t.Fatalf("missing token: %d", code)
	}
	if code := call(token, map[string]string{"Authorization": "Bearer wrong"}); code != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", code)
	}
	if code := call(token, map[string]string{"Authorization": "Bearer s3cret", "X-Pressroom-Actor": "lead@example.com"}); code != http.StatusOK || actor != "lead@example.com" {
		t.Fatalf("valid token: %d actor=%q", code, actor)
	}

	iap := auth.Config{TrustIAP: true}
	if code := call(iap, map[string]string{"X-Goog-Authenticated-User-Email": "accounts.google.com:jane@example.com"}); code != http.StatusOK || actor != "jane@example.com" {
		t.Fatalf("iap: %d actor=%q", code, actor)
	}
	if code := call(iap, map[string]string{"X-Pressroom-Actor": "mallory@example.com"}); code != http.StatusUnauthorized {
		t.Fatalf("behind IAP the actor header must not be trusted: %d", code)
	}
}
