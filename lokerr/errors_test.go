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

	var lErr *lokerr.Error
	if !errors.As(wrapped, &lErr) {
		t.Error("errors.As must find *lokerr.Error")
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
		t.Error("As must find *Error for lokerr.New result")
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
}
