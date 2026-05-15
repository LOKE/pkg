package lokerr

import (
	"errors"
	"fmt"
)

// Error is a structured error that serializes to the @loke/errors wire format.
type Error struct {
	Message   string `json:"message"`
	Code      string `json:"code,omitempty"`
	Expose    bool   `json:"expose,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Type      string `json:"type,omitempty"`
	wrapped   error
}

func (e *Error) Error() string     { return e.Message }
func (e *Error) Public() bool      { return e.Expose }
func (e *Error) ErrorCode() string { return e.Code }
func (e *Error) Unwrap() error     { return e.wrapped }

// New creates a public error (Expose=true).
func New(msg, code string) *Error {
	return &Error{Message: msg, Code: code, Expose: true}
}

// Errorf creates a public error with a formatted message.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Message: fmt.Sprintf(format, args...), Code: code, Expose: true}
}

// Wrap wraps an internal error. Expose is false — not safe to show to end users.
// If err is nil, returns a non-public error with the given code.
func Wrap(err error, code string) *Error {
	if err == nil {
		return &Error{Message: "unknown error", Code: code}
	}
	return &Error{Message: err.Error(), Code: code, wrapped: err}
}

// WrapPublic wraps an internal error with a safe public message.
// Expose is true — publicMsg is safe to show to end users.
func WrapPublic(err error, code, publicMsg string) *Error {
	return &Error{Message: publicMsg, Code: code, Expose: true, wrapped: err}
}

// As returns the *Error in the error chain, if any.
func As(err error) (*Error, bool) {
	var e *Error
	return e, errors.As(err, &e)
}

// IsPublic reports whether err or any error in its chain is a public lokerr.Error.
func IsPublic(err error) bool {
	e, ok := As(err)
	return ok && e.Expose
}
