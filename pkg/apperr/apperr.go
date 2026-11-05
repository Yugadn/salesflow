// Package apperr defines the error kinds usecases return, so transport layers
// can map them to status codes without knowing business rules.
package apperr

import (
	"errors"
	"fmt"
)

// Kind classifies an application error.
type Kind int

const (
	KindInternal     Kind = iota
	KindInvalid           // the request breaks a validation or business rule
	KindNotFound          // the referenced resource does not exist
	KindConflict          // the resource is in a state that forbids the action
	KindUnauthorized      // no valid credentials were presented
	KindForbidden         // the caller is known but not allowed to do this
	KindUnavailable       // a dependency is down; the request may be retried
)

// Error is an application error with a stable machine-readable code.
type Error struct {
	Kind    Kind
	Code    string
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// Invalid reports a validation or business-rule violation.
func Invalid(code, format string, args ...any) *Error {
	return &Error{Kind: KindInvalid, Code: code, Message: fmt.Sprintf(format, args...)}
}

// NotFound reports a missing resource.
func NotFound(code, format string, args ...any) *Error {
	return &Error{Kind: KindNotFound, Code: code, Message: fmt.Sprintf(format, args...)}
}

// Conflict reports an action that the resource's current state forbids.
func Conflict(code, format string, args ...any) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: fmt.Sprintf(format, args...)}
}

// Unauthorized reports missing or invalid credentials.
func Unauthorized(code, format string, args ...any) *Error {
	return &Error{Kind: KindUnauthorized, Code: code, Message: fmt.Sprintf(format, args...)}
}

// Forbidden reports an authenticated caller acting outside their permissions.
func Forbidden(code, format string, args ...any) *Error {
	return &Error{Kind: KindForbidden, Code: code, Message: fmt.Sprintf(format, args...)}
}

// Unavailable reports a dependency outage the client may retry after.
func Unavailable(code, format string, args ...any) *Error {
	return &Error{Kind: KindUnavailable, Code: code, Message: fmt.Sprintf(format, args...)}
}

// As extracts an *Error from err's chain.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// IsNotFound reports whether err is a KindNotFound application error.
func IsNotFound(err error) bool {
	e, ok := As(err)
	return ok && e.Kind == KindNotFound
}
