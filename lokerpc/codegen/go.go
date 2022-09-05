package codegen

import jtd "github.com/jsontypedef/json-typedef-go"

func GenGo(schema jtd.Schema) string {
	var t string

	for k, v := range schema.Definitions {
		t += "\n"
		t += "type " + capitalize(k) + " " + GenGo(v) + "\n"
	}

	switch schema.Form() {
	case jtd.FormRef:
		t += capitalize(*schema.Ref)
	case jtd.FormType:
		switch schema.Type {
		case jtd.TypeString:
			t += "string"
		case jtd.TypeTimestamp:
			t += "time.Time"
		case jtd.TypeInt8:
			t += "int8"
		case jtd.TypeInt16:
			t += "int16"
		case jtd.TypeInt32:
			t += "int32"
		case jtd.TypeUint8:
			t += "uint8"
		case jtd.TypeUint16:
			t += "uint16"
		case jtd.TypeUint32:
			t += "uint32"
		case jtd.TypeFloat32:
			t += "float32"
		case jtd.TypeFloat64:
			t += "float64"
		case jtd.TypeBoolean:
			t += "bool"
		}
	case jtd.FormElements:
		t += "[]" + GenGo(*schema.Elements)
	case jtd.FormValues:
		t += "map[string]" + GenGo(*schema.Values)
	case jtd.FormProperties:
		t += "struct {\n"
		for k, v := range schema.Properties {
			t += "\t" + capitalize(k) + " " + GenGo(v) + "`json:\"" + k + "\"`\n"
		}
		for k, v := range schema.OptionalProperties {
			t += "\t" + capitalize(k) + " " + GenGo(v) + "`json:\"" + k + ",omitempty\"`\n"
		}
		t += "}"
	case jtd.FormDiscriminator:
		panic("discriminator not supported")
	case jtd.FormEnum:
		// Could do more here, but this is good enough for now
		t += "string"
	case jtd.FormEmpty:
		// not sure if this is the best thing, but it'll work I guess
		t += "interface{}"
	}

	if schema.Nullable {
		t = "*" + t
	}

	return t
}
