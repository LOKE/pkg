package lokerpc

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Compile generated open fields once so the hot walk avoids protobuf
// reflection's per-list boxing. Unsupported shapes keep the reflection path.
type legacyMessagePlan struct {
	fields []legacyFieldPlan
}

type legacyFieldPlan struct {
	index      int
	name       []byte
	descriptor protoreflect.FieldDescriptor
	kind       protoreflect.Kind
	list       bool
	child      *legacyMessagePlan
}

type legacyPlanKey struct {
	typeOf     reflect.Type
	descriptor protoreflect.MessageDescriptor
}

var legacyPlans sync.Map // legacyPlanKey -> *legacyMessagePlan (possibly nil)

func legacyPlan(typeOf reflect.Type, descriptor protoreflect.MessageDescriptor) *legacyMessagePlan {
	key := legacyPlanKey{typeOf, descriptor}
	if cached, ok := legacyPlans.Load(key); ok {
		return cached.(*legacyMessagePlan)
	}
	// Build recursive graphs privately; publish only after child links are set.
	plan := compileLegacyPlan(typeOf, descriptor, make(map[legacyPlanKey]*legacyMessagePlan))
	cached, _ := legacyPlans.LoadOrStore(key, plan)
	return cached.(*legacyMessagePlan)
}

func compileLegacyPlan(typeOf reflect.Type, descriptor protoreflect.MessageDescriptor, plans map[legacyPlanKey]*legacyMessagePlan) *legacyMessagePlan {
	key := legacyPlanKey{typeOf, descriptor}
	if plan, ok := plans[key]; ok {
		return plan
	}
	plans[key] = nil
	if !legacyOrdinaryMessage(descriptor) || typeOf.Kind() != reflect.Pointer || typeOf.Elem().Kind() != reflect.Struct {
		return nil
	}
	fields, err := legacyFields(descriptor)
	if err != nil {
		return nil
	}
	plan := &legacyMessagePlan{}
	for _, field := range fields {
		if field.descriptor.HasPresence() || field.descriptor.IsMap() {
			return nil
		}
		index := -1
		for i := range typeOf.Elem().NumField() {
			goField := typeOf.Elem().Field(i)
			parts := strings.Split(goField.Tag.Get("protobuf"), ",")
			if len(parts) > 1 && parts[1] == strconv.Itoa(int(field.descriptor.Number())) && goField.IsExported() {
				index = i
				break
			}
		}
		if index < 0 {
			return nil
		}
		plan.fields = append(plan.fields, legacyFieldPlan{index: index, name: field.name, descriptor: field.descriptor, kind: field.descriptor.Kind(), list: field.descriptor.IsList()})
	}
	// Link recursive schemas within this private compilation graph.
	plans[key] = plan
	for i := range plan.fields {
		field := &plan.fields[i]
		if field.descriptor.Kind() != protoreflect.MessageKind {
			continue
		}
		childType := typeOf.Elem().Field(field.index).Type
		if field.list {
			childType = childType.Elem()
		}
		field.child = compileLegacyPlan(childType, field.descriptor.Message(), plans)
	}
	return plan
}

func (plan *legacyMessagePlan) append(data []byte, value reflect.Value, depth int) ([]byte, error) {
	if depth > 100 {
		return nil, fmt.Errorf("legacy JSON: message nesting exceeds fast path")
	}
	data = append(data, '{')
	for i := range plan.fields {
		field := &plan.fields[i]
		if i > 0 {
			data = append(data, legacyJSONComma...)
		}
		data = append(data, field.name...)
		fieldValue := value.Field(field.index)
		var err error
		if field.list {
			data = append(data, '[')
			for j := range fieldValue.Len() {
				if j > 0 {
					data = append(data, legacyJSONComma...)
				}
				data, err = field.append(data, fieldValue.Index(j), depth+1)
				if err != nil {
					return nil, err
				}
			}
			data = append(data, ']')
		} else {
			data, err = field.append(data, fieldValue, depth+1)
		}
		if err != nil {
			return nil, err
		}
	}
	return append(data, '}'), nil
}

func (field *legacyFieldPlan) append(data []byte, value reflect.Value, depth int) ([]byte, error) {
	switch field.kind {
	case protoreflect.StringKind:
		return appendLegacyString(data, value.String())
	case protoreflect.BoolKind:
		return strconv.AppendBool(data, value.Bool()), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return strconv.AppendInt(data, value.Int(), 10), nil
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return strconv.AppendUint(data, value.Uint(), 10), nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		data = strconv.AppendInt(append(data, '"'), value.Int(), 10)
		return append(data, '"'), nil
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		data = strconv.AppendUint(append(data, '"'), value.Uint(), 10)
		return append(data, '"'), nil
	case protoreflect.BytesKind:
		return appendLegacyScalar(data, protoreflect.ValueOfBytes(value.Bytes()), field.descriptor, depth)
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return appendLegacyScalar(data, protoreflect.ValueOfFloat64(value.Float()), field.descriptor, depth)
	case protoreflect.EnumKind:
		return appendLegacyScalar(data, protoreflect.ValueOfEnum(protoreflect.EnumNumber(value.Int())), field.descriptor, depth)
	case protoreflect.MessageKind:
		if field.child != nil {
			if value.IsNil() {
				value = reflect.Zero(value.Type().Elem())
			} else {
				value = value.Elem()
			}
			return field.child.append(data, value, depth)
		}
		return appendLegacyMessage(data, value.Interface().(proto.Message).ProtoReflect(), depth)
	default:
		return nil, fmt.Errorf("legacy JSON: unsupported field %s", field.descriptor.FullName())
	}
}
