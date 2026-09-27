package graphql

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	gql "github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/davidmm07/pressroom/internal/adapter/graphql/model"
	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/platform/logging"
)

// fieldNames maps domain field paths to input field names where the two
// differ, e.g. the domain's budget.maxCost is the input's maxCost.
type fieldNames map[string]string

var agentInputFields = fieldNames{
	"budget.maxSteps": "maxSteps",
	"budget.maxCost":  "maxCost",
	"model.provider":  "model",
	"model.name":      "model",
}

var indexPattern = regexp.MustCompile(`\[(\d+)\]`)

// userErrors turns expected failures into payload errors. It returns a
// non-nil error only for unexpected failures, which then surface as a
// top-level GraphQL error through the presenter.
func userErrors(err error, root string, names fieldNames) ([]*model.UserError, error) {
	var ve *domain.ValidationError
	switch {
	case errors.As(err, &ve):
		out := make([]*model.UserError, 0, len(ve.Fields))
		for _, f := range ve.Fields {
			out = append(out, &model.UserError{Field: fieldPath(root, f.Field, names), Code: model.UserErrorCode(f.Code), Message: f.Message})
		}
		return out, nil
	case errors.Is(err, domain.ErrNotFound):
		return single(model.UserErrorCodeNotFound, err), nil
	case errors.Is(err, domain.ErrConflict):
		return single(model.UserErrorCodeConflict, err), nil
	case errors.Is(err, domain.ErrInvalidTransition):
		return single(model.UserErrorCodeInvalidTransition, err), nil
	}
	return nil, err
}

func single(code model.UserErrorCode, err error) []*model.UserError {
	return []*model.UserError{{Field: []string{}, Code: code, Message: err.Error()}}
}

// fieldPath converts "tools[1]" under root "input" into ["input","tools","1"].
func fieldPath(root, field string, names fieldNames) []string {
	if renamed, ok := names[field]; ok {
		field = renamed
	}
	field = indexPattern.ReplaceAllString(field, ".$1")
	path := strings.Split(field, ".")
	if root != "" {
		path = append([]string{root}, path...)
	}
	return path
}

// ErrorPresenter maps errors that escape resolvers to stable codes. Unknown
// errors are logged in full and returned as a generic message with the
// request ID, so internals never reach the client but support can find them.
func ErrorPresenter(log *slog.Logger) gql.ErrorPresenterFunc {
	return func(ctx context.Context, err error) *gqlerror.Error {
		gqlErr := gql.DefaultErrorPresenter(ctx, err)
		if gqlErr.Extensions != nil && gqlErr.Extensions["code"] != nil {
			return gqlErr // parsing and validation errors from gqlgen itself
		}
		var ve *domain.ValidationError
		var se *model.ScalarError
		switch {
		case errors.As(err, &ve):
			gqlErr.Extensions = map[string]any{"code": "BAD_USER_INPUT", "fields": ve.Fields}
		case errors.As(err, &se):
			gqlErr.Extensions = map[string]any{"code": "BAD_USER_INPUT"}
		case errors.Is(err, domain.ErrNotFound):
			gqlErr.Extensions = map[string]any{"code": "NOT_FOUND"}
		case errors.Is(err, domain.ErrConflict):
			gqlErr.Extensions = map[string]any{"code": "CONFLICT"}
		case errors.Is(err, domain.ErrInvalidTransition):
			gqlErr.Extensions = map[string]any{"code": "INVALID_TRANSITION"}
		case errors.Is(err, context.DeadlineExceeded):
			gqlErr.Message = "the request took too long"
			gqlErr.Extensions = map[string]any{"code": "TIMEOUT"}
		default:
			requestID := logging.RequestID(ctx)
			log.ErrorContext(ctx, "graphql resolver failed", "error", err, "path", gqlErr.Path.String())
			gqlErr.Message = "internal error"
			gqlErr.Extensions = map[string]any{"code": "INTERNAL", "requestId": requestID}
		}
		return gqlErr
	}
}

// RecoverFunc turns a resolver panic into an INTERNAL error instead of a
// dropped connection.
func RecoverFunc(log *slog.Logger) gql.RecoverFunc {
	return func(ctx context.Context, p any) error {
		log.ErrorContext(ctx, "graphql resolver panicked", "panic", p)
		return fmt.Errorf("panic: %v", p)
	}
}
