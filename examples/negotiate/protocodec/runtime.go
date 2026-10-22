package protocodec

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	jtd "github.com/jsontypedef/json-typedef-go"
	"google.golang.org/protobuf/encoding/protowire"
)

// Generated types implement these so Marshal and Unmarshal can skip
// reflection. The fingerprint identifies the schema the code was generated
// for; on a mismatch the reflection path is used instead.
type (
	Marshaler interface {
		ProtoFingerprint() string
		MarshalProto() ([]byte, error)
	}
	Unmarshaler interface {
		ProtoFingerprint() string
		UnmarshalProto(data []byte) error
	}
)

// Fingerprint hashes the structure of schema and the defs it reaches, ignoring
// def names, the root's nullability, enum values and optional-vs-required.
func Fingerprint(schema *jtd.Schema, defs map[string]jtd.Schema) (string, error) {
	var canonical strings.Builder
	f := fingerprinter{defs: defs, indexes: map[string]int{}}

	root := *schema
	root.Nullable = false
	err := f.write(&canonical, &root)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(sum[:16]), nil
}

type fingerprinter struct {
	defs    map[string]jtd.Schema
	indexes map[string]int
}

func (f *fingerprinter) write(out *strings.Builder, schema *jtd.Schema) error {
	if schema.Nullable {
		out.WriteString("?")
	}

	if schema.Ref != nil {
		index, seen := f.indexes[*schema.Ref]
		if seen {
			fmt.Fprintf(out, "&%d", index)
			return nil
		}
		def, ok := f.defs[*schema.Ref]
		if !ok {
			return fmt.Errorf("undefined ref %q", *schema.Ref)
		}
		index = len(f.indexes)
		f.indexes[*schema.Ref] = index
		fmt.Fprintf(out, "#%d=", index)
		return f.write(out, &def)
	}

	switch schema.Form() {
	case jtd.FormEmpty, jtd.FormDiscriminator:
		out.WriteString("j")
	case jtd.FormEnum:
		out.WriteString("t:string")
	case jtd.FormType:
		out.WriteString("t:" + string(schema.Type))
	case jtd.FormElements:
		out.WriteString("[")
		err := f.write(out, schema.Elements)
		out.WriteString("]")
		return err
	case jtd.FormValues:
		out.WriteString("{")
		err := f.write(out, schema.Values)
		out.WriteString("}")
		return err
	case jtd.FormProperties:
		out.WriteString("(")
		props := make([]string, 0, len(schema.Properties)+len(schema.OptionalProperties))
		for name := range schema.Properties {
			props = append(props, name)
		}
		for name := range schema.OptionalProperties {
			props = append(props, name)
		}
		slices.Sort(props)
		for _, name := range props {
			child, ok := schema.Properties[name]
			if !ok {
				child = schema.OptionalProperties[name]
			}
			fmt.Fprintf(out, "%q:", name)
			err := f.write(out, &child)
			if err != nil {
				return err
			}
			out.WriteString(";")
		}
		out.WriteString(")")
	default:
		return fmt.Errorf("unsupported schema form %v", schema.Form())
	}
	return nil
}

// Writer builds a message back to front, so a nested message's length is known
// once its body is written and no length prefix ever needs widening. Generated
// code writes fields in descending order and repeated elements in reverse.
type Writer struct {
	buf []byte
	off int
}

// NewWriter starts a message buffer of sizeHint bytes (at least 64); it grows
// if the message turns out larger.
func NewWriter(sizeHint int) *Writer {
	size := max(sizeHint, 64)
	return &Writer{buf: make([]byte, size), off: size}
}

// Len is the number of bytes written so far.
func (w *Writer) Len() int { return len(w.buf) - w.off }

// Bytes returns the message in wire order.
func (w *Writer) Bytes() []byte { return w.buf[w.off:] }

func (w *Writer) reserve(n int) {
	if w.off >= n {
		return
	}
	used := w.Len()
	grown := make([]byte, max(2*len(w.buf), used+n))
	copy(grown[len(grown)-used:], w.buf[w.off:])
	w.buf, w.off = grown, len(grown)-used
}

func (w *Writer) PrependBytes(p []byte) {
	w.reserve(len(p))
	w.off -= len(p)
	copy(w.buf[w.off:], p)
}

func (w *Writer) PrependString(s string) {
	w.reserve(len(s))
	w.off -= len(s)
	copy(w.buf[w.off:], s)
}

func (w *Writer) PrependVarint(v uint64) {
	size := protowire.SizeVarint(v)
	w.reserve(size)
	w.off -= size
	protowire.AppendVarint(w.buf[w.off:w.off], v)
}

func (w *Writer) PrependTag(num protowire.Number, wireType protowire.Type) {
	w.PrependVarint(protowire.EncodeTag(num, wireType))
}

func (w *Writer) PrependFixed32(v uint32) {
	w.reserve(4)
	w.off -= 4
	protowire.AppendFixed32(w.buf[w.off:w.off], v)
}

func (w *Writer) PrependFixed64(v uint64) {
	w.reserve(8)
	w.off -= 8
	protowire.AppendFixed64(w.buf[w.off:w.off], v)
}

// PrependLength closes a nested message that started at mark = Len().
func (w *Writer) PrependLength(mark int) {
	w.PrependVarint(uint64(w.Len() - mark))
}

// PrependTimestamp writes t as a length-delimited google.protobuf.Timestamp,
// in UTC. The zero Seconds and Nanos fields are omitted.
func (w *Writer) PrependTimestamp(t time.Time) {
	t = t.UTC()
	mark := w.Len()
	if nanos := t.Nanosecond(); nanos != 0 {
		w.PrependVarint(uint64(nanos))
		w.PrependTag(2, protowire.VarintType)
	}
	if seconds := t.Unix(); seconds != 0 {
		w.PrependVarint(uint64(seconds))
		w.PrependTag(1, protowire.VarintType)
	}
	w.PrependLength(mark)
}

// ConsumeTimestamp decodes the body of a Timestamp message into UTC.
func ConsumeTimestamp(body []byte) (time.Time, error) {
	var seconds, nanos int64
	err := eachField(body, func(num protowire.Number, wireType protowire.Type, value []byte) (int, error) {
		if num != 1 && num != 2 {
			return skipField, nil
		}
		if wireType != protowire.VarintType {
			return 0, fmt.Errorf("timestamp wire type %d, want varint", wireType)
		}
		raw, consumed := protowire.ConsumeVarint(value)
		if consumed < 0 {
			return 0, protowire.ParseError(consumed)
		}
		if num == 1 {
			seconds = int64(raw)
			return consumed, nil
		}
		nanos = int64(raw)
		return consumed, nil
	})
	return time.Unix(seconds, nanos).UTC(), err
}
