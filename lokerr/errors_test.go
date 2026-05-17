package lokerr_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/LOKE/pkg/lokerr"
)

func TestWireFormat(t *testing.T) {
	e := lokerr.New("Something went wrong", "validation_failed")
	e.Type = "https://example.com/errors/validation_failed"
	e.Namespace = "payments"

	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}

	if got["message"] != "Something went wrong" {
		t.Errorf("message = %v", got["message"])
	}
	if got["code"] != "validation_failed" {
		t.Errorf("code = %v", got["code"])
	}
	if got["expose"] != true {
		t.Errorf("expose = %v, want true", got["expose"])
	}
	if got["type"] != "https://example.com/errors/validation_failed" {
		t.Errorf("type = %v", got["type"])
	}
	if got["namespace"] != "payments" {
		t.Errorf("namespace = %v", got["namespace"])
	}
}

func TestExposeOmitEmpty(t *testing.T) {
	e := lokerr.Wrap(errors.New("internal"), "some_code")

	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}

	if _, present := got["expose"]; present {
		t.Error("expose must be absent in JSON when false")
	}
}

func TestErrorChain(t *testing.T) {
	sentinel := errors.New("original error")
	wrapped := lokerr.Wrap(sentinel, "wrap_code")

	if !errors.Is(wrapped, sentinel) {
		t.Error("errors.Is must find sentinel through wrapped chain")
	}

	var lErr *lokerr.BaseError
	if !errors.As(wrapped, &lErr) {
		t.Error("errors.As must find *lokerr.BaseError")
	}

	if errors.Unwrap(wrapped) != sentinel {
		t.Error("errors.Unwrap must return sentinel")
	}
}

func TestConstructorDefaults(t *testing.T) {
	pub := lokerr.New("msg", "code")
	if !pub.Expose {
		t.Error("New must set Expose=true")
	}

	internal := lokerr.Wrap(errors.New("inner"), "code")
	if internal.Expose {
		t.Error("Wrap must set Expose=false")
	}

	nilWrap := lokerr.Wrap(nil, "code")
	if nilWrap.Message != "unknown error" {
		t.Errorf("Wrap(nil) message = %v, want unknown error", nilWrap.Message)
	}
}

func TestHelpers(t *testing.T) {
	pub := lokerr.New("msg", "code")
	priv := lokerr.Wrap(errors.New("inner"), "code")
	plain := errors.New("plain")

	e, ok := lokerr.As(pub)
	if !ok || e == nil {
		t.Error("As must find lokerr.Error for lokerr.New result")
	}

	_, ok = lokerr.As(plain)
	if ok {
		t.Error("As must return false for plain errors")
	}

	if !lokerr.IsPublic(pub) {
		t.Error("IsPublic must return true for public error")
	}
	if lokerr.IsPublic(priv) {
		t.Error("IsPublic must return false for non-public error")
	}
	if lokerr.IsPublic(plain) {
		t.Error("IsPublic must return false for plain errors")
	}

	if lokerr.ErrorCode(pub) != "code" {
		t.Errorf("ErrorCode = %v, want code", lokerr.ErrorCode(pub))
	}
	if lokerr.ErrorCode(plain) != "" {
		t.Errorf("ErrorCode on plain error = %v, want empty", lokerr.ErrorCode(plain))
	}
}

// customError demonstrates a user-defined type implementing lokerr.Error.
type customError struct {
	msg   string
	code  string
	Field string `json:"field"`
}

func (e *customError) Error() string     { return e.msg }
func (e *customError) Public() bool      { return true }
func (e *customError) ErrorCode() string { return e.code }

func TestCustomTypeImplementsInterface(t *testing.T) {
	err := &customError{msg: "field required", code: "required", Field: "email"}

	var _ lokerr.Error = err // compile-time interface check

	if !lokerr.IsPublic(err) {
		t.Error("IsPublic must return true for custom public error")
	}
	if lokerr.ErrorCode(err) != "required" {
		t.Errorf("ErrorCode = %v, want required", lokerr.ErrorCode(err))
	}

	// Custom field must serialize correctly
	b, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var got map[string]any
	if jsonErr := json.Unmarshal(b, &got); jsonErr != nil {
		t.Fatal(jsonErr)
	}
	if got["field"] != "email" {
		t.Errorf("field = %v, want email", got["field"])
	}
}
