package lokerr

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// Error is a structured error that serializes to the @loke/errors wire format.
type Error struct {
	Message   string         `json:"message"`
	Code      string         `json:"code,omitempty"`
	Instance  string         `json:"instance"`
	Expose    bool           `json:"expose,omitempty"`
	Namespace string         `json:"namespace,omitempty"`
	Type      string         `json:"type,omitempty"`
	Meta      map[string]any `json:"-"`
	wrapped   error
}

func (e *Error) Error() string     { return e.Message }
func (e *Error) Public() bool      { return e.Expose }
func (e *Error) ErrorCode() string { return e.Code }
func (e *Error) ErrorID() string   { return e.Instance }
func (e *Error) Unwrap() error     { return e.wrapped }

// MarshalJSON flattens Meta keys at the top level, matching @loke/errors behavior.
// Struct fields always win over Meta keys in case of conflict.
func (e *Error) MarshalJSON() ([]byte, error) {
	m := make(map[string]any, len(e.Meta)+7)
	for k, v := range e.Meta {
		m[k] = v
	}
	m["message"] = e.Message
	m["instance"] = e.Instance
	if e.Code != "" {
		m["code"] = e.Code
	}
	if e.Expose {
		m["expose"] = e.Expose
	}
	if e.Namespace != "" {
		m["namespace"] = e.Namespace
	}
	if e.Type != "" {
		m["type"] = e.Type
	}
	return json.Marshal(m)
}

// New creates a public error (Expose=true).
func New(msg, code string) *Error {
	return &Error{Message: msg, Code: code, Instance: newID(), Expose: true}
}

// Errorf creates a public error with a formatted message.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Message: fmt.Sprintf(format, args...), Code: code, Instance: newID(), Expose: true}
}

// Wrap wraps an internal error. Expose is false — not safe to show to end users.
// If err is nil, returns a non-public error with the given code.
func Wrap(err error, code string) *Error {
	if err == nil {
		return &Error{Message: "unknown error", Code: code, Instance: newID()}
	}
	return &Error{Message: err.Error(), Code: code, Instance: newID(), wrapped: err}
}

// WrapPublic wraps an internal error with a safe public message.
// Expose is true — publicMsg is safe to show to end users.
func WrapPublic(err error, code, publicMsg string) *Error {
	return &Error{Message: publicMsg, Code: code, Instance: newID(), Expose: true, wrapped: err}
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

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
