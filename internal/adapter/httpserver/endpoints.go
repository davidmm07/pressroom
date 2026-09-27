package httpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/davidmm07/pressroom/internal/domain"
)

// Health serves /healthz (the process is up) and /readyz (its dependencies
// answer). Cloud Run uses them as startup and liveness probes.
func Health(mux *http.ServeMux, ready func(context.Context) error) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
}

// pushEnvelope is the body Pub/Sub POSTs to a push subscription.
type pushEnvelope struct {
	Message struct {
		Data       string            `json:"data"`
		MessageID  string            `json:"messageId"`
		Attributes map[string]string `json:"attributes"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// Dispatcher starts runs for an inbound event.
type Dispatcher func(ctx context.Context, eventID, eventType string, data json.RawMessage) ([]*domain.Run, error)

// PubSubPush receives storefront events (artwork.uploaded, ticket.created,
// shipment.stalled, customer.reorder_due) from a Pub/Sub push subscription.
//
// Authentication is Cloud Run's job: the worker service only admits the
// push subscription's service account, verified from its OIDC token before
// the request reaches this code.
//
// Status codes drive Pub/Sub's retry: 2xx acks. A malformed message is acked
// and logged, because redelivering it cannot fix it (the dead-letter topic is
// for failures that might). Unexpected errors return 500 so Pub/Sub retries.
func PubSubPush(dispatch Dispatcher, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env pushEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			log.WarnContext(r.Context(), "dropping undecodable push body", "error", err)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		eventType := env.Message.Attributes["eventType"]
		data, err := base64.StdEncoding.DecodeString(env.Message.Data)
		if err != nil || eventType == "" {
			log.WarnContext(r.Context(), "dropping malformed event", "message_id", env.Message.MessageID, "event_type", eventType)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		runs, err := dispatch(r.Context(), env.Message.MessageID, eventType, data)
		var ve *domain.ValidationError
		switch {
		case errors.As(err, &ve):
			log.WarnContext(r.Context(), "dropping invalid event", "message_id", env.Message.MessageID, "error", err)
			w.WriteHeader(http.StatusNoContent)
		case err != nil:
			log.ErrorContext(r.Context(), "event dispatch failed; Pub/Sub will retry", "message_id", env.Message.MessageID, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dispatch failed"})
		default:
			ids := make([]domain.ID, len(runs))
			for i, run := range runs {
				ids[i] = run.ID
			}
			log.InfoContext(r.Context(), "event dispatched", "event_type", eventType, "message_id", env.Message.MessageID, "runs", ids)
			writeJSON(w, http.StatusOK, map[string]any{"runs": ids})
		}
	})
}

// Job exposes a maintenance task (such as the daily crew evaluation) for
// Cloud Scheduler, which authenticates with OIDC like Pub/Sub does.
func Job(name string, run func(ctx context.Context) (any, error), log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := run(r.Context())
		if err != nil {
			log.ErrorContext(r.Context(), "job failed", "job", name, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": name + " failed"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
