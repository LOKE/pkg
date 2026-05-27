package errors

import (
	"errors"
)

// IsPublic returns the result of calling the Public method on err, if err's
// type contains a Public method returning bool.
// Otherwise, Public returns false.
//
// IsPublic only calls Public on the immediate error value, not on any wrapped
// errors.
func IsPublic(err error) bool {
	p, ok := err.(interface {
		Public() bool
	})
	if !ok {
		return false
	}
	return p.Public()
}

// ErrorCode returns the result of calling the ErrorCode method on err, if err's
// type contains an ErrorCode method returning string.
// Otherwise, ErrorCode returns an empty string.
//
// ErrorCode only calls ErrorCode on the immediate error value, not on any
// wrapped errors.
func ErrorCode(err error) string {
	c, ok := err.(interface {
		ErrorCode() string
	})
	if !ok {
		return ""
	}
	return c.ErrorCode()
}

// ErrorType returns the same thing as ErrorCode for now
//
// TODO: in the future we may want this to be richer the same way it is in
// nodejs projects. That is <project_prefix>/<package>/<error_code>, eg.
// `https://errors.loke.global//global-auth/clients/not_found`
func ErrorType(err error) string {
	return ErrorCode(err)
}

// NewPublicError returns a new public error with the given code and message.
func NewPublicError(code string, message string) error {
	return &publicError{
		code:    code,
		message: message,
	}
}

// WrapWithPublicMessage wraps err with a public error that has the given code
// and message.
func WrapWithPublicMessage(err error, code string, message string) error {
	return &publicError{
		code:    code,
		message: message,
		inner:   err,
	}
}

type publicError struct {
	code    string
	message string
	inner   error
}

func (e *publicError) Public() bool {
	return true
}

func (e *publicError) ErrorCode() string {
	return e.code
}

func (e *publicError) Error() string {
	return e.message
}

func (e *publicError) Unwrap() error {
	return e.inner
}

// The following are re-exports of the standard library errors package. They are
// here so that callers don't need to import the standard library errors
// package, making it more ergonomic to use this as a drop-in replacement.
//
// We only re-export functions that are commonly used. Notably, we do not
// re-export New as you would normally use fmt.Errorf instead. And we don't
// export As because AsType should be used instead.

// AsType is a re-export of [errors.AsType] for convenience.
func AsType[E error](err error) (E, bool) {
	return errors.AsType[E](err)
}

// Is is a re-export of [errors.Is] for convenience.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// Unwrap is a re-export of [errors.Unwrap] for convenience.
func Unwrap(err error) error {
	return errors.Unwrap(err)
}
