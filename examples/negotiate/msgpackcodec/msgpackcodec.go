// Package msgpackcodec carries lokerpc messages as MessagePack, keyed by JSON
// field names. Types with tinylib/msgp generated methods use them; everything
// else goes through vmihailenco/msgpack reflection. Both write timestamps as
// the MessagePack timestamp extension, so their output interoperates.
//
// The format names its fields, so it is schema free: lokerpc does not require
// If-Match for it and a schema change never invalidates a negotiated client.
//
// The reflection encoder writes nil slices and maps as msgpack nil, which msgp
// generated decoders reject; decoding then retries by reflection.
package msgpackcodec

import (
	"bytes"
	"fmt"
	"reflect"
	"sync"

	jtd "github.com/jsontypedef/json-typedef-go"
	"github.com/vmihailenco/msgpack/v5"
)

const msgpackNil = 0xc0

type marshaler interface {
	MarshalMsg(b []byte) ([]byte, error)
}

type unmarshaler interface {
	UnmarshalMsg(b []byte) ([]byte, error)
}

// Codec implements lokerpc.Codec for MessagePack.
type Codec struct {
	pointerMarshalers sync.Map
}

func New() *Codec {
	return &Codec{}
}

func (*Codec) ContentType() string {
	return "application/vnd.msgpack"
}

func (*Codec) SchemaFree() bool {
	return true
}

// generatedMarshaler returns v's generated MarshalMsg, copying v to make it
// addressable when the method has a pointer receiver.
func (c *Codec) generatedMarshaler(v any) (marshaler, bool) {
	generated, ok := v.(marshaler)
	if ok {
		return generated, true
	}

	value := reflect.ValueOf(v)
	if value.Kind() == reflect.Pointer {
		return nil, false
	}
	cached, ok := c.pointerMarshalers.Load(value.Type())
	if !ok {
		cached, _ = c.pointerMarshalers.LoadOrStore(value.Type(), reflect.PointerTo(value.Type()).Implements(reflect.TypeFor[marshaler]()))
	}
	if !cached.(bool) {
		return nil, false
	}

	addressable := reflect.New(value.Type())
	addressable.Elem().Set(value)
	return addressable.Interface().(marshaler), true
}

func (c *Codec) Marshal(v any, _ *jtd.Schema, _ map[string]jtd.Schema) ([]byte, error) {
	if v == nil {
		return []byte{msgpackNil}, nil
	}
	if value := reflect.ValueOf(v); value.Kind() == reflect.Pointer && value.IsNil() {
		return []byte{msgpackNil}, nil
	}

	generated, ok := c.generatedMarshaler(v)
	if ok {
		return generated.MarshalMsg(nil)
	}

	var buf bytes.Buffer
	encoder := msgpack.NewEncoder(&buf)
	encoder.SetCustomStructTag("json")
	err := encoder.Encode(v)
	return buf.Bytes(), err
}

func (c *Codec) Unmarshal(data []byte, v any, _ *jtd.Schema, _ map[string]jtd.Schema) error {
	target := reflect.ValueOf(v)
	if target.Kind() != reflect.Pointer || target.IsNil() {
		return fmt.Errorf("msgpackcodec: unmarshal target must be a non-nil pointer, got %T", v)
	}

	if len(data) == 0 || len(data) == 1 && data[0] == msgpackNil {
		target.Elem().SetZero()
		return nil
	}

	generated, ok := v.(unmarshaler)
	if ok {
		_, err := generated.UnmarshalMsg(data)
		if err == nil {
			return nil
		}
		// Generated decoders reject the msgpack nil that the reflection encoder
		// writes for nil slices and maps; the reflection decoder accepts it, and
		// reports the real error for genuinely malformed data.
	}

	decoder := msgpack.NewDecoder(bytes.NewReader(data))
	decoder.SetCustomStructTag("json")
	return decoder.Decode(v)
}
