// Package domain holds Pressroom's enterprise rules: the agent crew, the
// runs they perform and the policies that decide which agents keep their job.
//
// It is the innermost ring of the clean architecture and imports nothing but
// the standard library. Every other package depends on it; it depends on none
// of them (Dependency Inversion Principle).
package domain

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors. Adapters translate them into transport codes (GraphQL
// extensions, HTTP statuses) so the domain never knows how it is delivered.
var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrInvalidTransition = errors.New("invalid state transition")
)

// Field error codes shared by every validator. They are part of the public
// API contract (see docs/api.md), so treat renames as breaking changes.
const (
	CodeRequired     = "REQUIRED"
	CodeTooLong      = "TOO_LONG"
	CodeOutOfRange   = "OUT_OF_RANGE"
	CodeInvalidFmt   = "INVALID_FORMAT"
	CodeInvalidValue = "INVALID_VALUE"
)

// FieldError describes one invalid input field.
type FieldError struct {
	Field   string
	Code    string
	Message string
}

// ValidationError carries every field problem found in a single pass so a
// client can fix a form in one round trip instead of one error at a time.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, fmt.Sprintf("%s: %s", f.Field, f.Message))
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// NewValidationError is a shortcut for the common single-field case.
func NewValidationError(field, code, message string) *ValidationError {
	return &ValidationError{Fields: []FieldError{{Field: field, Code: code, Message: message}}}
}

// TransitionError explains why a lifecycle change was refused. It unwraps to
// ErrInvalidTransition so callers can match on the category with errors.Is.
type TransitionError struct {
	Entity string
	From   string
	To     string
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("%s cannot move from %s to %s", e.Entity, e.From, e.To)
}

func (e *TransitionError) Unwrap() error { return ErrInvalidTransition }
