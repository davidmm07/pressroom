package domain

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Validator accumulates field errors instead of failing on the first one
// (Notification pattern). Constructors create one, run their checks and
// return v.Err(), which is nil when everything passed.
type Validator struct {
	errs []FieldError
}

func (v *Validator) add(field, code, msg string) {
	v.errs = append(v.errs, FieldError{Field: field, Code: code, Message: msg})
}

// Check records an error when ok is false.
func (v *Validator) Check(ok bool, field, code, msg string) {
	if !ok {
		v.add(field, code, msg)
	}
}

// Text validates a required, length-limited string.
func (v *Validator) Text(field, value string, maxLen int) {
	switch {
	case strings.TrimSpace(value) == "":
		v.add(field, CodeRequired, "is required")
	case utf8.RuneCountInString(value) > maxLen:
		v.add(field, CodeTooLong, fmt.Sprintf("must be at most %d characters", maxLen))
	}
}

// OptionalText validates a length limit only.
func (v *Validator) OptionalText(field, value string, maxLen int) {
	if utf8.RuneCountInString(value) > maxLen {
		v.add(field, CodeTooLong, fmt.Sprintf("must be at most %d characters", maxLen))
	}
}

// Pattern validates a required string against a regular expression.
func (v *Validator) Pattern(field, value string, re *regexp.Regexp, hint string) {
	if value == "" {
		v.add(field, CodeRequired, "is required")
		return
	}
	if !re.MatchString(value) {
		v.add(field, CodeInvalidFmt, hint)
	}
}

// Email validates a bare address such as lead@example.com.
func (v *Validator) Email(field, value string) {
	if value == "" {
		v.add(field, CodeRequired, "is required")
		return
	}
	addr, err := mail.ParseAddress(value)
	if err != nil || addr.Address != value {
		v.add(field, CodeInvalidFmt, "must be a valid email address")
	}
}

// IntRange validates min <= value <= max.
func (v *Validator) IntRange(field string, value, lo, hi int) {
	if value < lo || value > hi {
		v.add(field, CodeOutOfRange, fmt.Sprintf("must be between %d and %d", lo, hi))
	}
}

// FloatRange validates min <= value <= max.
func (v *Validator) FloatRange(field string, value, lo, hi float64) {
	if value < lo || value > hi {
		v.add(field, CodeOutOfRange, fmt.Sprintf("must be between %g and %g", lo, hi))
	}
}

// Enum validates membership through the type's own Valid method.
func (v *Validator) Enum(field string, valid bool, value string) {
	if value == "" {
		v.add(field, CodeRequired, "is required")
		return
	}
	if !valid {
		v.add(field, CodeInvalidValue, fmt.Sprintf("%q is not a supported value", value))
	}
}

// Merge adds errors produced by a nested validation under a field prefix.
func (v *Validator) Merge(prefix string, err error) {
	if err == nil {
		return
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		v.add(prefix, CodeInvalidValue, err.Error())
		return
	}
	for _, f := range ve.Fields {
		v.add(prefix+"."+f.Field, f.Code, f.Message)
	}
}

// Err returns the accumulated errors, or nil.
func (v *Validator) Err() error {
	if len(v.errs) == 0 {
		return nil
	}
	return &ValidationError{Fields: v.errs}
}
