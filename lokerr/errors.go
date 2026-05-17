package lokerr

import (
	"errors"
	"fmt"
)

// Error is the interface for structured, serializable RPC errors.
// Any type implementing this interface is serialized in full by lokerpc.
type Error interface {
	error
	Public() bool
	ErrorCode() string
}

// ErrorCode returns the error code of err if it implements ErrorCode(), otherwise "".
func ErrorCode(err error) string {
	type coder interface{ ErrorCode() string }
	if e, ok := err.(coder); ok {
		return e.ErrorCode()
	}
	return ""
}

// IsPublic reports whether err implements lokerr.Error and is marked as public.
func IsPublic(err error) bool {
	e, ok := err.(Error)
	return ok && e.Public()
}

// As returns the first lokerr.Error in err's chain, if any.
func As(err error) (Error, bool) {
	var e Error
	ok := errors.As(err, &e)
	return e, ok
}

// BaseError is a convenience type implementing Error.
// For errors that require additional wire fields, embed *BaseError in a custom struct — see README.
type BaseError struct {
	Message   string `json:"message"`
	Code      string `json:"code,omitempty"`
	Expose    bool   `json:"expose,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Type      string `json:"type,omitempty"`
	wrapped   error
}

func (e *BaseError) Error() string     { return e.Message }
func (e *BaseError) Public() bool      { return e.Expose }
func (e *BaseError) ErrorCode() string { return e.Code }
func (e *BaseError) Unwrap() error     { return e.wrapped }

// New creates a public BaseError (Expose=true).
func New(msg, code string) *BaseError {
	return &BaseError{Message: msg, Code: code, Expose: true}
}

// Errorf creates a public BaseError with a formatted message.
func Errorf(code, format string, args ...any) *BaseError {
	return &BaseError{Message: fmt.Sprintf(format, args...), Code: code, Expose: true}
}

// Wrap wraps an internal error as a non-public BaseError (Expose=false).
// If err is nil, returns a non-public error with message "unknown error".
func Wrap(err error, code string) *BaseError {
	if err == nil {
		return &BaseError{Message: "unknown error", Code: code}
	}
	return &BaseError{Message: err.Error(), Code: code, wrapped: err}
}

// WrapPublic wraps an internal error with a safe public message (Expose=true).
func WrapPublic(err error, code, publicMsg string) *BaseError {
	return &BaseError{Message: publicMsg, Code: code, Expose: true, wrapped: err}
}
